# Building Netdata on Windows

## Build environment setup

Netdata is built using MSYS2 on Windows. You can run the `install-dependencies.ps1` PowerShell script in this
directory to set up the required build dependencies. If MSYS2 is not installed, it will prompt you about installing it
(using CHocolatey if available, otherwise falling back to fetching, verifying, and running the MSYS2 installer itself).

If you have an existing install of MSYS2, you can skip the PowerShell script and use the `msys2-dependencies.sh`
script from this directory from an MSYS2 bash session to install the required dependencies inside MSYS2 (this
script will be called automatically by the PowerShell script).
