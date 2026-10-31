#Requires -Version 4.0

$ErrorActionPreference = "Stop"

$ndPath = "C:\Program Files\Netdata\usr\bin\netdata.exe"
$runtimePath = "C:\Program Files\Netdata\usr\bin"

if (-not (Test-Path -LiteralPath $ndPath -PathType Leaf)) {
    Write-Output "$ndPath does not exist"
    exit 1
}

$env:PATH = "$runtimePath;$env:PATH"
Write-Output "$ndPath found, checking that Windows can load and run it"

try {
    & $ndPath -W buildinfo
    $exitCode = $LASTEXITCODE
} catch {
    Write-Output "Windows could not load netdata.exe: $_"
    exit 1
}

if ($exitCode -ne 0) {
    Write-Output "netdata.exe buildinfo exited with code $exitCode"
    exit 1
}

Write-Output "Windows loaded netdata.exe and buildinfo completed successfully"
