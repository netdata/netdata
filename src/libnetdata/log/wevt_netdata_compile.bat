@echo off
setlocal enabledelayedexpansion

echo PATH=%PATH%

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
set "BIN_DIR=%~2"
set "MC_FILE=%BIN_DIR%\wevt_netdata.mc"
set "MAN_FILE=%BIN_DIR%\wevt_netdata_manifest.xml"
set "BASE_NAME=wevt_netdata"
set "SDK_PATH=C:\Program Files (x86)\Windows Kits\10\bin\10.0.26100.0\x64"
set "VS_PATH=C:\Program Files\Microsoft Visual Studio\2022\Community\VC\Tools\MSVC\14.39.33519\bin\Hostx64\x64"

if not exist "%SRC_DIR%" (
    echo Error: Source directory does not exist.
    exit /b 1
)

if not exist "%BIN_DIR%" (
    echo Error: Destination directory does not exist.
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
cd /d "%BIN_DIR%"

echo.
echo Running mc.exe...
mc -v -b -U "%MC_FILE%" "%MAN_FILE%"
if %errorlevel% neq 0 (
    echo Error: mc.exe failed on messages.
    exit /b 1
)

if not exist "%BASE_NAME%.rc" (
    echo Error: %BASE_NAME%.rc not found.
    exit /b 1
)

echo.
echo Modifying %BASE_NAME%.rc to include the manifest...
copy "%MAN_FILE%" %BASE_NAME%_manifest.man
echo 1 2004 "%BASE_NAME%_manifest.man" >> %BASE_NAME%.rc

echo.
echo %BASE_NAME%.rc contents:
type %BASE_NAME%.rc

echo.
echo Running rc.exe...
rc /v /fo %BASE_NAME%.res %BASE_NAME%.rc
if %errorlevel% neq 0 (
    echo Error: rc.exe failed.
    exit /b 1
)

if not exist "%BASE_NAME%.res" (
    echo Error: %BASE_NAME%.res not found.
    exit /b 1
)

echo.
echo Running link.exe...
link /dll /noentry /machine:x64 /out:%BASE_NAME%.dll %BASE_NAME%.res
if %errorlevel% neq 0 (
    echo Error: link.exe failed.
    exit /b 1
)

echo.
echo Process completed successfully.
exit /b 0

:usage
echo Usage: %~nx0 [path_to_mc_file] [destination_directory]
exit /b 1
