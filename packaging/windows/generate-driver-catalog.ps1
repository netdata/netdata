# Generate the Netdata driver catalog file required by WiX packaging.
#
# Requires INF and SYS files already staged in the target directory.

#Requires -Version 4.0

param(
    [Parameter(Mandatory = $true)]
    [string]$DriverDirectory,

    [Parameter()]
    [string]$OsTargets = "10_GE_X64,10_25H2_X64,Server2025_X64,10_NI_X64,10_CO_X64,ServerFE_X64,10_VB_X64,10_19H1_X64,10_RS5_X64,ServerRS5_X64,10_RS4_X64,10_RS3_X64,10_RS2_X64,10_AU_X64,10_X64,Server10_X64"
)

$ErrorActionPreference = "Stop"

function Find-Inf2Cat {
    $command = Get-Command Inf2Cat.exe -ErrorAction SilentlyContinue
    if ($null -ne $command) {
        return $command.Source
    }

    $kitRoots = @(
        (Join-Path ${env:ProgramFiles(x86)} "Windows Kits\10\bin"),
        (Join-Path $env:ProgramFiles "Windows Kits\10\bin")
    )

    $candidates = @()
    foreach ($root in $kitRoots) {
        if (Test-Path $root) {
            # Prefer the typical WDK/SDK layout: Windows Kits\10\bin\<version>\x64\Inf2Cat.exe
            $patternX64 = Join-Path $root '*\x64\Inf2Cat.exe'
            $found = Get-ChildItem -Path $patternX64 -File -ErrorAction SilentlyContinue
            if (-not $found) {
                # Fallback: any versioned subfolder directly under bin containing Inf2Cat.exe
                $patternAnyArch = Join-Path $root '*\Inf2Cat.exe'
                $found = Get-ChildItem -Path $patternAnyArch -File -ErrorAction SilentlyContinue
                if ($found) {
                    $candidates += $found
                }
            }
        }
    }

    if ($candidates.Count -eq 0) {
        throw "Inf2Cat.exe not found. Install Windows Driver Kit (WDK) or Windows SDK tools with Inf2Cat."
    }

    $x64Candidates = $candidates | Where-Object { $_.FullName -match '\\x64\\Inf2Cat\.exe$' }
    if ($x64Candidates.Count -gt 0) {
        $candidates = $x64Candidates
    }

    $latest = $candidates |
        Sort-Object -Property {
            $match = [regex]::Match($_.FullName, '\\10\\bin\\(?<ver>\d+\.\d+\.\d+\.\d+)\\')
            if ($match.Success) {
                [version]$match.Groups['ver'].Value
            } else {
                [version]"0.0.0.0"
            }
        } -Descending |
        Select-Object -First 1

    return $latest.FullName
}

$driverDir = (Resolve-Path -LiteralPath $DriverDirectory).Path
$driverInf = Join-Path $driverDir "netdata_driver.inf"
$driverSys = Join-Path $driverDir "netdata_driver.sys"
$driverCat = Join-Path $driverDir "netdata_driver.cat"

if (-not (Test-Path $driverInf)) {
    throw "Missing driver INF: $driverInf"
}

if (-not (Test-Path $driverSys)) {
    throw "Missing driver SYS: $driverSys"
}

$inf2cat = Find-Inf2Cat
Write-Host "Using Inf2Cat: $inf2cat"
Write-Host "Generating driver catalog in: $driverDir"

& $inf2cat "/driver:$driverDir" "/os:$OsTargets"

if ($LastExitCode -ne 0) {
    throw "Inf2Cat failed with exit code $LastExitCode"
}

if (-not (Test-Path $driverCat)) {
    throw "Catalog generation failed, missing output: $driverCat"
}

Write-Host "Generated driver catalog: $driverCat"
