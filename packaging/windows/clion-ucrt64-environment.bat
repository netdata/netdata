@echo off
:: In CLion, select the native UCRT64 toolchain and use this file as its environment.
set "batch_dir=%~dp0"
set "batch_dir=%batch_dir:\=/%"
set MSYSTEM=UCRT64
set GOROOT=C:\msys64\ucrt64\lib\go

if defined NETDATA_CLION_UCRT64_EXTRA_PATH (
    set "PATH=C:\msys64\ucrt64\bin;%PATH%;%NETDATA_CLION_UCRT64_EXTRA_PATH%"
    set "NETDATA_CLION_UCRT64_EXTRA_PATH="
) else (
    set "PATH=C:\msys64\ucrt64\bin;%PATH%"
)
