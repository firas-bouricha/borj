package main

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// Alertes : notification Windows + bandeau "mémoire saturée" quand RAM ou commit >= alertOn.
// Hystérésis : réarmement sous alertOff pour ne pas spammer.
// Les pics (historique) sont notés plus tôt, dès peakOn.

const (
	alertOn  = 95.0
	alertOff = 90.0
	peakOn   = 85.0
	peakOff  = 80.0
)

type alerter struct {
	ramActive    bool
	commitActive bool
}

// check renvoie un message si une nouvelle alerte se déclenche.
func (a *alerter) check(s Snapshot, top string) (title, body string) {
	ram, com := s.MemUsedPct(), s.CommitPct()
	var parts []string
	if !a.ramActive && ram >= alertOn {
		a.ramActive = true
		parts = append(parts, fmt.Sprintf("RAM %.0f %%", ram))
	} else if a.ramActive && ram < alertOff {
		a.ramActive = false
	}
	if !a.commitActive && com >= alertOn {
		a.commitActive = true
		parts = append(parts, fmt.Sprintf("commit %.0f %% (risque de plantage)", com))
	} else if a.commitActive && com < alertOff {
		a.commitActive = false
	}
	if len(parts) == 0 {
		return "", ""
	}
	return "BORJ : mémoire saturée", strings.Join(parts, " · ") + ". Plus gros : " + top
}

func (a *alerter) Active() bool { return a.ramActive || a.commitActive }

// toast : notification native Windows (WinRT via PowerShell, AppID de PowerShell déjà enregistré).
func toast(title, body string) {
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "'", "''").Replace
	script := `
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null
$x = New-Object Windows.Data.Xml.Dom.XmlDocument
$x.LoadXml('<toast scenario="urgent"><visual><binding template="ToastGeneric"><text>` + esc(title) + `</text><text>` + esc(body) + `</text></binding></visual></toast>')
$id = '{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe'
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($id).Show([Windows.UI.Notifications.ToastNotification]::new($x))
`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encodePS(script))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if cmd.Start() == nil {
		go cmd.Wait()
	}
}
