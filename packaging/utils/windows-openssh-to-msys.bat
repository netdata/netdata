@echo off
:: Check if OpenSSH Server is installed
sc query sshd >nul 2>&1
if %errorlevel% neq 0 (
    echo "OpenSSH Server not found. Installing manually..."

    :: Download the latest OpenSSH release
    set DOWNLOAD_URL=https://github.com/PowerShell/Win32-OpenSSH/releases/download/v9.5.0.0p1-Beta/OpenSSH-Win64.zip
    set DOWNLOAD_FILE=%temp%\OpenSSH-Win64.zip
    set INSTALL_DIR=C:\Program Files\OpenSSH-Win64

    powershell -Command "(New-Object Net.WebClient).DownloadFile('%DOWNLOAD_URL%', '%DOWNLOAD_FILE%')"

    :: Unzip the downloaded file
    powershell -Command "Expand-Archive -Path '%DOWNLOAD_FILE%' -DestinationPath '%INSTALL_DIR%' -Force"

    :: Add the OpenSSH binaries to the system PATH
    setx /M PATH "%INSTALL_DIR%;%PATH%"

    :: Install and configure the OpenSSH service
    powershell -Command "New-Service -Name 'sshd' -BinaryPathName '%INSTALL_DIR%\sshd.exe' -StartupType Automatic"

    :: Register OpenSSH utilities as services
    %INSTALL_DIR%\install-sshd.ps1
)

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

echo "OpenSSH has been successfully configured with MSYS2 as the default shell."
pause
