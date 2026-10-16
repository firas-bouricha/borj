package main

import (
	"net"
	"os"
	"testing"
)

func TestNtohs(t *testing.T) {
	// port 445 = 0x01BD, stocké en ordre réseau puis lu en little-endian
	if got := ntohs(0x0000BD01); got != 445 {
		t.Fatalf("ntohs = %d, attendu 445", got)
	}
}

func TestMaskIP(t *testing.T) {
	cases := map[string]string{
		"41.226.12.7":       "41.226.•••.•••",
		"2a01:e0a:12:34::1": "2a01:e0a:••••:••••",
		"pas-une-ip":        "•••",
	}
	for in, want := range cases {
		if got := maskIP(in); got != want {
			t.Errorf("maskIP(%q) = %q, attendu %q", in, got, want)
		}
	}
}

// Test réseau réel, désactivé par défaut (CI) : BORJ_NET_TEST=1 go test -run PublicIP
func TestPublicIPLive(t *testing.T) {
	if os.Getenv("BORJ_NET_TEST") != "1" {
		t.Skip("requête réseau : BORJ_NET_TEST=1 pour l'activer")
	}
	ip, err := publicIP()
	if err != nil {
		t.Fatal(err)
	}
	if net.ParseIP(ip) == nil {
		t.Fatalf("IP invalide")
	}
	t.Logf("IP publique reçue : %s", maskIP(ip))
}

func TestIfaceKind(t *testing.T) {
	cases := []struct {
		name, desc string
		typ        uint32
		want       string
	}{
		{"Ethernet 2", "VirtualBox Host-Only Ethernet Adapter", 6, "VirtualBox"},
		{"vEthernet (WSL (Hyper-V firewall))", "Hyper-V Virtual Ethernet Adapter", 6, "WSL / Docker"},
		{"Wi-Fi", "Intel Wi-Fi 6 AX201", 71, "Wi-Fi"},
		{"Tailscale", "Tailscale Tunnel", 53, "Tailscale"},
	}
	for _, c := range cases {
		if got := ifaceKind(c.name, c.desc, c.typ); got != c.want {
			t.Errorf("%s : %s, attendu %s", c.name, got, c.want)
		}
	}
}

func TestBuildNetView(t *testing.T) {
	socks := []Socket{
		{Proto: "TCP", Local: net.IPv4zero, Port: 445, State: tcpStateListen, PID: 4},
		{Proto: "TCP", Local: net.IPv6unspecified, Port: 445, State: tcpStateListen, PID: 4}, // doublon v6
		{Proto: "TCP", Local: net.IPv4(127, 0, 0, 1), Port: 9229, State: tcpStateListen, PID: 10},
		{Proto: "TCP", Local: net.IPv4(10, 0, 0, 2), Port: 51000, Remote: net.IPv4(1, 1, 1, 1), RPort: 443, State: tcpStateEstablished, PID: 20},
		{Proto: "TCP", Local: net.IPv4(127, 0, 0, 1), Port: 51001, Remote: net.IPv4(127, 0, 0, 1), RPort: 9229, State: tcpStateEstablished, PID: 20},
		{Proto: "UDP", Local: net.IPv4zero, Port: 60000, PID: 20}, // port temporaire : ignoré
	}
	names := map[uint32]string{4: "Windows (noyau)", 10: "Claude", 20: "Brave"}
	v := buildNetView(socks, func(pid uint32) string { return names[pid] })

	if len(v.Ports) != 2 {
		t.Fatalf("%d ports, attendu 2 : %+v", len(v.Ports), v.Ports)
	}
	if p := v.Ports[0]; p.Port != 445 || !p.Exposed || p.Service == "" {
		t.Errorf("445 doit être en tête et exposé : %+v", p)
	}
	if p := v.Ports[1]; p.Port != 9229 || p.Exposed {
		t.Errorf("9229 doit être local uniquement : %+v", p)
	}
	if len(v.Conns) != 1 || v.Conns[0].App != "Brave" || v.Conns[0].Count != 1 {
		t.Errorf("connexions (loopback exclue) : %+v", v.Conns)
	}
}
