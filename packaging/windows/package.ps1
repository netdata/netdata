# Package the build

#Requires -Version 4.0

$ErrorActionPreference = "Stop"

. "$PSScriptRoot\functions.ps1"

$msysbash = Get-MSYS2Bash "$msysprefix"
$env:CHERE_INVOKING = 'yes'

& $msysbash -l "$PSScriptRoot\package-windows.sh"

if ($LastExitcode -ne 0) {
    exit 1
}

if ($null -eq $env:BUILD_DIR) {
    $builddir = & $msysbash -l "$PSScriptRoot\get-win-build-path.sh"

    if ($LastExitcode -ne 0) {
        exit 1
    }
} else {
    $builddir = $env:BUILD_DIR
}

$wixarch = "x64"

wix build -arch $wixarch -ext WixToolset.Util.wixext -ext WixToolset.UI.wixext -out "$PSScriptRoot\netdata-$wixarch.msi" "$builddir\netdata.wxs"

if ($LastExitcode -ne 0) {
    exit 1
}
