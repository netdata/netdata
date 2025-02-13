# Set up Windows build dependencies.
#
# This script first sees if msys is installed. If so, it just uses it. If not, it tries to bootstrap it with chocolatey or winget.

#Requires -Version 4.0

$ErrorActionPreference = "Stop"

. "$PSScriptRoot\functions.ps1"

$msysprefix = Get-MSYS2Prefix

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

$msysbash = Get-MSYS2Bash "$msysprefix"
$env:CHERE_INVOKING = 'yes'

& $msysbash -l "$PSScriptRoot\msys2-dependencies.sh"

if ($LastExitcode -ne 0) {
    Write-Host "First update attempt failed. This is expected if the msys-runtime package needed updated, trying again."

    & $msysbash -l "$PSScriptRoot\msys2-dependencies.sh"

    if ($LastExitcode -ne 0) {
        exit 1
    }
}

$wixVersion = "5.0.2"

Write-Host "Installing WiX toolset"
dotnet tool install -g wix --version $wixVersion

if ($LastExitcode -ne 0) {
    exit 1
}

Write-Host "Adding WiX extensions"

wix extension -g add WixToolset.Util.wixext/$wixVersion

if ($LastExitcode -ne 0) {
    exit 1
}

wix extension -g add WixToolset.UI.wixext/$wixVersion

if ($LastExitcode -ne 0) {
    exit 1
}
