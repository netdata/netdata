# Set up Windows build dependencies.
#
# This script first sees if msys is installed. If so, it just uses it. If not, it tries to bootstrap it with chocolatey or winget.

#Requires -Version 4.0

$ErrorActionPreference = "Stop"

. "$PSScriptRoot\functions.ps1"

$msysprefix = Get-MSYS2Prefix
$env:MSYSTEM = 'UCRT64'

function Check-FileHash {
    $file_path = $args[0]

    Write-Host "Checking SHA256 hash of $file_path"

    $actual_hash = (Get-FileHash -Algorithm SHA256 -Path $file_path).Hash.toLower()
    $expected_hash = (Get-Content "$file_path.sha256").split()[0]

    if ($actual_hash -ne $expected_hash) {
        Write-Host "SHA256 hash mismatch!"
        Write-Host "Expected: $expected_hash"
        Write-Host "Actual: $actual_hash"
        exit 1
    }
}

function Install-MSYS2 {
    $repo = 'msys2/msys2-installer'
    $uri = "https://api.github.com/repos/$repo/releases"
    $headers = @{
        'Accept' = 'application/vnd.github+json'
        'X-GitHub-API-Version' = '2022-11-28'
    }
    $installer_path = "$env:TEMP\msys2-base.exe"

    if ($env:PROCESSOR_ARCHITECTURE -ne "AMD64") {
        Write-Host "We can only install MSYS2 for 64-bit x86 systems, but you appear to have a different processor architecture ($env:PROCESSOR_ARCHITECTURE)."
        Write-Host "You will need to install MSYS2 yourself instead."
        exit 1
    }

    Write-Host "Determining latest release"
    $release_list = Invoke-RestMethod -Uri $uri -Headers $headers -TimeoutSec 30

    $release = $release_list[0]
    $release_name = $release.name
    $version = $release.tag_name.Replace('-', '')
    $installer_url = "https://github.com/$repo/releases/download/$release_name/msys2-x86_64-$version.exe"

    Write-Host "Fetching $installer_url"
    Invoke-WebRequest $installer_url -OutFile $installer_path
    Write-Host "Fetching $installer_url.sha256"
    Invoke-WebRequest "$installer_url.sha256" -OutFile "$installer_path.sha256"

    Write-Host "Checking file hash"
    Check-FileHash $installer_path

    Write-Host "Installing"
    & $installer_path in --confirm-command --accept-messages --root C:/msys64

    return "C:\msys64"
}

if (-Not ($msysprefix)) {
    Write-Host "Could not find MSYS2, attempting to install it"
    $msysprefix = Install-MSYS2
}

$pacman = Join-Path $msysprefix 'usr\bin\pacman.exe'
$env:MSYSTEM = 'UCRT64'
$env:CHERE_INVOKING = 'yes'
$env:PATH = "$msysprefix\ucrt64\bin;$msysprefix\usr\bin;$env:PATH"

& $pacman -Syuu --noconfirm
if ($LASTEXITCODE -ne 0) {
    Write-Host "The first package database update failed; retrying after the runtime update."
    & $pacman -Syuu --noconfirm
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}

$packages = @(
    'ucrt64/mingw-w64-ucrt-x86_64-rust',
    'ucrt64/mingw-w64-ucrt-x86_64-toolchain',
    'ucrt64/mingw-w64-ucrt-x86_64-tcl',
    'ucrt64/mingw-w64-ucrt-x86_64-brotli',
    'ucrt64/mingw-w64-ucrt-x86_64-cmake',
    'ucrt64/mingw-w64-ucrt-x86_64-curl',
    'ucrt64/mingw-w64-ucrt-x86_64-git',
    'ucrt64/mingw-w64-ucrt-x86_64-go',
    'ucrt64/mingw-w64-ucrt-x86_64-headers',
    'ucrt64/mingw-w64-ucrt-x86_64-libuv',
    'ucrt64/mingw-w64-ucrt-x86_64-libyaml',
    'ucrt64/mingw-w64-ucrt-x86_64-lld',
    'ucrt64/mingw-w64-ucrt-x86_64-lz4',
    'ucrt64/mingw-w64-ucrt-x86_64-ninja',
    'ucrt64/mingw-w64-ucrt-x86_64-openssl',
    'ucrt64/mingw-w64-ucrt-x86_64-pcre2',
    'ucrt64/mingw-w64-ucrt-x86_64-protobuf',
    'ucrt64/mingw-w64-ucrt-x86_64-python',
    'ucrt64/mingw-w64-ucrt-x86_64-zlib',
    'ucrt64/mingw-w64-ucrt-x86_64-zstd'
)
& $pacman -S --noconfirm --needed @packages
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}

$wixVersion = "5.0.2"

$globalTools = & dotnet tool list --global
if ($LASTEXITCODE -ne 0) { throw 'Could not query globally installed .NET tools.' }
$wixToolLine = $globalTools | Where-Object { $_ -match '^\s*wix\s+' } | Select-Object -First 1
$installedWixVersion = if ($wixToolLine -match '^\s*wix\s+([^\s]+)') { $Matches[1] } else { $null }

if (-not $installedWixVersion) {
    Write-Host "Installing WiX $wixVersion"
    & dotnet tool install --global wix --version $wixVersion
} elseif ($installedWixVersion -ne $wixVersion) {
    Write-Host "Updating WiX from $installedWixVersion to $wixVersion"
    & dotnet tool update --global wix --version $wixVersion
} else {
    Write-Host "WiX $wixVersion is already installed"
}
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$wix = Resolve-WixExecutable
if (-not $wix) { throw 'WiX executable was not found in PATH or the global .NET tool locations.' }

$extensions = @(
    "WixToolset.Util.wixext/$wixVersion",
    "WixToolset.UI.wixext/$wixVersion"
)
$previousErrorActionPreference = $ErrorActionPreference
try {
    # PowerShell 5.1 can promote native stderr to a terminating error when the preference is Stop.
    $ErrorActionPreference = 'Continue'
    $extensionListOutput = @(& $wix extension list --global 2>&1)
    $extensionListExitCode = $LASTEXITCODE
} finally {
    $ErrorActionPreference = $previousErrorActionPreference
}
$extensionListText = ($extensionListOutput | ForEach-Object { "$_" }) -join [Environment]::NewLine
$hasExtensionEntries = $extensionListText -match '(?m)^\s*\S+\s+\d+\.\d+'
$extensionListHasError = $extensionListText -match '(?i)\b(error|exception|fatal)\b|\bWIX\d{4}\b'
# WiX returns 2 for an empty extension list; the add commands below populate it.
$emptyExtensionCache = $extensionListExitCode -eq 2 -and -not $hasExtensionEntries -and -not $extensionListHasError
if ($extensionListExitCode -ne 0 -and -not $emptyExtensionCache) {
    throw "Could not query globally installed WiX extensions (exit code $extensionListExitCode). $extensionListText"
}
$installedExtensions = @($extensionListText -split '\r?\n')
foreach ($extension in $extensions) {
    $extensionId, $extensionVersion = $extension -split '/', 2
    $found = $installedExtensions | Where-Object {
        $_ -match ('^\s*' + [regex]::Escape($extensionId) + '\s+' + [regex]::Escape($extensionVersion) + '(\s|$)')
    } | Select-Object -First 1
    if (-not $found) {
        Write-Host "Adding WiX extension $extension"
        & $wix extension add --global $extension
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    } else {
        Write-Host "WiX extension $extension is already installed"
    }
}
