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
set "SRC_DIR=%~1"
set "DEST_DIR=%~2"
set "MC_FILE=%SRC_DIR%\nd_wevents.mc"
set "MAN_FILE=%SRC_DIR%\nd_wevents_manifest.xml"
set "SDK_PATH=C:\Program Files (x86)\Windows Kits\10\bin\10.0.26100.0\x64"
set "VS_PATH=C:\Program Files\Microsoft Visual Studio\2022\Community\VC\Tools\MSVC\14.39.33519\bin\Hostx64\x64"

if not exist "%SDK_PATH%" (
    echo Error: Windows SDK path not found.
    exit /b 1
)
if not exist "%VS_PATH%" (
    echo Error: Visual Studio path not found.
    exit /b 1
)

if not exist "%MC_FILE%" (
    echo Error: %MC_FILE% not found.
    exit /b 1
)

if not exist "%MAN_FILE%" (
    echo Error: %MAN_FILE% not found.
    exit /b 1
)

if not exist "%DEST_DIR%" (
    echo Error: Destination directory does not exist.
    exit /b 1
)

REM Add SDK paths to PATH
set "PATH=C:\Windows\System32;%SDK_PATH%;%VS_PATH%;%PATH%"

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
where wevtutil >nul 2>nul
if %errorlevel% neq 0 (
    echo Error: wevtutil.exe not found in PATH.
    exit /b 1
)

REM Change to the destination directory
cd /d "%DEST_DIR%"

echo.
echo Running mc.exe...
mc /v -U "%MC_FILE%" "%MAN_FILE%"
if %errorlevel% neq 0 (
    echo Error: mc.exe failed on messages.
    exit /b 1
)

echo.
echo Modifying nd_wevents.rc to include the manifest...
copy "%MAN_FILE%" nd_wevents_manifest.man
echo 1 2004 "nd_wevents_manifest.man" >> nd_wevents.rc

echo.
echo nd_wevents.rc contents:
type nd_wevents.rc

echo.
echo Running rc.exe...
rc /v /fo nd_wevents.res nd_wevents.rc
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
echo Copying nd_wevents.dll to %SystemRoot%\System32\...
copy /y nd_wevents.dll "%SystemRoot%\System32"
if %errorlevel% neq 0 (
    echo Error: Failed to copy nd_wevents.dll to System32.
    exit /b 1
)

echo.
echo Granting access to nd_wevents.dll for Windows Event Logging...
icacls %SystemRoot%\System32\nd_wevents.dll /grant "NT SERVICE\EventLog":R
if %errorlevel% neq 0 (
    echo Error: Failed to grant access to nd_wevents.dll.
    exit /b 1
)

echo.
echo Uninstalling previous manifest (if any)...
wevtutil um "%MAN_FILE%"

echo.
echo Importing the manifest...
wevtutil im "%MAN_FILE%" /rf:"%SystemRoot%\System32\nd_wevents.dll" /mf:"%SystemRoot%\System32\nd_wevents.dll"
if %errorlevel% neq 0 (
    echo Error: Failed to import the manifest.
    exit /b 1
)

echo.
echo Verifying publisher...
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
