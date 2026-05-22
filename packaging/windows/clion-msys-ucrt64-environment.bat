@echo off
:: CLion environment for the MSYS2 UCRT64 toolchain.
::
:: This is the only supported developer environment for the Windows build.
:: The MSYS and MINGW64 variants have been removed (see SOW-0033).
::
:: In CLion Toolchains:
:: 1. Add a MinGW profile.
:: 2. Set Toolset to C:\msys64\ucrt64.
:: 3. Add Environment and set the full path to this file, e.g.:
::    C:\msys64\home\<user>\src\netdata\packaging\windows\clion-msys-ucrt64-environment.bat
:: 4. Leave everything else Bundled and auto-detected.
::
set "batch_dir=%~dp0"
set "batch_dir=%batch_dir:\=/%"
set MSYSTEM=UCRT64
set GOROOT=C:\msys64\ucrt64
set "PATH=%PATH%;C:\msys64\ucrt64\bin;C:\msys64\usr\bin;C:\msys64\bin"
set PROTOBUF_PROTOC_EXECUTABLE=C:/msys64/ucrt64/bin/protoc.exe
