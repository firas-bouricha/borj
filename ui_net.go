package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var helpNet = []help{
	{"i", "IP publique", "la demande à Cloudflare (1.1.1.1). Aucune requête sans cette touche"},
	{"v", "Afficher / masquer", "l'IP publique est masquée par défaut (captures, partage d'écran)"},
	{"↑ ↓", "Faire défiler", "la liste des ports ouverts"},
	{"", "Ports", "● rouge = exposé : écoute sur le réseau, d'autres machines peuvent tenter de s'y connecter (le pare-feu Windows peut encore bloquer). ● vert = local uniquement"},
}

// appOf : nom d'application pour un PID, à partir des familles déjà regroupées.
func (m model) appOf() func(uint32) string {
	byPID := map[uint32]string{}
	for _, f := range m.snap.Families {
		for _, p := range f.Members {
			byPID[p.PID] = f.Name
		}
	}
	return func(pid uint32) string {
		switch pid {
		case 0:
			return "Windows"
		case 4:
			return "Windows (noyau)"
		}
		if n, ok := byPID[pid]; ok {
			return n
		}
		return fmt.Sprintf("PID %d", pid)
	}
}

func section(t string, c lipgloss.Color, inner int, right string) string {
	head := fg(c).Bold(true).Render(t)
	if right != "" {
		head += " " + sDim.Render(right)
	}
	return head + " " + fg(cBorder).Render(strings.Repeat("─", max(0, inner-lipgloss.Width(head)-1)))
}

func (m model) netBody(w, n int) []string {
	inner := w - 4
	var lines []string

	// ── Mes adresses
	lines = append(lines, section("MES ADRESSES", cAccent, inner, ""))
	var pub string
	switch {
	case m.pubBusy:
		pub = fg(cCyan).Render("⟳ demande en cours…")
	case m.pubIP == "":
		pub = sDim.Render("inconnue — appuie sur ") + fg(cCyan).Bold(true).Render("i") + sDim.Render(" pour la demander")
	case m.pubShow:
		pub = fg(cWarn).Bold(true).Render(m.pubIP) + sDim.Render("   v pour masquer")
	default:
		pub = sText.Bold(true).Render(maskIP(m.pubIP)) + sDim.Render("   masquée · v pour afficher · i pour actualiser")
	}
	if m.pubErr != nil && !m.pubBusy {
		pub += fg(cBad).Render("   " + m.pubErr.Error())
	}
	lines = append(lines, sDim.Render(pad("  Publique", 15))+pub)
	if len(m.ifaces) == 0 {
		lines = append(lines, sDim.Render("  aucune carte réseau active"))
	}
	for _, it := range m.ifaces {
		ip := sText.Bold(true).Render(strings.Join(it.IPv4, ", "))
		extra := ""
		if it.Gateway != "" {
			extra = " · passerelle " + it.Gateway
		}
		if it.Name != it.Kind {
			extra += " · " + it.Name
		}
		kindCol := cCyan
		if it.Kind == "Wi-Fi" || it.Kind == "Ethernet" {
			kindCol = cOK
		}
		lines = append(lines, fg(kindCol).Render(pad("  "+it.Kind, 15))+ip+sDim.Render(trunc(extra, max(0, inner-15-lipgloss.Width(ip)))))
	}

	view := buildNetView(m.socks, m.appOf())
	room := max(4, n-len(lines)-3)
	portRows := max(2, room*3/5)
	connRows := max(1, room-portRows-3)

	// ── Ports ouverts
	exposed := 0
	for _, p := range view.Ports {
		if p.Exposed {
			exposed++
		}
	}
	lines = append(lines, "", section("PORTS OUVERTS", cTitle, inner,
		fmt.Sprintf("%d exposés au réseau · %d locaux", exposed, len(view.Ports)-exposed)))
	lines = append(lines, sDim.Render("    "+fmt.Sprintf("%5s  ", "PORT")+pad("TYPE", 5)+pad("SERVICE", 26)+pad("APPLICATION", 24)+"ACCÈS"))
	off := max(0, min(m.netOff, len(view.Ports)-portRows))
	if off > 0 {
		lines = append(lines, sDim.Render(fmt.Sprintf("    ↑ %d ports au-dessus", off)))
		portRows--
	}
	for i := off; i < len(view.Ports); i++ {
		p := view.Ports[i]
		if i-off >= portRows {
			lines = append(lines, sDim.Render(fmt.Sprintf("    ↓ %d autres ports", len(view.Ports)-i)))
			break
		}
		dot, access := fg(cOK).Render("●"), fg(cOK).Render("local uniquement")
		if p.Exposed {
			dot = fg(cBad).Render("●")
			access = fg(cBad).Bold(true).Render("exposé") + sDim.Render(" · "+p.Scope)
		}
		svc := p.Service
		if svc == "" {
			svc = "—"
		}
		lines = append(lines, "  "+dot+" "+sText.Bold(true).Render(fmt.Sprintf("%5d  ", p.Port))+sDim.Render(pad(p.Proto, 5))+
			sText.Render(pad(svc, 25)+" ")+fg(cAccent).Render(pad(p.App, 23)+" ")+access)
	}

	// ── Connexions actives
	total := 0
	for _, c := range view.Conns {
		total += c.Count
	}
	lines = append(lines, "", section("CONNEXIONS ACTIVES", cBlue, inner, fmt.Sprintf("%d connexions vers Internet ou le réseau local", total)))
	if len(view.Conns) == 0 {
		lines = append(lines, sDim.Render("  aucune connexion établie"))
	}
	for i, c := range view.Conns {
		if i >= connRows {
			lines = append(lines, sDim.Render(fmt.Sprintf("  + %d autres applications", len(view.Conns)-connRows)))
			break
		}
		more := ""
		if len(c.Remotes) > 0 {
			more = "→ " + strings.Join(c.Remotes, ", ")
		}
		cnt := fmt.Sprintf("%3d connexion", c.Count)
		if c.Count > 1 {
			cnt += "s"
		}
		lines = append(lines, "  "+fg(cAccent).Bold(true).Render(pad(c.App, 24))+sText.Render(pad(cnt, 16))+sDim.Render(trunc(more, max(0, inner-42))))
	}
	return lines
}
