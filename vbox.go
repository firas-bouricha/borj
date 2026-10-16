package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Pilotage VirtualBox via VBoxManage. Les actions passent par l'UUID (les noms peuvent contenir des espaces).

type VM struct {
	Name    string
	UUID    string
	State   string // running, paused, saved, powered off, aborted...
	MemMB   int
	CPUs    int
	Balloon int // Mo repris à la VM par le ballon
}

func (v VM) Running() bool { return v.State == "running" }
func (v VM) Paused() bool  { return v.State == "paused" }

// vboxManage : seulement dans le dossier d'installation de VirtualBox (variable posée par son
// installeur, sinon Program Files). Jamais via le PATH, pour éviter un exécutable usurpé.
var vboxManage = func() string {
	for _, dir := range []string{os.Getenv("VBOX_MSI_INSTALL_PATH"), filepath.Join(os.Getenv("ProgramFiles"), "Oracle", "VirtualBox")} {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, "VBoxManage.exe")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}()

func vbox(args ...string) ([]byte, error) {
	if vboxManage == "" {
		return nil, fmt.Errorf("VirtualBox non trouvé")
	}
	cmd := exec.Command(vboxManage, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if i := strings.LastIndex(msg, "error:"); i >= 0 {
			msg = strings.TrimSpace(msg[i+6:])
		}
		return out, fmt.Errorf("%s", firstLine(msg))
	}
	return out, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func atoiMB(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "MB")))
	return n
}

// listVMs : un seul appel "list -l vms" pour toutes les VMs.
func listVMs() ([]VM, error) {
	out, err := vbox("list", "-l", "vms")
	if err != nil {
		return nil, err
	}
	var vms []VM
	var cur *VM
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "Name":
			// "Name: 'x', Host path" = dossier partagé, pas une VM
			if strings.HasPrefix(v, "'") {
				continue
			}
			if cur == nil || cur.UUID != "" {
				vms = append(vms, VM{Name: v})
				cur = &vms[len(vms)-1]
			}
		case "UUID":
			if cur != nil && cur.UUID == "" {
				cur.UUID = v
			}
		case "Memory size":
			if cur != nil && cur.MemMB == 0 {
				cur.MemMB = atoiMB(v)
			}
		case "Number of CPUs":
			if cur != nil && cur.CPUs == 0 {
				cur.CPUs, _ = strconv.Atoi(v)
			}
		case "State":
			if cur != nil && cur.State == "" {
				if i := strings.Index(v, " (since"); i > 0 {
					v = v[:i]
				}
				cur.State = v
			}
		case "Configured memory balloon":
			if cur != nil {
				cur.Balloon = atoiMB(v)
			}
		}
	}
	return vms, nil
}

func stateFR(s string) string {
	switch s {
	case "running":
		return "allumée"
	case "paused":
		return "en pause"
	case "saved":
		return "sauvegardée"
	case "powered off":
		return "éteinte"
	case "aborted":
		return "plantée"
	case "starting", "restoring":
		return "démarrage…"
	case "saving":
		return "sauvegarde…"
	case "stopping":
		return "arrêt…"
	}
	return s
}

// Actions (bloquantes : appelées dans une tea.Cmd)

func vmStart(v VM) error { _, err := vbox("startvm", v.UUID, "--type", "headless"); return err }

func vmTogglePause(v VM) error {
	op := "pause"
	if v.Paused() {
		op = "resume"
	}
	_, err := vbox("controlvm", v.UUID, op)
	return err
}

func vmSave(v VM) error { _, err := vbox("controlvm", v.UUID, "savestate"); return err }
func vmACPI(v VM) error { _, err := vbox("controlvm", v.UUID, "acpipowerbutton"); return err }

// vmBalloon : ballon = RAM reprise à la VM (Guest Additions requises). Borné à 0..75 % de la RAM de la VM.
func vmBalloon(v VM, deltaMB int) (int, error) {
	n := v.Balloon + deltaMB
	n = max(0, min(n, v.MemMB*3/4))
	_, err := vbox("controlvm", v.UUID, "guestmemoryballoon", strconv.Itoa(n))
	return n, err
}
