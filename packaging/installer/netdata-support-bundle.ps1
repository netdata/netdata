# SPDX-License-Identifier: GPL-3.0-or-later
#
# netdata-support-bundle - collect a sanitized diagnostic bundle for Netdata support tickets.
# Windows counterpart of netdata-support-bundle.sh. Same bundle layout, same MANIFEST
# schema (netdata-support-bundle/v1), same sanitization rules.
#
# What is collected and WHY each item is included is documented in
# packaging/installer/SUPPORT-BUNDLE.md - read it before adding or changing
# a collection item, and keep it in sync with this script.
#
# Usage (run in an elevated PowerShell):
#   powershell -ExecutionPolicy Bypass -File netdata-support-bundle.ps1 [-Output DIR]
#              [-SinceHours 24] [-NoObfuscate] [-KeepStaging] [-SelfTest]
#
# Requires Windows PowerShell 5.1+ (ships with Windows Server 2016+) or
# PowerShell 7. Output: netdata-support-bundle-<timestamp>.zip

[CmdletBinding()]
param(
    [string]$Output = $(if ($env:TEMP) { $env:TEMP } elseif ($env:TMPDIR) { $env:TMPDIR } else { [System.IO.Path]::GetTempPath() }),
    [int]$SinceHours = 24,
    [int]$TimeoutSeconds = 10,
    [switch]$NoObfuscate,
    [switch]$KeepStaging,
    [switch]$SelfTest,
    [switch]$Version
)

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'SilentlyContinue'

$ToolVersion = '1.0.0'
if ($Version) { Write-Output "netdata-support-bundle $ToolVersion"; exit 0 }

$Obfuscate = -not $NoObfuscate
$LogCap = 5MB
$FileCap = 1MB
$GlobalDeadline = (Get-Date).AddSeconds(240)

# --- run at lowest priority: never compete with real workloads ---------------
try {
    $proc = Get-Process -Id $PID
    $proc.PriorityClass = [System.Diagnostics.ProcessPriorityClass]::Idle
} catch { Write-Verbose "could not lower process priority: $_" }

$StartTime = Get-Date
$Stamp = (Get-Date).ToUniversalTime().ToString('yyyyMMdd-HHmmss')
$BundleName = "netdata-support-bundle-$Stamp-$PID"
$TempBase = if ($env:TEMP) { $env:TEMP } elseif ($env:TMPDIR) { $env:TMPDIR } else { [System.IO.Path]::GetTempPath() }
$Staging = Join-Path $TempBase ("netdata-support-bundle-staging-" + [System.IO.Path]::GetRandomFileName())
$Work = Join-Path $Staging $BundleName
New-Item -ItemType Directory -Path $Work -Force | Out-Null
$script:ManifestRows = New-Object System.Collections.ArrayList
$script:PseudoMap = @{}   # original -> pseudonym
$script:IpCount = 0
$script:Ip6Count = 0
$script:FqdnCount = 0
$script:SeededHosts = @() # child/mirrored hostnames pre-seeded from the local API

function Show-Info([string]$msg) { Write-Host " [*] $msg" }

# PS 5.1 Set-Content writes UTF-16LE by default; every bundle file must be UTF-8
$script:Utf8NoBom = New-Object System.Text.UTF8Encoding($false)
function Write-Utf8([string]$path, [string]$text) {
    [System.IO.File]::WriteAllText($path, $text, $script:Utf8NoBom)
}

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
$HostFqdn = try { [System.Net.Dns]::GetHostEntry('').HostName } catch { Write-Verbose "no FQDN: $_"; '' }
$RunUser = $env:USERNAME
if ($RunUser -in @('SYSTEM', 'Administrator') -or -not $RunUser -or $RunUser.Length -lt 3) { $RunUser = '' }

function Test-SecretKey([string]$key) {
    $k = ($key -replace '[-_]', ' ').ToLower().Trim(' ', '#', "`t")
    foreach ($w in $SecretKeyWords) { if ($k.Contains($w)) { return $true } }
    return $false
}

function Test-DiagnosticKey([string]$key) {
    # keys that DESCRIBE secrets rather than hold them ("bearer token protection",
    # "netdata management api key file") end in a diagnostic noun; their values are
    # settings, not credentials. Exemption is KEY-based on purpose: a value like
    # "false" or "/root/x" attached to a real secret key must still be redacted.
    $k = ($key -replace '[-_]', ' ').ToLower().Trim(' ', '#', "`t")
    foreach ($w in @('file', 'path', 'dir', 'directory', 'protection', 'support', 'mode',
                     'level', 'port', 'timeout', 'cookies', 'secure', 'log', 'size', 'options')) {
        if ($k -eq $w -or $k.EndsWith(" $w")) { return $true }
    }
    return $false
}

function Get-Pseudonym([string]$orig, [string]$prefix) {
    if (-not $script:PseudoMap.ContainsKey($orig)) {
        # cap: past 4096 mappings, use a constant non-correlating placeholder
        if ($script:PseudoMap.Count -ge 4096) { return "redacted-$prefix" }
        if ($prefix -eq 'ip') { $script:IpCount++; $script:PseudoMap[$orig] = "ip-$($script:IpCount)" }
        elseif ($prefix -eq 'ip6') { $script:Ip6Count++; $script:PseudoMap[$orig] = "ip6-$($script:Ip6Count)" }
        elseif ($prefix -eq 'fqdn') { $script:FqdnCount++; $script:PseudoMap[$orig] = "private-host-$($script:FqdnCount)" }
        else { $script:PseudoMap[$orig] = 'redacted-host' }
    }
    return $script:PseudoMap[$orig]
}

function Invoke-RedactSecretLine([string]$line) {
    # pass 1 (always on): credential redaction
    # stream.conf-style [<uuid>] section headers are api keys
    if ($line -match '^\s*\[[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\]\s*$') {
        return '[REDACTED-KEY-SECTION]'
    }
    # ini/yaml/env key = value | key: value
    # (JSON-shaped lines are owned by the json rule below, which preserves quoting)
    # only plausible config keys: short, no sentence/shell punctuation, no path
    # separators (command lines with paths belong to the argv rule below)
    if ($line -notmatch '^\s*"' -and $line -match '^([^=:]{1,64})([=:])(.+)$' -and $Matches[1] -notmatch '["`;|()/]') {
        if ((Test-SecretKey $Matches[1]) -and $Matches[3].Trim() -ne '' -and -not (Test-DiagnosticKey $Matches[1])) {
            $line = $Matches[1] + $Matches[2] + ' [REDACTED]'
        }
    }
    # json "key": "value" pairs (possibly several per line)
    $line = [regex]::Replace($line, '"([^"]+)"\s*:\s*"([^"]*)"', {
        param($m)
        if ((Test-SecretKey $m.Groups[1].Value) -and -not (Test-DiagnosticKey $m.Groups[1].Value)) { '"' + $m.Groups[1].Value + '": "[REDACTED]"' }
        else { $m.Value }
    })
    # scalar (unquoted) JSON values under secret keys: "key": 12345 / true / null
    $line = [regex]::Replace($line, '"([^"]+)"\s*:\s*(-?\d[\d.eE+-]*|true|false|null)', {
        param($m)
        if ((Test-SecretKey $m.Groups[1].Value) -and -not (Test-DiagnosticKey $m.Groups[1].Value)) { '"' + $m.Groups[1].Value + '": "[REDACTED]"' }
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
    # incl. two-word keys ("api key = X"); diagnostic-noun keys are kept
    $line = [regex]::Replace($line, '(?i)(([\w.-]*(token|password|passwd|secret|apikey|api_key|community|bearer)|(api|license|auth|access) key|proxy (user|pass|password)) ?[=:] ?)([^&"\s\[]+)', {
        param($m)
        if (Test-DiagnosticKey $m.Groups[2].Value) { $m.Value } else { $m.Groups[1].Value + '[REDACTED]' }
    })
    # NOTE: multiline PEM private-key blocks are handled by Invoke-SanitizeFile,
    # which tracks BEGIN/END state across lines and withholds the whole block.
    return $line
}

function Invoke-RedactDestinationLine([string]$line) {
    # stream.conf destination hosts are user infrastructure regardless of TLD.
    # Token syntax: [PROTOCOL:]HOST[%IFACE][:PORT][:SSL]. Runs BEFORE the IP
    # rules (pure-IP tokens are skipped and left to them).
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
    return $head + ' ' + ($rebuilt -join ' ')
}

function Invoke-ReplaceAddressText([string]$line) {
    # emails, MACs, IPv4, IPv6
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
            $c.EndsWith(':') -or
            ($nc -lt 6 -and $c -notmatch '[A-Fa-f]' -and $c -notmatch '::')) { return $c }
        return (Get-Pseudonym $c 'ip6')
    })
    return $line
}

function Invoke-ObfuscatePiiLine([string]$line) {
    # pass 2 (default on): PII pseudonymization
    if ($line -match '^[\s#]*(proxy )?destination\s*=') { $line = Invoke-RedactDestinationLine $line }
    $line = Invoke-ReplaceAddressText $line
    # clearly-private FQDNs
    $line = [regex]::Replace($line, '\b[A-Za-z0-9][A-Za-z0-9.-]*\.(internal|local|lan|corp|intranet|localdomain)\b', {
        param($m); return (Get-Pseudonym $m.Value 'fqdn')
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
    if ($HostFqdn -and $HostFqdn.Length -ge 4) { $line = $line.Replace($HostFqdn, (Get-Pseudonym $HostFqdn 'host')) }
    if ($HostShort -and $HostShort.Length -ge 4 -and $HostShort -ne $HostFqdn) {
        $line = $line -ireplace [regex]::Escape($HostShort), (Get-Pseudonym $HostShort 'host')
    }
    if ($RunUser) {
        if (-not $script:PseudoMap.ContainsKey($RunUser)) { $script:PseudoMap[$RunUser] = 'redacted-user' }
        $line = $line -ireplace [regex]::Escape($RunUser), 'redacted-user'
    }
    return $line
}

function Invoke-SanitizeLine([string]$line) {
    $line = Invoke-RedactSecretLine $line
    if ($Obfuscate) { $line = Invoke-ObfuscatePiiLine $line }
    return $line
}

function Invoke-SanitizeFile([string]$path) {
    if (-not (Test-Path $path)) { return }
    # binary/UTF-16 input would make line-based redaction byte-unsafe: withhold
    $fs = [System.IO.File]::OpenRead($path)
    try {
        $probe = New-Object byte[] ([Math]::Min(1MB, $fs.Length))
        $null = $fs.Read($probe, 0, $probe.Length)
    } finally { $fs.Close() }
    if ($probe -contains 0) {
        Write-Utf8 $path '[content withheld: file contains NUL bytes (binary or UTF-16?)]'
        return
    }
    $out = New-Object System.Collections.ArrayList
    $inPem = $false
    $inYaml = $false
    $yamlIndent = 0
    foreach ($l in [System.IO.File]::ReadAllLines($path)) {
        # multiline PEM private keys: withhold the WHOLE block, BEGIN through END;
        # if the file ends before END, everything after BEGIN stays withheld (fail closed)
        if ($inPem) {
            if ($l -match '-----END [A-Z0-9 ]*PRIVATE KEY') { $inPem = $false }
            continue
        }
        # YAML block scalars under secret keys (password: | ...) span lines:
        # withhold until indentation returns to the key level or shallower
        if ($inYaml) {
            if ($l -match '^\s*$') { continue }
            if (([regex]::Match($l, '^ *')).Length -gt $yamlIndent) { continue }
            $inYaml = $false
        }
        if ($l -match '^(\s*)([\w. -]+):\s*[|>][+-]?\s*$') {
            $yInd = $Matches[1]; $yKey = $Matches[2]
            if ((Test-SecretKey $yKey) -and -not (Test-DiagnosticKey $yKey)) {
                [void]$out.Add($yInd + $yKey + ': [REDACTED BLOCK]')
                $inYaml = $true
                $yamlIndent = $yInd.Length
                continue
            }
        }
        if ($l -match '-----BEGIN [A-Z0-9 ]*PRIVATE KEY') {
            $inPem = $true
            [void]$out.Add('[REDACTED PRIVATE KEY BLOCK]')
            continue
        }
        [void]$out.Add((Invoke-SanitizeLine $l))
    }
    [System.IO.File]::WriteAllLines($path, $out, $script:Utf8NoBom)
}

# --- sanitizer self-test (-SelfTest) ----------------------------------------------
# Table-driven regression suite for the redaction rules. Run it after ANY change
# to the sanitizer, and keep it in parity with the POSIX script's --selftest.
if ($SelfTest) {
    $HostShort = 'testhost99'
    $HostFqdn = 'testhost99.example.com'
    $RunUser = 'testuser9'
    $vectors = @(
        @{ in = 'api key = SENTINEL-1';                                            mustNot = @('SENTINEL');            must = @('[REDACTED]') }
        # assembled at runtime so secret scanners do not flag the source
        @{ in = ('    Authorization: ' + ('Bea' + 'rer') + ' SENTINEL-2abc123');    mustNot = @('SENTINEL');            must = @('[REDACTED]') }
        @{ in = 'password: SENTINEL-3';                                            mustNot = @('SENTINEL');            must = @('[REDACTED]') }
        @{ in = '"claim_token": "SENTINEL-4"';                                     mustNot = @('SENTINEL');            must = @('[REDACTED]') }
        @{ in = 'url: https://admin:SENTINEL-5@app.example.com/x';                 mustNot = @('SENTINEL');            must = @('[REDACTED]@') }
        @{ in = 'dsn: user:SENTINEL-6@tcp(10.1.2.3:3306)/db';                      mustNot = @('SENTINEL');            must = @('[REDACTED]') }
        @{ in = 'TELEGRAM_BOT_TOKEN="SENTINEL-7"';                                 mustNot = @('SENTINEL');            must = @('[REDACTED]') }
        @{ in = 'DEFAULT_RECIPIENT_SLACK="SENTINEL-8"';                            mustNot = @('SENTINEL');            must = @('[REDACTED]') }
        @{ in = '[11111111-2222-3333-4444-555555555555]';                          mustNot = @('11111111');            must = @('[REDACTED-KEY-SECTION]') }
        @{ in = 'destination = parent.example.internal:19999';                     mustNot = @('parent.example');      must = @('private-host') }
        @{ in = 'server at 10.1.2.3 talked to 192.168.5.7 then 10.1.2.3 again';    mustNot = @('10.1.2.3', '192.168'); must = @('ip-') }
        @{ in = 'admin email is ops@customer-corp.com on host testhost99';         mustNot = @('customer-corp', 'testhost99'); must = @('[EMAIL]', 'redacted-host') }
        @{ in = 'GET /api/v1/data?chart=x&token=SENTINEL-9&after=-60';             mustNot = @('SENTINEL');            must = @('[REDACTED]', 'after=-60') }
        @{ in = 'netdata 1234 /usr/sbin/netdata-claim.sh -token=SENTINEL-10 -rooms=abc'; mustNot = @('SENTINEL');      must = @('[REDACTED]', '-rooms=abc') }
        @{ in = 'Environment: NETDATA_CLAIM_TOKEN=SENTINEL-11 PATH=/usr/bin';      mustNot = @('SENTINEL');            must = @('[REDACTED]') }
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
        @{ in = '# destination = old-parent.example.org:19999';                    mustNot = @('old-parent.example');  must = @('private-host', ':19999') }
        @{ in = 'connect user:SENTINEL-13@unix(/run/x)/db ok';                     mustNot = @('SENTINEL');            must = @('[REDACTED]@unix(/run/x)/db', 'ok') }
        @{ in = '/etc/netdata/claim_token: SENTINEL-14';                           mustNot = @('SENTINEL');            must = @('[REDACTED]') }
        @{ in = 'destination = [2001:db8::77]:19999 unix:/run/nd.sock';            mustNot = @('2001:db8::77');        must = @('ip6-', ']:19999', 'unix:/run/nd.sock') }
        @{ in = 'password: q';                                                     mustNot = @(': q');                 must = @('[REDACTED]') }
        @{ in = '"api_token": 731942';                                             mustNot = @('731942');              must = @('[REDACTED]') }
    )
    $fails = 0
    foreach ($v in $vectors) {
        $result = Invoke-SanitizeLine $v.in
        $ok = $true
        foreach ($p in $v.mustNot) { if ($result.IndexOf($p, [System.StringComparison]::OrdinalIgnoreCase) -ge 0) { $ok = $false } }
        foreach ($p in $v.must) { if ($result.IndexOf($p, [System.StringComparison]::OrdinalIgnoreCase) -lt 0) { $ok = $false } }
        if ($ok) { Write-Output "ok:   $result" } else { Write-Output "FAIL: '$($v.in)' -> '$result'"; $fails++ }
    }
    # multiline PEM block must be withheld end-to-end by the file-level sanitizer
    $pemTmp = Join-Path $Staging 'pem-test.txt'
    @('before line',
      '-----BEGIN RSA PRIVATE KEY-----',
      'MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQ',
      'aGVsbG8gd29ybGQgdGhpcyBpcyBrZXkgbWF0ZXJpYWw=',
      '-----END RSA PRIVATE KEY-----',
      'after line') | Out-String | ForEach-Object { Write-Utf8 $pemTmp $_ }
    Invoke-SanitizeFile $pemTmp
    $pem = Get-Content $pemTmp -Raw
    if ($pem -match 'MIIEvQ' -or $pem -match 'aGVsbG8' -or $pem -notmatch '\[REDACTED PRIVATE KEY BLOCK\]' -or
        $pem -notmatch 'before line' -or $pem -notmatch 'after line') {
        Write-Output 'FAIL: PEM block not fully withheld'; $fails++
    } else { Write-Output 'ok:   PEM block withheld (begin/body/end gone, surrounding lines kept)' }
    # YAML block scalar under a secret key must be withheld end-to-end
    $yamlTmp = Join-Path $Staging 'yaml-test.txt'
    @('jobs:',
      '  - name: x',
      '    private_key: |',
      '      SENTINEL-YAML-LINE1',
      '      SENTINEL-YAML-LINE2',
      '    after: ok') | Out-String | ForEach-Object { Write-Utf8 $yamlTmp $_ }
    Invoke-SanitizeFile $yamlTmp
    $yaml = Get-Content $yamlTmp -Raw
    if ($yaml -match 'SENTINEL-YAML' -or $yaml -notmatch '\[REDACTED BLOCK\]' -or $yaml -notmatch 'after: ok') {
        Write-Output 'FAIL: YAML block scalar not withheld correctly'; $fails++
    } else { Write-Output 'ok:   YAML block scalar withheld (body gone, dedented content kept)' }
    Remove-Item $Staging -Recurse -Force
    if ($fails -gt 0) { Write-Output "SELF TEST: $fails FAILURES"; exit 1 }
    Write-Output 'SELF TEST: ALL PASS'
    exit 0
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
function Save-Cmd([string]$rel, [string]$title, [scriptblock]$cmd, [string]$originText) {
    if (Test-Deadline) { return }
    $full = Join-Path $Work $rel
    New-Item -ItemType Directory -Path (Split-Path $full) -Force | Out-Null
    $header = "# netdata-support-bundle v$ToolVersion | command: $originText | captured: $((Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ'))"
    try {
        $job = Start-Job -ScriptBlock $cmd
        $done = Wait-Job $job -Timeout $TimeoutSeconds
        if ($done) { $result = Receive-Job $job 2>&1 | Select-Object -First 50000 | Out-String } else { $result = "TIMEOUT after ${TimeoutSeconds}s" }
        Remove-Job $job -Force
    } catch { $result = "ERROR: $_" }
    if ($result.Length -gt 2MB) {
        # truncate at a LINE boundary so a secret cannot straddle the cut
        $cut = $result.LastIndexOf("`n", 2MB)
        if ($cut -lt 0) { $result = '[content withheld: output exceeds the cap without a line break]' }
        else { $result = $result.Substring(0, $cut + 1) + '### TRUNCATED at 2MB (line-aligned) ###' }
    }
    Write-Utf8 $full ($header + "`r`n" + $result)
    Invoke-SanitizeFile $full
    Add-Manifest $rel 'cmd' $originText $title
}

function Save-CmdRaw([string]$rel, [string]$title, [scriptblock]$cmd, [string]$originText) {
    # like Save-Cmd but with NO header/trailer, for commands whose output must
    # stay parseable (JSON). Provenance lives in MANIFEST.json only. The file is
    # removed if the command produced nothing.
    if (Test-Deadline) { return }
    $full = Join-Path $Work $rel
    New-Item -ItemType Directory -Path (Split-Path $full) -Force | Out-Null
    $result = ''
    try {
        $job = Start-Job -ScriptBlock $cmd
        $done = Wait-Job $job -Timeout $TimeoutSeconds
        if ($done) { $result = Receive-Job $job 2>&1 | Select-Object -First 50000 | Out-String }
        Remove-Job $job -Force
    } catch { Write-Verbose "collector failed: $_" }
    if ($result -and $result.Trim().Length -gt 0) {
        # a truncated JSON document is worse than none: fail closed
        if ($result.Length -gt 2MB) { $result = '{"error":"output exceeded the cap and was withheld"}' }
        Write-Utf8 $full $result
        Invoke-SanitizeFile $full
        Add-Manifest $rel 'cmd' $originText $title
    } elseif (Test-Path $full) {
        Remove-Item $full -Force
    }
}

function Save-File([string]$rel, [string]$title, [string]$src, [long]$cap = 0) {
    if (Test-Deadline) { return }
    if ($cap -eq 0) { $cap = $FileCap }
    if (-not (Test-Path $src -PathType Leaf)) { return }
    $full = Join-Path $Work $rel
    New-Item -ItemType Directory -Path (Split-Path $full) -Force | Out-Null
    $item = Get-Item $src -Force
    if ($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) {
        # reparse points can silently retarget: fail closed rather than follow
        Write-Utf8 $full '[content withheld: source is a symlink/reparse point]'
        Add-Manifest $rel 'file' "$src (reparse point - withheld)" $title
        return
    }
    $size = $item.Length
    $origin = $src
    if ($size -gt $cap) {
        # keep the tail at a LINE boundary (drop the first, possibly partial,
        # line) so a secret can never straddle the cut and dodge the
        # line-based sanitizer; a tail with no line break at all is withheld
        $fs = [System.IO.File]::OpenRead($src)
        try {
            $slack = [Math]::Min($size, $cap + 4096)
            $fs.Seek(-$slack, [System.IO.SeekOrigin]::End) | Out-Null
            $buf = New-Object byte[] $slack
            $read = $fs.Read($buf, 0, $slack)
            $text = $script:Utf8NoBom.GetString($buf, 0, $read)
            $nl = $text.IndexOf("`n")
            if ($nl -lt 0) {
                Write-Utf8 $full '[content withheld: file tail exceeds the cap without a line break]'
            } else {
                Write-Utf8 $full $text.Substring($nl + 1)
            }
        } finally { $fs.Close() }
        $origin = "$src (last ~$cap of $size bytes, line-aligned)"
    } else {
        try { Copy-Item $src $full -Force -ErrorAction Stop }
        catch {
            Write-Utf8 $full "[content withheld: could not read source ($_)]"
            $origin = "$src (unreadable - withheld)"
        }
    }
    Invoke-SanitizeFile $full
    Add-Manifest $rel 'file' $origin $title
}

$NdPort = 19999
$CloudHost = 'app.netdata.cloud'   # reachability probe target
function Save-Api([string]$rel, [string]$title, [string]$urlPath) {
    if (Test-Deadline) { return }
    $full = Join-Path $Work $rel
    New-Item -ItemType Directory -Path (Split-Path $full) -Force | Out-Null
    try {
        $resp = Invoke-WebRequest -Uri "http://127.0.0.1:$NdPort$urlPath" -UseBasicParsing -TimeoutSec $TimeoutSeconds
        $content = $resp.Content
        # a JSON body cut at any point is malformed, so overflow is withheld whole
        if ($content.Length -gt 2MB) { $content = '{"error":"response exceeded the cap and was withheld"}' }
        Write-Utf8 $full $content
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
} catch { Write-Verbose "agent API not reachable: $_" }

# pre-seed pseudonyms for child/mirrored hostnames, so a parent's children are
# obfuscated consistently in every file (node_instances, stream configs, logs)
if ($ApiOk -and $Obfuscate) {
    try {
        $names = @()
        $ni = (Invoke-WebRequest -Uri "http://127.0.0.1:$NdPort/api/v2/node_instances" -UseBasicParsing -TimeoutSec $TimeoutSeconds).Content | ConvertFrom-Json
        if ($ni -and $ni.PSObject.Properties['nodes']) { $names += @($ni.nodes | ForEach-Object { $_.nm }) }
        $v1 = (Invoke-WebRequest -Uri "http://127.0.0.1:$NdPort/api/v1/info" -UseBasicParsing -TimeoutSec $TimeoutSeconds).Content | ConvertFrom-Json
        if ($v1 -and $v1.PSObject.Properties['mirrored_hosts']) { $names += @($v1.mirrored_hosts) }
        foreach ($n in ($names | Sort-Object -Unique)) {
            if (-not $n -or $n.Length -lt 4 -or $n -eq 'localhost' -or $n -eq $HostShort -or $n -eq $HostFqdn) { continue }
            if (-not $script:PseudoMap.ContainsKey($n)) {
                $script:FqdnCount++
                $script:PseudoMap[$n] = "private-host-$($script:FqdnCount)"
            }
        }
        # longest first so overlapping names (host, host-2) replace correctly
        $script:SeededHosts = @($script:PseudoMap.Keys |
            Where-Object { $script:PseudoMap[$_] -like 'private-host*' } |
            Sort-Object { $_.Length } -Descending)
    } catch { Write-Verbose "child hostname pre-seed failed: $_" }
}

Show-Info "netdata-support-bundle $ToolVersion (Windows)"
Show-Info ("service: {0} | process: {1} | api: {2}" -f `
    $(if ($NetdataSvc) { $NetdataSvc.Status } else { 'not installed' }), `
    $(if ($NetdataProc) { "pid $($NetdataProc.Id)" } else { 'not running' }), `
    $(if ($ApiOk) { 'up' } else { 'unreachable' }))

# ============================================================================
# 01-system
# ============================================================================
Show-Info 'collecting: system'
Save-Cmd '01-system\os-version.txt' 'OS version and build' { Get-CimInstance Win32_OperatingSystem | Format-List Caption, Version, BuildNumber, OSArchitecture, LastBootUpTime, TotalVisibleMemorySize, FreePhysicalMemory } 'Get-CimInstance Win32_OperatingSystem'
Save-Cmd '01-system\computer-info.txt' 'Hardware, domain role, virtualization' { Get-CimInstance Win32_ComputerSystem | Format-List Manufacturer, Model, SystemType, NumberOfProcessors, NumberOfLogicalProcessors, TotalPhysicalMemory, DomainRole, HypervisorPresent } 'Get-CimInstance Win32_ComputerSystem'
Save-Cmd '01-system\disk-usage.txt' 'Volume usage' { Get-CimInstance Win32_LogicalDisk | Format-Table DeviceID, @{n='SizeGB';e={[math]::Round($_.Size/1GB,1)}}, @{n='FreeGB';e={[math]::Round($_.FreeSpace/1GB,1)}}, FileSystem -AutoSize } 'Get-CimInstance Win32_LogicalDisk'
Save-Cmd '01-system\clock-timesync.txt' 'Clock and time sync (drift breaks streaming/cloud)' { Get-Date -Format o; w32tm /query /status 2>&1 } 'Get-Date; w32tm /query /status'
Save-Cmd '01-system\uptime.txt' 'System uptime' { (Get-Date) - (Get-CimInstance Win32_OperatingSystem).LastBootUpTime | Format-List Days, Hours, Minutes } 'uptime via Win32_OperatingSystem'

# ============================================================================
# 02-install
# ============================================================================
Show-Info 'collecting: install'
# NOTE: never use Win32_Product here - querying it triggers MSI reconfiguration
# of every installed package. The uninstall registry keys are the safe source.
Save-Cmd '02-install\msi-info.txt' 'Installed Netdata MSI package info (from uninstall registry)' {
    foreach ($root in @('HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall',
                        'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall')) {
        Get-ItemProperty "$root\*" -ErrorAction SilentlyContinue |
            Where-Object { $_.DisplayName -like '*Netdata*' } |
            Format-List DisplayName, DisplayVersion, InstallDate, InstallLocation, Publisher
    }
} 'registry uninstall keys (Netdata)'
Save-Cmd '02-install\install-tree.txt' 'Install dir layout (top levels)' { Get-ChildItem $using:NetdataPrefix -Depth 1 | Format-Table Mode, LastWriteTime, Length, Name -AutoSize } "Get-ChildItem $NetdataPrefix -Depth 1"
if (Test-Path (Join-Path $ConfDir '.install-type')) {
    Save-File '02-install\install-type.file.txt' 'Install type marker' (Join-Path $ConfDir '.install-type')
}

# ============================================================================
# 03-process
# ============================================================================
Show-Info 'collecting: process'
Save-Cmd '03-process\netdata-processes.txt' 'Netdata process tree with CPU/memory' { Get-Process | Where-Object { $_.ProcessName -match 'netdata|go.d|ebpf|windows.plugin' } | Format-Table Id, ProcessName, CPU, WorkingSet64, HandleCount, Threads -AutoSize } 'Get-Process (netdata family)'
Save-Cmd '03-process\service-status.txt' 'Netdata service state and config' { Get-Service Netdata | Format-List *; (Get-CimInstance Win32_Service -Filter "Name='Netdata'") | Format-List StartMode, StartName, PathName, State, ExitCode } 'Get-Service Netdata + Win32_Service'

# ============================================================================
# 04-config
# ============================================================================
Show-Info 'collecting: config'
if ($ApiOk) {
    Save-Api '04-config\effective-netdata.conf' 'EFFECTIVE running config (merged, annotated) - authoritative over on-disk file' '/netdata.conf'
}
if (Test-Path $ConfDir) {
    Save-Cmd '04-config\config-tree.txt' 'User config dir tree (files here = user-customized; ssl and key material excluded)' { Get-ChildItem $using:ConfDir -Recurse -ErrorAction SilentlyContinue | Where-Object { $_.FullName -notmatch '[\\/]ssl([\\/]|$)' -and $_.Extension -notin @('.pem', '.key') } | Select-Object -First 2000 | Format-Table Mode, LastWriteTime, Length, FullName -AutoSize } "Get-ChildItem $ConfDir -Recurse (ssl/key material excluded)"
    Save-File '04-config\netdata.conf' 'On-disk main config' (Join-Path $ConfDir 'netdata.conf')
    Save-File '04-config\stream.conf' 'Streaming config (parent/child; api key redacted)' (Join-Path $ConfDir 'stream.conf')
    Save-File '04-config\claim.conf' 'Cloud claim config (token redacted)' (Join-Path $ConfDir 'claim.conf')
    Save-File '04-config\go.d.conf' 'go.d orchestrator config' (Join-Path $ConfDir 'go.d.conf')
    # every user-customized collector/health config, keeping subdirectory
    # structure (go.d\sd\*.conf must not collide with go.d\*.conf); ssl dirs
    # and key material are never collected. Capped at 200 files.
    $script:ConfCollected = 0
    foreach ($sub in @('go.d', 'health.d', 'python.d', 'charts.d', 'statsd.d')) {
        $subDir = Join-Path $ConfDir $sub
        if (-not (Test-Path $subDir)) { continue }
        Get-ChildItem $subDir -File -Recurse -Include '*.conf', '*.yml', '*.yaml' -ErrorAction SilentlyContinue |
            Where-Object { $_.FullName -notmatch '\\ssl\\' -and $_.Extension -notin @('.pem', '.key') } |
            ForEach-Object {
                if ($script:ConfCollected -ge 200) { return }
                $relSub = $_.FullName.Substring($subDir.Length).TrimStart('\')
                Save-File "04-config\$sub\$relSub" "User-customized $sub config (secrets redacted)" $_.FullName 256KB
                $script:ConfCollected++
            }
    }
}

# ============================================================================
# 05-logs (Windows: Event Log is the primary destination)
# ============================================================================
Show-Info "collecting: logs (last ${SinceHours}h, Event Log + files)"
Save-Cmd '05-logs\eventlog-netdata.txt' 'Netdata events from Windows Event Log (NetdataWEL + Application)' {
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
        Save-File "05-logs\$($_.Name)" "Agent log file: $($_.Name)" $_.FullName $LogCap
    }
}

# ============================================================================
# 06-state
# ============================================================================
Show-Info 'collecting: state'
$StatusFile = Join-Path $LibDir 'status-netdata.json'
if (Test-Path $StatusFile) {
    Save-File '06-state\status-file.json' 'Daemon status file: LAST EXIT/CRASH RECORD incl. fatal stack trace (read this first for crashes)' $StatusFile
}
if (Test-Path $LibDir) {
    # bearer_tokens FILENAMES are live API tokens - list only their count
    Save-Cmd '06-state\state-tree.txt' 'State dir listing (bearer token filenames withheld - they are live tokens)' {
        $items = Get-ChildItem $using:LibDir -Recurse -ErrorAction SilentlyContinue | Select-Object -First 2000
        $tokenCount = @($items | Where-Object { $_.FullName -match '\\bearer_tokens\\' }).Count
        $items | Where-Object { $_.FullName -notmatch '\\bearer_tokens\\' } |
            Format-Table Mode, LastWriteTime, Length, FullName -AutoSize
        "[$tokenCount token file(s) - names withheld, they ARE the tokens]"
    } "Get-ChildItem $LibDir -Recurse (bearer_tokens contents withheld)"
    $CloudDir = Join-Path $LibDir 'cloud.d'
    if (Test-Path (Join-Path $CloudDir 'claimed_id')) {
        Save-File '06-state\claimed-id.txt' 'Cloud claim id (safe identifier; token/private.pem never collected)' (Join-Path $CloudDir 'claimed_id')
    }
}
if (Test-Path $CacheDir) {
    Save-Cmd '06-state\db-disk-usage.txt' 'Database disk usage per tier + sqlite sizes' { Get-ChildItem $using:CacheDir -Directory | ForEach-Object { $s = (Get-ChildItem $_.FullName -Recurse -File | Measure-Object Length -Sum).Sum; '{0}  {1:N1} MB' -f $_.Name, ($s/1MB) }; Get-ChildItem $using:CacheDir -File -Filter '*.db*' | Format-Table Name, Length -AutoSize } "du of $CacheDir"
}

# ============================================================================
# 07-runtime
# ============================================================================
if ($ApiOk) {
    Show-Info 'collecting: runtime (agent is up)'
    Save-Api '07-runtime\info-v3.json' 'BEST SINGLE CALL: buildinfo, features, cloud status, per-tier retention' '/api/v3/info'
    Save-Api '07-runtime\info-v1.json' 'Agent info v1' '/api/v1/info'
    Save-Api '07-runtime\node-instances.json' 'Node instances: children, streaming state, db_size, metric counts' '/api/v2/node_instances'
    Save-Api '07-runtime\stream-info.json' 'Streaming diagnostics' '/api/v3/stream_info'
    Save-Api '07-runtime\aclk.json' 'Cloud/ACLK connection state' '/api/v1/aclk'
    Save-Api '07-runtime\alerts-active.json' 'Currently raised alerts' '/api/v3/alerts?options=active'
    Save-Api '07-runtime\alerts-all.json' 'All alert instances (summary)' '/api/v1/alarms?all'
    Save-Api '07-runtime\functions.json' 'Registered functions' '/api/v1/functions'
    Save-Api '07-runtime\ml-info.json' 'Machine learning status' '/api/v1/ml_info'
    Save-Api '07-runtime\self-cpu.csv' 'Netdata CPU last 10min (csv)' '/api/v1/data?chart=netdata.server_cpu&after=-600&points=60&format=csv'
    Save-Api '07-runtime\self-memory.csv' 'Netdata memory last 10min (csv)' '/api/v1/data?chart=netdata.memory&after=-600&points=60&format=csv'
    Save-Api '07-runtime\self-api-clients.csv' 'Netdata API clients last 10min (csv)' '/api/v1/data?chart=netdata.clients&after=-600&points=60&format=csv'
} else {
    Show-Info 'agent API unreachable - skipping runtime section'
    $marker = Join-Path $Work '07-runtime'
    New-Item -ItemType Directory -Path $marker -Force | Out-Null
    Write-Utf8 (Join-Path $marker 'AGENT-WAS-DOWN.txt') "Agent API at 127.0.0.1:$NdPort was unreachable when this bundle was created. See 05-logs and 06-state\status-file.json for why."
    Add-Manifest '07-runtime\AGENT-WAS-DOWN.txt' 'file' 'generated' 'Marker: agent was not running'
}
if (Test-Path $NetdataExe) {
    Save-Cmd '07-runtime\buildinfo.txt' 'netdata -W buildinfo (verbatim; works with daemon down)' { & $using:NetdataExe -W buildinfo 2>&1 } 'netdata.exe -W buildinfo'
    Save-CmdRaw '07-runtime\buildinfo.json' 'netdata -W buildinfojson (machine-readable; no header so it parses as JSON)' { & $using:NetdataExe -W buildinfojson 2>&1 } 'netdata.exe -W buildinfojson'
}
if ((Test-Path $NetdataCli) -and $NetdataProc) {
    Save-CmdRaw '07-runtime\aclk-state.json' 'Cloud connectivity state (netdatacli aclk-state json; no header so it parses as JSON)' { & $using:NetdataCli aclk-state json 2>&1 } 'netdatacli.exe aclk-state json'
}

# ============================================================================
# 08-network
# ============================================================================
Show-Info 'collecting: network'
Save-Cmd '08-network\listening-sockets.txt' 'Listening sockets (netdata-related)' { Get-NetTCPConnection -State Listen | Where-Object { $_.LocalPort -eq $using:NdPort -or (Get-Process -Id $_.OwningProcess -ErrorAction SilentlyContinue).ProcessName -match 'netdata' } | Format-Table LocalAddress, LocalPort, OwningProcess -AutoSize } 'Get-NetTCPConnection -State Listen (netdata)'
Save-Cmd '08-network\dns-config.txt' 'DNS resolver config' { Get-DnsClientServerAddress | Where-Object { $_.ServerAddresses } | Format-Table InterfaceAlias, ServerAddresses -AutoSize } 'Get-DnsClientServerAddress'
Save-Cmd '08-network\proxy-config.txt' 'System proxy configuration' { netsh winhttp show proxy 2>&1; Get-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' | Format-List ProxyEnable, ProxyServer, AutoConfigURL } 'netsh winhttp show proxy + registry'
Save-Cmd '08-network\cloud-connectivity.txt' 'Reachability of Netdata Cloud (TCP/TLS only, no data sent)' { Test-NetConnection -ComputerName $using:CloudHost -Port 443 -WarningAction SilentlyContinue | Format-List ComputerName, RemotePort, TcpTestSucceeded, PingSucceeded } "Test-NetConnection ${CloudHost}:443"

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
tool version:     $ToolVersion (windows)
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
Write-Utf8 (Join-Path $Work 'summary.txt') $summary
Add-Manifest 'summary.txt' 'file' 'generated' 'Human summary'

$readme = @"
# Netdata Support Bundle (Windows)

Generated by ``netdata-support-bundle.ps1``. Contents are SANITIZED: secrets (tokens, api
keys, passwords) are always redacted; by default IPs, MACs, emails and
hostnames are replaced with stable pseudonyms - consistent across all files.
The pseudonym map stays on this machine, next to the zip - it is NOT in this
bundle.

Layout matches the POSIX bundle (see MANIFEST.json for every file):
01-system, 02-install, 03-process, 04-config, 05-logs (Windows Event Log),
06-state, 07-runtime, 08-network. Start with summary.txt.
"@
Write-Utf8 (Join-Path $Work 'README.md') $readme
Add-Manifest 'README.md' 'file' 'generated' 'Bundle documentation'

# emit MANIFEST.json LAST so every file (incl. summary.txt and README.md) is indexed
$manifest = [ordered]@{
    schema = 'netdata-support-bundle/v1'
    tool_version = "$ToolVersion-windows"
    generated_utc = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
    runtime_seconds = $RuntimeSecs
    pii_obfuscated = [bool]$Obfuscate
    secrets_redacted = $true
    agent_running = [bool]$NetdataProc
    agent_api_reachable = [bool]$ApiOk
    is_container = $false
    files = $script:ManifestRows
}
Write-Utf8 (Join-Path $Work 'MANIFEST.json') (($manifest | ConvertTo-Json -Depth 4) + "`n")

# ============================================================================
# zip
# ============================================================================
New-Item -ItemType Directory -Path $Output -Force | Out-Null
$ZipPath = Join-Path $Output "$BundleName.zip"
# build the zip inside the owner-only staging dir, then publish without
# overwrite: Move-Item without -Force fails if the destination exists
$StagingZip = Join-Path $Staging "$BundleName.zip"
Compress-Archive -Path $Work -DestinationPath $StagingZip
try {
    Move-Item -Path $StagingZip -Destination $ZipPath -ErrorAction Stop
} catch {
    # Write-Error would be swallowed by $ErrorActionPreference='SilentlyContinue'
    Show-Info "ERROR: could not publish $ZipPath ($_)"
    if (-not $KeepStaging) { Remove-Item $Staging -Recurse -Force -ErrorAction SilentlyContinue }
    exit 1
}

$MapPath = Join-Path $Output "$BundleName.pseudonym-map.tsv"
if ($Obfuscate -and $script:PseudoMap.Count -gt 0) {
    # same format as the POSIX script: type<TAB>original<TAB>pseudonym
    $mapText = ($script:PseudoMap.GetEnumerator() | ForEach-Object {
        $type = 'other'
        if ($_.Value -match '^ip6-\d+$') { $type = 'ip6' }
        elseif ($_.Value -match '^ip-\d+$') { $type = 'ip' }
        elseif ($_.Value -match '^private-host-\d+$') { $type = 'fqdn' }
        elseif ($_.Value -eq 'redacted-host') { $type = 'host' }
        elseif ($_.Value -eq 'redacted-user') { $type = 'user' }
        "$type`t$($_.Key)`t$($_.Value)"
    }) -join "`n"
    # build in the private staging dir, then move without -Force so a
    # pre-existing file or symlink in a shared output dir is never followed
    $mapStaged = Join-Path $Staging 'pseudonym-map.tsv'
    Write-Utf8 $mapStaged ($mapText + "`n")
    try { Move-Item -Path $mapStaged -Destination $MapPath -ErrorAction Stop }
    catch { Show-Info "could not publish pseudonym map to $MapPath ($_) - left in staging"; $MapPath = $mapStaged; $KeepStaging = $true }
}

if (-not $KeepStaging) { Remove-Item $Staging -Recurse -Force }

Show-Info ''
Show-Info ("done in {0}s" -f [int]((Get-Date) - $StartTime).TotalSeconds)
Show-Info "bundle:  $ZipPath"
if ($Obfuscate -and $script:PseudoMap.Count -gt 0) {
    Show-Info "pseudonym map (KEEP PRIVATE, do not send): $MapPath"
}
Show-Info 'attach the bundle to your support ticket.'
