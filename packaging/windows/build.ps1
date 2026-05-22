# Run the build

#Requires -Version 4.0

$ErrorActionPreference = "Stop"

. "$PSScriptRoot\functions.ps1"

$msysbash = Get-MSYS2Bash "$msysprefix"
$env:CHERE_INVOKING = 'yes'
$env:MSYSTEM = 'UCRT64'

& $msysbash -l "$PSScriptRoot\compile-on-windows.sh"

if ($LastExitcode -ne 0) {
    exit 1
}
