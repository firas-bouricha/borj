package main

import (
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestKillableSkipsSystemAndVM(t *testing.T) {
	f := Family{Name: "Test", Members: []Member{
		{PID: 1, Name: "brave"},
		{PID: 2, Name: "svchost"},
		{PID: 3, Name: "csrss"},
	}}
	refs, skipped, hasVM := f.Killable()
	if len(refs) != 1 || refs[0].PID != 1 || skipped != 2 || hasVM {
		t.Fatalf("refs=%v skipped=%d hasVM=%v", refs, skipped, hasVM)
	}

	vm := Family{Name: "VM VirtualBox", Members: []Member{{PID: 4, Name: "VBoxHeadless"}}}
	if _, _, hasVM := vm.Killable(); !hasVM {
		t.Fatal("un processus de VM doit être signalé")
	}
}

func TestProtectedIsCaseInsensitive(t *testing.T) {
	for _, n := range []string{"LSASS", "Csrss", "TrustedInstaller", "msiexec", "BORJ"} {
		if !isProtected(n) {
			t.Errorf("%s devrait être protégé", n)
		}
	}
	if isProtected("brave") {
		t.Error("brave ne doit pas être protégé")
	}
}

// startChild lance un processus inoffensif et renvoie sa référence (PID + heure de création).
func startChild(t *testing.T) (*exec.Cmd, procRef) {
	t.Helper()
	cmd := exec.Command(systemExe(`System32\PING.EXE`), "-n", "30", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	pid := uint32(cmd.Process.Pid)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		procs, err := listProcs()
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range procs {
			if p.PID == pid {
				return cmd, procRef{pid, p.Create}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("processus enfant introuvable")
	return nil, procRef{}
}

func alive(pid uint32) bool {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	ev, _ := windows.WaitForSingleObject(h, 0)
	return ev == uint32(windows.WAIT_TIMEOUT)
}

// Un PID réutilisé (heure de création différente) ne doit jamais être arrêté.
func TestKillRefusesReusedPID(t *testing.T) {
	_, ref := startChild(t)
	stale := procRef{ref.PID, ref.Create - 1}
	if ok, _ := killProcs([]procRef{stale}); ok != 0 {
		t.Fatal("un processus avec une autre heure de création a été arrêté")
	}
	if !alive(ref.PID) {
		t.Fatal("le processus ne devrait pas avoir été arrêté")
	}
	if ok, _ := killProcs([]procRef{ref}); ok != 1 {
		t.Fatal("le bon processus n'a pas été arrêté")
	}
}

// Le mode éco doit rendre la priorité d'origine.
func TestEcoRestoresPriority(t *testing.T) {
	_, ref := startChild(t)
	prio := func() uint32 {
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, ref.PID)
		if err != nil {
			t.Fatal(err)
		}
		defer windows.CloseHandle(h)
		p, _, _ := procGetPriorityClass.Call(uintptr(h))
		return uint32(p)
	}
	before := prio()
	if err := setEco(ref.PID, true); err != nil {
		t.Fatal(err)
	}
	if prio() != windows.BELOW_NORMAL_PRIORITY_CLASS {
		t.Fatal("priorité non abaissée")
	}
	if err := setEco(ref.PID, false); err != nil {
		t.Fatal(err)
	}
	if after := prio(); after != before {
		t.Fatalf("priorité %x au lieu de %x", after, before)
	}
}

func TestVolumesListed(t *testing.T) {
	vols := listVolumes()
	if len(vols) == 0 {
		t.Fatal("aucun volume trouvé")
	}
	for _, v := range vols {
		if v.Total == 0 || v.Free > v.Total {
			t.Errorf("volume incohérent : %+v", v)
		}
	}
}
