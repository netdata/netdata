# SPDX-License-Identifier: GPL-3.0-or-later
#
# netdata-sos - collect a sanitized diagnostic bundle for Netdata support tickets.
# Windows counterpart of netdata-sos.sh. Same bundle layout, same MANIFEST
# schema (netdata-sos-bundle/v1), same sanitization rules.
#
# What is collected and WHY each item is included is documented in
# packaging/installer/SUPPORT-BUNDLE.md - read it before adding or changing
# a collection item, and keep it in sync with this script.
#
# Usage (run in an elevated PowerShell):
#   powershell -ExecutionPolicy Bypass -File netdata-sos.ps1 [-Output DIR]
#              [-SinceHours 24] [-NoObfuscate] [-KeepStaging]
#
# Requires Windows PowerShell 5.1+ (ships with Windows Server 2016+) or
# PowerShell 7. Output: netdata-sos-<timestamp>.zip

[CmdletBinding()]
param(
    [string]$Output = $env:TEMP,
    [int]$SinceHours = 24,
    [int]$TimeoutSeconds = 10,
    [switch]$NoObfuscate,
    [switch]$KeepStaging,
    [switch]$Version
)

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'SilentlyContinue'

$SosVersion = '1.0.0'
if ($Version) { Write-Output "netdata-sos $SosVersion"; exit 0 }

$Obfuscate = -not $NoObfuscate
$LogCap = 5MB
$FileCap = 1MB
$GlobalDeadline = (Get-Date).AddSeconds(240)

# --- run at lowest priority: never compete with real workloads ---------------
try {
    $proc = Get-Process -Id $PID
    $proc.PriorityClass = [System.Diagnostics.ProcessPriorityClass]::Idle
} catch { }

$StartTime = Get-Date
$Stamp = (Get-Date).ToUniversalTime().ToString('yyyyMMdd-HHmmss')
$BundleName = "netdata-sos-$Stamp-$PID"
$Staging = Join-Path $env:TEMP ("netdata-sos-staging-" + [System.IO.Path]::GetRandomFileName())
$Work = Join-Path $Staging $BundleName
New-Item -ItemType Directory -Path $Work -Force | Out-Null
$script:ManifestRows = New-Object System.Collections.ArrayList
$script:PseudoMap = @{}   # original -> pseudonym
$script:IpCount = 0
$script:FqdnCount = 0

function Write-Info([string]$msg) { Write-Host " [*] $msg" }

function Test-Deadline { return ((Get-Date) -gt $GlobalDeadline) }

# --- sanitizer ----------------------------------------------------------------
# pass 1 (always): credential-bearing key values, URL/DSN creds, JWT,
#                  Bearer/Basic values, private keys, UUID section headers
# pass 2 (default): emails, MACs, IPv4 pseudonyms, this host's names,
#                   private-TLD FQDNs
$SecretKeyWords = @(
    'api key','apikey','token','password','passwd','secret','community',
    'bearer','webhook','license key','auth','credential','cookie','passphrase',
    'proxy user','proxy pass','username','dsn','private key','access key',
    'session','recipient','account sid','priv key'
)
$HostShort = $env:COMPUTERNAME
$HostFqdn = try { [System.Net.Dns]::GetHostEntry('').HostName } catch { '' }
$RunUser = $env:USERNAME
if ($RunUser -in @('SYSTEM', 'Administrator') -or -not $RunUser -or $RunUser.Length -lt 3) { $RunUser = '' }

function Test-SecretKey([string]$key) {
    $k = ($key -replace '[-_]', ' ').ToLower().Trim(' ', '#', "`t")
    foreach ($w in $SecretKeyWords) { if ($k.Contains($w)) { return $true } }
    return $false
}

function Test-HarmlessValue([string]$v) {
    # boolean/mode literals and paths are never secrets; keeping them preserves
    # diagnostics like "bearer token protection = no"
    $v = $v.Trim()
    if ($v -match '^(yes|no|true|false|on|off|auto|none|enabled|disabled)$') { return $true }
    if ($v -match '^([A-Za-z]:\\|/)') { return $true }
    return $false
}

function Get-Pseudonym([string]$orig, [string]$prefix) {
    if (-not $script:PseudoMap.ContainsKey($orig)) {
        if ($prefix -eq 'ip') { $script:IpCount++; $script:PseudoMap[$orig] = "ip-$($script:IpCount)" }
        elseif ($prefix -eq 'fqdn') { $script:FqdnCount++; $script:PseudoMap[$orig] = "private-host-$($script:FqdnCount)" }
        else { $script:PseudoMap[$orig] = 'redacted-host' }
    }
    return $script:PseudoMap[$orig]
}

function Invoke-SanitizeLine([string]$line) {
    # stream.conf-style [<uuid>] section headers are api keys
    if ($line -match '^\s*\[[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\]\s*$') {
        return '[REDACTED-KEY-SECTION]'
    }
    # ini/yaml/env key = value | key: value
    # (JSON-shaped lines are owned by the json rule below, which preserves quoting)
    # only plausible config keys: short, no sentence/shell punctuation
    if ($line -notmatch '^\s*"' -and $line -match '^([^=:]{1,64})([=:])(.+)$' -and $Matches[1] -notmatch '["`;|()]') {
        if ((Test-SecretKey $Matches[1]) -and $Matches[3].Trim().Length -gt 1 -and -not (Test-HarmlessValue $Matches[3])) {
            $line = $Matches[1] + $Matches[2] + ' [REDACTED]'
        }
    }
    # json "key": "value" pairs (possibly several per line)
    $line = [regex]::Replace($line, '"([^"]+)"\s*:\s*"([^"]*)"', {
        param($m)
        if ((Test-SecretKey $m.Groups[1].Value) -and -not (Test-HarmlessValue $m.Groups[2].Value)) { '"' + $m.Groups[1].Value + '": "[REDACTED]"' }
        else { $m.Value }
    })
    # URL creds and Go-style DSN creds
    $line = $line -replace '://[^:/@\s]+:[^@\s]+@', '://[REDACTED]@'
    $line = $line -replace '\b[\w]+:[^@\s]+@(tcp|unix)\(', '[REDACTED]@$1('
    # JWTs, HTTP auth header values
    $line = $line -replace 'eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]+', '[REDACTED-JWT]'
    # Bearer value must contain a digit: real tokens do, prose after "bearer" does not
    $line = $line -replace '[Bb]earer\s+[A-Za-z._~+/=-]*\d[A-Za-z0-9._~+/=-]*', 'Bearer [REDACTED]'
    $line = $line -replace '[Bb]asic\s+[A-Za-z0-9+/=]{8,}', 'Basic [REDACTED]'
    # secrets passed as URL query parameters (access-log request lines etc.)
    $line = $line -replace '([?&](token|apikey|api_key|password|passwd|secret|bearer|claim_token|claim_rooms|key|auth)=)[^&"\s]+', '$1[REDACTED]'
    # argv/env-style secrets mid-line (process command lines: -token=X, CLAIM_TOKEN=X),
    # incl. two-word keys ("api key = X"); harmless literal values are kept
    $line = [regex]::Replace($line, '(?i)(([\w.-]*(token|password|passwd|secret|apikey|api_key|community|bearer)|(api|license|auth|access) key|proxy (user|pass|password)) ?= ?)([^&"\s\[]+)', {
        param($m)
        if (Test-HarmlessValue $m.Groups[6].Value) { $m.Value } else { $m.Groups[1].Value + '[REDACTED]' }
    })
    if ($line -match '-----BEGIN .*PRIVATE KEY') { return '[REDACTED PRIVATE KEY]' }

    if ($Obfuscate) {
        $line = $line -replace '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}', '[EMAIL]'
        $line = $line -replace '\b([0-9A-Fa-f]{2}[:-]){5}[0-9A-Fa-f]{2}\b', '[MAC]'
        # IPv4 (keep loopback/wildcard/broadcast)
        $line = [regex]::Replace($line, '\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b', {
            param($m)
            $ip = $m.Value
            if ($ip -like '127.*' -or $ip -eq '0.0.0.0' -or $ip -like '255.*') { return $ip }
            return (Get-Pseudonym $ip 'ip')
        })
        # IPv6: hex-and-colon runs, validated to skip timestamps/:: tokens/loopback
        $line = [regex]::Replace($line, '(?<![\w.-])[0-9A-Fa-f:]{5,}', {
            param($m)
            $c = $m.Value
            $nc = ($c.ToCharArray() | Where-Object { $_ -eq ':' }).Count
            if ($nc -eq 0 -or $c -eq '::1' -or
                ($nc -lt 3 -and $c -notmatch '::') -or
                ($c -notmatch '[A-Fa-f]' -and $c -notmatch '::')) { return $c }
            return (Get-Pseudonym $c 'ip')
        })
        # clearly-private FQDNs
        $line = [regex]::Replace($line, '\b[A-Za-z0-9][A-Za-z0-9.-]*\.(internal|local|lan|corp|intranet|localdomain)\b', {
            param($m); return (Get-Pseudonym $m.Value 'fqdn')
        })
        # this host's names
        if ($HostFqdn -and $HostFqdn.Length -ge 4) { $line = $line.Replace($HostFqdn, (Get-Pseudonym $HostFqdn 'host')) }
        if ($HostShort -and $HostShort.Length -ge 4 -and $HostShort -ne $HostFqdn) {
            $line = $line -ireplace [regex]::Escape($HostShort), (Get-Pseudonym $HostShort 'host')
        }
        if ($RunUser) { $line = $line -ireplace [regex]::Escape($RunUser), 'redacted-user' }
    }
    return $line
}

function Invoke-SanitizeFile([string]$path) {
    if (-not (Test-Path $path)) { return }
    $out = New-Object System.Collections.ArrayList
    foreach ($l in [System.IO.File]::ReadAllLines($path)) {
        [void]$out.Add((Invoke-SanitizeLine $l))
    }
    [System.IO.File]::WriteAllLines($path, $out)
}

# --- manifest -------------------------------------------------------------------
function Add-Manifest([string]$rel, [string]$kind, [string]$origin, [string]$title) {
    $full = Join-Path $Work $rel
    $bytes = 0
    if (Test-Path $full) { $bytes = (Get-Item $full).Length }
    [void]$script:ManifestRows.Add(@{
        path = $rel.Replace('\', '/'); kind = $kind; origin = $origin; title = $title
        bytes = [long]$bytes; pii_obfuscated = [bool]$Obfuscate
    })
}

# --- collectors -------------------------------------------------------------------
function Collect-Cmd([string]$rel, [string]$title, [scriptblock]$cmd, [string]$originText) {
    if (Test-Deadline) { return }
    $full = Join-Path $Work $rel
    New-Item -ItemType Directory -Path (Split-Path $full) -Force | Out-Null
    $header = "# netdata-sos v$SosVersion | command: $originText | captured: $((Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ'))"
    try {
        $job = Start-Job -ScriptBlock $cmd
        $done = Wait-Job $job -Timeout $TimeoutSeconds
        if ($done) { $result = Receive-Job $job 2>&1 | Out-String } else { $result = "TIMEOUT after ${TimeoutSeconds}s" }
        Remove-Job $job -Force
    } catch { $result = "ERROR: $_" }
    Set-Content -Path $full -Value ($header + "`r`n" + $result)
    Invoke-SanitizeFile $full
    Add-Manifest $rel 'cmd' $originText $title
}

function Collect-File([string]$rel, [string]$title, [string]$src, [long]$cap = 0) {
    if (Test-Deadline) { return }
    if ($cap -eq 0) { $cap = $FileCap }
    if (-not (Test-Path $src -PathType Leaf)) { return }
    $full = Join-Path $Work $rel
    New-Item -ItemType Directory -Path (Split-Path $full) -Force | Out-Null
    $size = (Get-Item $src).Length
    $origin = $src
    if ($size -gt $cap) {
        # keep the tail (most recent content) like the POSIX variant
        $fs = [System.IO.File]::OpenRead($src)
        try {
            $fs.Seek(-$cap, [System.IO.SeekOrigin]::End) | Out-Null
            $buf = New-Object byte[] $cap
            $read = $fs.Read($buf, 0, $cap)
            [System.IO.File]::WriteAllBytes($full, $buf[0..($read-1)])
        } finally { $fs.Close() }
        $origin = "$src (last $cap of $size bytes)"
    } else {
        Copy-Item $src $full -Force
    }
    Invoke-SanitizeFile $full
    Add-Manifest $rel 'file' $origin $title
}

$NdPort = 19999
function Collect-Api([string]$rel, [string]$title, [string]$urlPath) {
    if (Test-Deadline) { return }
    $full = Join-Path $Work $rel
    New-Item -ItemType Directory -Path (Split-Path $full) -Force | Out-Null
    try {
        $resp = Invoke-WebRequest -Uri "http://127.0.0.1:$NdPort$urlPath" -UseBasicParsing -TimeoutSec $TimeoutSeconds
        Set-Content -Path $full -Value $resp.Content
        Invoke-SanitizeFile $full
        Add-Manifest $rel 'api' $urlPath $title
    } catch {
        if (Test-Path $full) { Remove-Item $full -Force }
    }
}

# --- environment detection --------------------------------------------------------
$NetdataPrefix = 'C:\Program Files\Netdata'
$ConfDir = Join-Path $NetdataPrefix 'etc\netdata'
$LibDir = Join-Path $NetdataPrefix 'var\lib\netdata'
$CacheDir = Join-Path $NetdataPrefix 'var\cache\netdata'
$LogDir = Join-Path $NetdataPrefix 'var\log\netdata'
$NetdataExe = Join-Path $NetdataPrefix 'usr\bin\netdata.exe'
$NetdataCli = Join-Path $NetdataPrefix 'usr\bin\netdatacli.exe'

$NetdataSvc = Get-Service -Name 'Netdata' -ErrorAction SilentlyContinue
$NetdataProc = Get-Process -Name 'netdata' -ErrorAction SilentlyContinue | Select-Object -First 1
$ApiOk = $false
try {
    Invoke-WebRequest -Uri "http://127.0.0.1:$NdPort/api/v1/info" -UseBasicParsing -TimeoutSec 3 | Out-Null
    $ApiOk = $true
} catch { }

Write-Info "netdata-sos $SosVersion (Windows)"
Write-Info ("service: {0} | process: {1} | api: {2}" -f `
    $(if ($NetdataSvc) { $NetdataSvc.Status } else { 'not installed' }), `
    $(if ($NetdataProc) { "pid $($NetdataProc.Id)" } else { 'not running' }), `
    $(if ($ApiOk) { 'up' } else { 'unreachable' }))

# ============================================================================
# 01-system
# ============================================================================
Write-Info 'collecting: system'
Collect-Cmd '01-system\os-version.txt' 'OS version and build' { Get-CimInstance Win32_OperatingSystem | Format-List Caption, Version, BuildNumber, OSArchitecture, LastBootUpTime, TotalVisibleMemorySize, FreePhysicalMemory } 'Get-CimInstance Win32_OperatingSystem'
Collect-Cmd '01-system\computer-info.txt' 'Hardware, domain role, virtualization' { Get-CimInstance Win32_ComputerSystem | Format-List Manufacturer, Model, SystemType, NumberOfProcessors, NumberOfLogicalProcessors, TotalPhysicalMemory, DomainRole, HypervisorPresent } 'Get-CimInstance Win32_ComputerSystem'
Collect-Cmd '01-system\disk-usage.txt' 'Volume usage' { Get-CimInstance Win32_LogicalDisk | Format-Table DeviceID, @{n='SizeGB';e={[math]::Round($_.Size/1GB,1)}}, @{n='FreeGB';e={[math]::Round($_.FreeSpace/1GB,1)}}, FileSystem -AutoSize } 'Get-CimInstance Win32_LogicalDisk'
Collect-Cmd '01-system\clock-timesync.txt' 'Clock and time sync (drift breaks streaming/cloud)' { Get-Date -Format o; w32tm /query /status 2>&1 } 'Get-Date; w32tm /query /status'
Collect-Cmd '01-system\uptime.txt' 'System uptime' { (Get-Date) - (Get-CimInstance Win32_OperatingSystem).LastBootUpTime | Format-List Days, Hours, Minutes } 'uptime via Win32_OperatingSystem'

# ============================================================================
# 02-install
# ============================================================================
Write-Info 'collecting: install'
# NOTE: never use Win32_Product here - querying it triggers MSI reconfiguration
# of every installed package. The uninstall registry keys are the safe source.
Collect-Cmd '02-install\msi-info.txt' 'Installed Netdata MSI package info (from uninstall registry)' {
    foreach ($root in @('HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall',
                        'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall')) {
        Get-ItemProperty "$root\*" -ErrorAction SilentlyContinue |
            Where-Object { $_.DisplayName -like '*Netdata*' } |
            Format-List DisplayName, DisplayVersion, InstallDate, InstallLocation, Publisher
    }
} 'registry uninstall keys (Netdata)'
Collect-Cmd '02-install\install-tree.txt' 'Install dir layout (top levels)' { Get-ChildItem 'C:\Program Files\Netdata' -Depth 1 | Format-Table Mode, LastWriteTime, Length, Name -AutoSize } 'Get-ChildItem C:\Program Files\Netdata -Depth 1'
if (Test-Path (Join-Path $ConfDir '.install-type')) {
    Collect-File '02-install\install-type.file.txt' 'Install type marker' (Join-Path $ConfDir '.install-type')
}

# ============================================================================
# 03-process
# ============================================================================
Write-Info 'collecting: process'
Collect-Cmd '03-process\netdata-processes.txt' 'Netdata process tree with CPU/memory' { Get-Process | Where-Object { $_.ProcessName -match 'netdata|go.d|ebpf|windows.plugin' } | Format-Table Id, ProcessName, CPU, WorkingSet64, HandleCount, Threads -AutoSize } 'Get-Process (netdata family)'
Collect-Cmd '03-process\service-status.txt' 'Netdata service state and config' { Get-Service Netdata | Format-List *; (Get-CimInstance Win32_Service -Filter "Name='Netdata'") | Format-List StartMode, StartName, PathName, State, ExitCode } 'Get-Service Netdata + Win32_Service'

# ============================================================================
# 04-config
# ============================================================================
Write-Info 'collecting: config'
if ($ApiOk) {
    Collect-Api '04-config\effective-netdata.conf' 'EFFECTIVE running config (merged, annotated) - authoritative over on-disk file' '/netdata.conf'
}
if (Test-Path $ConfDir) {
    Collect-Cmd '04-config\config-tree.txt' 'User config dir tree (files here = user-customized)' { Get-ChildItem 'C:\Program Files\Netdata\etc\netdata' -Recurse | Format-Table Mode, LastWriteTime, Length, FullName -AutoSize } "Get-ChildItem $ConfDir -Recurse"
    Collect-File '04-config\netdata.conf' 'On-disk main config' (Join-Path $ConfDir 'netdata.conf')
    Collect-File '04-config\stream.conf' 'Streaming config (parent/child; api key redacted)' (Join-Path $ConfDir 'stream.conf')
    Collect-File '04-config\claim.conf' 'Cloud claim config (token redacted)' (Join-Path $ConfDir 'claim.conf')
    Collect-File '04-config\go.d.conf' 'go.d orchestrator config' (Join-Path $ConfDir 'go.d.conf')
    foreach ($sub in @('go.d', 'health.d')) {
        $subDir = Join-Path $ConfDir $sub
        if (Test-Path $subDir) {
            Get-ChildItem $subDir -File -Include '*.conf', '*.yml', '*.yaml' -Recurse | ForEach-Object {
                Collect-File "04-config\$sub\$($_.Name)" "User-customized $sub config (secrets redacted)" $_.FullName 256KB
            }
        }
    }
}

# ============================================================================
# 05-logs (Windows: Event Log is the primary destination)
# ============================================================================
Write-Info "collecting: logs (last ${SinceHours}h, Event Log + files)"
$SinceTime = (Get-Date).AddHours(-$SinceHours)
Collect-Cmd '05-logs\eventlog-netdata.txt' 'Netdata events from Windows Event Log (NetdataWEL + Application)' {
    $since = (Get-Date).AddHours(-24)
    foreach ($logName in @('NetdataWEL', 'Application')) {
        Get-WinEvent -FilterHashtable @{ LogName = $logName; StartTime = $since } -MaxEvents 2000 -ErrorAction SilentlyContinue |
            Where-Object { $_.ProviderName -match 'Netdata' } |
            Select-Object TimeCreated, ProviderName, LevelDisplayName, Message | Format-List
    }
} "Get-WinEvent NetdataWEL/Application (Netdata providers, last ${SinceHours}h)"
if (Test-Path $LogDir) {
    Get-ChildItem $LogDir -File -Filter '*.log' | ForEach-Object {
        Collect-File "05-logs\$($_.Name)" "Agent log file: $($_.Name)" $_.FullName $LogCap
    }
}

# ============================================================================
# 06-state
# ============================================================================
Write-Info 'collecting: state'
$StatusFile = Join-Path $LibDir 'status-netdata.json'
if (Test-Path $StatusFile) {
    Collect-File '06-state\status-file.json' 'Daemon status file: LAST EXIT/CRASH RECORD incl. fatal stack trace (read this first for crashes)' $StatusFile
}
if (Test-Path $LibDir) {
    Collect-Cmd '06-state\state-tree.txt' 'State dir listing (secret files listed by name only, never read)' { Get-ChildItem 'C:\Program Files\Netdata\var\lib\netdata' -Recurse | Format-Table Mode, LastWriteTime, Length, FullName -AutoSize } "Get-ChildItem $LibDir -Recurse"
    $CloudDir = Join-Path $LibDir 'cloud.d'
    if (Test-Path (Join-Path $CloudDir 'claimed_id')) {
        Collect-File '06-state\claimed-id.txt' 'Cloud claim id (safe identifier; token/private.pem never collected)' (Join-Path $CloudDir 'claimed_id')
    }
}
if (Test-Path $CacheDir) {
    Collect-Cmd '06-state\db-disk-usage.txt' 'Database disk usage per tier + sqlite sizes' { Get-ChildItem 'C:\Program Files\Netdata\var\cache\netdata' -Directory | ForEach-Object { $s = (Get-ChildItem $_.FullName -Recurse -File | Measure-Object Length -Sum).Sum; '{0}  {1:N1} MB' -f $_.Name, ($s/1MB) }; Get-ChildItem 'C:\Program Files\Netdata\var\cache\netdata' -File -Filter '*.db*' | Format-Table Name, Length -AutoSize } "du of $CacheDir"
}

# ============================================================================
# 07-runtime
# ============================================================================
if ($ApiOk) {
    Write-Info 'collecting: runtime (agent is up)'
    Collect-Api '07-runtime\info-v3.json' 'BEST SINGLE CALL: buildinfo, features, cloud status, per-tier retention' '/api/v3/info'
    Collect-Api '07-runtime\info-v1.json' 'Agent info v1' '/api/v1/info'
    Collect-Api '07-runtime\node-instances.json' 'Node instances: children, streaming state, db_size, metric counts' '/api/v2/node_instances'
    Collect-Api '07-runtime\stream-info.json' 'Streaming diagnostics' '/api/v3/stream_info'
    Collect-Api '07-runtime\aclk.json' 'Cloud/ACLK connection state' '/api/v1/aclk'
    Collect-Api '07-runtime\alerts-active.json' 'Currently raised alerts' '/api/v3/alerts?options=active'
    Collect-Api '07-runtime\functions.json' 'Registered functions' '/api/v1/functions'
    Collect-Api '07-runtime\self-cpu.csv' 'Netdata CPU last 10min (csv)' '/api/v1/data?chart=netdata.server_cpu&after=-600&points=60&format=csv'
    Collect-Api '07-runtime\self-memory.csv' 'Netdata memory last 10min (csv)' '/api/v1/data?chart=netdata.memory&after=-600&points=60&format=csv'
} else {
    Write-Info 'agent API unreachable - skipping runtime section'
    $marker = Join-Path $Work '07-runtime'
    New-Item -ItemType Directory -Path $marker -Force | Out-Null
    Set-Content -Path (Join-Path $marker 'AGENT-WAS-DOWN.txt') -Value "Agent API at 127.0.0.1:$NdPort was unreachable when this bundle was created. See 05-logs and 06-state\status-file.json for why."
    Add-Manifest '07-runtime\AGENT-WAS-DOWN.txt' 'file' 'generated' 'Marker: agent was not running'
}
if (Test-Path $NetdataExe) {
    Collect-Cmd '07-runtime\buildinfo.txt' 'netdata -W buildinfo (verbatim; works with daemon down)' { & 'C:\Program Files\Netdata\usr\bin\netdata.exe' -W buildinfo 2>&1 } 'netdata.exe -W buildinfo'
    Collect-Cmd '07-runtime\buildinfo.json' 'netdata -W buildinfojson (machine-readable)' { & 'C:\Program Files\Netdata\usr\bin\netdata.exe' -W buildinfojson 2>&1 } 'netdata.exe -W buildinfojson'
}
if ((Test-Path $NetdataCli) -and $NetdataProc) {
    Collect-Cmd '07-runtime\aclk-state.json' 'Cloud connectivity state (netdatacli aclk-state json)' { & 'C:\Program Files\Netdata\usr\bin\netdatacli.exe' aclk-state json 2>&1 } 'netdatacli.exe aclk-state json'
}

# ============================================================================
# 08-network
# ============================================================================
Write-Info 'collecting: network'
Collect-Cmd '08-network\listening-sockets.txt' 'Listening sockets (netdata-related)' { Get-NetTCPConnection -State Listen | Where-Object { $_.LocalPort -eq 19999 -or (Get-Process -Id $_.OwningProcess -ErrorAction SilentlyContinue).ProcessName -match 'netdata' } | Format-Table LocalAddress, LocalPort, OwningProcess -AutoSize } 'Get-NetTCPConnection -State Listen (netdata)'
Collect-Cmd '08-network\dns-config.txt' 'DNS resolver config' { Get-DnsClientServerAddress | Where-Object { $_.ServerAddresses } | Format-Table InterfaceAlias, ServerAddresses -AutoSize } 'Get-DnsClientServerAddress'
Collect-Cmd '08-network\proxy-config.txt' 'System proxy configuration' { netsh winhttp show proxy 2>&1; Get-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' | Format-List ProxyEnable, ProxyServer, AutoConfigURL } 'netsh winhttp show proxy + registry'
Collect-Cmd '08-network\cloud-connectivity.txt' 'Reachability of Netdata Cloud (TCP/TLS only, no data sent)' { Test-NetConnection -ComputerName 'app.netdata.cloud' -Port 443 -WarningAction SilentlyContinue | Format-List ComputerName, RemotePort, TcpTestSucceeded, PingSucceeded } 'Test-NetConnection app.netdata.cloud:443'

# ============================================================================
# summary + manifest + README
# ============================================================================
Write-Info 'writing summary and manifest'
$AgentVersion = ''
$infoV1 = Join-Path $Work '07-runtime\info-v1.json'
if (Test-Path $infoV1) {
    try { $AgentVersion = (Get-Content $infoV1 -Raw | ConvertFrom-Json).version } catch { }
}
$CrashHint = ''
$sfPath = Join-Path $Work '06-state\status-file.json'
if (Test-Path $sfPath) {
    try {
        $sf = Get-Content $sfPath -Raw | ConvertFrom-Json
        if ($sf.agent -and $sf.agent.exit_reason) { $CrashHint = ($sf.agent.exit_reason -join ',') }
    } catch { }
}
$RuntimeSecs = [int]((Get-Date) - $StartTime).TotalSeconds

$summary = @"
NETDATA SUPPORT BUNDLE SUMMARY
generated:        $((Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ'))
tool version:     $SosVersion (windows)
runtime seconds:  $RuntimeSecs
ran elevated:     $(([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator))
pii obfuscation:  $(if ($Obfuscate) { 'on' } else { 'OFF' })

agent version:    $(if ($AgentVersion) { $AgentVersion } else { 'unknown' })
service status:   $(if ($NetdataSvc) { $NetdataSvc.Status } else { 'NOT INSTALLED' })
agent process:    $(if ($NetdataProc) { "yes (pid $($NetdataProc.Id))" } else { 'NOT RUNNING' })
agent api:        $(if ($ApiOk) { 'reachable' } else { 'UNREACHABLE' })
$(if ($CrashHint) { "last exit reason: $CrashHint   <-- check 06-state\status-file.json" })

READ ORDER FOR TRIAGE:
  crashes/won't start -> 06-state\status-file.json, 05-logs\eventlog-netdata.txt
  collector issues    -> 04-config\go.d*, 05-logs\
  streaming issues    -> 04-config\stream.conf, 07-runtime\node-instances.json, 01-system\clock-timesync.txt
  cloud/claiming      -> 06-state\claimed-id.txt, 07-runtime\aclk.json, 08-network\
  performance         -> 03-process\netdata-processes.txt, 06-state\db-disk-usage.txt
"@
Set-Content -Path (Join-Path $Work 'summary.txt') -Value $summary
Add-Manifest 'summary.txt' 'file' 'generated' 'Human summary'

$readme = @"
# Netdata Support Bundle (Windows)

Generated by ``netdata-sos.ps1``. Contents are SANITIZED: secrets (tokens, api
keys, passwords) are always redacted; by default IPs, MACs, emails and
hostnames are replaced with stable pseudonyms - consistent across all files.
The pseudonym map stays on this machine, next to the zip - it is NOT in this
bundle.

Layout matches the POSIX bundle (see MANIFEST.json for every file):
01-system, 02-install, 03-process, 04-config, 05-logs (Windows Event Log),
06-state, 07-runtime, 08-network. Start with summary.txt.
"@
Set-Content -Path (Join-Path $Work 'README.md') -Value $readme
Add-Manifest 'README.md' 'file' 'generated' 'Bundle documentation'

# emit MANIFEST.json LAST so every file (incl. summary.txt and README.md) is indexed
$manifest = [ordered]@{
    schema = 'netdata-sos-bundle/v1'
    tool_version = "$SosVersion-windows"
    generated_utc = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
    runtime_seconds = $RuntimeSecs
    pii_obfuscated = [bool]$Obfuscate
    secrets_redacted = $true
    agent_running = [bool]$NetdataProc
    agent_api_reachable = [bool]$ApiOk
    is_container = $false
    files = $script:ManifestRows
}
Set-Content -Path (Join-Path $Work 'MANIFEST.json') -Value ($manifest | ConvertTo-Json -Depth 4)

# ============================================================================
# zip
# ============================================================================
New-Item -ItemType Directory -Path $Output -Force | Out-Null
$ZipPath = Join-Path $Output "$BundleName.zip"
if (Test-Path $ZipPath) { Write-Error "refusing to overwrite existing $ZipPath"; exit 1 }
Compress-Archive -Path $Work -DestinationPath $ZipPath

$MapPath = Join-Path $Output "$BundleName.pseudonym-map.txt"
if ($Obfuscate -and $script:PseudoMap.Count -gt 0) {
    $script:PseudoMap.GetEnumerator() | ForEach-Object { "$($_.Value)`t$($_.Key)" } | Set-Content $MapPath
}

if (-not $KeepStaging) { Remove-Item $Staging -Recurse -Force }

Write-Host ''
Write-Info ("done in {0}s" -f [int]((Get-Date) - $StartTime).TotalSeconds)
Write-Info "bundle:  $ZipPath"
if ($Obfuscate -and $script:PseudoMap.Count -gt 0) {
    Write-Info "pseudonym map (KEEP PRIVATE, do not send): $MapPath"
}
Write-Info 'attach the bundle to your support ticket.'
