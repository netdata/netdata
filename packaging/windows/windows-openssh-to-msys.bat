@echo off
::
:: This script will:
::
:: 1. install the windows OpenSSH server (either via dsim or download it)
:: 2. activate the windows OpenSSH service
:: 3. open OpenSSH TCP port at windows firewall
:: 4. create a small batch file to start an MSYS session
:: 5. Set the default OpenSSH startup script to start the MSYS session
::
:: Problems:
:: On older windows versions, terminal emulation is broken.
:: So, on windows 10 or windows server before 2019, the ssh session
:: will not have proper terminal emulation and will be not be able to
:: be used for editing files.
:: For more info check:
:: https://github.com/PowerShell/Win32-OpenSSH/issues/1260
::

:: Check if OpenSSH Server is already installed
sc query sshd >nul 2>&1
if %errorlevel% neq 0 (
    echo "OpenSSH Server not found. Attempting to install via dism..."
    goto :install_openssh_dism
) else (
    echo "OpenSSH Server is already installed."
    goto :configure_openssh
)

:: Install OpenSSH using dism
:install_openssh_dism
dism /online /Enable-Feature /FeatureName:OpenSSH-Client /All >nul 2>&1
dism /online /Enable-Feature /FeatureName:OpenSSH-Server /All >nul 2>&1

:: Check if dism succeeded in installing OpenSSH
sc query sshd >nul 2>&1
if %errorlevel% neq 0 (
    echo "OpenSSH installation via dism failed or is unavailable."
    goto :install_openssh_manual
) else (
    echo "OpenSSH installed successfully using dism."
    goto :configure_openssh
)

:: Function to Install OpenSSH manually if dism fails
:install_openssh_manual
echo "Installing OpenSSH manually..."

:: Download the latest OpenSSH release
set DOWNLOAD_URL=https://github.com/PowerShell/Win32-OpenSSH/releases/download/v9.5.0.0p1-Beta/OpenSSH-Win64.zip
set DOWNLOAD_FILE=%temp%\OpenSSH-Win64.zip
set INSTALL_DIR=C:\Program Files\OpenSSH-Win64

:: Create the installation directory if it doesn't exist
if not exist "%INSTALL_DIR%" mkdir "%INSTALL_DIR%"

:: Attempt to download OpenSSH using Invoke-WebRequest and TLS configuration
powershell -Command "[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12; try { Invoke-WebRequest -Uri '%DOWNLOAD_URL%' -OutFile '%DOWNLOAD_FILE%' -UseBasicParsing; exit 0 } catch { exit 1 }"
if %errorlevel% neq 0 (
    echo "Invoke-WebRequest download failed. Attempting to download using curl..."
    curl -L -o "%DOWNLOAD_FILE%" "%DOWNLOAD_URL%"
    if %errorlevel% neq 0 (
        echo "Failed to download OpenSSH using curl. Exiting..."
        exit /b 1
    )
)

:: Unzip directly to INSTALL_DIR (flatten the folder structure)
powershell -Command "Expand-Archive -Path '%DOWNLOAD_FILE%' -DestinationPath '%INSTALL_DIR%' -Force"
if %errorlevel% neq 0 (
    echo "Failed to unzip OpenSSH package."
    exit /b 1
)

:: Move inner contents to INSTALL_DIR if nested OpenSSH-Win64 folder exists
if exist "%INSTALL_DIR%\OpenSSH-Win64" (
    xcopy "%INSTALL_DIR%\OpenSSH-Win64\*" "%INSTALL_DIR%\" /s /e /y
    rmdir "%INSTALL_DIR%\OpenSSH-Win64" /s /q
)

:: Add the OpenSSH binaries to the system PATH
setx /M PATH "%INSTALL_DIR%;%PATH%"

:: Register OpenSSH utilities as services using PowerShell
powershell -ExecutionPolicy Bypass -Command "& '%INSTALL_DIR%\install-sshd.ps1'"

:: Verify if manual installation succeeded
sc query sshd >nul 2>&1
if %errorlevel% neq 0 (
    echo "Manual OpenSSH installation failed. Exiting..."
    exit /b 1
) else (
    echo "OpenSSH installed successfully manually."
    goto :configure_openssh
)

:configure_openssh
:: Ensure OpenSSH Server service is set to start automatically and start the service
sc config sshd start= auto
net start sshd

:: Create msys2.bat file with specific content
set MSYS2_PATH=C:\msys64
if not exist "%MSYS2_PATH%" (
    echo "Error: %MSYS2_PATH% does not exist."
    exit /b 1
)

echo @%MSYS2_PATH%\msys2_shell.cmd -defterm -here -no-start -msys > %MSYS2_PATH%\msys2.bat

:: Run PowerShell command to set default shell
powershell -Command "New-ItemProperty -Path 'HKLM:\SOFTWARE\OpenSSH' -Name 'DefaultShell' -Value '%MSYS2_PATH%\msys2.bat' -PropertyType String -Force"

:: Open the Windows Firewall for sshd (using PowerShell)
powershell -Command "New-NetFirewallRule -Name 'OpenSSH-Server-In-TCP' -DisplayName 'OpenSSH Server (sshd) Incoming' -Description 'Allow incoming SSH traffic via OpenSSH server' -Enabled True -Direction Inbound -Protocol TCP -LocalPort 22 -Action Allow"

echo "OpenSSH has been successfully configured with MSYS2 as the default shell, and the firewall has been opened for sshd."
pause
