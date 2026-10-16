package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ---------- Adresses des cartes réseau ----------

type Iface struct {
	Kind    string // Wi-Fi, Ethernet, VirtualBox, WSL / Docker, Tailscale, VPN, Autre
	Name    string // nom Windows de la carte
	IPv4    []string
	Gateway string
}

// ifaceKind : nom simple à partir du nom / de la description de la carte.
func ifaceKind(name, desc string, ifType uint32) string {
	s := strings.ToLower(name + " " + desc)
	switch {
	case strings.Contains(s, "virtualbox"):
		return "VirtualBox"
	case strings.Contains(s, "wsl") || strings.Contains(s, "docker"):
		return "WSL / Docker"
	case strings.Contains(s, "hyper-v") || strings.Contains(s, "vethernet"):
		return "Hyper-V"
	case strings.Contains(s, "tailscale"):
		return "Tailscale"
	case strings.Contains(s, "vmware"):
		return "VMware"
	case strings.Contains(s, "wireguard") || strings.Contains(s, "openvpn") || strings.Contains(s, "tap-windows") || strings.Contains(s, "vpn"):
		return "VPN"
	case ifType == windows.IF_TYPE_IEEE80211:
		return "Wi-Fi"
	case ifType == windows.IF_TYPE_ETHERNET_CSMACD:
		return "Ethernet"
	}
	return "Autre"
}

func listIfaces() []Iface {
	size := uint32(32 * 1024)
	var buf []byte
	for i := 0; i < 3; i++ {
		buf = make([]byte, size)
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC,
			windows.GAA_FLAG_SKIP_ANYCAST|windows.GAA_FLAG_SKIP_MULTICAST|windows.GAA_FLAG_SKIP_DNS_SERVER|windows.GAA_FLAG_INCLUDE_GATEWAYS,
			0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil {
			break
		}
		if err != windows.ERROR_BUFFER_OVERFLOW {
			return nil
		}
	}
	var out []Iface
	for a := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); a != nil; a = a.Next {
		if a.OperStatus != windows.IfOperStatusUp || a.IfType == windows.IF_TYPE_SOFTWARE_LOOPBACK {
			continue
		}
		name := windows.UTF16PtrToString(a.FriendlyName)
		desc := windows.UTF16PtrToString(a.Description)
		it := Iface{Kind: ifaceKind(name, desc, a.IfType), Name: name}
		for u := a.FirstUnicastAddress; u != nil; u = u.Next {
			if ip := u.Address.IP(); ip != nil && ip.To4() != nil && !ip.IsLinkLocalUnicast() {
				it.IPv4 = append(it.IPv4, ip.String())
			}
		}
		for g := a.FirstGatewayAddress; g != nil; g = g.Next {
			if ip := g.Address.IP(); ip != nil && ip.To4() != nil {
				it.Gateway = ip.String()
				break
			}
		}
		if len(it.IPv4) > 0 {
			out = append(out, it)
		}
	}
	// cartes physiques d'abord
	rank := map[string]int{"Wi-Fi": 0, "Ethernet": 1, "VPN": 2, "Tailscale": 3, "VirtualBox": 4, "WSL / Docker": 5, "Hyper-V": 6, "VMware": 7}
	sort.SliceStable(out, func(i, j int) bool {
		ri, ok := rank[out[i].Kind]
		if !ok {
			ri = 9
		}
		rj, ok := rank[out[j].Kind]
		if !ok {
			rj = 9
		}
		return ri < rj
	})
	return out
}

// ---------- IP publique (à la demande uniquement) ----------

// publicIP : une seule requête HTTPS vers Cloudflare, déclenchée par l'utilisateur (touche i).
func publicIP() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://1.1.1.1/cdn-cgi/trace", nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("pas de réponse de Cloudflare")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(body), "\n") {
		if ip, ok := strings.CutPrefix(line, "ip="); ok {
			if parsed := net.ParseIP(strings.TrimSpace(ip)); parsed != nil {
				return parsed.String(), nil
			}
		}
	}
	return "", fmt.Errorf("réponse inattendue")
}

// maskIP : 41.226.12.7 → 41.226.•••.••• ; IPv6 : seuls les 2 premiers groupes restent visibles.
func maskIP(ip string) string {
	if p := strings.Split(ip, "."); len(p) == 4 {
		return p[0] + "." + p[1] + ".•••.•••"
	}
	if p := strings.Split(ip, ":"); len(p) > 2 {
		return p[0] + ":" + p[1] + ":••••:••••"
	}
	return "•••"
}

// ---------- Ports et connexions ----------

const (
	tcpTableOwnerPIDAll = 5
	udpTableOwnerPID    = 1
	tcpStateListen      = 2
	tcpStateEstablished = 5
	ephemeralStart      = 49152
)

var (
	iphlpapi                = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTcpTable = iphlpapi.NewProc("GetExtendedTcpTable")
	procGetExtendedUdpTable = iphlpapi.NewProc("GetExtendedUdpTable")
)

type Socket struct {
	Proto  string // TCP, UDP
	Local  net.IP
	Port   uint16
	Remote net.IP
	RPort  uint16
	State  uint32
	PID    uint32
}

// ntohs : les ports sont stockés en ordre réseau dans les 16 bits de poids faible.
func ntohs(v uint32) uint16 { return uint16(v>>8&0xff | v<<8&0xff00) }

func extendedTable(proc *windows.LazyProc, family uint32, class uint32) []byte {
	size := uint32(64 * 1024)
	for i := 0; i < 4; i++ {
		buf := make([]byte, size)
		r, _, _ := proc.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, uintptr(family), uintptr(class), 0)
		if r == 0 {
			return buf[:size]
		}
		if r != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) {
			return nil
		}
		size += 16 * 1024
	}
	return nil
}

func le32(b []byte, off int) uint32 { return binary.LittleEndian.Uint32(b[off:]) }

// listSockets : sockets TCP (écoute + établies) et UDP, IPv4 et IPv6, avec le PID propriétaire. Sans admin.
func listSockets() []Socket {
	var out []Socket
	// MIB_TCPROW_OWNER_PID : state, localAddr, localPort, remoteAddr, remotePort, pid (6 × 4 octets)
	if b := extendedTable(procGetExtendedTcpTable, windows.AF_INET, tcpTableOwnerPIDAll); len(b) >= 4 {
		n := int(le32(b, 0))
		for i := 0; i < n && 4+(i+1)*24 <= len(b); i++ {
			r := b[4+i*24:]
			out = append(out, Socket{Proto: "TCP", State: le32(r, 0),
				Local: net.IP(append([]byte(nil), r[4:8]...)), Port: ntohs(le32(r, 8)),
				Remote: net.IP(append([]byte(nil), r[12:16]...)), RPort: ntohs(le32(r, 16)), PID: le32(r, 20)})
		}
	}
	// MIB_TCP6ROW_OWNER_PID : localAddr[16], scope, localPort, remoteAddr[16], scope, remotePort, state, pid (56 octets)
	if b := extendedTable(procGetExtendedTcpTable, windows.AF_INET6, tcpTableOwnerPIDAll); len(b) >= 4 {
		n := int(le32(b, 0))
		for i := 0; i < n && 4+(i+1)*56 <= len(b); i++ {
			r := b[4+i*56:]
			out = append(out, Socket{Proto: "TCP",
				Local: net.IP(append([]byte(nil), r[0:16]...)), Port: ntohs(le32(r, 20)),
				Remote: net.IP(append([]byte(nil), r[24:40]...)), RPort: ntohs(le32(r, 44)),
				State: le32(r, 48), PID: le32(r, 52)})
		}
	}
	// MIB_UDPROW_OWNER_PID : localAddr, localPort, pid (12 octets)
	if b := extendedTable(procGetExtendedUdpTable, windows.AF_INET, udpTableOwnerPID); len(b) >= 4 {
		n := int(le32(b, 0))
		for i := 0; i < n && 4+(i+1)*12 <= len(b); i++ {
			r := b[4+i*12:]
			out = append(out, Socket{Proto: "UDP", Local: net.IP(append([]byte(nil), r[0:4]...)), Port: ntohs(le32(r, 4)), PID: le32(r, 8)})
		}
	}
	// MIB_UDP6ROW_OWNER_PID : localAddr[16], scope, localPort, pid (28 octets)
	if b := extendedTable(procGetExtendedUdpTable, windows.AF_INET6, udpTableOwnerPID); len(b) >= 4 {
		n := int(le32(b, 0))
		for i := 0; i < n && 4+(i+1)*28 <= len(b); i++ {
			r := b[4+i*28:]
			out = append(out, Socket{Proto: "UDP", Local: net.IP(append([]byte(nil), r[0:16]...)), Port: ntohs(le32(r, 20)), PID: le32(r, 24)})
		}
	}
	return out
}

// ---------- Vue simplifiée ----------

var wellKnown = map[uint16]string{
	20: "FTP données", 21: "FTP", 22: "SSH", 23: "Telnet", 25: "SMTP", 53: "DNS", 67: "DHCP", 68: "DHCP",
	80: "HTTP", 110: "POP3", 123: "Heure (NTP)", 135: "RPC Windows", 137: "NetBIOS", 138: "NetBIOS",
	139: "NetBIOS", 143: "IMAP", 161: "SNMP", 389: "LDAP", 443: "HTTPS", 445: "Partage Windows (SMB)",
	500: "VPN (IKE)", 514: "Syslog", 636: "LDAPS", 902: "VMware (console)", 912: "VMware (auth)",
	1433: "SQL Server", 1434: "SQL Server", 1900: "Découverte (SSDP)", 2179: "Hyper-V (console)",
	3702: "Découverte (WS-Discovery)", 5050: "Serveur de dev", 5357: "Découverte d'appareils (WSD)",
	5800: "VNC (web)", 5901: "VNC", 8081: "Serveur web", 8888: "Serveur web / Jupyter", 41641: "Tailscale",
	3000: "Serveur de dev", 3306: "MySQL", 3389: "Bureau à distance", 4500: "VPN (IPsec)", 5000: "Serveur de dev",
	5040: "Service Windows", 5353: "mDNS", 5355: "LLMNR", 5432: "PostgreSQL", 5900: "VNC", 5985: "WinRM",
	5986: "WinRM HTTPS", 6379: "Redis", 7680: "Livraison Windows Update", 8000: "Serveur web", 8080: "Serveur web",
	8443: "HTTPS alternatif", 9000: "Serveur web", 9229: "Débogage Node.js", 11434: "Ollama", 27017: "MongoDB",
}

type OpenPort struct {
	Proto   string
	Port    uint16
	Service string
	App     string
	Exposed bool   // écoute sur toutes les cartes ou une carte réelle
	Scope   string // "toutes les cartes", "local uniquement", ou l'IP précise
}

type AppConns struct {
	App     string
	Count   int
	Remotes []string // adresses distantes les plus fréquentes
}

type NetView struct {
	Ports []OpenPort
	Conns []AppConns
}

// buildNetView : regroupe les sockets par port et par application. appOf(pid) donne le nom de l'application.
func buildNetView(socks []Socket, appOf func(uint32) string) NetView {
	type key struct {
		proto string
		port  uint16
		app   string
	}
	ports := map[key]*OpenPort{}
	conns := map[string]map[string]int{}

	for _, s := range socks {
		app := appOf(s.PID)
		listening := s.Proto == "TCP" && s.State == tcpStateListen || s.Proto == "UDP" && s.Port < ephemeralStart && s.Port != 0
		if listening {
			k := key{s.Proto, s.Port, app}
			p := ports[k]
			if p == nil {
				p = &OpenPort{Proto: s.Proto, Port: s.Port, Service: wellKnown[s.Port], App: app, Scope: "local uniquement"}
				ports[k] = p
			}
			switch {
			case s.Local.IsUnspecified():
				p.Exposed, p.Scope = true, "toutes les cartes"
			case !s.Local.IsLoopback() && !p.Exposed:
				p.Exposed, p.Scope = true, s.Local.String()
			}
			continue
		}
		if s.Proto == "TCP" && s.State == tcpStateEstablished && !s.Remote.IsLoopback() && !s.Remote.IsUnspecified() {
			if conns[app] == nil {
				conns[app] = map[string]int{}
			}
			conns[app][s.Remote.String()]++
		}
	}

	var v NetView
	for _, p := range ports {
		v.Ports = append(v.Ports, *p)
	}
	// exposés d'abord, puis TCP, puis par numéro
	sort.Slice(v.Ports, func(i, j int) bool {
		a, b := v.Ports[i], v.Ports[j]
		if a.Exposed != b.Exposed {
			return a.Exposed
		}
		if a.Proto != b.Proto {
			return a.Proto == "TCP"
		}
		return a.Port < b.Port
	})
	for app, remotes := range conns {
		c := AppConns{App: app}
		type rc struct {
			ip string
			n  int
		}
		var list []rc
		for ip, n := range remotes {
			c.Count += n
			list = append(list, rc{ip, n})
		}
		sort.Slice(list, func(i, j int) bool { return list[i].n > list[j].n || list[i].n == list[j].n && list[i].ip < list[j].ip })
		for i := 0; i < min(3, len(list)); i++ {
			c.Remotes = append(c.Remotes, list[i].ip)
		}
		v.Conns = append(v.Conns, c)
	}
	sort.Slice(v.Conns, func(i, j int) bool {
		return v.Conns[i].Count > v.Conns[j].Count || v.Conns[i].Count == v.Conns[j].Count && v.Conns[i].App < v.Conns[j].App
	})
	return v
}
