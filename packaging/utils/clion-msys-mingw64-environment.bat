@echo off
:: In Clion Toolchains
:: 1. Add a MinGW profile
:: 2. Set Toolset to C:\msys64\mingw64
:: 3. Add environment and set the full path to this file, like:
::    C:\msys64\home\costa\src\netdata-ktsaou.git\packaging\utils\clion-mingw64-environment.bat
:: 4. Let everything else to Bundled and auto-detected
::
set "batch_dir=%~dp0"
set "batch_dir=%batch_dir:\=/%"
set MSYSTEM=MINGW64
set GOROOT=C:\msys64\mingw64
set PATH="%PATH%;C:\msys64\mingw64\bin;C:\msys64\usr\bin;C:\msys64\bin"
::set PKG_CONFIG_EXECUTABLE=C:\msys64\mingw64\bin\pkg-config.exe
::set CMAKE_C_COMPILER=C:\msys64\mingw64\bin\gcc.exe
::set CMAKE_CC_COMPILER=C:\msys64\mingw64\bin\g++.exe
set PROTOBUF_PROTOC_EXECUTABLE=C:/msys64/mingw64/bin/protoc.exe
