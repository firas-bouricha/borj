package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os/exec"
	"syscall"
	"time"
	"unicode/utf16"
)

// La santé change rarement : collecte via PowerShell (Storage + CIM + journaux), toutes les 5 min.
// Le processus PowerShell vit ~2 s puis se termine : aucun coût permanent.

type Disk struct {
	Model  string `json:"model"`
	SizeGB int    `json:"sizeGB"`
	Bus    string `json:"bus"`
	Health string `json:"health"`
	Wear   int    `json:"wear"`   // % usure NVMe, -1 = inconnu (admin requis)
	Errors int    `json:"errors"` // erreurs lecture/écriture non corrigées, -1 = inconnu
	Hours  int    `json:"hours"`
}

type Stick struct {
	Loc   string `json:"loc"`
	GB    int    `json:"gb"`
	Type  string `json:"type"`
	Speed int    `json:"speed"`
	Part  string `json:"part"`
}

type Health struct {
	At    time.Time `json:"-"`
	Admin bool      `json:"admin"`
	Model string    `json:"model"`
	Disks []Disk    `json:"disks"`
	CPU   struct {
		Name    string `json:"name"`
		Cores   int    `json:"cores"`
		Threads int    `json:"threads"`
		Status  string `json:"status"`
	} `json:"cpu"`
	Whea struct {
		Mem   int `json:"mem"`
		CPU   int `json:"cpu"`
		Fatal int `json:"fatal"`
	} `json:"whea"`
	RAM struct {
		Slots    int     `json:"slots"`
		MaxGB    int     `json:"maxGB"`
		Sticks   []Stick `json:"sticks"`
		DiagID   int     `json:"diagId"` // 1201 = OK, 1202 = erreurs, 0 = jamais testée
		DiagDate string  `json:"diagDate"`
	} `json:"ram"`
	Sys struct {
		Drive   string  `json:"drive"`
		FreeGB  float64 `json:"freeGB"`
		UsedPct float64 `json:"usedPct"`
	} `json:"sys"`
}

const healthScript = `
$ErrorActionPreference='SilentlyContinue'
[Console]::OutputEncoding=[Text.Encoding]::UTF8
$admin=([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
$ev=@(Get-WinEvent -FilterHashtable @{LogName='System';ProviderName='Microsoft-Windows-WHEA-Logger';StartTime=(Get-Date).AddDays(-30)})
$disks=@(Get-PhysicalDisk | ForEach-Object {
  $rc=$_ | Get-StorageReliabilityCounter
  [ordered]@{
    model=("$($_.FriendlyName)" -replace '\s+',' ').Trim()
    sizeGB=[int]($_.Size/1GB)
    bus="$($_.BusType)"
    health="$($_.HealthStatus)"
    wear=$(if($rc -and $null -ne $rc.Wear){[int]$rc.Wear}else{-1})
    errors=$(if($rc){[int]$rc.ReadErrorsUncorrected+[int]$rc.WriteErrorsUncorrected}else{-1})
    hours=$(if($rc -and $rc.PowerOnHours){[int]$rc.PowerOnHours}else{-1})
  }
})
$p=Get-CimInstance Win32_Processor | Select-Object -First 1
$arr=Get-CimInstance Win32_PhysicalMemoryArray | Where-Object {$_.Use -eq 3} | Select-Object -First 1
$max=if($arr.MaxCapacityEx){$arr.MaxCapacityEx}else{$arr.MaxCapacity}
$sticks=@(Get-CimInstance Win32_PhysicalMemory | ForEach-Object {
  $t=switch([int]$_.SMBIOSMemoryType){26{'DDR4'}34{'DDR5'}30{'LPDDR4'}35{'LPDDR5'}default{'DDR'}}
  [ordered]@{loc="$($_.DeviceLocator)";gb=[int]($_.Capacity/1GB);type=$t;speed=[int]$_.Speed;part="$($_.PartNumber)".Trim()}
})
$diag=Get-WinEvent -FilterHashtable @{LogName='System';ProviderName='Microsoft-Windows-MemoryDiagnostics-Results'} -MaxEvents 1
$d=Get-PSDrive -Name $env:SystemDrive.TrimEnd(':')
[ordered]@{
  admin=$admin
  model="$((Get-CimInstance Win32_ComputerSystem).Model)"
  disks=$disks
  cpu=[ordered]@{name="$($p.Name)";cores=[int]$p.NumberOfCores;threads=[int]$p.NumberOfLogicalProcessors;status="$($p.Status)"}
  whea=[ordered]@{mem=@($ev|Where-Object{$_.Id -eq 47}).Count;cpu=@($ev|Where-Object{$_.Id -in 17,19}).Count;fatal=@($ev|Where-Object{$_.Id -in 18,20}).Count}
  ram=[ordered]@{slots=[int]$arr.MemoryDevices;maxGB=[int]($max/1MB);sticks=$sticks;diagId=$(if($diag){[int]$diag.Id}else{0});diagDate=$(if($diag){$diag.TimeCreated.ToString('dd/MM/yy')}else{''})}
  sys=[ordered]@{drive="$env:SystemDrive";freeGB=[math]::Round($d.Free/1GB,1);usedPct=[math]::Round(100*$d.Used/($d.Used+$d.Free),1)}
} | ConvertTo-Json -Depth 5 -Compress
`

func encodePS(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, len(u)*2)
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[i*2:], c)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func collectHealth() (*Health, error) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-EncodedCommand", encodePS(healthScript))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("powershell: %w", err)
	}
	out = bytes.TrimSpace(bytes.TrimPrefix(out, []byte("\xef\xbb\xbf")))
	var h Health
	if err := json.Unmarshal(out, &h); err != nil {
		return nil, fmt.Errorf("json santé: %w", err)
	}
	h.At = time.Now()
	return &h, nil
}

// Verdicts : 0 = bonne santé, 1 = à surveiller, 2 = à remplacer.

func (d Disk) Level() int {
	lvl := 0
	switch d.Health {
	case "Healthy":
	case "Warning":
		lvl = 1
	default:
		lvl = 2
	}
	if d.Wear >= 90 {
		lvl = max(lvl, 2)
	} else if d.Wear >= 70 {
		lvl = max(lvl, 1)
	}
	if d.Errors > 0 {
		lvl = max(lvl, 1)
	}
	return lvl
}

func (h *Health) CPULevel() int {
	if h.CPU.Status != "OK" || h.Whea.Fatal > 0 {
		return 2
	}
	if h.Whea.CPU > 0 {
		return 1
	}
	return 0
}

func (h *Health) RAMLevel() int {
	if h.RAM.DiagID == 1202 {
		return 2
	}
	if h.Whea.Mem > 0 {
		return 1
	}
	return 0
}

func (h *Health) InstalledGB() int {
	t := 0
	for _, s := range h.RAM.Sticks {
		t += s.GB
	}
	return t
}
