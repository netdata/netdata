#Requires -Version 4.0

$ErrorActionPreference = "Stop"

$ndPath = "C:\Program Files\Netdata\usr\bin\netdata.exe"
$lddPath = "C:\Program Files\Netdata\usr\bin\ldd.exe"

if (Test-Path -Path $ndPath) {
    Write-Output "$ndPath found, attempting to check linking"

    if (Test-Path -Path $lddPath) {
        & $lddPath $ndPath | Tee-Object -Variable lddResult

        if ($LastExitCode -ne 0) {
            Write-Output "Exit Code: $LastExitCode"
            exit 1
        }

        if ($lddResult.Contains('missing')) {
            Write-Output "Libraries missing from install"
            exit 1
        } else {
            Write-Output "Linking OK"
        }
    } else {
        Write-Output "$lddPath not found, unable to check linking"
        exit 2
    }
} else {
    Write-Output "$ndPath does not exist"
    exit 1
}
