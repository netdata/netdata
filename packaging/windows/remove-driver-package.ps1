# Remove Netdata driver package(s) from the Windows driver store.
#
# This script is intended to run during MSI uninstall.

#Requires -Version 5.1

$ErrorActionPreference = "Stop"

$targetInf = "netdata_driver.inf"
$pnputil = Join-Path $env:SystemRoot "System32\pnputil.exe"

if (-not (Test-Path $pnputil)) {
    Write-Host "pnputil not found at $pnputil, skipping driver package removal."
    exit 0
}

function Get-PublishedNamesFromPnpUtil {
    $output = & $pnputil /enum-drivers

    if ($LASTEXITCODE -ne 0) {
        Write-Host "WARNING: pnputil /enum-drivers failed with exit code $LASTEXITCODE"
        return @()
    }

    $published = @()
    $currentPublished = $null
    $currentOriginal = $null

    foreach ($line in $output) {
        if ($line -match '^\s*Published Name\s*:\s*(.+?)\s*$') {
            $currentPublished = $matches[1].Trim()
            continue
        }

        if ($line -match '^\s*Original Name\s*:\s*(.+?)\s*$') {
            $currentOriginal = $matches[1].Trim()

            if ($null -ne $currentPublished -and $currentOriginal -ieq $targetInf) {
                $published += $currentPublished
            }

            $currentPublished = $null
            $currentOriginal = $null
        }
    }

    return $published
}

$publishedNames = @()

try {
    $drivers = Get-WindowsDriver -Online | Where-Object { $_.OriginalFileName -ieq $targetInf }
    $publishedNames = @($drivers | ForEach-Object { $_.Driver } | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
}
catch {
    Write-Host "Get-WindowsDriver unavailable; falling back to pnputil parsing."
    $publishedNames = Get-PublishedNamesFromPnpUtil
}

if ($publishedNames.Count -eq 0) {
    Write-Host "No driver store entries found for $targetInf."
    exit 0
}

$exitCode = 0

foreach ($published in $publishedNames) {
    Write-Host "Removing driver package: $published"
    & $pnputil /delete-driver $published /uninstall /force

    if ($LASTEXITCODE -ne 0) {
        Write-Host "WARNING: pnputil failed for $published with exit code $LASTEXITCODE"
        $exitCode = $LASTEXITCODE
    }
}

exit $exitCode
