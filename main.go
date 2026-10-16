// BORJ : monitoring RAM/CPU/E-S, santé matériel, processus, VMs, alertes et historique,
// dans une seule interface terminal.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

const histLen = 300

const (
	tabApps = iota
	tabVMs
	tabHist
)

// Apps bridées par le profil labo (avant de lancer des VMs).
var labApps = []string{"Teams", "Discord", "Brave", "Edge", "OneDrive", "GoogleDriveFS"}

type tickMsg time.Time

type healthMsg struct {
	h   *Health
	err error
}

type vmListMsg struct {
	vms []VM
	err error
}

type vmActMsg struct {
	text string
	err  error
}

// row : une ligne du tableau (famille, ou processus d'une famille dépliée)
type row struct {
	key string
	fam *Family
	mem *Member
}

// pending : action en attente de confirmation (o/n)
type pending struct {
	label string
	do    func(m *model) tea.Cmd
}

type model struct {
	sampler *Sampler
	snap    Snapshot

	cpuHist, memHist, rxHist, txHist, diskHist []float64

	health *Health
	hErr   error
	hBusy  bool

	width, height int
	tab           int

	// tableau des applis
	sortKey   byte // m = RAM, c = CPU, d = disque, n = nom
	cursor    int
	selKey    string
	expanded  map[string]bool
	filter    string
	filtering bool
	confirm   *pending

	// mode éco "collant" : réappliqué aux nouveaux processus de la famille
	eco     map[string]bool
	ecoDone map[uint32]bool
	labOn   bool

	// VMs
	vms      []VM
	vmErr    error
	vmBusy   bool
	vmAt     time.Time
	vmCursor int
	vmSel    string

	alert alerter
	rec   *recorder

	flash    string
	flashBad bool
	flashAt  time.Time
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func loadHealth() tea.Cmd {
	return func() tea.Msg {
		h, err := collectHealth()
		return healthMsg{h, err}
	}
}

func loadVMs() tea.Cmd {
	return func() tea.Msg {
		v, err := listVMs()
		return vmListMsg{v, err}
	}
}

func vmAction(fn func() (string, error)) tea.Cmd {
	return func() tea.Msg {
		t, err := fn()
		return vmActMsg{t, err}
	}
}

func push(h []float64, v float64) []float64 {
	h = append(h, v)
	if len(h) > histLen {
		h = h[len(h)-histLen:]
	}
	return h
}

func (m *model) topApps(n int) string {
	fams := append([]Family(nil), m.snap.Families...)
	sort.Slice(fams, func(i, j int) bool { return fams[i].Private > fams[j].Private })
	var parts []string
	for i := 0; i < min(n, len(fams)); i++ {
		parts = append(parts, fams[i].Name+" "+gb(fams[i].Private))
	}
	return strings.Join(parts, ", ")
}

func (m *model) sample() {
	snap, err := m.sampler.Sample()
	if err != nil {
		return
	}
	m.snap = snap
	m.cpuHist = push(m.cpuHist, snap.CPUTotal)
	m.memHist = push(m.memHist, snap.MemUsedPct())
	m.rxHist = push(m.rxHist, snap.NetRx)
	m.txHist = push(m.txHist, snap.NetTx)
	m.diskHist = push(m.diskHist, snap.DiskRead+snap.DiskWrite)
	m.applyEco()

	top := m.topApps(3)
	if t, b := m.alert.check(snap, top); t != "" {
		toast(t, b)
		m.sayBad("⚠ " + b)
	}
	if msg := m.rec.record(snap, top); msg != "" {
		m.say(msg)
	}
}

func (m *model) applyEco() {
	for _, f := range m.snap.Families {
		if !m.eco[f.Name] {
			continue
		}
		for _, p := range f.Members {
			if !m.ecoDone[p.PID] && setEco(p.PID, true) == nil {
				m.ecoDone[p.PID] = true
			}
		}
	}
}

// setFamilyEco : active/désactive le mode éco sur toute une famille, renvoie le nb de processus touchés.
func (m *model) setFamilyEco(name string, on bool) int {
	m.eco[name] = on
	n := 0
	for _, f := range m.snap.Families {
		if f.Name != name {
			continue
		}
		for _, p := range f.Members {
			if setEco(p.PID, on) == nil {
				n++
			}
			m.ecoDone[p.PID] = on
		}
	}
	return n
}

func (m *model) say(s string)    { m.flash, m.flashBad, m.flashAt = s, false, time.Now() }
func (m *model) sayBad(s string) { m.flash, m.flashBad, m.flashAt = s, true, time.Now() }

// ---------- Tableau des applis ----------

func (m model) rows() []row {
	fams := append([]Family(nil), m.snap.Families...)
	sort.Slice(fams, func(i, j int) bool {
		a, b := fams[i], fams[j]
		switch m.sortKey {
		case 'c':
			return a.CPU > b.CPU
		case 'd':
			return a.IO > b.IO
		case 'n':
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		}
		return a.Private > b.Private
	})
	q := strings.ToLower(m.filter)
	var out []row
	for i := range fams {
		f := &fams[i]
		if q != "" && !strings.Contains(strings.ToLower(f.Name), q) {
			hit := false
			for _, p := range f.Members {
				if strings.Contains(strings.ToLower(p.Name), q) || fmt.Sprint(p.PID) == q {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		}
		out = append(out, row{key: "f:" + f.Name, fam: f})
		if m.expanded[f.Name] {
			for j := range f.Members {
				out = append(out, row{key: fmt.Sprintf("p:%d", f.Members[j].PID), fam: f, mem: &f.Members[j]})
			}
		}
	}
	return out
}

// syncCursor : la sélection suit la ligne (clé), pas la position, même quand le tri bouge.
func (m *model) syncCursor(rs []row) {
	for i, r := range rs {
		if r.key == m.selKey {
			m.cursor = i
			return
		}
	}
	m.cursor = max(0, min(m.cursor, len(rs)-1))
	if len(rs) > 0 {
		m.selKey = rs[m.cursor].key
	}
}

func (m *model) move(delta int) {
	rs := m.rows()
	if len(rs) == 0 {
		return
	}
	m.syncCursor(rs)
	m.cursor = max(0, min(len(rs)-1, m.cursor+delta))
	m.selKey = rs[m.cursor].key
}

func (m *model) selected() *row {
	rs := m.rows()
	m.syncCursor(rs)
	if m.cursor < 0 || m.cursor >= len(rs) {
		return nil
	}
	r := rs[m.cursor]
	return &r
}

// ---------- VMs ----------

func (m *model) syncVM() {
	for i, v := range m.vms {
		if v.UUID == m.vmSel {
			m.vmCursor = i
			return
		}
	}
	m.vmCursor = max(0, min(m.vmCursor, len(m.vms)-1))
	if len(m.vms) > 0 {
		m.vmSel = m.vms[m.vmCursor].UUID
	}
}

func (m *model) selVM() *VM {
	m.syncVM()
	if m.vmCursor < 0 || m.vmCursor >= len(m.vms) {
		return nil
	}
	v := m.vms[m.vmCursor]
	return &v
}

func (m *model) vmKey(k string) tea.Cmd {
	switch k {
	case "up", "k":
		m.vmCursor = max(0, m.vmCursor-1)
	case "down", "j":
		m.vmCursor = min(len(m.vms)-1, m.vmCursor+1)
	case "u":
		if !m.vmBusy {
			m.vmBusy = true
			return loadVMs()
		}
		return nil
	}
	if len(m.vms) > 0 && (k == "up" || k == "down" || k == "k" || k == "j") {
		m.vmSel = m.vms[m.vmCursor].UUID
		return nil
	}
	v := m.selVM()
	if v == nil {
		return nil
	}
	name := strings.TrimSpace(v.Name)
	switch k {
	case "s":
		if v.Running() || v.Paused() {
			m.say(name + " tourne déjà.")
			return nil
		}
		m.say("Démarrage de " + name + " (sans fenêtre)…")
		return vmAction(func() (string, error) { return name + " démarrée.", vmStart(*v) })
	case "p":
		if !v.Running() && !v.Paused() {
			m.say(name + " n'est pas allumée.")
			return nil
		}
		return vmAction(func() (string, error) {
			if v.Paused() {
				return name + " reprend.", vmTogglePause(*v)
			}
			return name + " en pause (CPU libéré).", vmTogglePause(*v)
		})
	case "v":
		if !v.Running() && !v.Paused() {
			m.say(name + " n'est pas allumée.")
			return nil
		}
		m.say("Sauvegarde de " + name + "…")
		return vmAction(func() (string, error) { return name + " sauvegardée : RAM libérée.", vmSave(*v) })
	case "a":
		if !v.Running() {
			m.say(name + " n'est pas allumée.")
			return nil
		}
		vm := *v
		m.confirm = &pending{label: "éteindre " + name, do: func(*model) tea.Cmd {
			return vmAction(func() (string, error) { return "Extinction de " + name + " demandée.", vmACPI(vm) })
		}}
	case "+", "=", "-":
		if !v.Running() {
			m.say("Ballon : la VM doit être allumée.")
			return nil
		}
		d := 512
		if k == "-" {
			d = -512
		}
		vm := *v
		return vmAction(func() (string, error) {
			n, err := vmBalloon(vm, d)
			return fmt.Sprintf("Ballon de %s : %d Mo repris à la VM.", name, n), err
		})
	}
	return nil
}

// ---------- Update ----------

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{tick(), loadHealth()}
	if vboxManage != "" {
		cmds = append(cmds, loadVMs())
	}
	return tea.Batch(cmds...)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case tea.KeyMsg:
		k := msg.String()

		if m.confirm != nil {
			var cmd tea.Cmd
			if k == "o" || k == "y" {
				cmd = m.confirm.do(&m)
			} else {
				m.say("Annulé.")
			}
			m.confirm = nil
			return m, cmd
		}

		if m.filtering {
			switch msg.Type {
			case tea.KeyEnter:
				m.filtering = false
			case tea.KeyEsc:
				m.filtering, m.filter = false, ""
			case tea.KeyBackspace:
				if r := []rune(m.filter); len(r) > 0 {
					m.filter = string(r[:len(r)-1])
				}
			case tea.KeyRunes, tea.KeySpace:
				m.filter += string(msg.Runes)
			}
			m.cursor, m.selKey = 0, ""
			return m, nil
		}

		// touches globales
		switch k {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.tab = (m.tab + 1) % 3
			return m, nil
		case "shift+tab":
			m.tab = (m.tab + 2) % 3
			return m, nil
		case "1", "2", "3":
			m.tab = int(k[0] - '1')
			return m, nil
		case "L":
			m.labOn = !m.labOn
			n := 0
			for _, a := range labApps {
				n += m.setFamilyEco(a, m.labOn)
			}
			if m.labOn {
				m.say(fmt.Sprintf("Profil labo ON : %d processus bridés (Teams, Discord, Brave, Edge…). Tu peux lancer tes VMs.", n))
			} else {
				m.say("Profil labo OFF : apps remises en priorité normale.")
			}
			return m, nil
		case "r":
			if !m.hBusy {
				m.hBusy = true
				m.say("Analyse santé relancée…")
				return m, loadHealth()
			}
			return m, nil
		}

		switch m.tab {
		case tabVMs:
			return m, m.vmKey(k)
		case tabHist:
			if k == "o" {
				_ = exec.Command("explorer.exe", m.rec.dir).Start()
				m.say("Dossier ouvert : " + m.rec.dir)
			}
			return m, nil
		}

		// onglet applis
		switch k {
		case "esc":
			m.filter = ""
		case "up", "k":
			m.move(-1)
		case "down", "j":
			m.move(1)
		case "pgup":
			m.move(-10)
		case "pgdown":
			m.move(10)
		case "home", "g":
			m.move(-1 << 20)
		case "end", "G":
			m.move(1 << 20)
		case "enter", " ", "right", "left":
			if r := m.selected(); r != nil {
				if r.mem != nil {
					m.expanded[r.fam.Name] = false
					m.selKey = "f:" + r.fam.Name
				} else if k == "left" {
					m.expanded[r.fam.Name] = false
				} else {
					m.expanded[r.fam.Name] = k == "right" || !m.expanded[r.fam.Name]
				}
			}
		case "/":
			m.filtering, m.filter = true, ""
		case "m", "c", "d", "n":
			m.sortKey = k[0]
		case "x", "delete":
			r := m.selected()
			switch {
			case r == nil:
			case r.mem != nil && isProtected(r.mem.Name):
				m.sayBad("Refusé : " + r.mem.Name + " est un processus système.")
			case r.mem != nil:
				pid, name := r.mem.PID, r.mem.Name
				m.confirm = &pending{label: fmt.Sprintf("%s (PID %d)", name, pid), do: func(m *model) tea.Cmd {
					ok, fail := killPIDs([]uint32{pid})
					m.say(fmt.Sprintf("%s : %d arrêté, %d refusé", name, ok, fail))
					return nil
				}}
			case isProtected(r.fam.Name) || r.fam.Name == "Services Windows" || r.fam.Name == "Affichage (dwm)" || r.fam.Name == "BORJ":
				m.sayBad("Refusé : " + r.fam.Name + " est protégé.")
			default:
				pids, name, cnt := r.fam.PIDs(), r.fam.Name, r.fam.Count
				m.confirm = &pending{label: fmt.Sprintf("%s (%d processus)", name, cnt), do: func(m *model) tea.Cmd {
					ok, fail := killPIDs(pids)
					m.say(fmt.Sprintf("%s : %d processus arrêté(s), %d refusé(s)", name, ok, fail))
					return nil
				}}
			}
		case "e":
			if r := m.selected(); r != nil {
				name := r.fam.Name
				if isProtected(name) || name == "Services Windows" || name == "BORJ" {
					m.sayBad("Mode éco non autorisé sur " + name)
					break
				}
				on := !m.eco[name]
				n := m.setFamilyEco(name, on)
				if on {
					m.say(fmt.Sprintf("Mode éco ON sur %s (%d processus) : priorité basse + EcoQoS", name, n))
				} else {
					m.say("Mode éco OFF sur " + name)
				}
			}
		}

	case tickMsg:
		m.sample()
		m.syncCursor(m.rows())
		cmds := []tea.Cmd{tick()}
		if m.health != nil && !m.hBusy && time.Since(m.health.At) > 5*time.Minute {
			m.hBusy = true
			cmds = append(cmds, loadHealth())
		}
		if vboxManage != "" && !m.vmBusy && time.Since(m.vmAt) > 15*time.Second {
			m.vmBusy = true
			cmds = append(cmds, loadVMs())
		}
		return m, tea.Batch(cmds...)

	case healthMsg:
		m.hBusy, m.hErr = false, msg.err
		if msg.h != nil {
			m.health = msg.h
		}

	case vmListMsg:
		m.vmBusy, m.vmErr, m.vmAt = false, msg.err, time.Now()
		if msg.err == nil {
			m.vms = msg.vms
			m.syncVM()
		}

	case vmActMsg:
		if msg.err != nil {
			m.sayBad("VM : " + msg.err.Error())
		} else {
			m.say(msg.text)
		}
		m.vmBusy = true
		return m, loadVMs()
	}
	return m, nil
}

func main() {
	once := flag.Bool("once", false, "affiche une seule frame puis quitte (test)")
	w := flag.Int("w", 180, "largeur pour -once")
	h := flag.Int("h", 50, "hauteur pour -once")
	tab := flag.Int("tab", 1, "onglet pour -once (1 applis, 2 VMs, 3 historique)")
	noColor := flag.Bool("nocolor", false, "désactive les couleurs")
	flag.Parse()

	// Couleurs forcées : NO_COLOR / CI hérités d'un shell parent les coupaient sans raison.
	if !*noColor {
		lipgloss.SetColorProfile(termenv.TrueColor)
		lipgloss.SetHasDarkBackground(true)
	}

	m := model{
		sampler:  NewSampler(),
		hBusy:    true,
		sortKey:  'm',
		expanded: map[string]bool{},
		eco:      map[string]bool{},
		ecoDone:  map[uint32]bool{},
		rec:      newRecorder(),
	}
	m.sample()
	time.Sleep(500 * time.Millisecond)
	m.sample()

	if *once {
		m.width, m.height, m.tab = *w, *h, *tab-1
		m.health, m.hErr = collectHealth()
		m.hBusy = false
		if vboxManage != "" {
			m.vms, m.vmErr = listVMs()
		}
		m.syncCursor(m.rows())
		fmt.Println(m.View())
		return
	}

	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
