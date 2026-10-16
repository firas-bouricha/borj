<#
.SYNOPSIS
  Tuning memoire Windows. DRY-RUN par defaut : affiche ce qui serait fait.
  -Apply pour executer (admin requis). Point de restauration cree avant.
.PARAMETER PageFileMinMB / PageFileMaxMB
  Pagefile fixe sur C: (defaut 24576 / 32768).
.PARAMETER WebView2Flags
  Ajoute la variable user WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS (experimental, global a toutes apps WebView2).
  Rollback : .\optimize-memory.ps1 -RemoveWebView2Flags -Apply
.EXAMPLE
  .\optimize-memory.ps1                      # dry-run
  .\optimize-memory.ps1 -Apply               # admin
  .\optimize-memory.ps1 -Apply -SkipPageFile -SkipSql
#>
param(
    [switch]$Apply,
    [int]$PageFileMinMB = 24576,
    [int]$PageFileMaxMB = 32768,
    [switch]$SkipPageFile,
    [switch]$SkipSql,
    [switch]$SkipRestorePoint,
    [switch]$WebView2Flags,
    [switch]$RemoveWebView2Flags
)

$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)

if ($Apply -and -not $isAdmin -and -not ($SkipPageFile -and $SkipSql)) {
    Write-Error "-Apply demande une console admin."
    exit 1
}

$mode = if ($Apply) { 'APPLY' } else { 'DRY-RUN' }
Write-Host "=== optimize-memory [$mode] ===" -ForegroundColor Cyan

function Step([string]$label, [scriptblock]$action) {
    if ($Apply) {
        Write-Host "[..] $label" -ForegroundColor Yellow
        try { & $action; Write-Host "[OK] $label" -ForegroundColor Green }
        catch { Write-Host "[KO] $label : $($_.Exception.Message)" -ForegroundColor Red }
    } else {
        Write-Host "[would] $label"
    }
}

# 0. Point de restauration
if (-not $SkipRestorePoint -and $isAdmin) {
    Step "Point de restauration 'pre-optimize-memory'" {
        Enable-ComputerRestore -Drive "$env:SystemDrive\"
        Checkpoint-Computer -Description 'pre-optimize-memory' -RestorePointType MODIFY_SETTINGS
    }
}

# 1. Page combining (dedup pages identiques). Compression laissee active.
Step "Enable-MMAgent -PageCombining (+ MemoryCompression garantie)" {
    Enable-MMAgent -PageCombining
    Enable-MMAgent -MemoryCompression
}

# 2. Pagefile fixe
if (-not $SkipPageFile) {
    Step "Pagefile fixe C:\pagefile.sys  min=$PageFileMinMB Mo  max=$PageFileMaxMB Mo (effectif apres reboot)" {
        $cs = Get-CimInstance Win32_ComputerSystem
        if ($cs.AutomaticManagedPagefile) {
            Set-CimInstance -InputObject $cs -Property @{ AutomaticManagedPagefile = $false }
        }
        $pf = Get-CimInstance Win32_PageFileSetting | Where-Object { $_.Name -like 'C:*' }
        if (-not $pf) {
            $pf = New-CimInstance -ClassName Win32_PageFileSetting -Property @{ Name = 'C:\pagefile.sys' }
        }
        Set-CimInstance -InputObject $pf -Property @{ InitialSize = [uint32]$PageFileMinMB; MaximumSize = [uint32]$PageFileMaxMB }
    }
}

# 3. SQL Server -> demarrage manuel
if (-not $SkipSql) {
    $sql = Get-Service | Where-Object { $_.Name -match '^(MSSQL|SQLAgent|SQLBrowser|SQLTELEMETRY|SSIS|MSSQLFDLauncher)' -and $_.StartType -eq 'Automatic' }
    if ($sql) {
        foreach ($s in $sql) {
            Step "Service $($s.Name) -> Manual (+ stop)" {
                Set-Service -Name $s.Name -StartupType Manual
                Stop-Service -Name $s.Name -Force
            }
        }
        Write-Host "     Astuce : plafonner aussi SQL Server quand il tourne :"
        Write-Host "     sqlcmd -S . -E -Q `"EXEC sp_configure 'show advanced options',1;RECONFIGURE;EXEC sp_configure 'max server memory (MB)',512;RECONFIGURE`""
    } else { Write-Host "[skip] Aucun service SQL Server en Automatic." }
}

# 4. WebView2 flags (scope utilisateur, pas d'admin requis)
$wvName = 'WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS'
$wvValue = '--renderer-process-limit=3 --js-flags=--max-old-space-size=512'
if ($WebView2Flags) {
    Step "Variable user $wvName = '$wvValue' (relancer Teams)" {
        [Environment]::SetEnvironmentVariable($wvName, $wvValue, 'User')
    }
}
if ($RemoveWebView2Flags) {
    Step "Suppression variable user $wvName" {
        [Environment]::SetEnvironmentVariable($wvName, $null, 'User')
    }
}

# 5. Bloat OEM : liste seulement, desinstallation manuelle (choix utilisateur)
Write-Host "`n=== Bloat OEM detecte (desinstallation manuelle) ===" -ForegroundColor Cyan
$pattern = 'SmartByte|SupportAssist|Dell Optimizer|Dell Digital Delivery|Dell Update|McAfee|Killer'
$found = Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*',
                          'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*' -ErrorAction SilentlyContinue |
    Where-Object { $_.DisplayName -match $pattern } |
    Select-Object DisplayName, DisplayVersion -Unique
if ($found) {
    $found | Format-Table -AutoSize
    Write-Host "Commande type : winget uninstall --name `"<DisplayName>`""
} else { "Rien trouve." }

Write-Host "`nVerif : Get-MMAgent ; Get-CimInstance Win32_PageFileSetting ; .\audit.ps1"
if ($Apply -and -not $SkipPageFile) { Write-Host "Reboot requis pour le pagefile." -ForegroundColor Yellow }
