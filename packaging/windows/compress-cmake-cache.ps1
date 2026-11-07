#Requires -Version 5.1
param(
    [Parameter(Mandatory = $true, Position = 0)][string]$InputPath,
    [Parameter(Mandatory = $true, Position = 1)][string]$OutputPath
)

$ErrorActionPreference = 'Stop'
$source = [IO.File]::OpenRead($InputPath)
$temporaryPath = "$OutputPath.$PID.tmp"
$destination = $null
$gzip = $null
try {
    $destination = [IO.File]::Create($temporaryPath)
    $gzip = New-Object IO.Compression.GZipStream($destination, [IO.Compression.CompressionMode]::Compress)
    $source.CopyTo($gzip)
} finally {
    if ($gzip) { $gzip.Dispose() }
    if ($destination) { $destination.Dispose() }
    $source.Dispose()
}
Move-Item -LiteralPath $temporaryPath -Destination $OutputPath -Force
