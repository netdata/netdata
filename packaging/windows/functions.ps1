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

function Get-LatestVersionDirectory([string]$Root) {
    if (-not (Test-Path -LiteralPath $Root -PathType Container)) { return $null }
    return Get-ChildItem -LiteralPath $Root -Directory |
        Where-Object { $_.Name -match '^\d+\.\d+' } |
        Sort-Object { [version]$_.Name } -Descending |
        Select-Object -First 1
}

function Add-DirectoryToPathIfToolExists([string]$Directory, [string]$Tool) {
    if (Test-Path -LiteralPath (Join-Path $Directory $Tool) -PathType Leaf) {
        $env:PATH = "$Directory;$env:PATH"
        return $true
    }
    return $false
}

function Resolve-WixExecutable([string]$Override) {
    if ($Override) { return $Override }

    foreach ($name in @('wix.exe', 'wix')) {
        $command = Get-Command $name -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($command -and $command.Source) { return $command.Source }
    }

    $toolRoots = @()
    if ($env:DOTNET_CLI_HOME) { $toolRoots += $env:DOTNET_CLI_HOME }
    if ($env:USERPROFILE) { $toolRoots += $env:USERPROFILE }
    foreach ($root in $toolRoots) {
        $candidate = Join-Path (Join-Path $root '.dotnet\tools') 'wix.exe'
        if (Test-Path -LiteralPath $candidate -PathType Leaf) { return $candidate }
    }

    return $null
}

function Initialize-WindowsBuildTools {
    $sdkRoot = Join-Path ${env:ProgramFiles(x86)} 'Windows Kits\10\bin'
    $sdkVersion = Get-LatestVersionDirectory $sdkRoot
    if ($sdkVersion) {
        $sdkTools = Join-Path $sdkVersion.FullName 'x64'
        foreach ($tool in @('mc.exe', 'rc.exe')) {
            if (-not (Test-Path (Join-Path $sdkTools $tool))) { throw "Windows SDK tool missing: $tool" }
        }
        $env:PATH = "$sdkTools;$env:PATH"
    }

    $vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio\Installer\vswhere.exe'
    if (Test-Path $vswhere) {
        $vsRoot = & $vswhere -latest -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
        if ($vsRoot) {
            $vcTools = Join-Path $vsRoot 'VC\Tools\MSVC'
            $vcVersion = Get-LatestVersionDirectory $vcTools
            if ($vcVersion) {
                $linkDir = Join-Path $vcVersion.FullName 'bin\Hostx64\x64'
                [void](Add-DirectoryToPathIfToolExists $linkDir 'link.exe')
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
    return [IO.Path]::GetFullPath((Join-Path $RepoRoot $Path))
}

if (-not ('NetdataWindowsCommandLine' -as [type])) {
    Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;

public static class NetdataWindowsCommandLine {
    [DllImport("shell32.dll", CharSet = CharSet.Unicode, ExactSpelling = true, SetLastError = true)]
    private static extern IntPtr CommandLineToArgvW(string commandLine, out int argumentCount);

    [DllImport("kernel32.dll")]
    private static extern IntPtr LocalFree(IntPtr memory);

    public static string[] Parse(string commandLine) {
        int count;
        // Supply argv[0] so the options use the normal argument parsing rules.
        IntPtr arguments = CommandLineToArgvW("netdata-cmake-options.exe " + commandLine, out count);
        if (arguments == IntPtr.Zero)
            throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());

        try {
            string[] result = new string[count - 1];
            for (int i = 1; i < count; i++)
                result[i - 1] = Marshal.PtrToStringUni(Marshal.ReadIntPtr(arguments, i * IntPtr.Size));
            return result;
        }
        finally {
            LocalFree(arguments);
        }
    }
}
'@
}

function ConvertFrom-WindowsCommandLine([string]$CommandLine) {
    if ([string]::IsNullOrWhiteSpace($CommandLine)) { return @() }
    return [NetdataWindowsCommandLine]::Parse($CommandLine.Trim())
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
