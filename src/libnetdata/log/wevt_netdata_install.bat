@echo off
setlocal enabledelayedexpansion

set "MAN_SRC=%~dp0wevt_netdata_manifest.xml"
set "DLL_SRC=%~dp0wevt_netdata.dll"
set "DLL_DST=%SystemRoot%\System32\wevt_netdata.dll"

where wevtutil >nul 2>nul
if %errorlevel% neq 0 (
    echo Error: wevtutil.exe not found in PATH.
    exit /b 1
)

echo.
echo Uninstalling previous manifest (if any)...
wevtutil um "%MAN_SRC%"

echo.
echo Copying %DLL_SRC% to %DLL_DST%
copy /y "%DLL_SRC%" "%DLL_DST%"
if %errorlevel% neq 0 (
    echo Error: Failed to copy %DLL_SRC% to %DLL_DST%
    exit /b 1
)

echo.
echo Granting access to %DLL_DST% for Windows Event Logging...
icacls "%DLL_DST%" /grant "NT SERVICE\EventLog":R
if %errorlevel% neq 0 (
    echo Error: Failed to grant access to %DLL_DST%.
    exit /b 1
)

echo.
echo Importing the manifest...
wevtutil im "%MAN_SRC%" /rf:"%DLL_DST%" /mf:"%DLL_DST%"
if %errorlevel% neq 0 (
    echo Error: Failed to import the manifest.
    exit /b 1
)

echo.
echo Verifying Netdata Publisher for Event Tracing for Windows (ETW)...
wevtutil gp "Netdata"
if %errorlevel% neq 0 (
    echo Error: Failed to get publisher Netdata.
    exit /b 1
)

echo.
echo Netdata Event Tracing for Windows manifest installed successfully.
exit /b 0
