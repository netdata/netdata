#Requires -Version 5.1
param([string]$WindowsPathPrefix)

$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\functions.ps1"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$buildDir = if ($env:BUILD_DIR) { Resolve-WindowsBuildDirectory $env:BUILD_DIR $repoRoot } else { Join-Path $repoRoot 'build' }
$msysRoot = Get-MSYS2Prefix
if (-not $msysRoot) { throw 'MSYS2 UCRT64 is not installed. Run install-dependencies.ps1 first.' }
$toolBin = Join-Path $msysRoot 'ucrt64\bin'
$env:MSYSTEM = 'UCRT64'
$env:PATH = "$toolBin;$msysRoot\ucrt64\sbin;$env:PATH"
$env:PKG_CONFIG_PATH = "$toolBin\..\lib\pkgconfig;$toolBin\..\share\pkgconfig"
$env:PKG_CONFIG_LIBDIR = $env:PKG_CONFIG_PATH
Initialize-WindowsBuildTools

$cmake = Join-Path $toolBin 'cmake.exe'
$ninja = Join-Path $toolBin 'ninja.exe'
$gcc = Join-Path $toolBin 'gcc.exe'
$gxx = Join-Path $toolBin 'g++.exe'
$rustc = Join-Path $toolBin 'rustc.exe'
$go = Join-Path $toolBin 'go.exe'
$goRoot = Join-Path $msysRoot 'ucrt64\lib\go'
$lld = Join-Path $toolBin 'ld.lld.exe'
foreach ($required in @($cmake, $ninja, $gcc, $gxx, $rustc, $go, $lld)) {
    if (-not (Test-Path $required)) { throw "Required UCRT64 tool is missing: $required" }
}
if (-not (Test-Path -LiteralPath $goRoot -PathType Container)) { throw "UCRT64 Go root is missing: $goRoot" }
$env:GOROOT = $goRoot

$installPrefix = Join-Path $buildDir 'stage\opt\netdata'
$buildType = if ($env:CMAKE_BUILD_TYPE) { $env:CMAKE_BUILD_TYPE } else { 'RelWithDebInfo' }
$cFlags = if ($buildType -eq 'Debug') { '-O0 -ggdb -Wall -Wextra -Wno-char-subscripts -DNETDATA_INTERNAL_CHECKS=1 -Wa,-mbig-obj -pipe -D_FILE_OFFSET_BITS=64' } else { '-O2 -Wa,-mbig-obj -pipe -D_FILE_OFFSET_BITS=64' }
$originalCFlags = $env:CFLAGS
$env:CFLAGS = if ($originalCFlags) { "$originalCFlags $cFlags" } else { $cFlags }
$cmakePrefixArgument = "-DCMAKE_PREFIX_PATH:PATH=$($msysRoot)\ucrt64"
$configureArgs = @(
    '-S', $repoRoot, '-B', $buildDir, '-G', 'Ninja', "-DCMAKE_MAKE_PROGRAM=$ninja",
    "-DCMAKE_C_COMPILER=$gcc", "-DCMAKE_CXX_COMPILER=$gxx", "-DCMAKE_BUILD_TYPE=$buildType",
    $cmakePrefixArgument, "-DCMAKE_INSTALL_PREFIX=$installPrefix",
    '-DNETDATA_PACKAGE_KIND=msi', "-DNETDATA_USER=$env:USERNAME", '-DENABLE_ACLK=On',
    '-DENABLE_CLOUD=On', '-DENABLE_ML=On', '-DENABLE_PLUGIN_GO=On',
    '-DENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE=Off', '-DENABLE_PLUGIN_SYSTEMD_JOURNAL=Off',
    '-DENABLE_BUNDLED_JSONC=On', '-DENABLE_BUNDLED_PROTOBUF=On', "-DRust_COMPILER=$rustc",
    '-DCMAKE_NINJA_FORCE_RESPONSE_FILE=ON',
    "-DCMAKE_C_FLAGS_RELWITHDEBINFO=-O2 -g1 -DNDEBUG",
    "-DCMAKE_CXX_FLAGS_RELWITHDEBINFO=-O2 -g1 -DNDEBUG",
    '-DCMAKE_EXE_LINKER_FLAGS=-fuse-ld=lld', '-DCMAKE_SHARED_LINKER_FLAGS=-fuse-ld=lld'
)
if ($WindowsPathPrefix) { $configureArgs += "-DNETDATA_WINDOWS_PATH_PREFIX=$WindowsPathPrefix" }
if ($env:EXTRA_CMAKE_OPTIONS) { $configureArgs += ConvertFrom-WindowsCommandLine $env:EXTRA_CMAKE_OPTIONS }
# WiX embeds this configure-time prefix, so caller options must not redirect it.
$configureArgs += "-DCMAKE_INSTALL_PREFIX=$installPrefix"

New-Item -ItemType Directory -Force -Path $buildDir | Out-Null
$stageMarker = Join-Path $buildDir '.netdata-package-stage-ready'
if (Test-Path -LiteralPath $stageMarker) { Remove-Item -LiteralPath $stageMarker -Force }
try {
    & $cmake @configureArgs
    $configureExitCode = $LASTEXITCODE
} finally {
    $env:CFLAGS = $originalCFlags
}
if ($configureExitCode -ne 0) { exit $configureExitCode }

& $cmake --build $buildDir
$buildExitCode = $LASTEXITCODE
if ($buildExitCode -ne 0) {
    Write-Host "CMake build failed with exit code $buildExitCode. Inspect the first FAILED target above." -ForegroundColor Red
    exit $buildExitCode
}
$agent = Join-Path $buildDir 'netdata.exe'
if (-not (Test-Path $agent)) { throw "Build returned success but $agent was not produced." }

$stdout = Join-Path $buildDir 'netdata-buildinfo.out'
$stderr = Join-Path $buildDir 'netdata-buildinfo.err'
$process = Start-Process -FilePath $agent -ArgumentList @('-W', 'buildinfo') -PassThru -NoNewWindow -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if (-not $process.WaitForExit(60000)) {
    Stop-Process -Id $process.Id -Force
    Write-Warning 'netdata.exe -W buildinfo exceeded 60 seconds; the build artifact is present.'
} else {
    Get-Content $stdout -ErrorAction SilentlyContinue
    Get-Content $stderr -ErrorAction SilentlyContinue | ForEach-Object { Write-Host $_ }
    if ($process.ExitCode -ne 0) { Write-Warning "Buildinfo exited with $($process.ExitCode); build artifact is present." }
}
