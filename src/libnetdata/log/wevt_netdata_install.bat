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
echo Removing legacy classic Event Log registrations (if any)...
for %%K in (
    "HKLM\SYSTEM\CurrentControlSet\Services\EventLog\Netdata"
    "HKLM\SYSTEM\CurrentControlSet\Services\EventLog\Netdata/Daemon"
    "HKLM\SYSTEM\CurrentControlSet\Services\EventLog\Netdata/Collectors"
    "HKLM\SYSTEM\CurrentControlSet\Services\EventLog\Netdata/Access"
    "HKLM\SYSTEM\CurrentControlSet\Services\EventLog\Netdata/Health"
    "HKLM\SYSTEM\CurrentControlSet\Services\EventLog\Netdata/Aclk"
    "HKLM\SYSTEM\CurrentControlSet\Services\EventLog\NetdataWEL"
    "HKLM\SYSTEM\CurrentControlSet\Services\EventLog\Application\NetdataDaemon"
    "HKLM\SYSTEM\CurrentControlSet\Services\EventLog\Application\NetdataCollectors"
    "HKLM\SYSTEM\CurrentControlSet\Services\EventLog\Application\NetdataAccess"
    "HKLM\SYSTEM\CurrentControlSet\Services\EventLog\Application\NetdataHealth"
    "HKLM\SYSTEM\CurrentControlSet\Services\EventLog\Application\NetdataAclk"
) do reg delete "%%~K" /f >nul 2>nul

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
    echo Warning: ETW publisher 'Netdata' not found - this is expected on WEL-only builds.
)

echo.
echo Setting default event sizes...
wevtutil sl "Netdata/Daemon" /ms:104857600
wevtutil sl "Netdata/Collectors" /ms:104857600
wevtutil sl "Netdata/Health" /ms:104857600
wevtutil sl "Netdata/Access" /ms:104857600
wevtutil sl "Netdata/Aclk" /ms:5242880

echo.
echo Netdata Event Tracing for Windows manifest installed successfully.
exit /b 0
