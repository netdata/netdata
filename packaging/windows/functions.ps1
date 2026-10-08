# Functions used by the PowerShell scripts in this directory.

#Requires -Version 4.0

function Get-MSYS2Prefix {
    if (-Not ($msysprefix)) {
        if (Test-Path -Path C:\msys64\usr\bin\pacman.exe) {
            return "C:\msys64"
        } elseif ($env:ChocolateyToolsLocation) {
            if (Test-Path -Path "$env:ChocolateyToolsLocation\msys64\usr\bin\pacman.exe") {
                Write-Host "Found MSYS2 installed via Chocolatey"
                return "$env:ChocolateyToolsLocation\msys64"
            }
        }
    }

    return ""
}

function Initialize-WindowsBuildTools {
    $sdkRoot = Join-Path ${env:ProgramFiles(x86)} 'Windows Kits\10\bin'
    if (Test-Path $sdkRoot) {
        $sdkVersion = Get-ChildItem -Path $sdkRoot -Directory |
            Where-Object { $_.Name -match '^\d+\.\d+' } |
            Sort-Object { [version]$_.Name } -Descending |
            Select-Object -First 1
        if ($sdkVersion) {
            $sdkTools = Join-Path $sdkVersion.FullName 'x64'
            foreach ($tool in @('mc.exe', 'rc.exe')) {
                if (-not (Test-Path (Join-Path $sdkTools $tool))) { throw "Windows SDK tool missing: $tool" }
            }
            $env:PATH = "$sdkTools;$env:PATH"
        }
    }

    $vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio\Installer\vswhere.exe'
    if (Test-Path $vswhere) {
        $vsRoot = & $vswhere -latest -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
        if ($vsRoot) {
            $vcTools = Join-Path $vsRoot 'VC\Tools\MSVC'
            $vcVersion = Get-ChildItem -Path $vcTools -Directory | Sort-Object { [version]$_.Name } -Descending | Select-Object -First 1
            if ($vcVersion) {
                $linkDir = Join-Path $vcVersion.FullName 'bin\Hostx64\x64'
                if (Test-Path (Join-Path $linkDir 'link.exe')) { $env:PATH = "$linkDir;$env:PATH" }
            }
        }
    }

    foreach ($tool in @('mc.exe', 'rc.exe')) {
        if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) { throw "Required Windows build tool not found on PATH: $tool" }
    }

    $linkCommand = Get-Command 'link.exe' -ErrorAction SilentlyContinue
    if (-not $linkCommand -or $linkCommand.Source -notmatch '[\\/]VC[\\/]Tools[\\/]MSVC[\\/]') {
        throw 'Visual Studio link.exe from VC\Tools\MSVC is required; an unrelated link.exe was found or no VS linker is available.'
    }
}

function Resolve-WindowsBuildDirectory([string]$Path, [string]$RepoRoot) {
    if ([IO.Path]::IsPathRooted($Path)) { return [IO.Path]::GetFullPath($Path) }
    return [IO.Path]::GetFullPath((Join-Path $PWD.Path $Path))
}

function Clear-WindowsInstallStage([string]$BuildDirectory, [string]$RepoRoot) {
    $buildPath = [IO.Path]::GetFullPath($BuildDirectory).TrimEnd('\', '/')
    $repoPath = [IO.Path]::GetFullPath($RepoRoot).TrimEnd('\', '/')
    if ($buildPath -eq $repoPath -or -not (Test-Path -LiteralPath (Join-Path $buildPath 'CMakeCache.txt'))) {
        throw "Refusing to clean the Windows install stage outside a configured build directory: $buildPath"
    }

    $stagePath = [IO.Path]::GetFullPath((Join-Path $buildPath 'stage\opt\netdata'))
    if (-not $stagePath.StartsWith($buildPath + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to clean a stage path outside the build directory: $stagePath"
    }
    if (Test-Path -LiteralPath $stagePath) { Remove-Item -LiteralPath $stagePath -Recurse -Force }
    New-Item -ItemType Directory -Path $stagePath -Force | Out-Null
}
