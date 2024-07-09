# Build Netdata using MSYS2

#Requires -Version 4.0

$ErrorActionPreference = "Stop"

. "$PSScriptRoot\functions.ps1"

$msysbash = Get-MSYS2Bash "$msysprefix"
$env:CHERE_INVOKING = 'yes'

& $msysbash -l "$PSScriptRoot\build.sh"

if ($LastExitcode -ne 0) {
    exit 1
}
