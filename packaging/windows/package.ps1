# Package the build

#Requires -Version 4.0

$ErrorActionPreference = "Stop"

. "$PSScriptRoot\functions.ps1"

$msysbash = Get-MSYS2Bash "$msysprefix"
$env:CHERE_INVOKING = 'yes'
$driverDir = "C:\msys64\opt\netdata\usr\bin"
$driverFiles = @(
    (Join-Path $driverDir "netdata_driver.sys"),
    (Join-Path $driverDir "netdata_driver.inf"),
    (Join-Path $driverDir "netdata_driver.cat")
)

& $msysbash -l "$PSScriptRoot\package-windows.sh"

if ($LastExitcode -ne 0) {
    exit 1
}

if (-not (Test-Path (Join-Path $driverDir "netdata_driver.cat"))) {
    & "$PSScriptRoot\generate-driver-catalog.ps1" -DriverDirectory $driverDir

    if ($LastExitCode -ne 0) {
        exit 1
    }
}

foreach ($driverFile in $driverFiles) {
    if (-not (Test-Path $driverFile)) {
        Write-Error "Missing required driver packaging file: $driverFile"
        exit 1
    }
}

if ($null -eq $env:BUILD_DIR) {
    $builddir = & $msysbash -l "$PSScriptRoot\get-win-build-path.sh"

    if ($LastExitcode -ne 0) {
        exit 1
    }
} else {
    $builddir = $env:BUILD_DIR
}

Push-Location "$builddir"

$wixarch = "x64"

wix build -arch $wixarch -ext WixToolset.Util.wixext -ext WixToolset.UI.wixext -out "$PSScriptRoot\netdata-$wixarch.msi" netdata.wxs

if ($LastExitcode -ne 0) {
    Pop-Location
    exit 1
}

Pop-Location
