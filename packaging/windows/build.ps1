#Requires -Version 5.1
param([string]$WindowsPathPrefix)

$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\functions.ps1"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$buildDir = if ($env:BUILD_DIR) { [IO.Path]::GetFullPath($env:BUILD_DIR) } else { Join-Path $repoRoot 'build' }
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
foreach ($required in @($cmake, $ninja, $gcc, $gxx, $rustc)) {
    if (-not (Test-Path $required)) { throw "Required UCRT64 tool is missing: $required" }
}

$installPrefix = Join-Path $buildDir 'stage\opt\netdata'
$buildType = if ($env:CMAKE_BUILD_TYPE) { $env:CMAKE_BUILD_TYPE } else { 'RelWithDebInfo' }
$cFlags = if ($buildType -eq 'Debug') { '-O0 -ggdb -Wall -Wextra -Wno-char-subscripts -DNETDATA_INTERNAL_CHECKS=1 -Wa,-mbig-obj -pipe -D_FILE_OFFSET_BITS=64' } else { '-O2 -Wa,-mbig-obj -pipe -D_FILE_OFFSET_BITS=64' }
$env:CFLAGS = $cFlags
$configureArgs = @(
    '-S', $repoRoot, '-B', $buildDir, '-G', 'Ninja', "-DCMAKE_MAKE_PROGRAM=$ninja",
    "-DCMAKE_C_COMPILER=$gcc", "-DCMAKE_CXX_COMPILER=$gxx", "-DCMAKE_BUILD_TYPE=$buildType",
    '-DCMAKE_PREFIX_PATH=' + (Join-Path $msysRoot 'ucrt64'), "-DCMAKE_INSTALL_PREFIX=$installPrefix",
    '-DBUILD_FOR_PACKAGING=On', "-DNETDATA_USER=$env:USERNAME", '-DENABLE_ACLK=On',
    '-DENABLE_CLOUD=On', '-DENABLE_ML=On', '-DENABLE_PLUGIN_GO=On',
    '-DENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE=Off', '-DENABLE_PLUGIN_SYSTEMD_JOURNAL=Off',
    '-DENABLE_BUNDLED_JSONC=On', '-DENABLE_BUNDLED_PROTOBUF=On', "-DRust_COMPILER=$rustc",
    '-DCMAKE_NINJA_FORCE_RESPONSE_FILE=ON',
    "-DCMAKE_C_FLAGS_RELWITHDEBINFO=-O2 -g1 -DNDEBUG",
    "-DCMAKE_CXX_FLAGS_RELWITHDEBINFO=-O2 -g1 -DNDEBUG",
    '-DCMAKE_EXE_LINKER_FLAGS=-fuse-ld=lld', '-DCMAKE_SHARED_LINKER_FLAGS=-fuse-ld=lld'
)
if ($WindowsPathPrefix) { $configureArgs += "-DNETDATA_WINDOWS_PATH_PREFIX=$WindowsPathPrefix" }
if ($env:EXTRA_CMAKE_OPTIONS) { $configureArgs += ($env:EXTRA_CMAKE_OPTIONS -split '\s+') }

New-Item -ItemType Directory -Force -Path $buildDir | Out-Null
& $cmake @configureArgs
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

& $cmake --build $buildDir -- -k 1
if ($LASTEXITCODE -ne 0) {
    Write-Error "CMake build failed with exit code $LASTEXITCODE. Inspect the first FAILED target above."
    exit $LASTEXITCODE
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
    Get-Content $stderr -ErrorAction SilentlyContinue | Write-Error
    if ($process.ExitCode -ne 0) { Write-Warning "Buildinfo exited with $($process.ExitCode); build artifact is present." }
}
