package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	colorful "github.com/lucasb-eyer/go-colorful"
	"golang.org/x/sys/windows"
)

// Palette (Tokyo Night)
var (
	cBg     = lipgloss.Color("#1a1b26")
	cBorder = lipgloss.Color("#3b4261")
	cTitle  = lipgloss.Color("#7aa2f7")
	cAccent = lipgloss.Color("#bb9af7")
	cCyan   = lipgloss.Color("#7dcfff")
	cBlue   = lipgloss.Color("#2ac3de")
	cOK     = lipgloss.Color("#9ece6a")
	cWarn   = lipgloss.Color("#e0af68")
	cOrange = lipgloss.Color("#ff9e64")
	cBad    = lipgloss.Color("#f7768e")
	cDim    = lipgloss.Color("#565f89")
	cText   = lipgloss.Color("#c0caf5")
	cSelBg  = lipgloss.Color("#2e3c64")

	sDim   = lipgloss.NewStyle().Foreground(cDim)
	sText  = lipgloss.NewStyle().Foreground(cText)
	sTitle = lipgloss.NewStyle().Foreground(cTitle).Bold(true)
)

func fg(c lipgloss.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

func pill(text string, bg lipgloss.Color) string {
	return lipgloss.NewStyle().Background(bg).Foreground(cBg).Bold(true).Padding(0, 1).Render(text)
}

// Dégradé vert -> jaune -> orange -> rouge, précalculé sur 0..100
var gradient = func() [101]lipgloss.Color {
	stops := []string{"#9ece6a", "#9ece6a", "#e0af68", "#ff9e64", "#f7768e"}
	var g [101]lipgloss.Color
	for i := 0; i <= 100; i++ {
		t := float64(i) / 100 * float64(len(stops)-1)
		j := min(int(t), len(stops)-2)
		a, _ := colorful.Hex(stops[j])
		b, _ := colorful.Hex(stops[j+1])
		g[i] = lipgloss.Color(a.BlendLab(b, t-float64(j)).Clamped().Hex())
	}
	return g
}()

func grad(p float64) lipgloss.Color { return gradient[int(clamp(p, 0, 100))] }

func levelColor(p float64) lipgloss.Color {
	switch {
	case p >= 90:
		return cBad
	case p >= 75:
		return cWarn
	}
	return cOK
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// bar : barre en dégradé (chaque case prend la couleur de sa position). Largeur totale = w + 5.
func bar(p float64, w int) string {
	return gbar(p, w, 100, fmt.Sprintf(" %3.0f%%", clamp(p, 0, 100)), levelColor(p))
}

// gbar : scale = valeur qui correspond à 100 % du dégradé (pour colorer tôt les petites parts)
func gbar(p float64, w int, scale float64, label string, labelCol lipgloss.Color) string {
	w = max(1, w)
	p = clamp(p, 0, 100)
	n := int(p/100*float64(w) + 0.5)
	if n == 0 && p >= 0.5 {
		n = 1
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		pos := float64(i+1) / float64(w) * 100 * (100 / scale)
		b.WriteString(fg(grad(pos)).Render("█"))
	}
	b.WriteString(fg(lipgloss.Color("#292e42")).Render(strings.Repeat("━", w-n)))
	return b.String() + fg(labelCol).Bold(true).Render(label)
}

// spark : vals 0..100, chaque colonne colorée selon sa valeur
func spark(vals []float64, w int) string {
	ticks := []rune("▁▂▃▄▅▆▇█")
	if w < 1 {
		return ""
	}
	if len(vals) > w {
		vals = vals[len(vals)-w:]
	}
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", w-len(vals)))
	for _, v := range vals {
		b.WriteString(fg(grad(v)).Render(string(ticks[int(clamp(v, 0, 100)/100*7+0.5)])))
	}
	return b.String()
}

// sparkMono : une seule couleur, auto-échelle (débits)
func sparkMono(vals []float64, w int, c lipgloss.Color) string {
	ticks := []rune("▁▂▃▄▅▆▇█")
	if w < 1 {
		return ""
	}
	if len(vals) > w {
		vals = vals[len(vals)-w:]
	}
	peak := 1.0
	for _, v := range vals {
		peak = max(peak, v)
	}
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", w-len(vals)))
	for _, v := range vals {
		b.WriteRune(ticks[int(clamp(v/peak, 0, 1)*7+0.5)])
	}
	return fg(c).Render(b.String())
}

func trunc(s string, n int) string {
	r := []rune(s)
	if n <= 0 {
		return ""
	}
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

func pad(s string, n int) string {
	s = trunc(s, n)
	return s + strings.Repeat(" ", max(0, n-lipgloss.Width(s)))
}

func gb(b uint64) string {
	if b >= 1<<30 {
		return fmt.Sprintf("%.1f Go", float64(b)/(1<<30))
	}
	return fmt.Sprintf("%.0f Mo", float64(b)/(1<<20))
}

func rate(bps float64) string {
	switch {
	case bps >= 1<<20:
		return fmt.Sprintf("%.1f Mo/s", bps/(1<<20))
	case bps >= 1<<10:
		return fmt.Sprintf("%.0f Ko/s", bps/(1<<10))
	case bps >= 1:
		return fmt.Sprintf("%.0f o/s", bps)
	}
	return "—"
}

func kv(k, v string) string { return sDim.Render(pad(k, 11)) + v }

// panel : largeur totale w (bordure incluse), hauteur totale h (0 = auto). head est déjà stylé.
func panel(head, right, body string, w, h int, border lipgloss.Color) string {
	inner := w - 4
	if right != "" {
		head += strings.Repeat(" ", max(1, inner-lipgloss.Width(head)-lipgloss.Width(right))) + right
	}
	st := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 1).Width(w - 2)
	if h > 0 {
		st = st.Height(h - 2).MaxHeight(h)
	}
	return st.Render(head + "\n" + body)
}

func title(icon, t string, c lipgloss.Color) string {
	return fg(c).Bold(true).Render(icon + " " + t)
}

func verdict(level int) (string, lipgloss.Color) {
	switch level {
	case 0:
		return pill("✔ BONNE SANTÉ", cOK), cOK
	case 1:
		return pill("! À SURVEILLER", cWarn), cWarn
	}
	return pill("✖ À REMPLACER", cBad), cBad
}

// ---------- Panneaux live ----------

func (m model) cpuBody(w int) []string {
	inner := w - 4
	s := m.snap
	lines := []string{
		kv("Total", bar(s.CPUTotal, inner-11-5)),
		sDim.Render(pad("historique", 11)) + spark(m.cpuHist, inner-11),
	}
	colW := (inner - 2) / 2
	for i := 0; i < len(s.Cores); i += 2 {
		cell := func(j int) string {
			if j >= len(s.Cores) {
				return ""
			}
			return sDim.Render(fmt.Sprintf("C%-2d ", j)) + bar(s.Cores[j], max(1, colW-4-5))
		}
		lines = append(lines, cell(i)+"  "+cell(i+1))
	}
	return lines
}

func (m model) ramBody(w int) []string {
	inner := w - 4
	s := m.snap
	bw := inner - 11 - 5
	return []string{
		kv("Utilisée", bar(s.MemUsedPct(), bw)),
		kv("", sText.Bold(true).Render(gb(s.MemTotal-s.MemAvail))+sDim.Render(" / "+gb(s.MemTotal)+" · ")+fg(cOK).Bold(true).Render(gb(s.MemAvail)+" libres")),
		kv("Commit", bar(s.CommitPct(), bw)),
		kv("", sText.Render(gb(s.CommitTotal)+" / "+gb(s.CommitLimit))+sDim.Render(" · plante si 100 %")),
		kv("Compressée", fg(cAccent).Render(gb(s.Compressed))),
		sDim.Render(pad("historique", 11)) + spark(m.memHist, inner-11),
	}
}

func (m model) ioBody(w int) []string {
	inner := w - 4
	s := m.snap
	sw := inner - 11
	return []string{
		kv("↓ Reçu", fg(cCyan).Bold(true).Render(rate(s.NetRx))),
		sDim.Render(pad("", 11)) + sparkMono(m.rxHist, sw, cCyan),
		kv("↑ Envoyé", fg(cAccent).Bold(true).Render(rate(s.NetTx))),
		sDim.Render(pad("", 11)) + sparkMono(m.txHist, sw, cAccent),
		kv("Disque", fg(cOrange).Bold(true).Render("L "+rate(s.DiskRead))+sDim.Render(" · ")+fg(cOrange).Bold(true).Render("É "+rate(s.DiskWrite))),
		sDim.Render(pad("", 11)) + sparkMono(m.diskHist, sw, cOrange),
	}
}

// ---------- Onglet Applications ----------

const (
	colName = 26
	colProc = 7
	colRAM  = 10
	colCPU  = 8
	colIO   = 11
)

func (m model) appsBody(w, n int) []string {
	inner := w - 4
	bw := max(5, min(40, inner-2-colName-colProc-colRAM-colCPU-colIO-3-5))
	head := sDim.Render("  " + pad("APPLICATION", colName) + pad("PROC", colProc) +
		fmt.Sprintf("%*s%*s%*s   ", colRAM, "RAM", colCPU, "CPU", colIO, "DISQUE") + "PART DE LA RAM")
	lines := []string{head}

	rs := m.rows()
	if len(rs) == 0 {
		return append(lines, sDim.Render("  aucun résultat pour « "+m.filter+" »"))
	}
	cur := max(0, min(m.cursor, len(rs)-1))
	off := (cur / n) * n
	for i := off; i < min(len(rs), off+n); i++ {
		r := rs[i]
		sel := i == cur
		var name, procs string
		var private uint64
		var cpu, io float64
		if r.mem == nil {
			arrow := "▸ "
			if m.expanded[r.fam.Name] {
				arrow = "▾ "
			}
			name = arrow + r.fam.Name
			procs = fmt.Sprintf("×%d", r.fam.Count)
			private, cpu, io = r.fam.Private, r.fam.CPU, r.fam.IO
		} else {
			name = "   └ " + r.mem.Name
			procs = fmt.Sprint(r.mem.PID)
			private, cpu, io = r.mem.Private, r.mem.CPU, r.mem.IO
		}
		eco := ""
		if m.eco[r.fam.Name] && r.mem == nil {
			eco = "🍃"
		}

		nameSt := sText
		if r.mem == nil {
			nameSt = nameSt.Bold(true)
		} else {
			nameSt = sDim
		}
		marker := "  "
		if sel {
			marker = fg(cCyan).Bold(true).Render("▌ ")
			nameSt = lipgloss.NewStyle().Foreground(cCyan).Background(cSelBg).Bold(true)
		}
		cpu = clamp(cpu, 0, 100)
		cpuCol := cDim
		if cpu >= 0.5 {
			cpuCol = grad(cpu * 3)
		}
		ioCol := cDim
		if io >= 1<<20 {
			ioCol = cOrange
		} else if io >= 1<<10 {
			ioCol = cText
		}
		ramPct := 0.0
		if m.snap.MemTotal > 0 {
			ramPct = 100 * float64(private) / float64(m.snap.MemTotal)
		}
		nameW := colName - 1 - lipgloss.Width(eco)
		lines = append(lines, marker+nameSt.Render(pad(name, nameW))+eco+" "+
			sDim.Render(pad(procs, colProc))+
			fg(grad(ramPct*5)).Bold(r.mem == nil).Render(fmt.Sprintf("%*s", colRAM, gb(private)))+
			fg(cpuCol).Render(fmt.Sprintf("%*s", colCPU, fmt.Sprintf("%.1f%%", cpu)))+
			fg(ioCol).Render(fmt.Sprintf("%*s", colIO, rate(io)))+"   "+
			gbar(ramPct, bw, 20, fmt.Sprintf(" %3.0f%%", ramPct), grad(ramPct*5)))
	}
	if len(rs) > n {
		lines = append(lines, sDim.Render(fmt.Sprintf("  page %d/%d · %d lignes · PgUp/PgDn", off/n+1, (len(rs)+n-1)/n, len(rs))))
	}
	return lines
}

// ---------- Onglet VMs ----------

func vmDot(v VM) string {
	switch v.State {
	case "running":
		return fg(cOK).Render("●")
	case "paused":
		return fg(cWarn).Render("◐")
	case "saved":
		return fg(cBlue).Render("◉")
	case "aborted":
		return fg(cBad).Render("✖")
	}
	return sDim.Render("○")
}

func (m model) vmsBody(w, n int) []string {
	if vboxManage == "" {
		return []string{sDim.Render("VirtualBox non trouvé sur cette machine.")}
	}
	if m.vmErr != nil && len(m.vms) == 0 {
		return []string{fg(cBad).Render("Erreur VirtualBox : " + m.vmErr.Error())}
	}
	if len(m.vms) == 0 {
		return []string{sDim.Render("Lecture des VMs…")}
	}
	inner := w - 4
	// résumé
	var runRAM, runCPU, on int
	for _, v := range m.vms {
		if v.Running() || v.Paused() {
			on++
			runRAM += v.MemMB - v.Balloon
			runCPU += v.CPUs
		}
	}
	phys := 4
	if m.health != nil && m.health.CPU.Cores > 0 {
		phys = m.health.CPU.Cores
	}
	cpuCol := cOK
	if runCPU > phys {
		cpuCol = cBad
	}
	ramCol := cOK
	if uint64(runRAM)<<20 > m.snap.MemAvail {
		ramCol = cWarn
	}
	sum := sText.Render(fmt.Sprintf("%d allumée(s) · ", on)) +
		fg(ramCol).Bold(true).Render(fmt.Sprintf("%.1f Go réservés", float64(runRAM)/1024)) +
		sDim.Render(fmt.Sprintf(" (%s libres sur le PC) · ", gb(m.snap.MemAvail))) +
		fg(cpuCol).Bold(true).Render(fmt.Sprintf("%d vCPU", runCPU)) + sDim.Render(fmt.Sprintf(" / %d cœurs réels", phys))
	lines := []string{sum, ""}

	bw := max(5, min(30, inner-2-2-28-14-10-7-11-3-5))
	lines = append(lines, sDim.Render("    "+pad("MACHINE VIRTUELLE", 28)+pad("ÉTAT", 14)+fmt.Sprintf("%9s %6s %10s   ", "RAM", "vCPU", "BALLON")+"RAM / PC"))
	cur := max(0, min(m.vmCursor, len(m.vms)-1))
	rows := max(1, n-3)
	off := (cur / rows) * rows
	for i := off; i < min(len(m.vms), off+rows); i++ {
		v := m.vms[i]
		sel := i == cur
		marker := "  "
		nameSt := sText.Bold(true)
		if sel {
			marker = fg(cCyan).Bold(true).Render("▌ ")
			nameSt = lipgloss.NewStyle().Foreground(cCyan).Background(cSelBg).Bold(true)
		}
		stCol := cDim
		switch v.State {
		case "running":
			stCol = cOK
		case "paused":
			stCol = cWarn
		case "saved":
			stCol = cBlue
		case "aborted":
			stCol = cBad
		}
		balloon := sDim.Render(fmt.Sprintf("%10s", "—"))
		if v.Balloon > 0 {
			balloon = fg(cAccent).Render(fmt.Sprintf("%10s", fmt.Sprintf("-%d Mo", v.Balloon)))
		}
		share := 0.0
		if m.snap.MemTotal > 0 {
			share = 100 * float64(uint64(v.MemMB)<<20) / float64(m.snap.MemTotal)
		}
		barCol := grad(share * 3)
		if !(v.Running() || v.Paused()) {
			barCol = cDim
		}
		lines = append(lines, marker+vmDot(v)+" "+nameSt.Render(pad(strings.TrimSpace(v.Name), 27))+" "+
			fg(stCol).Render(pad(stateFR(v.State), 14))+
			sText.Render(fmt.Sprintf("%9s", fmt.Sprintf("%d Mo", v.MemMB)))+
			sText.Render(fmt.Sprintf(" %6d", v.CPUs))+" "+balloon+"   "+
			gbar(share, bw, 33, fmt.Sprintf(" %3.0f%%", share), barCol))
	}
	if len(m.vms) > rows {
		lines = append(lines, sDim.Render(fmt.Sprintf("  page %d/%d", off/rows+1, (len(m.vms)+rows-1)/rows)))
	}
	return lines
}

// ---------- Onglet Historique ----------

func (m model) histBody(w, n int) []string {
	inner := w - 4
	sw := inner - 22
	pts := m.rec.long
	mins := len(pts) * int(histEvery.Seconds()) / 60
	maxOf := func(f func(point) float64) float64 {
		v := 0.0
		for _, p := range pts {
			v = max(v, f(p))
		}
		return v
	}
	line := func(label string, f func(point) float64) string {
		mx := maxOf(f)
		return sDim.Render(pad(label, 9)) + spark(m.rec.series(f), sw) + "  " + sDim.Render("max ") + fg(levelColor(mx)).Bold(true).Render(fmt.Sprintf("%3.0f%%", mx))
	}
	period := fmt.Sprintf("Dernières %d min", max(1, mins))
	if mins >= 60 {
		period = "Dernière heure"
	}
	lines := []string{
		sDim.Render(period + " (1 point / 5 s) — le CSV complet est dans le dossier du journal"),
		line("RAM", func(p point) float64 { return p.ram }),
		line("Commit", func(p point) float64 { return p.commit }),
		line("CPU", func(p point) float64 { return p.cpu }),
		"",
		fg(cAccent).Bold(true).Render("PICS ENREGISTRÉS") + sDim.Render(fmt.Sprintf("  (RAM ou commit ≥ %.0f %%, du plus récent au plus ancien)", peakOn)),
	}
	if m.rec.inPeak {
		lines = append(lines, fg(cBad).Bold(true).Render(fmt.Sprintf("● PIC EN COURS depuis %s — RAM max %.0f%% — %s",
			m.rec.peakStart.Format("15:04:05"), m.rec.peakRAM, m.rec.peakTop)))
	}
	if len(m.rec.peaks) == 0 && !m.rec.inPeak {
		lines = append(lines, fg(cOK).Render("Aucun pic enregistré pour l'instant. 👍"))
	}
	room := n - len(lines) - 1
	for i := len(m.rec.peaks) - 1; i >= 0 && room > 0; i-- {
		lines = append(lines, fg(cWarn).Render("▲ ")+sText.Render(trunc(m.rec.peaks[i], inner-2)))
		room--
	}
	lines = append(lines, sDim.Render("Journal : "+m.rec.dir))
	return lines
}

// ---------- Cartes santé ----------

func (m model) ssdCard(w int) (string, lipgloss.Color, []string) {
	h := m.health
	if h == nil || len(h.Disks) == 0 {
		return "", cBorder, []string{sDim.Render("analyse en cours…")}
	}
	inner := w - 4
	worst := 0
	var lines []string
	// tous les disques physiques ; le verdict de la carte = le pire disque
	for _, d := range h.Disks {
		worst = max(worst, d.Level())
		status := map[string]string{"Healthy": "Sain", "Warning": "Avertissement", "Unhealthy": "Défaillant"}[d.Health]
		if status == "" {
			status = d.Health
		}
		media := d.Media
		if media == "" || media == "Unspecified" {
			media = "Disque"
		}
		lines = append(lines,
			fg(levelColor(float64(d.Level())*45+40)).Render("● ")+sText.Bold(true).Render(trunc(d.Model, inner-2)),
			kv("  Type", sText.Render(fmt.Sprintf("%s · %s · %d Go · %s", media, d.Bus, d.SizeGB, status))))
		switch {
		case d.Wear >= 0:
			lines = append(lines, kv("  Usure", bar(float64(d.Wear), inner-11-5)))
			if d.Errors > 0 {
				lines = append(lines, kv("  Erreurs", fg(cWarn).Render(fmt.Sprintf("%d non corrigée(s)", d.Errors))))
			}
		case !h.Admin:
			lines = append(lines, kv("  Usure", fg(cWarn).Render("lancer en admin pour la voir")))
		default:
			lines = append(lines, kv("  Usure", sDim.Render("non fournie par ce disque")))
		}
	}
	lines = append(lines,
		kv("Espace "+h.Sys.Drive, bar(h.Sys.UsedPct, inner-11-5)),
		kv("", fg(levelColor(h.Sys.UsedPct)).Render(fmt.Sprintf("%.0f Go libres", h.Sys.FreeGB))))
	v, c := verdict(worst)
	return v, c, lines
}

func (m model) cpuCard(w int) (string, lipgloss.Color, []string) {
	h := m.health
	if h == nil {
		return "", cBorder, []string{sDim.Render("analyse en cours…")}
	}
	inner := w - 4
	name := strings.NewReplacer("(R)", "", "(TM)", "", " CPU", "", "11th Gen ", "", "12th Gen ", "", "13th Gen ", "").Replace(h.CPU.Name)
	if i := strings.Index(name, "@"); i > 0 {
		name = name[:i]
	}
	errCol := cOK
	if h.Whea.CPU+h.Whea.Fatal > 0 {
		errCol = cWarn
	}
	v, c := verdict(h.CPULevel())
	return v, c, []string{
		sText.Bold(true).Render(trunc(strings.TrimSpace(name), inner)),
		kv("Cœurs", sText.Render(fmt.Sprintf("%d cœurs · %d threads", h.CPU.Cores, h.CPU.Threads))),
		kv("Statut", fg(cOK).Render(h.CPU.Status)),
		kv("Erreurs", fg(errCol).Render(fmt.Sprintf("%d corrigée(s) · %d fatale(s)", h.Whea.CPU, h.Whea.Fatal))),
		kv("", sDim.Render("sur les 30 derniers jours")),
	}
}

func (m model) ramCard(w int) (string, lipgloss.Color, []string) {
	h := m.health
	if h == nil {
		return "", cBorder, []string{sDim.Render("analyse en cours…")}
	}
	inst := h.InstalledGB()
	used := len(h.RAM.Sticks)
	free := h.RAM.Slots - used
	slot := fg(cOK).Render(fmt.Sprintf("%d libre(s)", free))
	if free <= 0 {
		slot = fg(cWarn).Render("aucun libre")
	}
	upg := fg(cOK).Render(fmt.Sprintf("+%d Go possibles", h.RAM.MaxGB-inst))
	if inst >= h.RAM.MaxGB {
		upg = fg(cWarn).Render("déjà au max (BIOS)")
	}
	lines := []string{
		kv("Installée", sText.Bold(true).Render(fmt.Sprintf("%d Go", inst))+sDim.Render(fmt.Sprintf(" / max %d Go", h.RAM.MaxGB))),
		kv("Slots", sText.Render(fmt.Sprintf("%d/%d occupés · ", used, h.RAM.Slots))+slot),
		kv("Extension", upg),
	}
	for _, s := range h.RAM.Sticks {
		lines = append(lines, kv("  "+s.Loc, sText.Render(fmt.Sprintf("%d Go %s-%d", s.GB, s.Type, s.Speed))))
	}
	var diag string
	switch h.RAM.DiagID {
	case 1201:
		diag = fg(cOK).Render("OK le " + h.RAM.DiagDate)
	case 1202:
		diag = fg(cBad).Render("ERREURS le " + h.RAM.DiagDate)
	default:
		diag = sDim.Render("jamais lancé (mdsched)")
	}
	lines = append(lines, kv("Test", diag))
	v, c := verdict(h.RAMLevel())
	return v, c, lines
}

// ---------- Barre latérale : commandes expliquées ----------

type help struct{ key, what, why string }

var helpApps = []help{
	{"↑ ↓", "Se déplacer", "choisir une appli dans la liste"},
	{"Entrée", "Déplier", "voir chaque processus de l'appli"},
	{"/", "Chercher", "taper un nom ou un PID"},
	{"m c d n", "Trier", "par RAM, CPU, disque ou nom"},
	{"x", "Arrêter", "ferme l'appli (demande confirmation)"},
	{"e", "Mode éco 🍃", "bride l'appli, libère CPU et batterie"},
}

var helpVMs = []help{
	{"↑ ↓", "Choisir une VM", ""},
	{"s", "Démarrer", "sans fenêtre = moins de RAM"},
	{"p", "Pause / reprise", "gèle la VM, libère le CPU"},
	{"v", "Sauvegarder", "fige sur disque, libère TOUTE sa RAM"},
	{"a", "Éteindre", "arrêt propre (confirmation)"},
	{"+  −", "Ballon RAM", "reprend / rend 512 Mo à la VM"},
	{"u", "Actualiser", "relire l'état des VMs"},
}

var helpHist = []help{
	{"o", "Ouvrir le dossier", "CSV toutes les 5 s + journal des pics"},
	{"", "Automatique", "chaque pic ≥ 85 % est noté avec les apps responsables"},
}

var helpGlobal = []help{
	{"Tab", "Changer d'onglet", "ou 1 · 2 · 3"},
	{"L", "Profil labo", "mode éco sur Teams, Discord, Brave, Edge… avant tes VMs"},
	{"r", "Santé", "relancer l'analyse SSD / CPU / RAM"},
	{"q", "Quitter", ""},
}

func (m model) sidebar(w, h int) string {
	inner := w - 4
	renderHelp := func(items []help) []string {
		var out []string
		for _, it := range items {
			k := ""
			if it.key != "" {
				k = pill(it.key, cCyan) + " "
			}
			out = append(out, k+sText.Bold(true).Render(it.what))
			if it.why != "" {
				for _, l := range wrap(it.why, inner-2) {
					out = append(out, sDim.Render("  "+l))
				}
			}
		}
		return out
	}
	section := func(t string, c lipgloss.Color) string {
		return fg(c).Bold(true).Render(t) + " " + fg(cBorder).Render(strings.Repeat("─", max(0, inner-lipgloss.Width(t)-1)))
	}

	tabName := []string{"APPLICATIONS", "MACHINES VIRTUELLES", "HISTORIQUE"}[m.tab]
	items := [][]help{helpApps, helpVMs, helpHist}[m.tab]

	var lines []string
	lines = append(lines, section("ONGLET "+tabName, cAccent))
	lines = append(lines, renderHelp(items)...)
	lines = append(lines, "", section("PARTOUT", cTitle))
	lines = append(lines, renderHelp(helpGlobal)...)

	lines = append(lines, "", section("ÉTAT", cOK))
	lab := pill("LABO OFF", cDim)
	if m.labOn {
		lab = pill("LABO ON 🍃", cOK)
	}
	al := pill("ALERTES ARMÉES", cBlue)
	if m.alert.Active() {
		al = pill("⚠ ALERTE", cBad)
	}
	lines = append(lines, lab+" "+al)
	lines = append(lines, sDim.Render(fmt.Sprintf("notification si RAM ou commit ≥ %.0f %%", alertOn)))
	lines = append(lines, "", section("COULEURS", cWarn))
	lines = append(lines, fg(cOK).Render("■")+sDim.Render(" ok  ")+fg(cWarn).Render("■")+sDim.Render(" attention  ")+fg(cBad).Render("■")+sDim.Render(" critique"))

	return panel(title("⌨", "COMMANDES", cAccent), "", strings.Join(lines, "\n"), w, h, cAccent)
}

func wrap(s string, w int) []string {
	if w <= 0 {
		return []string{s}
	}
	var out []string
	cur := ""
	for _, word := range strings.Fields(s) {
		if cur == "" {
			cur = word
		} else if lipgloss.Width(cur)+1+lipgloss.Width(word) <= w {
			cur += " " + word
		} else {
			out = append(out, cur)
			cur = word
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// ---------- Assemblage ----------

func fmtUptime(d time.Duration) string {
	d = d.Round(time.Minute)
	if days := int(d.Hours()) / 24; days > 0 {
		return fmt.Sprintf("%dj %dh", days, int(d.Hours())%24)
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

func maxLines(ls ...[]string) int {
	n := 0
	for _, l := range ls {
		n = max(n, len(l))
	}
	return n
}

func (m model) header(W int) string {
	logo := lipgloss.NewStyle().Background(cAccent).Foreground(cBg).Bold(true).Render(" ◆ BORJ ") + sDim.Render(" "+version)
	ram := m.snap.MemUsedPct()
	chips := []string{
		pill(fmt.Sprintf("RAM %.0f%%", ram), levelColor(ram)),
		pill(fmt.Sprintf("COMMIT %.0f%%", m.snap.CommitPct()), levelColor(m.snap.CommitPct())),
		pill(fmt.Sprintf("CPU %.0f%%", m.snap.CPUTotal), levelColor(m.snap.CPUTotal)),
	}
	if m.labOn {
		chips = append(chips, pill("LABO 🍃", cOK))
	}
	if m.alert.Active() && time.Now().Second()%2 == 0 {
		chips = append(chips, pill("⚠ MÉMOIRE SATURÉE", cBad))
	}
	adminTag := fg(cWarn).Render("non-admin")
	modelName := ""
	if m.health != nil {
		modelName = m.health.Model + " · "
		if m.health.Admin {
			adminTag = fg(cOK).Render("admin")
		}
	}
	left := logo + " " + strings.Join(chips, " ") + sDim.Render(fmt.Sprintf("  %sallumé depuis %s · %d processus · ", modelName, fmtUptime(windows.DurationSinceBoot()), m.snap.Procs)) + adminTag
	right := fg(cCyan).Bold(true).Render(time.Now().Format("15:04:05")) + " "
	return lipgloss.NewStyle().MaxWidth(W).Render(left + strings.Repeat(" ", max(1, W-lipgloss.Width(left)-lipgloss.Width(right))) + right)
}

func (m model) statusLine(W int, withKeys bool) string {
	var s string
	switch {
	case m.confirm != nil:
		s = pill("⚠ CONFIRMER", cBad) + fg(cBad).Bold(true).Render(" "+m.confirm.label+" ? ") + sText.Render("o = oui · autre touche = non")
	case m.filtering:
		s = pill("RECHERCHE", cCyan) + " " + sText.Bold(true).Render(m.filter+"█") + sDim.Render("   Entrée valider · Échap annuler")
	case m.flash != "" && time.Since(m.flashAt) < 6*time.Second:
		if m.flashBad {
			s = fg(cBad).Bold(true).Render(" " + m.flash)
		} else {
			s = fg(cOK).Render(" ✔ " + m.flash)
		}
	case m.filter != "":
		s = fg(cWarn).Render(" filtre actif : « " + m.filter + " » — Échap pour l'enlever")
	case withKeys:
		s = sDim.Render(" Tab onglet · ↑↓ naviguer · Entrée déplier · / chercher · x arrêter · e éco · L labo · q quitter")
	default:
		s = sDim.Render(" prêt")
	}
	if m.hBusy {
		s += fg(cCyan).Render("   ⟳ analyse santé…")
	}
	return lipgloss.NewStyle().MaxWidth(W).Render(s)
}

func (m model) tabs() string {
	names := []string{"1 APPLICATIONS", "2 MACHINES VIRTUELLES", "3 HISTORIQUE"}
	var parts []string
	for i, n := range names {
		if i == m.tab {
			parts = append(parts, pill(n, cTitle))
		} else {
			parts = append(parts, sDim.Render(" "+n+" "))
		}
	}
	return strings.Join(parts, " ")
}

func (m model) View() string {
	W, H := m.width, m.height
	if W <= 0 {
		W = 180
	}
	if H <= 0 {
		H = 50
	}

	side := 0
	if W >= 150 {
		side = 40
	}
	mw := W - side

	var top, health string
	if mw >= 105 {
		a, b := mw/3, mw/3
		c := mw - a - b
		cb, rb, ib := m.cpuBody(a), m.ramBody(b), m.ioBody(c)
		h1 := maxLines(cb, rb, ib) + 3
		top = lipgloss.JoinHorizontal(lipgloss.Top,
			panel(title("▣", "PROCESSEUR", cCyan), fg(levelColor(m.snap.CPUTotal)).Bold(true).Render(fmt.Sprintf("%.0f%%", m.snap.CPUTotal)), strings.Join(cb, "\n"), a, h1, cCyan),
			panel(title("▤", "MÉMOIRE", cAccent), fg(levelColor(m.snap.MemUsedPct())).Bold(true).Render(fmt.Sprintf("%.0f%%", m.snap.MemUsedPct())), strings.Join(rb, "\n"), b, h1, cAccent),
			panel(title("⇅", "RÉSEAU & DISQUE", cBlue), "", strings.Join(ib, "\n"), c, h1, cBlue))

		v1, c1, l1 := m.ssdCard(a)
		v2, c2, l2 := m.cpuCard(b)
		v3, c3, l3 := m.ramCard(c)
		hh := maxLines(l1, l2, l3) + 3
		health = lipgloss.JoinHorizontal(lipgloss.Top,
			panel(title("◈", "SANTÉ DISQUES", c1), v1, strings.Join(l1, "\n"), a, hh, c1),
			panel(title("◈", "SANTÉ CPU", c2), v2, strings.Join(l2, "\n"), b, hh, c2),
			panel(title("◈", "SANTÉ RAM", c3), v3, strings.Join(l3, "\n"), c, hh, c3))
	} else {
		v1, c1, l1 := m.ssdCard(mw)
		v2, c2, l2 := m.cpuCard(mw)
		v3, c3, l3 := m.ramCard(mw)
		top = lipgloss.JoinVertical(lipgloss.Left,
			panel(title("▣", "PROCESSEUR", cCyan), "", strings.Join(m.cpuBody(mw), "\n"), mw, 0, cCyan),
			panel(title("▤", "MÉMOIRE", cAccent), "", strings.Join(m.ramBody(mw), "\n"), mw, 0, cAccent),
			panel(title("⇅", "RÉSEAU & DISQUE", cBlue), "", strings.Join(m.ioBody(mw), "\n"), mw, 0, cBlue))
		health = lipgloss.JoinVertical(lipgloss.Left,
			panel(title("◈", "SANTÉ DISQUES", c1), v1, strings.Join(l1, "\n"), mw, 0, c1),
			panel(title("◈", "SANTÉ CPU", c2), v2, strings.Join(l2, "\n"), mw, 0, c2),
			panel(title("◈", "SANTÉ RAM", c3), v3, strings.Join(l3, "\n"), mw, 0, c3))
	}

	// hauteur du panneau central = reste de l'écran
	midH := max(8, H-2-lipgloss.Height(top)-lipgloss.Height(health))
	n := midH - 5 // bordure (2) + onglets (1) + en-têtes (1) + pagination (1)
	var body []string
	var right string
	sortName := map[byte]string{'m': "RAM", 'c': "CPU", 'd': "disque", 'n': "nom"}[m.sortKey]
	switch m.tab {
	case tabApps:
		body = m.appsBody(mw, n)
		right = sDim.Render(fmt.Sprintf("%d apps · tri %s", len(m.snap.Families), sortName))
	case tabVMs:
		body = m.vmsBody(mw, n)
		if m.vmBusy {
			right = fg(cCyan).Render("⟳ lecture…")
		}
	case tabHist:
		body = m.histBody(mw, n+1)
	}
	mid := panel(m.tabs(), right, strings.Join(body, "\n"), mw, midH, cTitle)

	main := lipgloss.JoinVertical(lipgloss.Left, top, mid, health)
	if side > 0 {
		main = lipgloss.JoinHorizontal(lipgloss.Top, main, m.sidebar(side, lipgloss.Height(main)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.header(W), main, m.statusLine(W, side == 0))
}
