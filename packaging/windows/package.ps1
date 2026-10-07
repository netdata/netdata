#Requires -Version 5.1
$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\functions.ps1"

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$buildDir = if ($env:BUILD_DIR) { [IO.Path]::GetFullPath($env:BUILD_DIR) } else { Join-Path $repoRoot 'build' }
$msysRoot = Get-MSYS2Prefix
if (-not $msysRoot) { throw 'MSYS2 UCRT64 is not installed. Run install-dependencies.ps1 first.' }
$toolBin = Join-Path $msysRoot 'ucrt64\bin'
$env:MSYSTEM = 'UCRT64'
$env:PATH = "$toolBin;$msysRoot\ucrt64\sbin;$env:PATH"
$env:NETDATA_WINDOWS_RUNTIME_DLL_DIR = $toolBin
Initialize-WindowsBuildTools

$cmake = Join-Path $toolBin 'cmake.exe'
if (-not (Test-Path (Join-Path $buildDir 'cmake_install.cmake'))) {
    throw "No configured CMake build found at $buildDir. Run build.ps1 first."
}
& $cmake --install $buildDir
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$packageRoot = Join-Path $buildDir 'stage\opt\netdata'
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'copy_files.ps1') -Destination (Join-Path $packageRoot 'usr\libexec\netdata\copy_files.ps1') -Force

$legacyRuntimePaths = @(
    Get-ChildItem -LiteralPath $packageRoot -Recurse -File -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -in @('msys2.exe', 'bash.exe', 'sh.exe', 'msys-2.0.dll', 'mintty.exe') } |
        ForEach-Object { $_.FullName }
)
if (Test-Path (Join-Path $packageRoot 'msys64')) { $legacyRuntimePaths += (Join-Path $packageRoot 'msys64') }
if ($legacyRuntimePaths) {
    throw "Staging tree contains legacy MSYS2 runtime files. Use a fresh BUILD_DIR before packaging: $($legacyRuntimePaths -join ', ')"
}

$wixCommand = Get-Command wix.exe -ErrorAction SilentlyContinue
if (-not $wixCommand) { $wixCommand = Get-Command wix -ErrorAction SilentlyContinue }
if (-not $wixCommand) {
    $dotnetTools = Join-Path $env:USERPROFILE '.dotnet\tools'
    if (Test-Path (Join-Path $dotnetTools 'wix.exe')) { $wix = Join-Path $dotnetTools 'wix.exe' }
    else { throw 'WiX v5 is not installed. Run install-dependencies.ps1 first.' }
} else { $wix = $wixCommand.Source }

$installer = Join-Path $PSScriptRoot 'netdata-x64.msi'
Push-Location $buildDir
try {
    & $wix build -arch x64 -ext WixToolset.Util.wixext -ext WixToolset.UI.wixext -out $installer 'netdata.wxs'
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} finally {
    Pop-Location
}
