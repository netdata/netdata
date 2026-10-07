#Requires -Version 5.1
param([Parameter(Mandatory = $true, Position = 0)][string]$ConfigFile)

$ErrorActionPreference = 'Stop'
$installRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..\..')).Path
$configRoot = Join-Path $installRoot 'etc\netdata'
$stockRoot = Join-Path $installRoot 'usr\lib\netdata\conf.d'
$relativePath = $ConfigFile.Replace('/', '\').TrimStart('\')
if ([IO.Path]::IsPathRooted($relativePath) -or $relativePath -match '(^|\\)\.\.(\\|$)') {
    throw 'Specify a configuration file name relative to the Netdata configuration directory.'
}

$destination = Join-Path $configRoot $relativePath
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
    Start-Process -FilePath $notepad -ArgumentList @($destination)
}
