#Requires -Version 5.1
param([ValidateSet('install', 'service')][string]$Mode)

$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$windowsDir = Join-Path $repoRoot 'packaging\windows'
. (Join-Path $windowsDir 'functions.ps1')
if ($Mode -eq 'install') {
    & (Join-Path $windowsDir 'install-dependencies.ps1')
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}

$env:BUILD_DIR = if ($env:BUILD_DIR) { Resolve-WindowsBuildDirectory $env:BUILD_DIR $repoRoot } else { Join-Path $repoRoot 'build' }
& (Join-Path $windowsDir 'build.ps1')
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

if (Get-Service -Name Netdata -ErrorAction SilentlyContinue) {
    Stop-Service -Name Netdata -Force -ErrorAction SilentlyContinue
    if ($Mode -eq 'service') { & sc.exe delete Netdata | Out-Null }
}

if ($Mode -eq 'service') {
    & (Join-Path $windowsDir 'package.ps1')
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} else {
    $msysRoot = Get-MSYS2Prefix
    if (-not $msysRoot) { throw 'MSYS2 UCRT64 is not installed.' }
    $env:NETDATA_WINDOWS_RUNTIME_DLL_DIR = Join-Path $msysRoot 'ucrt64\bin'
    $cmake = Join-Path $msysRoot 'ucrt64\bin\cmake.exe'
    Clear-WindowsInstallStage $env:BUILD_DIR $repoRoot
    & $cmake --install $env:BUILD_DIR
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}

$packageRoot = Join-Path $env:BUILD_DIR 'stage\opt\netdata'
$agent = Join-Path $packageRoot 'usr\bin\netdata.exe'
$etwInstaller = Join-Path $packageRoot 'usr\bin\wevt_netdata_install.bat'
if (Test-Path $etwInstaller) {
    & $env:ComSpec /d /c "`"$etwInstaller`""
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}

if ($Mode -eq 'service') {
    & sc.exe create Netdata binPath= "`"$agent`"" start= auto
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    & sc.exe start Netdata
    exit $LASTEXITCODE
}

& $agent -D
exit $LASTEXITCODE
