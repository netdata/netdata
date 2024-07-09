# Functions used by the PowerShell scripts in this directory.

#Requires -Version 4.0

function Get-MSYS2Prefix {
    if (-Not ($msysprefix)) {
        if (Test-Path -Path C:\msys64\usr\bin\bash.exe) {
            return "C:\msys64"
        } elseif ($env:ChocolateyToolsLocation) {
            if (Test-Path -Path "$env:ChocolateyToolsLocation\msys64\usr\bin\bash.exe") {
                return "$env:ChocolateyToolsLocation\msys64"
            }
        }
    }

    return ""
}

function Get-MSYS2Bash {
    $msysprefix = $args[0]

    if (-Not ($msysprefix)) {
        $msysprefix = Get-MSYS2Prefix
    }

    Write-Host "Using MSYS2 from $msysprefix"

    return "$msysprefix\usr\bin\bash.exe"
}
