package main

import (
	"errors"
	"runtime"
	"sort"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Classes NtQuerySystemInformation : un seul appel donne tous les processus
// (mémoire privée, temps CPU, E/S, parent) sans OpenProcess ni WMI.
const (
	classProcessInformation   = 5
	classProcessorPerformance = 8
)

// SYSTEM_PROCESS_INFORMATION (x64). Offsets vérifiés : UniqueProcessId=0x50, PrivatePageCount=0xC8.
type sysProcInfo struct {
	NextEntryOffset              uint32
	NumberOfThreads              uint32
	WorkingSetPrivateSize        int64
	HardFaultCount               uint32
	NumberOfThreadsHighWatermark uint32
	CycleTime                    uint64
	CreateTime                   int64
	UserTime                     int64
	KernelTime                   int64
	ImageName                    windows.NTUnicodeString
	BasePriority                 int32
	UniqueProcessId              uintptr
	InheritedFromUniqueProcessId uintptr
	HandleCount                  uint32
	SessionId                    uint32
	UniqueProcessKey             uintptr
	PeakVirtualSize              uintptr
	VirtualSize                  uintptr
	PageFaultCount               uint32
	PeakWorkingSetSize           uintptr
	WorkingSetSize               uintptr
	QuotaPeakPagedPoolUsage      uintptr
	QuotaPagedPoolUsage          uintptr
	QuotaPeakNonPagedPoolUsage   uintptr
	QuotaNonPagedPoolUsage       uintptr
	PagefileUsage                uintptr
	PeakPagefileUsage            uintptr
	PrivatePageCount             uintptr
	ReadOperationCount           int64
	WriteOperationCount          int64
	OtherOperationCount          int64
	ReadTransferCount            int64
	WriteTransferCount           int64
	OtherTransferCount           int64
}

// SYSTEM_PROCESSOR_PERFORMANCE_INFORMATION (KernelTime inclut IdleTime).
type corePerf struct {
	Idle, Kernel, User, Dpc, Interrupt int64
	InterruptCount                     uint32
	_                                  uint32
}

// PERFORMANCE_INFORMATION (psapi) : valeurs en pages sauf PageSize.
type perfInfo struct {
	Cb                                                 uint32
	CommitTotal, CommitLimit, CommitPeak               uintptr
	PhysicalTotal, PhysicalAvailable, SystemCache      uintptr
	KernelTotal, KernelPaged, KernelNonpaged, PageSize uintptr
	HandleCount, ProcessCount, ThreadCount             uint32
}

// PROCESS_POWER_THROTTLING_STATE : EcoQoS = "mode efficacité" du Gestionnaire des tâches.
type powerThrottling struct {
	Version, ControlMask, StateMask uint32
}

var (
	procGetPerformanceInfo   = windows.NewLazySystemDLL("psapi.dll").NewProc("GetPerformanceInfo")
	procSetProcessInformation = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetProcessInformation")
)

func init() {
	if unsafe.Offsetof(sysProcInfo{}.UniqueProcessId) != 0x50 ||
		unsafe.Offsetof(sysProcInfo{}.PrivatePageCount) != 0xC8 ||
		unsafe.Offsetof(sysProcInfo{}.ReadTransferCount) != 0xE8 {
		panic("layout SYSTEM_PROCESS_INFORMATION invalide")
	}
}

type Proc struct {
	PID, PPID   uint32
	Name        string
	Private, WS uint64
	CPUTime     int64 // unités de 100 ns
	Create      int64
	IOBytes     int64 // lecture + écriture cumulées
	ReadBytes   int64
	WriteBytes  int64
}

func querySystem(class int32, size uint32) ([]byte, error) {
	for {
		buf := make([]byte, size)
		var ret uint32
		err := windows.NtQuerySystemInformation(class, unsafe.Pointer(&buf[0]), size, &ret)
		if err == nil {
			return buf, nil
		}
		if err != windows.STATUS_INFO_LENGTH_MISMATCH && err != windows.STATUS_BUFFER_TOO_SMALL {
			return nil, err
		}
		if ret > size {
			size = ret + 64*1024
		} else {
			size *= 2
		}
	}
}

func listProcs() ([]Proc, error) {
	buf, err := querySystem(classProcessInformation, 1<<20)
	if err != nil {
		return nil, err
	}
	out := make([]Proc, 0, 400)
	for off := 0; ; {
		p := (*sysProcInfo)(unsafe.Pointer(&buf[off]))
		if pid := uint32(p.UniqueProcessId); pid != 0 {
			name := "System"
			if p.ImageName.Buffer != nil {
				name = strings.TrimSuffix(p.ImageName.String(), ".exe")
			}
			out = append(out, Proc{
				PID:        pid,
				PPID:       uint32(p.InheritedFromUniqueProcessId),
				Name:       name,
				Private:    uint64(p.PrivatePageCount),
				WS:         uint64(p.WorkingSetSize),
				CPUTime:    p.UserTime + p.KernelTime,
				Create:     p.CreateTime,
				ReadBytes:  p.ReadTransferCount,
				WriteBytes: p.WriteTransferCount,
				IOBytes:    p.ReadTransferCount + p.WriteTransferCount,
			})
		}
		if p.NextEntryOffset == 0 {
			break
		}
		off += int(p.NextEntryOffset)
	}
	return out, nil
}

func coreTimes() ([]corePerf, error) {
	n := runtime.NumCPU()
	buf := make([]corePerf, n)
	var ret uint32
	sz := uint32(n) * uint32(unsafe.Sizeof(buf[0]))
	if err := windows.NtQuerySystemInformation(classProcessorPerformance, unsafe.Pointer(&buf[0]), sz, &ret); err != nil {
		return nil, err
	}
	return buf[:int(ret)/int(unsafe.Sizeof(buf[0]))], nil
}

func memInfo() (perfInfo, error) {
	var pi perfInfo
	pi.Cb = uint32(unsafe.Sizeof(pi))
	if r, _, e := procGetPerformanceInfo.Call(uintptr(unsafe.Pointer(&pi)), uintptr(pi.Cb)); r == 0 {
		return pi, e
	}
	return pi, nil
}

// netTotals : octets reçus/envoyés cumulés sur les cartes réseau actives (hors loopback/tunnels).
// Passe par GetAdaptersAddresses pour ignorer les interfaces "filtres" (WFP, QoS) qui doubleraient les compteurs.
func netTotals() (rx, tx uint64) {
	size := uint32(16 * 1024)
	var buf []byte
	for i := 0; i < 3; i++ {
		buf = make([]byte, size)
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC,
			windows.GAA_FLAG_SKIP_UNICAST|windows.GAA_FLAG_SKIP_ANYCAST|windows.GAA_FLAG_SKIP_MULTICAST|windows.GAA_FLAG_SKIP_DNS_SERVER,
			0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil {
			break
		}
		if err != windows.ERROR_BUFFER_OVERFLOW {
			return 0, 0
		}
	}
	for a := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); a != nil; a = a.Next {
		if a.OperStatus != windows.IfOperStatusUp || a.IfType == windows.IF_TYPE_SOFTWARE_LOOPBACK || a.IfType == windows.IF_TYPE_TUNNEL {
			continue
		}
		row := windows.MibIfRow2{InterfaceLuid: a.Luid, InterfaceIndex: a.IfIndex}
		if windows.GetIfEntry2Ex(windows.MibIfEntryNormal, &row) == nil {
			rx += row.InOctets
			tx += row.OutOctets
		}
	}
	return rx, tx
}

// ---------- Actions ----------

// Processus qu'on refuse de tuer ou de brider (stabilité de Windows).
var protected = map[string]bool{
	"system": true, "registry": true, "smss": true, "csrss": true, "wininit": true, "winlogon": true,
	"services": true, "lsass": true, "svchost": true, "dwm": true, "memory compression": true,
	"secure system": true, "fontdrvhost": true, "lsaiso": true, "borj": true,
}

func isProtected(name string) bool { return protected[strings.ToLower(name)] }

func killPIDs(pids []uint32) (ok, fail int) {
	for _, pid := range pids {
		h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
		if err != nil {
			fail++
			continue
		}
		if windows.TerminateProcess(h, 1) == nil {
			ok++
		} else {
			fail++
		}
		windows.CloseHandle(h)
	}
	return
}

// setEco active/désactive EcoQoS + priorité basse sur un processus.
func setEco(pid uint32, on bool) error {
	h, err := windows.OpenProcess(windows.PROCESS_SET_INFORMATION, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	st := powerThrottling{Version: 1, ControlMask: 1}
	prio := uint32(windows.NORMAL_PRIORITY_CLASS)
	if on {
		st.StateMask = 1
		prio = windows.BELOW_NORMAL_PRIORITY_CLASS
	}
	const processPowerThrottling = 4
	if r, _, e := procSetProcessInformation.Call(uintptr(h), processPowerThrottling, uintptr(unsafe.Pointer(&st)), unsafe.Sizeof(st)); r == 0 {
		return e
	}
	return windows.SetPriorityClass(h, prio)
}

// ---------- Échantillonnage ----------

type Member struct {
	PID     uint32
	Name    string
	Private uint64
	CPU     float64
	IO      float64 // octets/s
}

type Family struct {
	Name    string
	Count   int
	Private uint64
	CPU     float64
	IO      float64
	Members []Member
}

func (f Family) PIDs() []uint32 {
	out := make([]uint32, len(f.Members))
	for i, m := range f.Members {
		out[i] = m.PID
	}
	return out
}

type Snapshot struct {
	CPUTotal    float64
	Cores       []float64
	MemTotal    uint64
	MemAvail    uint64
	CommitTotal uint64
	CommitLimit uint64
	Compressed  uint64
	Procs       int
	Families    []Family
	DiskRead    float64 // octets/s, somme des processus
	DiskWrite   float64
	NetRx       float64
	NetTx       float64
}

func (s Snapshot) MemUsedPct() float64 {
	if s.MemTotal == 0 {
		return 0
	}
	return 100 * float64(s.MemTotal-s.MemAvail) / float64(s.MemTotal)
}

func (s Snapshot) CommitPct() float64 {
	if s.CommitLimit == 0 {
		return 0
	}
	return 100 * float64(s.CommitTotal) / float64(s.CommitLimit)
}

type procKey struct{ create, cpu, read, write int64 }

type Sampler struct {
	prevCores    []corePerf
	prevProc     map[uint32]procKey
	prevAt       time.Time
	prevRx       uint64
	prevTx       uint64
	ncpu         int
}

func NewSampler() *Sampler { return &Sampler{prevProc: map[uint32]procKey{}, ncpu: runtime.NumCPU()} }

func pct(part, total int64) float64 {
	if total <= 0 {
		return 0
	}
	v := 100 * float64(part) / float64(total)
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func (s *Sampler) Sample() (Snapshot, error) {
	var snap Snapshot
	now := time.Now()
	secs := now.Sub(s.prevAt).Seconds()
	first := s.prevAt.IsZero()

	cores, err := coreTimes()
	if err != nil {
		return snap, err
	}
	if len(s.prevCores) == len(cores) {
		var busyAll, totAll int64
		for i, c := range cores {
			p := s.prevCores[i]
			tot := (c.Kernel - p.Kernel) + (c.User - p.User)
			busy := tot - (c.Idle - p.Idle)
			snap.Cores = append(snap.Cores, pct(busy, tot))
			busyAll += busy
			totAll += tot
		}
		snap.CPUTotal = pct(busyAll, totAll)
	} else {
		snap.Cores = make([]float64, len(cores))
	}
	s.prevCores = cores

	if pi, err := memInfo(); err == nil {
		ps := uint64(pi.PageSize)
		snap.MemTotal = uint64(pi.PhysicalTotal) * ps
		snap.MemAvail = uint64(pi.PhysicalAvailable) * ps
		snap.CommitTotal = uint64(pi.CommitTotal) * ps
		snap.CommitLimit = uint64(pi.CommitLimit) * ps
	}

	rx, tx := netTotals()
	if !first && secs > 0 && rx >= s.prevRx && tx >= s.prevTx {
		snap.NetRx = float64(rx-s.prevRx) / secs
		snap.NetTx = float64(tx-s.prevTx) / secs
	}
	s.prevRx, s.prevTx = rx, tx

	procs, err := listProcs()
	if err != nil {
		return snap, err
	}
	cpuTicks := secs * 1e7 * float64(s.ncpu) // secondes -> unités de 100 ns, tous cœurs
	cpu := make(map[uint32]float64, len(procs))
	io := make(map[uint32]float64, len(procs))
	next := make(map[uint32]procKey, len(procs))
	for _, p := range procs {
		next[p.PID] = procKey{p.Create, p.CPUTime, p.ReadBytes, p.WriteBytes}
		if prev, ok := s.prevProc[p.PID]; ok && prev.create == p.Create && !first && secs > 0 {
			cpu[p.PID] = 100 * float64(p.CPUTime-prev.cpu) / cpuTicks
			r := float64(p.ReadBytes-prev.read) / secs
			w := float64(p.WriteBytes-prev.write) / secs
			io[p.PID] = r + w
			snap.DiskRead += r
			snap.DiskWrite += w
		}
		if p.Name == "Memory Compression" {
			snap.Compressed = p.WS
		}
	}
	s.prevProc, s.prevAt = next, now
	snap.Procs = len(procs)
	snap.Families = groupFamilies(procs, cpu, io)
	return snap, nil
}

// Processus hébergés qu'on rattache à l'app qui les lance (Teams v2 = ms-teams + msedgewebview2).
var transparent = map[string]bool{"msedgewebview2": true, "conhost": true, "crashpad_handler": true}

// Parents "système" : on ne remonte pas au-dessus.
var stopParents = map[string]bool{
	"explorer": true, "svchost": true, "services": true, "wininit": true, "winlogon": true,
	"system": true, "sihost": true, "runtimebroker": true, "userinit": true, "smss": true,
}

var friendly = map[string]string{
	"ms-teams": "Teams", "code": "VS Code", "claude": "Claude", "brave": "Brave", "discord": "Discord",
	"winword": "Word", "excel": "Excel", "powerpnt": "PowerPoint", "outlook": "Outlook",
	"virtualboxvm": "VM VirtualBox", "vboxheadless": "VM VirtualBox", "vboxsvc": "VirtualBox",
	"vmmem": "VM Hyper-V/WSL", "vmmemwsl": "VM WSL", "msmpeng": "Defender", "svchost": "Services Windows",
	"explorer": "Explorateur", "dwm": "Affichage (dwm)", "msedge": "Edge", "sqlservr": "SQL Server",
	"windowsterminal": "Terminal", "searchhost": "Recherche Windows", "onedrive": "OneDrive",
	"borj": "BORJ",
}

func friendlyName(n string) string {
	if f, ok := friendly[strings.ToLower(n)]; ok {
		return f
	}
	return n
}

func groupFamilies(procs []Proc, cpu, io map[uint32]float64) []Family {
	byPID := make(map[uint32]*Proc, len(procs))
	for i := range procs {
		byPID[procs[i].PID] = &procs[i]
	}
	root := func(p *Proc) string {
		cur := p
		for depth := 0; depth < 32; depth++ {
			par, ok := byPID[cur.PPID]
			// parent absent, PID réutilisé (parent plus jeune) ou parent système -> fin
			if !ok || par.Create > cur.Create || stopParents[strings.ToLower(par.Name)] {
				break
			}
			if par.Name == cur.Name || transparent[strings.ToLower(cur.Name)] {
				cur = par
				continue
			}
			break
		}
		return cur.Name
	}

	agg := map[string]*Family{}
	for i := range procs {
		p := &procs[i]
		if p.Name == "Memory Compression" || p.Name == "Registry" || p.Name == "Secure System" {
			continue
		}
		n := friendlyName(root(p))
		f := agg[n]
		if f == nil {
			f = &Family{Name: n}
			agg[n] = f
		}
		f.Count++
		f.Private += p.Private
		f.CPU += cpu[p.PID]
		f.IO += io[p.PID]
		f.Members = append(f.Members, Member{PID: p.PID, Name: p.Name, Private: p.Private, CPU: cpu[p.PID], IO: io[p.PID]})
	}
	out := make([]Family, 0, len(agg))
	for _, f := range agg {
		sort.Slice(f.Members, func(i, j int) bool { return f.Members[i].Private > f.Members[j].Private })
		out = append(out, *f)
	}
	return out
}

var errProtected = errors.New("processus système protégé")
