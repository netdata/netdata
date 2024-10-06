@echo off
setlocal enabledelayedexpansion

REM Check if both parameters are provided
if "%~1"=="" (
    echo Error: Missing .mc file path.
    goto :usage
)
if "%~2"=="" (
    echo Error: Missing destination directory.
    goto :usage
)

REM Set variables
set "MC_FILE=%~1"
set "DEST_DIR=%~2"
set "SDK_PATH=c:\Program Files (x86)\Windows Kits\10\bin\10.0.26100.0\x64"
set "VS_PATH=C:\Program Files\Microsoft Visual Studio\2022\Community\VC\Tools\MSVC\14.39.33519\bin\Hostx64\x64"

REM Check if required directories exist
if not exist "%SDK_PATH%" (
    echo Error: Windows SDK path not found.
    exit /b 1
)
if not exist "%VS_PATH%" (
    echo Error: Visual Studio path not found.
    exit /b 1
)

REM Add paths to PATH
set "PATH=%SDK_PATH%;%VS_PATH%;%PATH%"

REM Check if commands are available
where mc >nul 2>nul
if %errorlevel% neq 0 (
    echo Error: mc.exe not found in PATH.
    exit /b 1
)
where rc >nul 2>nul
if %errorlevel% neq 0 (
    echo Error: rc.exe not found in PATH.
    exit /b 1
)
where link >nul 2>nul
if %errorlevel% neq 0 (
    echo Error: link.exe not found in PATH.
    exit /b 1
)

REM Check if destination directory exists
if not exist "%DEST_DIR%" (
    echo Error: Destination directory does not exist.
    exit /b 1
)

REM Change to the destination directory
cd /d "%DEST_DIR%"

echo.
echo Running mc.exe...
mc "%MC_FILE%"
if %errorlevel% neq 0 (
    echo Error: mc.exe failed.
    exit /b 1
)

echo.
echo Running rc.exe...
rc nd_wevents.rc
if %errorlevel% neq 0 (
    echo Error: rc.exe failed.
    exit /b 1
)

echo.
echo Running link.exe...
link /dll /noentry /machine:x64 /out:nd_wevents.dll nd_wevents.res
if %errorlevel% neq 0 (
    echo Error: link.exe failed.
    exit /b 1
)

echo.
echo Copying nd_wevents.dll to C:\Windows\System32\...
copy /y nd_wevents.dll C:\Windows\System32
if %errorlevel% neq 0 (
    echo Error: Failed to copy nd_wevents.dll to System32.
    exit /b 1
)

echo.
echo Granting access to nd_wevents.dll for Windows Event Logging...
icacls C:\Windows\System32\nd_wevents.dll /grant "NT SERVICE\EventLog":R
if %errorlevel% neq 0 (
    echo Error: Failed to grant access to nd_wevents.dll.
    exit /b 1
)

echo.
echo Importing the manifest...
REM Construct the manifest file path
for %%F in ("%MC_FILE%") do set "MANIFEST_FILE=%%~dpnF.xml"

if not exist "%MANIFEST_FILE%" (
    echo Error: Manifest file not found: %MANIFEST_FILE%
    exit /b 1
)

wevtutil um "%MANIFEST_FILE%"
wevtutil im "%MANIFEST_FILE%"
if %errorlevel% neq 0 (
    echo Error: Failed to import the manifest.
    exit /b 1
)

wevtutil gp "Netdata"
if %errorlevel% neq 0 (
    echo Error: Failed to get publisher Netdata.
    exit /b 1
)

echo.
echo Process completed successfully.
exit /b 0

:usage
echo Usage: %~nx0 [path_to_mc_file] [destination_directory]
exit /b 1
