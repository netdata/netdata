#Requires -Version 5.1
param(
    [Parameter(Position = 0)][string]$ConfigFile,
    [switch]$PauseOnError
)

$ErrorActionPreference = 'Stop'
$installRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..\..')).Path
$configRoot = Join-Path $installRoot 'etc\netdata'
$stockRoot = Join-Path $installRoot 'usr\lib\netdata\conf.d'

try {
    if (-not $ConfigFile) {
        $files = @(
            Get-ChildItem -LiteralPath $configRoot -File -Recurse -ErrorAction SilentlyContinue |
                ForEach-Object { $_.FullName.Substring($configRoot.Length).TrimStart('\') }
            Get-ChildItem -LiteralPath $stockRoot -File -Recurse -ErrorAction SilentlyContinue |
                ForEach-Object { $_.FullName.Substring($stockRoot.Length).TrimStart('\') }
        ) | Sort-Object -Unique
        if ($files) { $files } else { Write-Output 'No Netdata configuration files are available.' }
        return
    }

    $relativePath = $ConfigFile.Replace('/', '\').TrimStart('\')
    $segments = $relativePath -split '\\'
    if ([IO.Path]::IsPathRooted($ConfigFile) -or
        ($segments | Where-Object { $_.TrimEnd(' ', '.') -in @('.', '..') })) {
        throw 'Specify a configuration file name relative to the Netdata configuration directory.'
    }

    $destination = [IO.Path]::GetFullPath((Join-Path $configRoot $relativePath))
    $configRootFull = [IO.Path]::GetFullPath($configRoot).TrimEnd('\') + '\'
    if (-not $destination.StartsWith($configRootFull, [StringComparison]::OrdinalIgnoreCase)) {
        throw 'The requested configuration file must remain inside the Netdata configuration directory.'
    }

    $principal = [Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
    $isAdministrator = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    if (-not $isAdministrator) {
        $powershell = Join-Path $env:WINDIR 'System32\WindowsPowerShell\v1.0\powershell.exe'
        $argumentList = @(
            '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', ('"' + $PSCommandPath + '"'),
            ('"' + $ConfigFile + '"'), '-PauseOnError'
        )
        $elevated = Start-Process -FilePath $powershell -ArgumentList $argumentList -Verb RunAs -Wait -PassThru
        if ($elevated.ExitCode -ne 0) { throw "The elevated configuration editor exited with code $($elevated.ExitCode)." }
        return
    }

    $stockFile = Join-Path $stockRoot $relativePath
    if (-not (Test-Path -LiteralPath $destination)) {
        if (-not (Test-Path -LiteralPath $stockFile -PathType Leaf)) {
            throw "Neither an existing config nor a stock default exists for '$ConfigFile'."
        }
        $destinationDirectory = Split-Path -Parent $destination
        New-Item -ItemType Directory -Path $destinationDirectory -Force | Out-Null
        Copy-Item -LiteralPath $stockFile -Destination $destination
    }

    try {
        Start-Process -FilePath $destination
    } catch {
        $notepad = Join-Path $env:WINDIR 'System32\notepad.exe'
        Start-Process -FilePath $notepad -ArgumentList ('"{0}"' -f $destination)
    }
} catch {
    Write-Host "Could not open Netdata configuration: $_" -ForegroundColor Red
    if ($PauseOnError) { Read-Host 'Press Enter to close' | Out-Null }
    exit 1
}
