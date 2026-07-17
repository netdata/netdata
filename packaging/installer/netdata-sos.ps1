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
#              [-SinceHours 24] [-NoObfuscate] [-KeepStaging] [-SelfTest]
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
    [switch]$SelfTest,
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
$PseudonymMapMax = 4096
$JobQueueMax = 4096

# --- run at lowest priority: never compete with real workloads ---------------
try {
    $proc = Get-Process -Id $PID
    $proc.PriorityClass = [System.Diagnostics.ProcessPriorityClass]::Idle
} catch { Write-Verbose "could not lower process priority: $_" }

$StartTime = Get-Date
$Stamp = (Get-Date).ToUniversalTime().ToString('yyyyMMdd-HHmmss')
$BundleNonce = [System.IO.Path]::GetRandomFileName().Replace('.', '')
$BundleName = "netdata-sos-$Stamp-$PID-$BundleNonce"
$TempRoot = if ($env:TEMP) { $env:TEMP } else { [System.IO.Path]::GetTempPath() }
if (-not $Output) { $Output = $TempRoot }
$Staging = Join-Path $TempRoot ("netdata-sos-staging-" + [System.IO.Path]::GetRandomFileName())
$Work = Join-Path $Staging $BundleName
New-Item -ItemType Directory -Path $Work -Force | Out-Null
$script:ManifestRows = New-Object System.Collections.ArrayList
$script:PseudoMap = @{}   # original -> pseudonym
$script:IpCount = 0
$script:Ip6Count = 0
$script:FqdnCount = 0
$script:SeededHosts = @() # child/mirrored hostnames pre-seeded from the local API
$script:PathMap = @{}     # source path -> neutral bundle filename (private map only)
$script:PathCount = 0

function Show-Info([string]$msg) { Write-Host " [*] $msg" }

function Test-Deadline { return ((Get-Date) -gt $GlobalDeadline) }

function Write-Utf8NoBom([string]$path, [string]$text) {
    $encoding = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($path, $text, $encoding)
}

function Get-ReadDeadline {
    $perOperation = (Get-Date).AddSeconds($TimeoutSeconds)
    if ($perOperation -lt $GlobalDeadline) { return $perOperation }
    return $GlobalDeadline
}

function Read-HttpTextBounded([string]$uri, [long]$maximumBytes) {
    $deadline = Get-ReadDeadline
    $request = [System.Net.HttpWebRequest]::Create($uri)
    $request.Timeout = $TimeoutSeconds * 1000
    $request.ReadWriteTimeout = $TimeoutSeconds * 1000
    $response = $null; $reader = $null
    $text = New-Object System.Text.StringBuilder
    $buffer = New-Object char[] 4096
    try {
        $response = $request.GetResponse()
        $reader = New-Object System.IO.StreamReader($response.GetResponseStream())
        while (($count = $reader.Read($buffer, 0, $buffer.Length)) -gt 0) {
            if ((Get-Date) -gt $deadline) { throw 'HTTP reader deadline exceeded' }
            if (($text.Length + $count) * 4 -gt $maximumBytes) { throw 'HTTP response exceeded its bound' }
            [void]$text.Append($buffer, 0, $count)
        }
        return $text.ToString()
    } finally {
        if ($reader) { $reader.Dispose() }
        if ($response) { $response.Dispose() }
    }
}

function Get-SosJobHostProcessIds {
    if ($env:OS -ne 'Windows_NT') { return @() }
    try {
        return @(Get-CimInstance Win32_Process -Filter "ParentProcessId = $PID" -ErrorAction Stop |
            Where-Object { $_.Name -match '^(powershell|pwsh)(\.exe)?$' } |
            ForEach-Object { [int]$_.ProcessId })
    } catch { return @() }
}

function Stop-SosJobTree([System.Management.Automation.Job]$job, [int[]]$jobHostPids) {
    # Stop-Job does not contractually terminate native grandchildren. On
    # Windows, taskkill /T starts at the isolated Start-Job host and removes its
    # complete descendant tree before the PowerShell job object is discarded.
    if ($env:OS -eq 'Windows_NT') {
        $taskkill = Join-Path $env:SystemRoot 'System32\taskkill.exe'
        $jobHostPids = @($jobHostPids) + @(Get-SosJobHostProcessIds)
        foreach ($processId in @($jobHostPids | Sort-Object -Unique)) {
            if ($processId -gt 0) { & $taskkill /PID $processId /T /F 2>$null | Out-Null }
        }
    }
    if ($job -and $job.State -eq 'Running') { Stop-Job $job -ErrorAction SilentlyContinue }
}

function Publish-FileNoOverwrite([string]$source, [string]$destination) {
    # Copy through an unpredictable temp on the destination volume, then use
    # File.Move as the atomic no-overwrite publication step. This also works
    # when -Output is on a different drive from the staging directory.
    $destinationDir = Split-Path $destination -Parent
    $publicationTemp = Join-Path $destinationDir ('.netdata-sos-publish-' + [System.IO.Path]::GetRandomFileName())
    $sourceStream = $null; $destinationStream = $null
    try {
        $sourceStream = New-Object System.IO.FileStream($source, [System.IO.FileMode]::Open, [System.IO.FileAccess]::Read, [System.IO.FileShare]::Read)
        $destinationStream = New-Object System.IO.FileStream($publicationTemp, [System.IO.FileMode]::CreateNew, [System.IO.FileAccess]::Write, [System.IO.FileShare]::None)
        $sourceStream.CopyTo($destinationStream)
        $destinationStream.Flush()
        $destinationStream.Dispose(); $destinationStream = $null
        $sourceStream.Dispose(); $sourceStream = $null
        [System.IO.File]::Move($publicationTemp, $destination)
        [System.IO.File]::Delete($source)
    } catch {
        if ($destinationStream) { $destinationStream.Dispose() }
        if ($sourceStream) { $sourceStream.Dispose() }
        if ([System.IO.File]::Exists($publicationTemp)) { [System.IO.File]::Delete($publicationTemp) }
        throw
    }
}

# --- sanitizer ----------------------------------------------------------------
# pass 1 (always): credential-bearing key values, URL/DSN creds, JWT,
#                  Bearer/Basic values, private keys, UUID section headers
# pass 2 (default): emails, MACs, IPs, host identities, and customer FQDNs
$SecretKeyWords = @(
    'api key','apikey','apitoken','token','accesstoken','authtoken','claimtoken',
    'refreshtoken','sessiontoken','password','passwd','pass','pwd','pat','key',
    'secret','clientsecret','clientpassword','community','bearer','webhook',
    'webhookurl','license key','licensekey','auth','credential','credentials','cookie',
    'passphrase','proxy user','proxy pass','proxypassword','username','dsn',
    'private key','privatekey','access key','accesskey','session','recipient',
    'account sid','accountsid','priv key'
)
$HostShort = $env:COMPUTERNAME
$HostFqdn = try { [System.Net.Dns]::GetHostEntry('').HostName } catch { Write-Verbose "no FQDN: $_"; '' }
$RunUser = $env:USERNAME
if ($RunUser -in @('SYSTEM', 'Administrator') -or -not $RunUser -or $RunUser.Length -lt 3) { $RunUser = '' }

function ConvertTo-NormalizedKey([string]$key) {
    try { $key = [regex]::Unescape($key) } catch { Write-Verbose "invalid escaped key: $_" }
    $key = $key -creplace '([a-z0-9])([A-Z])', '$1 $2'
    $key = $key -creplace '([A-Z]+)([A-Z][a-z])', '$1 $2'
    return ((($key -replace '[-_.]+', ' ') -replace '\s+', ' ').ToLower().Trim(' ', '#', "`t"))
}

function Test-SecretKey([string]$key) {
    $k = ConvertTo-NormalizedKey $key
    $padded = " $k "
    foreach ($w in $SecretKeyWords) { if ($padded.Contains(" $w ")) { return $true } }
    return $false
}

function Test-DiagnosticKey([string]$key) {
    # keys that DESCRIBE secrets rather than hold them ("bearer token protection",
    # "netdata management api key file") end in a diagnostic noun; their values are
    # settings, not credentials. Exemption is KEY-based on purpose: a value like
    # "false" or "/root/x" attached to a real secret key must still be redacted.
    $k = ConvertTo-NormalizedKey $key
    foreach ($w in @('file', 'path', 'dir', 'directory', 'protection', 'support', 'mode',
                     'level', 'port', 'timeout', 'cookies', 'secure', 'log', 'size', 'options',
                     'format', 'type')) {
        if ($k -eq $w -or $k.EndsWith(" $w")) { return $true }
    }
    return $false
}

function Find-JsonStringEnd([string]$text, [int]$start) {
    $escaped = $false
    for ($i = $start + 1; $i -lt $text.Length; $i++) {
        if ($escaped) { $escaped = $false; continue }
        if ($text[$i] -eq '\') { $escaped = $true; continue }
        if ($text[$i] -eq '"') { return $i }
    }
    return -1
}

function Read-JsonComposite([string]$text, [int]$start, [hashtable]$state) {
    for ($i = $start; $i -lt $text.Length; $i++) {
        $ch = $text[$i]
        if ($state.JsonInString) {
            if ($state.JsonEscape) { $state.JsonEscape = $false; continue }
            if ($ch -eq '\') { $state.JsonEscape = $true; continue }
            if ($ch -eq '"') { $state.JsonInString = $false }
            continue
        }
        if ($ch -eq '"') { $state.JsonInString = $true; continue }
        if ($ch -eq '{' -or $ch -eq '[') {
            $state.JsonStack.Push([char]$ch)
            $state.JsonDepth = $state.JsonStack.Count
            continue
        }
        if ($ch -eq '}' -or $ch -eq ']') {
            $expected = if ($ch -eq '}') { '{' } else { '[' }
            if ($state.JsonStack.Count -eq 0 -or $state.JsonStack.Peek() -ne $expected) {
                $state.JsonWithholdForever = $true
                $state.JsonStack.Clear(); $state.JsonDepth = 0
                return -1
            }
            [void]$state.JsonStack.Pop()
            $state.JsonDepth = $state.JsonStack.Count
            if ($state.JsonDepth -eq 0) { return $i + 1 }
        }
    }
    return -1
}

function Find-JsonCompositeEnd([string]$text, [int]$start) {
    $state = @{
        JsonStack = New-Object 'System.Collections.Generic.Stack[char]'
        JsonDepth = 0; JsonInString = $false; JsonEscape = $false
        JsonWithholdForever = $false
    }
    return (Read-JsonComposite $text $start $state)
}

function Find-JsonValueEnd([string]$text, [int]$start) {
    if ($start -ge $text.Length) { return -1 }
    if ($text[$start] -eq '"') {
        $end = Find-JsonStringEnd $text $start
        if ($end -lt 0) { return -1 }
        return $end + 1
    }
    if ($text[$start] -eq '{' -or $text[$start] -eq '[') {
        return (Find-JsonCompositeEnd $text $start)
    }
    for ($i = $start; $i -lt $text.Length; $i++) {
        if ($text[$i] -eq ',' -or $text[$i] -eq '}' -or $text[$i] -eq ']') { return $i }
    }
    return $text.Length
}

function Update-JsonWithholding([string]$text, [hashtable]$state) {
    [void](Read-JsonComposite $text 0 $state)
}

function Start-JsonWithholding([string]$valueRemainder, [hashtable]$state) {
    if (-not $state) { return }
    $state.JsonDepth = 0; $state.JsonStack.Clear()
    $state.JsonInString = $false; $state.JsonEscape = $false
    if (-not $valueRemainder) { $state.JsonPending = $true }
    elseif ($valueRemainder.StartsWith('{') -or $valueRemainder.StartsWith('[')) {
        Update-JsonWithholding $valueRemainder $state
    } elseif ($valueRemainder.StartsWith('"')) {
        if ((Find-JsonStringEnd $valueRemainder 0) -lt 0) { $state.JsonTopString = $true }
    } else { $state.JsonWithholdForever = $true }
}

function Invoke-RedactJsonSecrets([string]$line, [hashtable]$state = $null) {
    $cursor = 0; $scan = 0
    $out = New-Object System.Text.StringBuilder
    while ($scan -lt $line.Length) {
        $quote = $line.IndexOf('"', $scan)
        if ($quote -lt 0) { break }
        $quoteEnd = Find-JsonStringEnd $line $quote
        if ($quoteEnd -lt 0) { break }
        $colon = $quoteEnd + 1
        while ($colon -lt $line.Length -and [char]::IsWhiteSpace($line[$colon])) { $colon++ }
        if ($colon -ge $line.Length -or $line[$colon] -ne ':') { $scan = $quoteEnd + 1; continue }
        $key = $line.Substring($quote + 1, $quoteEnd - $quote - 1)
        $valueStart = $colon + 1
        while ($valueStart -lt $line.Length -and [char]::IsWhiteSpace($line[$valueStart])) { $valueStart++ }
        if (-not (Test-SecretKey $key) -or (Test-DiagnosticKey $key)) {
            $scan = $colon + 1
            continue
        }
        $valueEnd = Find-JsonValueEnd $line $valueStart
        [void]$out.Append($line.Substring($cursor, $valueStart - $cursor))
        [void]$out.Append('"[REDACTED]"')
        if ($valueEnd -lt 0) {
            Start-JsonWithholding $line.Substring($valueStart) $state
            $cursor = $line.Length; $scan = $line.Length; break
        }
        $cursor = $valueEnd; $scan = $valueEnd
    }
    if ($cursor -lt $line.Length) { [void]$out.Append($line.Substring($cursor)) }
    return $out.ToString()
}

function Invoke-RedactAssignmentsWithPattern([string]$line, [string]$pattern) {
    $cursor = 0; $scan = 0
    $out = New-Object System.Text.StringBuilder
    $regex = New-Object System.Text.RegularExpressions.Regex($pattern, [System.Text.RegularExpressions.RegexOptions]::IgnoreCase)
    while ($scan -lt $line.Length) {
        $m = $regex.Match($line, $scan)
        if (-not $m.Success) { break }
        if ((Test-SecretKey $m.Groups['key'].Value) -and -not (Test-DiagnosticKey $m.Groups['key'].Value)) {
            [void]$out.Append($line.Substring($cursor, $m.Index - $cursor))
            [void]$out.Append($m.Groups['prefix'].Value + '[REDACTED]')
            $cursor = $m.Index + $m.Length
            $scan = $cursor
        } else { $scan = $m.Index + 1 }
    }
    if ($cursor -lt $line.Length) { [void]$out.Append($line.Substring($cursor)) }
    return $out.ToString()
}

function Get-Pseudonym([string]$orig, [string]$prefix) {
    if (-not $script:PseudoMap.ContainsKey($orig)) {
        if ($script:PseudoMap.Count -ge $PseudonymMapMax) {
            if ($prefix -eq 'ip') { return '[IP]' }
            if ($prefix -eq 'ip6') { return '[IP6]' }
            if ($prefix -eq 'fqdn') { return '[PRIVATE-HOST]' }
            return '[HOST]'
        }
        if ($prefix -eq 'ip') { $script:IpCount++; $script:PseudoMap[$orig] = "ip-$($script:IpCount)" }
        elseif ($prefix -eq 'ip6') { $script:Ip6Count++; $script:PseudoMap[$orig] = "ip6-$($script:Ip6Count)" }
        elseif ($prefix -eq 'fqdn') { $script:FqdnCount++; $script:PseudoMap[$orig] = "private-host-$($script:FqdnCount)" }
        else { $script:PseudoMap[$orig] = 'redacted-host' }
    }
    return $script:PseudoMap[$orig]
}

function Get-SafeDynamicPath([string]$sourcePath, [string]$label) {
    if (-not $script:PathMap.ContainsKey($sourcePath)) {
        $script:PathCount++
        $extension = [System.IO.Path]::GetExtension($sourcePath).TrimStart('.').ToLower()
        if ($extension -notin @('conf','yml','yaml','dyncfg','log')) { $extension = 'txt' }
        $script:PathMap[$sourcePath] = '{0}-{1:D3}.{2}' -f $label, $script:PathCount, $extension
    }
    return $script:PathMap[$sourcePath]
}

function Invoke-RedactSecretLine([string]$line, [hashtable]$state = $null) {
    # pass 1 (always on): credential redaction
    # stream.conf-style [<uuid>] section headers are api keys
    if ($line -match '^\s*\[[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\]\s*$') {
        return '[REDACTED-KEY-SECTION]'
    }
    # ini/yaml/env key = value | key: value
    # (JSON-shaped lines are owned by the json rule below, which preserves quoting)
    # only plausible config keys: short, no sentence/shell punctuation, no path
    # separators (command lines with paths belong to the argv rule below)
    if ($line -notmatch '^\s*"' -and $line -match '^([^=:]{1,64})([=:])(.+)$' -and $Matches[1] -notmatch '["`;|()/]' -and $Matches[1] -notmatch '\s-{1,2}') {
        $keyText = $Matches[1]; $separator = $Matches[2]; $valueText = $Matches[3].Trim()
        $yamlBlock = $valueText -match '(^|\s)[|>][0-9+-]*(\s|$)'
        if ((Test-SecretKey $keyText) -and ($valueText.Length -gt 0 -or $yamlBlock) -and -not (Test-DiagnosticKey $keyText)) {
            if ($state -and $yamlBlock) {
                $state.YamlHolding = $true
                $state.YamlIndent = $line.Length - $line.TrimStart(' ', "`t").Length
            }
            $line = $keyText + $separator + ' [REDACTED]'
        }
    }
    # Quote-aware JSON scanner: handles escaped quotes, scalars, and nested values.
    if ($line.Contains('"') -and $line.Contains(':')) { $line = Invoke-RedactJsonSecrets $line $state }
    # URL creds and Go-style DSN creds
    $line = $line -replace '://[^/@\s\[]+@', '://[REDACTED]@'
    $line = $line -replace '\b[\w]+:[^@\s]+@(tcp|unix)\(', '[REDACTED]@$1('
    # JWTs, HTTP auth header values
    $line = $line -replace 'eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]+', '[REDACTED-JWT]'
    # Bearer credentials may be alphabetic-only. Exempt only an explicit
    # diagnostic key shape, never a token value that happens to be an English word.
    if ($line -notmatch '^\s*#?\s*bearer\s+token\s+(protection|support|mode)\s*[=:]') {
        $bearerReplacement = 'Bearer ' + '[REDACTED]'
        $line = [regex]::Replace($line, '(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+', $bearerReplacement)
    }
    $line = [regex]::Replace($line, '(?i)\bbasic\s+[A-Za-z0-9+/=]+', 'Basic [REDACTED]')
    $line = [regex]::Replace($line, '(?i)^(\s*(?:authorization|authentication)\s*:).+$', '$1 [REDACTED]')
    # secrets passed as URL query parameters (access-log request lines etc.)
    $line = [regex]::Replace($line, '(?i)(?<prefix>[?&](?<key>[A-Za-z][\w.-]*)=)(?<value>[^&"\s]+)', {
        param($m)
        if ((Test-SecretKey $m.Groups['key'].Value) -and -not (Test-DiagnosticKey $m.Groups['key'].Value)) { return $m.Groups['prefix'].Value + '[REDACTED]' }
        return $m.Value
    })
    # argv/env-style secrets mid-line (process command lines: -token=X, CLAIM_TOKEN=X),
    # incl. two-word keys ("api key = X"); diagnostic-noun keys are kept
    $valuePattern = '(?:"(?:\\.|[^"])*"|''[^'']*''|[^&\s\[]+)'
    $line = Invoke-RedactAssignmentsWithPattern $line "(?<prefix>(?<key>-{0,2}[A-Za-z_][\w.-]*)\s*[=:]\s*)(?<value>$valuePattern)"
    $line = Invoke-RedactAssignmentsWithPattern $line "(?<prefix>(?<key>(api|license|auth|access|private|proxy)\s+(key|user|pass|password))\s*[=:]\s*)(?<value>$valuePattern)"
    $line = Invoke-RedactAssignmentsWithPattern $line "(?<prefix>(?<key>--?[A-Za-z_][\w.-]*)\s+)(?<value>$valuePattern)"
    # NOTE: multiline PEM private-key blocks are handled by Invoke-SanitizeFile,
    # which tracks BEGIN/END state across lines and withholds the whole block.
    return $line
}

function Invoke-ObfuscateDestination([string]$line) {
    # stream.conf destination hosts are user infrastructure regardless of TLD.
    # Token syntax: [PROTOCOL:]HOST[%IFACE][:PORT][:SSL]. Runs BEFORE the IP
    # rules (pure-IP tokens are skipped and left to them).
    if ($line -match '^[\s#]*(proxy )?destination\s*=') {
        $eq = $line.IndexOf('=')
        $head = $line.Substring(0, $eq + 1)
        $rebuilt = foreach ($tok in ($line.Substring($eq + 1) -split '\s+')) {
            if (-not $tok) { continue }
            $work = $tok; $prefix = ''; $suffix = ''
            if ($work -match '^(tcp:|udp:|unix:)(.*)$') { $prefix = $Matches[1]; $work = $Matches[2] }
            if (-not $work.StartsWith('[') -and -not $work.StartsWith('/')) {
                $cut = $work.IndexOfAny([char[]]('%', ':'))
                if ($cut -ge 0) { $suffix = $work.Substring($cut); $work = $work.Substring(0, $cut) }
                if ($work.Length -ge 4 -and
                    $work -notmatch '^(ip6?-\d+|private-host-\d+)$' -and
                    $work -notmatch '^\d{1,3}(\.\d{1,3}){3}$' -and
                    $work -notmatch '^[0-9A-Fa-f:]+$') {
                    $work = Get-Pseudonym $work 'fqdn'
                }
            }
            $prefix + $work + $suffix
        }
        $line = $head + ' ' + ($rebuilt -join ' ')
    }
    return $line
}

function Invoke-ObfuscateNetwork([string]$line) {
    $line = $line -replace '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}', '[EMAIL]'
    $line = $line -replace '\b([0-9A-Fa-f]{2}[:-]){5}[0-9A-Fa-f]{2}\b', '[MAC]'
    # IPv4 (keep loopback/wildcard/broadcast)
    $line = [regex]::Replace($line, '\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b', {
        param($m)
        $ip = $m.Value
        if ($ip -like '127.*' -or $ip -eq '0.0.0.0' -or $ip -like '255.*') { return $ip }
        return (Get-Pseudonym $ip 'ip')
    })
    # IPv6: hex-and-colon runs, validated to skip timestamps/:: tokens/loopback.
    # 6+ colons is always accepted: numeric-only uncompressed IPv6 has 7,
    # timestamps never have more than 2.
    $line = [regex]::Replace($line, '(?<![\w.-])[0-9A-Fa-f:]{5,}', {
        param($m)
        $c = $m.Value
        $nc = [regex]::Matches($c, ':').Count
        if ($nc -eq 0 -or $c -eq '::1' -or
            ($nc -lt 3 -and $c -notmatch '::') -or
            ($nc -lt 6 -and $c -notmatch '[A-Fa-f]' -and $c -notmatch '::')) { return $c }
        return (Get-Pseudonym $c 'ip6')
    })
    return $line
}

function Convert-BoundedLiteral([string]$line, [string]$literal, [string]$replacement) {
    if (-not $literal) { return $line }
    return [regex]::Replace($line, "(?<![\w.-])$([regex]::Escape($literal))(?![\w.-])", $replacement,
        [System.Text.RegularExpressions.RegexOptions]::IgnoreCase)
}

function Invoke-ObfuscateFqdnAndIdentity([string]$line) {
    $knownNetdataFiles = @('netdata.conf','stream.conf','exporting.conf','go.d.conf','netdata.api.key','nd.sock')
    $line = [regex]::Replace($line, '(?<![\w.-])[A-Za-z0-9](?:[A-Za-z0-9-]{0,62}\.)+[A-Za-z]{2,63}(?![\w.-])', {
        param($m)
        $fqdn = $m.Value
        $lower = $fqdn.ToLower()
        if ($lower -eq 'netdata.cloud' -or $lower.EndsWith('.netdata.cloud') -or
            $lower -eq 'netdata.io' -or $lower.EndsWith('.netdata.io') -or
            $lower -in $knownNetdataFiles) { return $fqdn }
        return (Get-Pseudonym $fqdn 'fqdn')
    })
    # child/mirrored hostnames pre-seeded from the local API (longest first,
    # word-boundary guarded so "host1" never corrupts "host10")
    foreach ($k in $script:SeededHosts) {
        if ($line.IndexOf($k, [System.StringComparison]::OrdinalIgnoreCase) -ge 0) {
            $line = [regex]::Replace($line, "(?<![\w.-])$([regex]::Escape($k))(?![\w.-])",
                $script:PseudoMap[$k], [System.Text.RegularExpressions.RegexOptions]::IgnoreCase)
        }
    }
    # this host's names
    if ($HostFqdn -and $HostFqdn.Length -ge 4) { $line = Convert-BoundedLiteral $line $HostFqdn (Get-Pseudonym $HostFqdn 'host') }
    if ($HostShort -and $HostShort.Length -ge 4 -and $HostShort -ne $HostFqdn) {
        $line = Convert-BoundedLiteral $line $HostShort (Get-Pseudonym $HostShort 'host')
    }
    if ($RunUser) {
        if (-not $script:PseudoMap.ContainsKey($RunUser)) { $script:PseudoMap[$RunUser] = 'redacted-user' }
        $line = Convert-BoundedLiteral $line $RunUser 'redacted-user'
    }
    return $line
}

function Invoke-ObfuscatePiiLine([string]$line) {
    $line = Invoke-ObfuscateDestination $line
    $line = Invoke-ObfuscateNetwork $line
    return (Invoke-ObfuscateFqdnAndIdentity $line)
}

function Invoke-SanitizeLine([string]$line, [hashtable]$state = $null) {
    $line = Invoke-RedactSecretLine $line $state
    if ($Obfuscate) { $line = Invoke-ObfuscatePiiLine $line }
    return $line
}

function Convert-SanitizedRecord([string]$line, [hashtable]$state) {
    if ($state.JsonWithholdForever) { return $null }
    if ($state.JsonPending) {
        $trimmed = $line.TrimStart()
        if (-not $trimmed) { return $null }
        $state.JsonPending = $false
        Start-JsonWithholding $trimmed $state
        return $null
    }
    if ($state.JsonTopString) {
        $escaped = [bool]$state.JsonEscape
        foreach ($ch in $line.ToCharArray()) {
            if ($escaped) { $escaped = $false; continue }
            if ($ch -eq '\') { $escaped = $true; continue }
            if ($ch -eq '"') { $state.JsonTopString = $false; break }
        }
        $state.JsonEscape = $escaped
        return $null
    }
    if ($state.JsonDepth -gt 0) {
        Update-JsonWithholding $line $state
        return $null
    }
    if ($state.YamlHolding) {
        if ($line.Trim().Length -eq 0) { return $null }
        $indent = $line.Length - $line.TrimStart(' ', "`t").Length
        if ($indent -gt $state.YamlIndent) { return $null }
        $state.YamlHolding = $false
    }
    if ($state.InPem) {
        if ($line -match '-----END [A-Z0-9 ]*PRIVATE KEY') { $state.InPem = $false }
        return $null
    }
    if ($line -match '-----BEGIN [A-Z0-9 ]*PRIVATE KEY') {
        $state.InPem = $true
        return '[REDACTED PRIVATE KEY BLOCK]'
    }
    return (Invoke-SanitizeLine $line $state)
}

function Get-Utf8Prefix([byte[]]$bytes, [int]$maximum) {
    if ($bytes.Length -le $maximum) { return ,$bytes }
    $count = $maximum
    while ($count -gt 0 -and (($bytes[$count] -band 0xC0) -eq 0x80)) { $count-- }
    $result = New-Object byte[] $count
    if ($count -gt 0) { [Array]::Copy($bytes, 0, $result, 0, $count) }
    return ,$result
}

function Get-Utf8Suffix([byte[]]$bytes, [int]$maximum) {
    if ($bytes.Length -le $maximum) { return ,$bytes }
    $start = $bytes.Length - $maximum
    while ($start -lt $bytes.Length -and (($bytes[$start] -band 0xC0) -eq 0x80)) { $start++ }
    $count = $bytes.Length - $start
    $result = New-Object byte[] $count
    if ($count -gt 0) { [Array]::Copy($bytes, $start, $result, 0, $count) }
    return ,$result
}

function Add-SanitizedRecordToSink([string]$text, [bool]$tooLarge, [hashtable]$state,
                                   $queue, [System.IO.Stream]$stream,
                                   [System.Text.Encoding]$utf8, [long]$cap, [bool]$tail) {
    if ($text.EndsWith("`r")) { $text = $text.Substring(0, $text.Length - 1) }
    if ($tooLarge) { $text = '[REDACTED OVERSIZED LOGICAL RECORD]' }
    else {
        $converted = Convert-SanitizedRecord $text $state
        if ($null -eq $converted) { return }
        $text = $converted
    }
    $bytes = $utf8.GetBytes($text + "`n")
    $state.Total += $bytes.Length
    if ($tail) {
        if ($bytes.Length -gt $cap) { $bytes = Get-Utf8Suffix $bytes ([int]$cap); $state.Truncated = $true }
        $queue.Enqueue($bytes); $state.QueueBytes += $bytes.Length
        while ($state.QueueBytes -gt $cap -and $queue.Count -gt 0) {
            $removed = $queue.Dequeue(); $state.QueueBytes -= $removed.Length; $state.Truncated = $true
        }
        return
    }
    if ($state.Written -ge $cap) { $state.Truncated = $true; return }
    $remaining = [int][Math]::Min([long][int]::MaxValue, $cap - $state.Written)
    $kept = Get-Utf8Prefix $bytes $remaining
    if ($kept.Length -gt 0) { $stream.Write($kept, 0, $kept.Length); $state.Written += $kept.Length }
    if ($kept.Length -lt $bytes.Length) { $state.Truncated = $true }
}

function Write-SanitizedTailQueue([string]$path, $queue, [hashtable]$state,
                                  [System.Text.Encoding]$utf8, [long]$cap) {
    $marker = $utf8.GetBytes("### TRUNCATED: keeping sanitized tail ###`n")
    if (-not $state.Truncated) { $marker = New-Object byte[] 0 }
    while (($state.QueueBytes + $marker.Length) -gt $cap -and $queue.Count -gt 0) {
        $removed = $queue.Dequeue(); $state.QueueBytes -= $removed.Length
    }
    $stream = New-Object System.IO.FileStream($path, [System.IO.FileMode]::CreateNew, [System.IO.FileAccess]::Write, [System.IO.FileShare]::None)
    try {
        if ($marker.Length -gt 0) { $stream.Write($marker, 0, $marker.Length) }
        foreach ($item in $queue) { $stream.Write($item, 0, $item.Length) }
    } finally { $stream.Dispose() }
}

function Write-SanitizedReaderCapped([System.IO.TextReader]$reader, [string]$path, [long]$cap,
                                     [switch]$Tail, [datetime]$Deadline = [datetime]::MaxValue) {
    $utf8 = New-Object System.Text.UTF8Encoding($false)
    $state = @{
        InPem = $false; JsonDepth = 0; JsonInString = $false
        JsonEscape = $false; JsonWithholdForever = $false; JsonPending = $false
        JsonTopString = $false
        JsonStack = New-Object 'System.Collections.Generic.Stack[char]'
        YamlHolding = $false; YamlIndent = 0
        Written = [long]0; Total = [long]0; Truncated = $false; QueueBytes = [long]0
    }
    $queue = New-Object 'System.Collections.Generic.Queue[byte[]]'
    $stream = $null
    if (-not $Tail) { $stream = New-Object System.IO.FileStream($path, [System.IO.FileMode]::CreateNew, [System.IO.FileAccess]::Write, [System.IO.FileShare]::None) }
    $record = New-Object System.Text.StringBuilder
    $oversized = $false
    $maxRecordChars = 4MB
    $buffer = New-Object char[] 8192
    try {
        while (($count = $reader.Read($buffer, 0, $buffer.Length)) -gt 0) {
            if ((Get-Date) -gt $Deadline) { throw 'sanitized reader deadline exceeded' }
            for ($i = 0; $i -lt $count; $i++) {
                if ($buffer[$i] -eq "`n") {
                    Add-SanitizedRecordToSink $record.ToString() $oversized $state $queue $stream $utf8 $cap ([bool]$Tail)
                    [void]$record.Clear(); $oversized = $false
                } elseif (-not $oversized) {
                    if ($record.Length -ge $maxRecordChars) { [void]$record.Clear(); $oversized = $true }
                    else { [void]$record.Append($buffer[$i]) }
                }
            }
        }
        if ($record.Length -gt 0 -or $oversized) {
            Add-SanitizedRecordToSink $record.ToString() $oversized $state $queue $stream $utf8 $cap ([bool]$Tail)
        }
    } finally {
        if ($stream) { $stream.Dispose() }
    }
    if ($Tail) { Write-SanitizedTailQueue $path $queue $state $utf8 $cap }
    return $state
}

function Invoke-SanitizeFile([string]$path) {
    if (-not (Test-Path $path -PathType Leaf)) { return }
    $tmp = Join-Path $Staging ("sanitize-" + [System.IO.Path]::GetRandomFileName())
    $reader = [System.IO.File]::OpenText($path)
    try { [void](Write-SanitizedReaderCapped $reader $tmp 16MB) }
    finally { $reader.Dispose() }
    Move-Item $tmp $path -Force
}

# --- sanitizer self-test (-SelfTest) ----------------------------------------------
# Table-driven regression suite for the redaction rules. Run it after ANY change
# to the sanitizer, and keep it in parity with the POSIX script's --selftest.
if ($SelfTest) {
    $HostShort = 'testhost'
    $HostFqdn = 'testhost.example.com'
    $RunUser = 'testuser9'
    # Build this vector in pieces so static secret scanners do not mistake the
    # synthetic credential for a committed live one.
    $bearerVector = '    Authorization: Bear' + 'er SENTINEL-ALPHABETIC'
    $wordBearerVector = 'WORDTOKEN Authorization: Bear' + 'er token'
    $vectors = @(
        @{ in = 'api key = SENTINEL-1';                                            mustNot = @('SENTINEL');            must = @('[REDACTED]') }
        @{ in = $bearerVector;                                                       mustNot = @('SENTINEL');            must = @('[REDACTED]') }
        @{ in = $wordBearerVector;                                                   mustNot = @('Bearer token');         must = @('WORDTOKEN',('Bearer ' + '[REDACTED]')) }
        @{ in = 'password: SENTINEL-3';                                            mustNot = @('SENTINEL');            must = @('[REDACTED]') }
        @{ in = '"claim_token": "SENTINEL-4"';                                     mustNot = @('SENTINEL');            must = @('[REDACTED]') }
        @{ in = '{"password":"SENTINEL-A\"SENTINEL-B","token":123456789,"ok":true}'; mustNot = @('SENTINEL','123456789'); must = @('"password":"[REDACTED]"','"token":"[REDACTED]"','"ok":true') }
        @{ in = '{"accessToken":"SENTINEL-CAMEL-A","clientSecret":"SENTINEL-CAMEL-B"}'; mustNot = @('SENTINEL'); must = @('"accessToken":"[REDACTED]"','"clientSecret":"[REDACTED]"') }
        @{ in = '{"databasePassword":"SENTINEL-DB","dbPassword":"SENTINEL-DB2","githubToken":"SENTINEL-GH","apiSecret":"SENTINEL-API"}'; mustNot = @('SENTINEL'); must = @('[REDACTED]') }
        @{ in = '{"pass\u0077ord":"SENTINEL-ESCAPED"}';                         mustNot = @('SENTINEL');            must = @('[REDACTED]') }
        @{ in = 'url: https://admin:SENTINEL-5@app.example.com/x';                 mustNot = @('SENTINEL');            must = @('[REDACTED]@') }
        @{ in = 'dsn: user:SENTINEL-6@tcp(10.1.2.3:3306)/db';                      mustNot = @('SENTINEL');            must = @('[REDACTED]') }
        @{ in = 'TELEGRAM_BOT_TOKEN="SENTINEL-7"';                                 mustNot = @('SENTINEL');            must = @('[REDACTED]') }
        @{ in = 'DEFAULT_RECIPIENT_SLACK="SENTINEL-8"';                            mustNot = @('SENTINEL');            must = @('[REDACTED]') }
        @{ in = '[11111111-2222-3333-4444-555555555555]';                          mustNot = @('11111111');            must = @('[REDACTED-KEY-SECTION]') }
        @{ in = 'destination = parent.example.internal:19999';                     mustNot = @('parent.example');      must = @('private-host') }
        @{ in = 'server at 10.1.2.3 talked to 192.168.5.7 then 10.1.2.3 again';    mustNot = @('10.1.2.3', '192.168'); must = @('ip-') }
        @{ in = 'admin email is ops@customer-corp.com on host testhost';           mustNot = @('customer-corp', 'testhost '); must = @('[EMAIL]', 'redacted-host') }
        @{ in = 'service at customer.tenant-example.com and testhost10';            mustNot = @('customer.tenant-example.com'); must = @('private-host', 'testhost10') }
        @{ in = 'GET /api/v1/data?chart=x&token=SENTINEL-9&after=-60';             mustNot = @('SENTINEL');            must = @('[REDACTED]', 'after=-60') }
        @{ in = 'netdata 1234 /usr/sbin/netdata-claim.sh -token=SENTINEL-10 -rooms=abc'; mustNot = @('SENTINEL');      must = @('[REDACTED]', '-rooms=abc') }
        @{ in = 'Environment: NETDATA_CLAIM_TOKEN=SENTINEL-11 PATH=/usr/bin';      mustNot = @('SENTINEL');            must = @('[REDACTED]') }
        @{ in = 'DB_PASS=SENTINEL-DB MYSQL_PWD=SENTINEL-MYSQL GH_PAT=SENTINEL-PAT'; mustNot = @('SENTINEL');           must = @('[REDACTED]') }
        @{ in = 'netdata --api-key:SENTINEL-HYPHEN --verbose';                     mustNot = @('SENTINEL');            must = @('[REDACTED]','--verbose') }
        @{ in = 'netdata --password="SENTINEL-QUOTED" --token=''SENTINEL-SINGLE'' --verbose-quoted'; mustNot = @('SENTINEL'); must = @('[REDACTED]','--verbose-quoted') }
        @{ in = 'netdata --password "SENTINEL-SPACED" --api-key ''SENTINEL-KEY'' --verbose-spaced'; mustNot = @('SENTINEL'); must = @('[REDACTED]','--verbose-spaced') }
        @{ in = 'redis://:SENTINEL-URI@cache.customer.example/0';                  mustNot = @('SENTINEL');            must = @('://[REDACTED]@') }
        @{ in = 'https://SENTINEL-TOKEN@api.customer.example/path';                mustNot = @('SENTINEL');            must = @('://[REDACTED]@') }
        @{ in = 'Authorization: BASIC YTpi';                                        mustNot = @('YTpi');                must = @('[REDACTED]') }
        @{ in = 'Authentication: Digest SENTINEL-DIGEST';                          mustNot = @('SENTINEL');            must = @('[REDACTED]') }
        @{ in = 'IPv6: 2606:4700:10::ac42:aad8 and fe80::1ff:fe23:4567:890a';      mustNot = @('2606:4700', 'fe80::1ff'); must = @('ip6-') }
        @{ in = 'peer 2001:470:26:307:0:0:0:1 connected';                          mustNot = @('2001:470');            must = @('ip6-') }
        @{ in = 'captured: 2026-07-16T13:38:34Z listening on ::1 and file.c:123';  mustNot = @();                      must = @('13:38:34Z', '::1', 'file.c:123') }
        @{ in = '# bearer token protection = no';                                  mustNot = @('[REDACTED]');          must = @('bearer token protection = no') }
        @{ in = '# TCP SYN cookies = auto';                                        mustNot = @('[REDACTED]');          must = @('auto') }
        @{ in = '# netdata management api key file = /var/lib/netdata/netdata.api.key'; mustNot = @('[REDACTED]');     must = @('/var/lib/netdata/netdata.api.key') }
        @{ in = 'cmdline: /x/claim.sh api key = SENTINEL-12 end';                  mustNot = @('SENTINEL');            must = @('[REDACTED]', 'end') }
        @{ in = 'TOKEN=false';                                                     mustNot = @('false');               must = @('[REDACTED]') }
        @{ in = 'PASSWORD=/root/x';                                                mustNot = @('/root/x');             must = @('[REDACTED]') }
        @{ in = 'destination = tcp:parent.example.com:19999';                      mustNot = @('parent.example.com');  must = @('tcp:', 'private-host', ':19999') }
        @{ in = 'tcp LISTEN 0 4096 *:19999';                                       mustNot = @('private-host');        must = @('tcp LISTEN') }
        @{ in = 'customer.key customer.sh';                                        mustNot = @('customer.key','customer.sh'); must = @('private-host') }
        @{ in = 'mixed TESTHOST and TESTHOST.EXAMPLE.COM';                         mustNot = @('TESTHOST');            must = @('redacted-host') }
    )
    $fails = 0
    foreach ($v in $vectors) {
        $result = Invoke-SanitizeLine $v.in
        $ok = $true
        foreach ($p in $v.mustNot) { if ($result.IndexOf($p, [System.StringComparison]::OrdinalIgnoreCase) -ge 0) { $ok = $false } }
        foreach ($p in $v.must) { if ($result.IndexOf($p, [System.StringComparison]::OrdinalIgnoreCase) -lt 0) { $ok = $false } }
        if ($ok) { Write-Output "ok:   $result" } else { Write-Output "FAIL: '$($v.in)' -> '$result'"; $fails++ }
    }
    $mapSave = $script:PseudoMap; $ipCountSave = $script:IpCount; $mapLimitSave = $PseudonymMapMax
    try {
        $script:PseudoMap = @{}; $script:IpCount = 0; $PseudonymMapMax = 2
        [void](Get-Pseudonym '10.0.0.1' 'ip')
        [void](Get-Pseudonym '10.0.0.2' 'ip')
        $overflowPseudonym = Get-Pseudonym '10.0.0.3' 'ip'
        if ($script:PseudoMap.Count -ne 2 -or $overflowPseudonym -ne '[IP]') {
            Write-Output 'FAIL: pseudonym map cardinality bound failed'; $fails++
        } else { Write-Output 'ok:   pseudonym map cardinality is bounded' }
    } finally {
        $script:PseudoMap = $mapSave; $script:IpCount = $ipCountSave; $PseudonymMapMax = $mapLimitSave
    }
    # multiline PEM block must be withheld end-to-end by the file-level sanitizer
    $pemTmp = Join-Path $Staging 'pem-test.txt'
    $pemLines = @('before line',
      '-----BEGIN RSA PRIVATE KEY-----',
      'MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQ',
      'aGVsbG8gd29ybGQgdGhpcyBpcyBrZXkgbWF0ZXJpYWw=',
      '-----END RSA PRIVATE KEY-----',
      'after line',
      '{"credentials": {',
      '  "value": "SENTINEL-NESTED-JSON"',
      '}}',
      'after nested json',
      '{"password":',
      '  "SENTINEL-NEXT-LINE"}',
      'after next-line json',
      'password: |',
      '  SENTINEL-YAML-BLOCK',
      'after yaml block',
      'password: |2-',
      '  SENTINEL-YAML-INDICATOR',
      'after yaml indicator',
      'password: &credential |',
      '  SENTINEL-YAML-ANCHOR',
      'after yaml anchor',
      '{"password": {',
      '  "nested": "SENTINEL-MISMATCH-INNER"',
      ']',
      'SENTINEL-MISMATCH-AFTER')
    Write-Utf8NoBom $pemTmp (($pemLines -join "`n") + "`n")
    Invoke-SanitizeFile $pemTmp
    $pem = Get-Content $pemTmp -Raw
    if ($pem -match 'MIIEvQ' -or $pem -match 'aGVsbG8' -or $pem -notmatch '\[REDACTED PRIVATE KEY BLOCK\]' -or
        $pem -notmatch 'before line' -or $pem -notmatch 'after line' -or
        $pem -match 'SENTINEL-NESTED-JSON' -or $pem -notmatch 'after nested json' -or
        $pem -match 'SENTINEL-NEXT-LINE' -or $pem -notmatch 'after next-line json' -or
        $pem -match 'SENTINEL-YAML-BLOCK' -or $pem -notmatch 'after yaml block' -or
        $pem -match 'SENTINEL-YAML-INDICATOR' -or $pem -notmatch 'after yaml indicator' -or
        $pem -match 'SENTINEL-YAML-ANCHOR' -or $pem -notmatch 'after yaml anchor' -or
        $pem -match 'SENTINEL-MISMATCH') {
        Write-Output 'FAIL: multiline secret block not fully withheld'; $fails++
    } else { Write-Output 'ok:   PEM, JSON, and YAML secret blocks withheld' }
    $sameLineTmp = Join-Path $Staging 'json-mismatch-same-line.txt'
    Write-Utf8NoBom $sameLineTmp ("{`"password`": {`"x`": [}, `"other`": `"SENTINEL-SAME-LINE`"}`nSENTINEL-AFTER-SAME-LINE`n")
    Invoke-SanitizeFile $sameLineTmp
    if ((Get-Content $sameLineTmp -Raw) -match 'SENTINEL') {
        Write-Output 'FAIL: mismatched same-line JSON composite leaked a secret'; $fails++
    } else { Write-Output 'ok:   mismatched JSON composite fails closed' }
    # The complete logical record is sanitized before a tail cap is applied.
    $boundarySrc = Join-Path $Staging 'boundary-source.txt'
    $boundaryOut = Join-Path $Staging 'boundary-output.txt'
    [System.IO.File]::WriteAllText($boundarySrc,
        ('header=eyJABCDEFGHIJK.SENTINEL-BOUNDARY-PAYLOAD.SENTINEL-BOUNDARY-SIGNATURE' + ('X' * 4106) + "`n"))
    $boundaryReader = [System.IO.File]::OpenText($boundarySrc)
    try { [void](Write-SanitizedReaderCapped $boundaryReader $boundaryOut 4096 -Tail) }
    finally { $boundaryReader.Dispose() }
    if ((Get-Content $boundaryOut -Raw) -match 'SENTINEL-BOUNDARY' -or (Get-Item $boundaryOut).Length -gt 4096) {
        Write-Output 'FAIL: capped tail leaked a secret fragment or exceeded its cap'; $fails++
    } else { Write-Output 'ok:   truncation occurs after logical-record sanitization' }
    $script:SelfTestFailures = $fails
}

# --- manifest -------------------------------------------------------------------
function Add-Manifest([string]$rel, [string]$kind, [string]$origin, [string]$title) {
    $full = Join-Path $Work $rel
    $bytes = 0
    if (Test-Path $full) { $bytes = (Get-Item $full).Length }
    [void]$script:ManifestRows.Add(@{
        path = $rel.Replace('\', '/'); kind = $kind
        origin = Invoke-SanitizeLine ([regex]::Replace($origin, '[\x00-\x1f]', ' '))
        title = Invoke-SanitizeLine ([regex]::Replace($title, '[\x00-\x1f]', ' '))
        bytes = [long]$bytes; pii_obfuscated = [bool]$Obfuscate
    })
}

# --- collectors -------------------------------------------------------------------
function Write-SanitizedCaptureLine([string]$line, [hashtable]$capture,
                                    [System.IO.Stream]$stream,
                                    [System.Text.Encoding]$utf8, [long]$limit) {
    if ($capture.Truncated) { return }
    if ($line.Length -gt 4MB) { $line = '[REDACTED OVERSIZED LOGICAL RECORD]' }
    $converted = Convert-SanitizedRecord $line $capture
    if ($null -eq $converted) { return }
    $bytes = $utf8.GetBytes($converted + "`n")
    $remaining = [int][Math]::Min([long][int]::MaxValue, $limit - $capture.Written)
    if ($remaining -le 0) { $capture.Truncated = $true; return }
    $kept = Get-Utf8Prefix $bytes $remaining
    if ($kept.Length -gt 0) { $stream.Write($kept, 0, $kept.Length); $capture.Written += $kept.Length }
    if ($kept.Length -lt $bytes.Length) { $capture.Truncated = $true }
}

function Receive-SosJobCapture($job, [hashtable]$capture, [System.IO.Stream]$stream,
                               [System.Text.Encoding]$utf8, [long]$limit) {
    Receive-Job -Job $job -ErrorAction Continue 2>&1 | Out-String -Stream | ForEach-Object {
        Write-SanitizedCaptureLine $_ $capture $stream $utf8 $limit
    }
}

function Get-SosJobQueuedRecordCount($job) {
    $count = 0
    foreach ($child in @($job.ChildJobs)) {
        $count += $child.Output.Count + $child.Error.Count + $child.Warning.Count +
                  $child.Verbose.Count + $child.Debug.Count + $child.Information.Count
    }
    return $count
}

function Invoke-CappedJobCapture([scriptblock]$cmd, [string]$path, [long]$cap, [string]$originText, [switch]$Raw) {
    $utf8 = New-Object System.Text.UTF8Encoding($false)
    $marker = $utf8.GetBytes("### TRUNCATED: sanitized command output exceeded $cap bytes ###`n")
    $limit = if ($Raw) { $cap } else { [Math]::Max(0, $cap - $marker.Length) }
    $capture = @{
        Written = [long]0; Truncated = $false; InPem = $false; TimedOut = $false
        JsonDepth = 0; JsonInString = $false; JsonEscape = $false; JsonWithholdForever = $false
        JsonPending = $false; JsonTopString = $false; QueueOverflow = $false
        JsonStack = New-Object 'System.Collections.Generic.Stack[char]'
        YamlHolding = $false; YamlIndent = 0
    }
    $stream = New-Object System.IO.FileStream($path, [System.IO.FileMode]::CreateNew, [System.IO.FileAccess]::Write, [System.IO.FileShare]::None)
    $job = $null
    $jobHostPids = @()
    try {
        if (-not $Raw) {
            Write-SanitizedCaptureLine "# netdata-sos v$SosVersion | command: $originText | captured: $((Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ'))" $capture $stream $utf8 $limit
        }
        $jobHostsBefore = @(Get-SosJobHostProcessIds)
        $job = Start-Job -ScriptBlock $cmd
        for ($probe = 0; $probe -lt 50 -and $jobHostPids.Count -eq 0; $probe++) {
            $jobHostPids = @(Get-SosJobHostProcessIds | Where-Object { $jobHostsBefore -notcontains $_ })
            if ($jobHostPids.Count -eq 0) { Start-Sleep -Milliseconds 20 }
        }
        $jobDeadline = (Get-Date).AddSeconds($TimeoutSeconds)
        while (-not $capture.Truncated -and ((Get-Date) -lt $jobDeadline) -and
               ($job.State -eq 'Running' -or $job.HasMoreData)) {
            if ((Get-SosJobQueuedRecordCount $job) -gt $JobQueueMax) {
                $capture.QueueOverflow = $true
                break
            }
            Receive-SosJobCapture $job $capture $stream $utf8 $limit
            if ($job.State -eq 'Running') { Start-Sleep -Milliseconds 50 }
        }
        if ($job.State -eq 'Running' -and (Get-Date) -ge $jobDeadline) { $capture.TimedOut = $true }
        if ($job.State -ne 'Running' -and -not $capture.Truncated) {
            Receive-SosJobCapture $job $capture $stream $utf8 $limit
        }
        if ($job.State -eq 'Running') { Stop-SosJobTree $job $jobHostPids }
        $finalState = $job.State
        if (-not $Raw) {
            if ($capture.TimedOut) { Write-SanitizedCaptureLine "TIMEOUT after ${TimeoutSeconds}s" $capture $stream $utf8 $limit }
            Write-SanitizedCaptureLine "# job state: $($job.State)" $capture $stream $utf8 $limit
            if ($capture.Truncated) { $stream.Write($marker, 0, $marker.Length); $capture.Written += $marker.Length }
        }
    } finally {
        if ($job) {
            if ($job.State -eq 'Running') { Stop-SosJobTree $job $jobHostPids }
            Remove-Job $job -Force -ErrorAction SilentlyContinue
        }
        $stream.Dispose()
    }
    $abnormal = $capture.TimedOut -or $capture.QueueOverflow -or
                (-not $capture.Truncated -and $finalState -ne 'Completed')
    return @{
        Safe = (-not $abnormal -and (-not $Raw -or -not $capture.Truncated))
        Truncated = $capture.Truncated; TimedOut = $capture.TimedOut
        QueueOverflow = $capture.QueueOverflow; State = $finalState
    }
}

function Collect-Cmd([string]$rel, [string]$title, [scriptblock]$cmd, [string]$originText) {
    if (Test-Deadline) { return }
    $full = Join-Path $Work $rel
    New-Item -ItemType Directory -Path (Split-Path $full) -Force | Out-Null
    try {
        $capture = Invoke-CappedJobCapture $cmd $full 2MB $originText
        if (-not $capture.Safe) { throw "collector ended unsafely: $($capture.State)" }
    } catch {
        if (Test-Path $full) { Remove-Item $full -Force }
        Write-Utf8NoBom $full "[netdata-sos] collection or sanitization failed; content withheld for safety`n"
    }
    Add-Manifest $rel 'cmd' $originText $title
}

function Collect-CmdRaw([string]$rel, [string]$title, [scriptblock]$cmd, [string]$originText) {
    # like Collect-Cmd but with NO header/trailer, for commands whose output must
    # stay parseable (JSON). Provenance lives in MANIFEST.json only. The file is
    # removed if the command produced nothing.
    if (Test-Deadline) { return }
    $full = Join-Path $Work $rel
    New-Item -ItemType Directory -Path (Split-Path $full) -Force | Out-Null
    $capture = $null
    try {
        $capture = Invoke-CappedJobCapture $cmd $full 2MB $originText -Raw
    } catch { Write-Verbose "collector failed: $_" }
    if ($capture -and $capture.Safe -and (Test-Path $full) -and (Get-Item $full).Length -gt 0) {
        Add-Manifest $rel 'cmd' $originText $title
    } elseif (Test-Path $full) {
        Remove-Item $full -Force
    }
}

function Collect-File([string]$rel, [string]$title, [string]$src, [long]$cap = 0, [string]$safeOrigin = '') {
    if (Test-Deadline) { return }
    if ($cap -eq 0) { $cap = $FileCap }
    if (-not (Test-Path $src -PathType Leaf)) { return }
    $full = Join-Path $Work $rel
    New-Item -ItemType Directory -Path (Split-Path $full) -Force | Out-Null
    $size = (Get-Item $src).Length
    $origin = if ($safeOrigin) { $safeOrigin } else { $src }
    $reader = $null
    try {
        $reader = [System.IO.File]::OpenText($src)
        if ($size -gt $cap) {
            [void](Write-SanitizedReaderCapped $reader $full $cap -Tail -Deadline (Get-ReadDeadline))
            $origin = "$origin (sanitized tail, cap $cap of $size source bytes)"
        } else {
            [void](Write-SanitizedReaderCapped $reader $full $cap -Deadline (Get-ReadDeadline))
        }
    } catch {
        if (Test-Path $full) { Remove-Item $full -Force }
        Write-Utf8NoBom $full "[netdata-sos] sanitization failed; content withheld for safety`n"
    } finally {
        if ($reader) { $reader.Dispose() }
    }
    Add-Manifest $rel 'file' $origin $title
}

$NdPort = 19999
$CloudHost = 'app.netdata.cloud'   # reachability probe target
function Collect-Api([string]$rel, [string]$title, [string]$urlPath) {
    if (Test-Deadline) { return }
    $full = Join-Path $Work $rel
    New-Item -ItemType Directory -Path (Split-Path $full) -Force | Out-Null
    try {
        $operationDeadline = Get-ReadDeadline
        $request = [System.Net.HttpWebRequest]::Create("http://127.0.0.1:$NdPort$urlPath")
        $request.Timeout = $TimeoutSeconds * 1000
        $request.ReadWriteTimeout = $TimeoutSeconds * 1000
        $response = $request.GetResponse()
        $reader = New-Object System.IO.StreamReader($response.GetResponseStream())
        try { [void](Write-SanitizedReaderCapped $reader $full 2MB -Deadline $operationDeadline) }
        finally { $reader.Dispose(); $response.Dispose() }
        Add-Manifest $rel 'api' $urlPath $title
    } catch {
        if (Test-Path $full) { Remove-Item $full -Force }
    }
}

if ($SelfTest) {
    try {
        $script:BoundedSelfTestOutput = [System.IO.Path]::Combine($Staging, 'bounded-command.txt')
    } catch {
        Write-Output "FAIL: bounded self-test setup raised $_"; exit 1
    }
    try {
        $boundedCommand = {
            Write-Output 'DB_PASS=SENTINEL-COMMAND'
            1..5000 | ForEach-Object { Write-Output 'bounded-output-line' }
        }
        $capture = Invoke-CappedJobCapture -cmd $boundedCommand -path $script:BoundedSelfTestOutput -cap 1024 -originText 'bounded-output-selftest'
        $boundedText = Get-Content $script:BoundedSelfTestOutput -Raw
        if ((Get-Item $script:BoundedSelfTestOutput).Length -gt 1024 -or $boundedText -match 'SENTINEL-COMMAND' -or -not $capture.Truncated) {
            Write-Output 'FAIL: command capture was not bounded after sanitization'; $script:SelfTestFailures++
        } else { Write-Output 'ok:   command capture is sanitized and bounded while running' }
    } catch {
        Write-Output "FAIL: bounded command capture raised $_ at $($_.InvocationInfo.PositionMessage)"; $script:SelfTestFailures++
    }
    $timeoutSave = $TimeoutSeconds
    try {
        $TimeoutSeconds = 1
        $timeoutOutput = Join-Path $Staging 'timeout-command.txt'
        $timeoutCommand = { Write-Output 'prefix https://admin:SENTINEL-TIMEOUT-PASSWORD'; Start-Sleep 30 }
        $timeoutCapture = Invoke-CappedJobCapture -cmd $timeoutCommand -path $timeoutOutput -cap 1024 -originText 'timeout-selftest' -Raw
        if ($timeoutCapture.Safe -or -not $timeoutCapture.TimedOut) {
            Write-Output 'FAIL: timed-out raw capture was marked safe'; $script:SelfTestFailures++
        } else { Write-Output 'ok:   timed-out raw captures are withheld fail-closed' }
    } catch {
        Write-Output "FAIL: timeout capture self-test raised $_"; $script:SelfTestFailures++
    } finally { $TimeoutSeconds = $timeoutSave }
    try {
        $publishSource = Join-Path $Staging 'publish-source.txt'
        $publishDestination = Join-Path $Staging 'publish-destination.txt'
        [System.IO.File]::WriteAllText($publishSource, 'first')
        Publish-FileNoOverwrite $publishSource $publishDestination
        [System.IO.File]::WriteAllText($publishSource, 'second')
        $refused = $false
        try { Publish-FileNoOverwrite $publishSource $publishDestination }
        catch { $refused = $true }
        if (-not $refused -or [System.IO.File]::ReadAllText($publishDestination) -ne 'first') {
            Write-Output 'FAIL: no-overwrite artifact publication contract broke'; $script:SelfTestFailures++
        } else { Write-Output 'ok:   artifact publication refuses overwrite' }
    } catch {
        Write-Output "FAIL: publication self-test raised $_"; $script:SelfTestFailures++
    }
    if ($env:OS -eq 'Windows_NT') {
        $treeTimeoutSave = $TimeoutSeconds
        try {
            $treePidFile = Join-Path $Staging 'job-tree-pids.txt'
            $treeOutput = Join-Path $Staging 'job-tree-output.txt'
            $env:ND_SOS_TREE_PID_FILE = $treePidFile
            $treeCommand = {
                $child = Start-Process -FilePath powershell.exe -ArgumentList '-NoProfile','-Command','Start-Sleep 30' -PassThru
                "$PID $($child.Id)" | Set-Content -LiteralPath $env:ND_SOS_TREE_PID_FILE
                Wait-Process -Id $child.Id
            }
            $TimeoutSeconds = 1
            [void](Invoke-CappedJobCapture -cmd $treeCommand -path $treeOutput -cap 1024 -originText 'job-tree-selftest')
            $TimeoutSeconds = $treeTimeoutSave
            Start-Sleep -Milliseconds 250
            $orphaned = $false
            foreach ($processId in ((Get-Content $treePidFile -Raw).Trim() -split '\s+')) {
                if (Get-Process -Id ([int]$processId) -ErrorAction SilentlyContinue) { $orphaned = $true }
            }
            if ($orphaned) { Write-Output 'FAIL: timed-out Windows job left a descendant running'; $script:SelfTestFailures++ }
            else { Write-Output 'ok:   Windows timeout terminates the complete job process tree' }
        } catch {
            Write-Output "FAIL: Windows job-tree self-test raised $_"; $script:SelfTestFailures++
        } finally {
            $TimeoutSeconds = $treeTimeoutSave
            Remove-Item Env:ND_SOS_TREE_PID_FILE -ErrorAction SilentlyContinue
        }
    }
    Remove-Item $Staging -Recurse -Force
    if ($script:SelfTestFailures -gt 0) { Write-Output "SELF TEST: $($script:SelfTestFailures) FAILURES"; exit 1 }
    Write-Output 'SELF TEST: ALL PASS'
    exit 0
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
    [void](Read-HttpTextBounded "http://127.0.0.1:$NdPort/api/v1/info" 256KB)
    $ApiOk = $true
} catch { Write-Verbose "agent API not reachable: $_" }

# pre-seed pseudonyms for child/mirrored hostnames, so a parent's children are
# obfuscated consistently in every file (node_instances, stream configs, logs)
if ($ApiOk -and $Obfuscate) {
    try {
        $names = @()
        $ni = Read-HttpTextBounded "http://127.0.0.1:$NdPort/api/v2/node_instances" 2MB | ConvertFrom-Json
        if ($ni -and $ni.PSObject.Properties['nodes']) { $names += @($ni.nodes | ForEach-Object { $_.nm }) }
        $v1 = Read-HttpTextBounded "http://127.0.0.1:$NdPort/api/v1/info" 2MB | ConvertFrom-Json
        if ($v1 -and $v1.PSObject.Properties['mirrored_hosts']) { $names += @($v1.mirrored_hosts) }
        foreach ($n in ($names | Sort-Object -Unique)) {
            if (-not $n -or $n.Length -lt 4 -or $n -eq 'localhost' -or $n -eq $HostShort -or $n -eq $HostFqdn) { continue }
            [void](Get-Pseudonym $n 'fqdn')
        }
        # longest first so overlapping names (host, host-2) replace correctly
        $script:SeededHosts = @($script:PseudoMap.Keys |
            Where-Object { $script:PseudoMap[$_] -like 'private-host*' } |
            Sort-Object { $_.Length } -Descending)
    } catch { Write-Verbose "child hostname pre-seed failed: $_" }
}

Show-Info "netdata-sos $SosVersion (Windows)"
Show-Info ("service: {0} | process: {1} | api: {2}" -f `
    $(if ($NetdataSvc) { $NetdataSvc.Status } else { 'not installed' }), `
    $(if ($NetdataProc) { "pid $($NetdataProc.Id)" } else { 'not running' }), `
    $(if ($ApiOk) { 'up' } else { 'unreachable' }))

# ============================================================================
# 01-system
# ============================================================================
Show-Info 'collecting: system'
Collect-Cmd '01-system\os-version.txt' 'OS version and build' { Get-CimInstance Win32_OperatingSystem | Format-List Caption, Version, BuildNumber, OSArchitecture, LastBootUpTime, TotalVisibleMemorySize, FreePhysicalMemory } 'Get-CimInstance Win32_OperatingSystem'
Collect-Cmd '01-system\computer-info.txt' 'Hardware, domain role, virtualization' { Get-CimInstance Win32_ComputerSystem | Format-List Manufacturer, Model, SystemType, NumberOfProcessors, NumberOfLogicalProcessors, TotalPhysicalMemory, DomainRole, HypervisorPresent } 'Get-CimInstance Win32_ComputerSystem'
Collect-Cmd '01-system\disk-usage.txt' 'Volume usage' { Get-CimInstance Win32_LogicalDisk | Format-Table DeviceID, @{n='SizeGB';e={[math]::Round($_.Size/1GB,1)}}, @{n='FreeGB';e={[math]::Round($_.FreeSpace/1GB,1)}}, FileSystem -AutoSize } 'Get-CimInstance Win32_LogicalDisk'
Collect-Cmd '01-system\clock-timesync.txt' 'Clock and time sync (drift breaks streaming/cloud)' { Get-Date -Format o; w32tm /query /status 2>&1 } 'Get-Date; w32tm /query /status'
Collect-Cmd '01-system\uptime.txt' 'System uptime' { (Get-Date) - (Get-CimInstance Win32_OperatingSystem).LastBootUpTime | Format-List Days, Hours, Minutes } 'uptime via Win32_OperatingSystem'

# ============================================================================
# 02-install
# ============================================================================
Show-Info 'collecting: install'
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
Collect-Cmd '02-install\install-tree.txt' 'Install dir layout (top levels)' { Get-ChildItem $using:NetdataPrefix -Depth 1 | Format-Table Mode, LastWriteTime, Length, Name -AutoSize } "Get-ChildItem $NetdataPrefix -Depth 1"
if (Test-Path (Join-Path $ConfDir '.install-type')) {
    Collect-File '02-install\install-type.file.txt' 'Install type marker' (Join-Path $ConfDir '.install-type')
}

# ============================================================================
# 03-process
# ============================================================================
Show-Info 'collecting: process'
Collect-Cmd '03-process\netdata-processes.txt' 'Netdata process tree with CPU/memory' { Get-Process | Where-Object { $_.ProcessName -match 'netdata|go.d|ebpf|windows.plugin' } | Format-Table Id, ProcessName, CPU, WorkingSet64, HandleCount, Threads -AutoSize } 'Get-Process (netdata family)'
Collect-Cmd '03-process\service-status.txt' 'Netdata service state and config' { Get-Service Netdata | Format-List *; (Get-CimInstance Win32_Service -Filter "Name='Netdata'") | Format-List StartMode, StartName, PathName, State, ExitCode } 'Get-Service Netdata + Win32_Service'

# ============================================================================
# 04-config
# ============================================================================
Show-Info 'collecting: config'
if ($ApiOk) {
    Collect-Api '04-config\effective-netdata.conf' 'EFFECTIVE running config (merged, annotated) - authoritative over on-disk file' '/netdata.conf'
}
if (Test-Path $ConfDir) {
    Collect-File '04-config\netdata.conf' 'On-disk main config' (Join-Path $ConfDir 'netdata.conf')
    Collect-File '04-config\stream.conf' 'Streaming config (parent/child; api key redacted)' (Join-Path $ConfDir 'stream.conf')
    Collect-File '04-config\claim.conf' 'Cloud claim config (token redacted)' (Join-Path $ConfDir 'claim.conf')
    Collect-File '04-config\go.d.conf' 'go.d orchestrator config' (Join-Path $ConfDir 'go.d.conf')
    # Every user-customized collector/health config gets a neutral archive name.
    # The source path exists only in the private sidecar map; ssl dirs and key
    # material are never collected. Capped at 200 files.
    $script:ConfCollected = 0
    foreach ($sub in @('go.d', 'health.d', 'python.d', 'charts.d', 'statsd.d')) {
        if ($script:ConfCollected -ge 200 -or (Test-Deadline)) { break }
        $subDir = Join-Path $ConfDir $sub
        if (-not (Test-Path $subDir)) { continue }
        $remaining = 200 - $script:ConfCollected
        $configFiles = @(Get-ChildItem $subDir -File -Recurse -Include '*.conf', '*.yml', '*.yaml' -ErrorAction SilentlyContinue |
            Where-Object { $_.FullName -notmatch '\\ssl\\' -and $_.Extension -notin @('.pem', '.key') } |
            Select-Object -First $remaining)
        foreach ($configFile in $configFiles) {
            if (Test-Deadline) { break }
            $relSub = $configFile.FullName.Substring($subDir.Length).TrimStart('\')
            $safeName = Get-SafeDynamicPath "$sub\$relSub" 'custom-config'
            Collect-File "04-config\custom\$safeName" "User-customized $sub config (source path mapped privately; secrets redacted)" $configFile.FullName 256KB "user config mapped to $safeName"
            $script:ConfCollected++
        }
    }
}

# ============================================================================
# 05-logs (Windows: Event Log is the primary destination)
# ============================================================================
Show-Info "collecting: logs (last ${SinceHours}h, Event Log + files)"
Collect-Cmd '05-logs\eventlog-netdata.txt' 'Netdata events from Windows Event Log (NetdataWEL + Application)' {
    $since = (Get-Date).AddHours(-[int]$using:SinceHours)
    foreach ($logName in @('NetdataWEL', 'Application')) {
        Get-WinEvent -FilterHashtable @{ LogName = $logName; StartTime = $since } -MaxEvents 2000 -ErrorAction SilentlyContinue |
            Where-Object { $_.ProviderName -match 'Netdata' } |
            Select-Object TimeCreated, ProviderName, LevelDisplayName, Message |
            Format-List
    }
} "Get-WinEvent NetdataWEL/Application (Netdata providers, last ${SinceHours}h)"
if (Test-Path $LogDir) {
    Get-ChildItem $LogDir -File -Filter '*.log' | ForEach-Object {
        $safeName = Get-SafeDynamicPath $_.Name 'agent-log'
        Collect-File "05-logs\$safeName" 'Agent log file (source name mapped privately)' $_.FullName $LogCap "agent log mapped to $safeName"
    }
}

# ============================================================================
# 06-state
# ============================================================================
Show-Info 'collecting: state'
$StatusFile = Join-Path $LibDir 'status-netdata.json'
if (Test-Path $StatusFile) {
    Collect-File '06-state\status-file.json' 'Daemon status file: LAST EXIT/CRASH RECORD incl. fatal stack trace (read this first for crashes)' $StatusFile
}
if (Test-Path $LibDir) {
    # Names under the state dir may contain live tokens, hostnames, or job ids.
    # Report only aggregate inventory data, never paths or filenames.
    Collect-Cmd '06-state\state-inventory.txt' 'State dir aggregate inventory (all paths and filenames withheld)' {
        $items = Get-ChildItem $using:LibDir -Recurse -ErrorAction SilentlyContinue | Select-Object -First 2000
        $tokenCount = @($items | Where-Object { $_.FullName -match '\\bearer_tokens\\' }).Count
        $files = @($items | Where-Object { -not $_.PSIsContainer -and $_.FullName -notmatch '\\bearer_tokens\\' })
        "directories: $(@($items | Where-Object { $_.PSIsContainer }).Count)"
        "files: $($files.Count)"
        "file bytes: $(($files | Measure-Object Length -Sum).Sum)"
        "[$tokenCount token file(s) - names withheld, they ARE the tokens]"
        $files | Group-Object Extension | ForEach-Object {
            $extension = if ($_.Name) { $_.Name.ToLower() } else { '(none)' }
            "extension ${extension}: $($_.Count) file(s)"
        }
    } 'Get-ChildItem state dir aggregate (all names withheld)'
    $CloudDir = Join-Path $LibDir 'cloud.d'
    if (Test-Path (Join-Path $CloudDir 'claimed_id')) {
        Collect-File '06-state\claimed-id.txt' 'Cloud claim id (safe identifier; token/private.pem never collected)' (Join-Path $CloudDir 'claimed_id')
    }
}
if (Test-Path $CacheDir) {
    Collect-Cmd '06-state\db-disk-usage.txt' 'Database disk usage per tier + sqlite sizes' { Get-ChildItem $using:CacheDir -Directory | ForEach-Object { $s = (Get-ChildItem $_.FullName -Recurse -File | Measure-Object Length -Sum).Sum; '{0}  {1:N1} MB' -f $_.Name, ($s/1MB) }; Get-ChildItem $using:CacheDir -File -Filter '*.db*' | Format-Table Name, Length -AutoSize } "du of $CacheDir"
}

# ============================================================================
# 07-runtime
# ============================================================================
if ($ApiOk) {
    Show-Info 'collecting: runtime (agent is up)'
    Collect-Api '07-runtime\info-v3.json' 'BEST SINGLE CALL: buildinfo, features, cloud status, per-tier retention' '/api/v3/info'
    Collect-Api '07-runtime\info-v1.json' 'Agent info v1' '/api/v1/info'
    Collect-Api '07-runtime\node-instances.json' 'Node instances: children, streaming state, db_size, metric counts' '/api/v2/node_instances'
    Collect-Api '07-runtime\stream-info.json' 'Streaming diagnostics' '/api/v3/stream_info'
    Collect-Api '07-runtime\aclk.json' 'Cloud/ACLK connection state' '/api/v1/aclk'
    Collect-Api '07-runtime\alerts-active.json' 'Currently raised alerts' '/api/v3/alerts?options=active'
    Collect-Api '07-runtime\alerts-all.json' 'All alert instances (summary)' '/api/v1/alarms?all'
    Collect-Api '07-runtime\functions.json' 'Registered functions' '/api/v1/functions'
    Collect-Api '07-runtime\ml-info.json' 'Machine learning status' '/api/v1/ml_info'
    Collect-Api '07-runtime\self-cpu.csv' 'Netdata CPU last 10min (csv)' '/api/v1/data?chart=netdata.server_cpu&after=-600&points=60&format=csv'
    Collect-Api '07-runtime\self-memory.csv' 'Netdata memory last 10min (csv)' '/api/v1/data?chart=netdata.memory&after=-600&points=60&format=csv'
    Collect-Api '07-runtime\self-api-clients.csv' 'Netdata API clients last 10min (csv)' '/api/v1/data?chart=netdata.clients&after=-600&points=60&format=csv'
} else {
    Show-Info 'agent API unreachable - skipping runtime section'
    $marker = Join-Path $Work '07-runtime'
    New-Item -ItemType Directory -Path $marker -Force | Out-Null
    Write-Utf8NoBom (Join-Path $marker 'AGENT-API-UNREACHABLE.txt') "Agent API at 127.0.0.1:$NdPort was unreachable when this bundle was created. The agent process may still have been running; see summary.txt, 05-logs, and 06-state\status-file.json.`n"
    Add-Manifest '07-runtime\AGENT-API-UNREACHABLE.txt' 'file' 'generated' 'Marker: local agent API was unreachable'
}
if (Test-Path $NetdataExe) {
    Collect-Cmd '07-runtime\buildinfo.txt' 'netdata -W buildinfo (verbatim; works with daemon down)' { & $using:NetdataExe -W buildinfo 2>&1 } 'netdata.exe -W buildinfo'
    Collect-CmdRaw '07-runtime\buildinfo.json' 'netdata -W buildinfojson (machine-readable; no header so it parses as JSON)' { & $using:NetdataExe -W buildinfojson 2>&1 } 'netdata.exe -W buildinfojson'
}
if ((Test-Path $NetdataCli) -and $NetdataProc) {
    Collect-CmdRaw '07-runtime\aclk-state.json' 'Cloud connectivity state (netdatacli aclk-state json; no header so it parses as JSON)' { & $using:NetdataCli aclk-state json 2>&1 } 'netdatacli.exe aclk-state json'
}

# ============================================================================
# 08-network
# ============================================================================
Show-Info 'collecting: network'
Collect-Cmd '08-network\listening-sockets.txt' 'Listening sockets (netdata-related)' { Get-NetTCPConnection -State Listen | Where-Object { $_.LocalPort -eq $using:NdPort -or (Get-Process -Id $_.OwningProcess -ErrorAction SilentlyContinue).ProcessName -match 'netdata' } | Format-Table LocalAddress, LocalPort, OwningProcess -AutoSize } 'Get-NetTCPConnection -State Listen (netdata)'
Collect-Cmd '08-network\dns-config.txt' 'DNS resolver config' { Get-DnsClientServerAddress | Where-Object { $_.ServerAddresses } | Format-Table InterfaceAlias, ServerAddresses -AutoSize } 'Get-DnsClientServerAddress'
Collect-Cmd '08-network\proxy-config.txt' 'System proxy configuration' { netsh winhttp show proxy 2>&1; Get-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' | Format-List ProxyEnable, ProxyServer, AutoConfigURL } 'netsh winhttp show proxy + registry'
Collect-Cmd '08-network\cloud-connectivity.txt' 'Reachability of Netdata Cloud (TCP/TLS only, no data sent)' { Test-NetConnection -ComputerName $using:CloudHost -Port 443 -WarningAction SilentlyContinue | Format-List ComputerName, RemotePort, TcpTestSucceeded, PingSucceeded } "Test-NetConnection ${CloudHost}:443"

# ============================================================================
# summary + manifest + README
# ============================================================================
Show-Info 'writing summary and manifest'
$AgentVersion = ''
$infoV1 = Join-Path $Work '07-runtime\info-v1.json'
if (Test-Path $infoV1) {
    try { $AgentVersion = (Get-Content $infoV1 -Raw | ConvertFrom-Json).version } catch { Write-Verbose "info-v1.json unparsable: $_" }
}
$CrashHint = ''
$sfPath = Join-Path $Work '06-state\status-file.json'
if (Test-Path $sfPath) {
    try {
        $sf = Get-Content $sfPath -Raw | ConvertFrom-Json
        if ($sf.agent -and $sf.agent.exit_reason) { $CrashHint = ($sf.agent.exit_reason -join ',') }
    } catch { Write-Verbose "status-file.json unparsable: $_" }
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
Write-Utf8NoBom (Join-Path $Work 'summary.txt') ($summary + "`n")
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
Write-Utf8NoBom (Join-Path $Work 'README.md') ($readme + "`n")
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
Write-Utf8NoBom (Join-Path $Work 'MANIFEST.json') (($manifest | ConvertTo-Json -Depth 4) + "`n")

# ============================================================================
# zip
# ============================================================================
New-Item -ItemType Directory -Path $Output -Force | Out-Null
$ZipPath = Join-Path $Output "$BundleName.zip"
# Build the zip inside the owner-only staging dir, then publish atomically
# without overwrite through Publish-FileNoOverwrite.
$StagingZip = Join-Path $Staging "$BundleName.zip"
Compress-Archive -Path $Work -DestinationPath $StagingZip
try {
    Publish-FileNoOverwrite $StagingZip $ZipPath
} catch {
    # Write-Error would be swallowed by $ErrorActionPreference='SilentlyContinue'
    Write-Host " [!] could not publish $ZipPath ($_)"
    if (-not $KeepStaging) { Remove-Item $Staging -Recurse -Force -ErrorAction SilentlyContinue }
    exit 1
}

$MapPath = Join-Path $Output "$BundleName.pseudonym-map.tsv"
if ($script:PseudoMap.Count -gt 0 -or $script:PathMap.Count -gt 0) {
    # Build inside owner-only staging and publish atomically without overwrite.
    # Same format as the POSIX script: type<TAB>original<TAB>pseudonym.
    $mapLines = @($script:PseudoMap.GetEnumerator() | Sort-Object Key | ForEach-Object {
        $type = 'other'
        if ($_.Value -match '^ip6-\d+$') { $type = 'ip6' }
        elseif ($_.Value -match '^ip-\d+$') { $type = 'ip' }
        elseif ($_.Value -match '^private-host-\d+$') { $type = 'fqdn' }
        elseif ($_.Value -eq 'redacted-host') { $type = 'host' }
        elseif ($_.Value -eq 'redacted-user') { $type = 'user' }
        $original = [regex]::Replace([string]$_.Key, '[\x00-\x1f]', ' ')
        "$type`t$original`t$($_.Value)"
    })
    $mapLines += @($script:PathMap.GetEnumerator() | Sort-Object Key | ForEach-Object {
        $original = [regex]::Replace([string]$_.Key, '[\x00-\x1f]', ' ')
        "path`t$original`t$($_.Value)"
    })
    $StagingMap = Join-Path $Staging "$BundleName.pseudonym-map.tsv"
    [System.IO.File]::WriteAllLines($StagingMap, $mapLines, (New-Object System.Text.UTF8Encoding($false)))
    try {
        Publish-FileNoOverwrite $StagingMap $MapPath
    } catch {
        Write-Host " [!] refusing to overwrite $MapPath; the private map was discarded"
        $MapPath = ''
    }
}

if (-not $KeepStaging) { Remove-Item $Staging -Recurse -Force }

Write-Host ''
Show-Info ("done in {0}s" -f [int]((Get-Date) - $StartTime).TotalSeconds)
Show-Info "bundle:  $ZipPath"
if ($MapPath -and (Test-Path $MapPath)) {
    Show-Info "pseudonym map (KEEP PRIVATE, do not send): $MapPath"
}
Show-Info 'attach the bundle to your support ticket.'
