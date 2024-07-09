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

function Install-MSYS2Chocolatey {
    Write-Host "Using Chocolatey to install MSYS2."
    choco install -y msys2

    if ($LastExitcode -ne 0) {
        Write-Host "Failed to install MSYS2 using Chocolatey"
        exit 1
    }

    if (Test-Path -Path "$env:ChocolateyToolsLocation\msys64\usr\bin\bash.exe") {
        return "$env:ChocolateyToolsLocation\msys2"
    } else {
        Write-Host "Can not find the copy of MSYS2 installed by Chocolatey"
        exit 1
    }
}

function Install-MSYS2Native {
    $repo = 'msys2/msys2-installer'
    $uri = "https://api.github.com/repos/$repo/releases/latest"
    $headers = @{
        'Accept' = 'application/vnd.github+json'
        'X-GitHub-API-Version' = '2022-11-28'
    }
    $installer_path = "$env:TEMP\msys2-base.exe"

    if ($PROCESSOR_ARCHITECTURE -ne "AMD64") {
        Write-Host "We can only install MSYS2 for 64-bit x86 systems, but you appear to have a different processor architecture ($PROCESSOR_ARCHITECTURE)."
        Write-Host "You will need to install MSYS2 yourself instead."
        exit 1
    }

    Write-Host "Installing MSYS2 using the official installer"

    $release = Invoke-RESTMethod -Uri $uri -Headers $headers -ConnectionTimeoutSeconds 15

    $release_name = $release.name
    $version = $release.tag_name.Replace('-', '')
    $installer_url = "https://github.com/$repo/releases/download/$release_name/msys2-x86_64-$version.exe"

    Invoke-WebRequest $installer_url -OutFile $installer_path
    Invoke-WebRequest "$installer_url.sha256" -OutFile "$installer_path.sha256"

    Check-FileHash $installer_path

    & $installer_path in --confirm-command --accept-messages --root C:/msys64

    return "C:\msys64"
}

if (-Not ($msysprefix)) {
    Write-Host "Could not find MSYS2, attempting to install it."

    if (Get-Command choco) {
        $msysprefix = Install-MSYS2Chocolatey
    } else {
        $msysprefix = Install-MSYS2Native
    }
}

$msysbash = Get-MSYS2Bash "$msysprefix"
$env:CHERE_INVOKING = 'yes'

& $msysbash -l "$PSScriptRoot\msys2-dependencies.sh"

if ($LastExitcode -ne 0) {
    exit 1
}
