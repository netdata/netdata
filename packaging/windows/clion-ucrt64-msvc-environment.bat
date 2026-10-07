@echo off
:: UCRT64 environment with the Windows SDK and MSVC tools added to PATH.
set "NETDATA_CLION_UCRT64_EXTRA_PATH=C:\Program Files (x86)\Windows Kits\10\bin\10.0.26100.0\x64;C:\Program Files\Microsoft Visual Studio\2022\Community\VC\Tools\MSVC\14.39.33519\bin\Hostx64\x64"
call "%~dp0clion-ucrt64-environment.bat"
