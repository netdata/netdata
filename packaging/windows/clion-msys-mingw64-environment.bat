@echo off
:: In Clion Toolchains
:: 1. Add an MSYS2 UCRT64 profile
:: 2. Set Toolset to C:\msys64\ucrt64
:: 3. Add environment and set the full path to this file, like:
::    This file
:: 4. Let everything else to Bundled and auto-detected
::
set "batch_dir=%~dp0"
set "batch_dir=%batch_dir:\=/%"
set MSYSTEM=UCRT64
set GOROOT=C:\msys64\ucrt64\lib\go
set PATH="%PATH%;C:\msys64\ucrt64\bin;C:\msys64\usr\bin;C:\msys64\bin"
::set PKG_CONFIG_EXECUTABLE=C:\msys64\ucrt64\bin\pkg-config.exe
::set CMAKE_C_COMPILER=C:\msys64\ucrt64\bin\gcc.exe
::set CMAKE_CC_COMPILER=C:\msys64\ucrt64\bin\g++.exe
set PROTOBUF_PROTOC_EXECUTABLE=C:/msys64/ucrt64/bin/protoc.exe
