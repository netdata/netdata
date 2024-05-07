@echo off
set MSYSTEM=MSYS
set PATH=c:\msys64\usr\bin;%PATH%"

set "batch_dir=%~dp0"
set "batch_dir=%batch_dir:\=/%"
bash %batch_dir%/bash_execute.sh protoc %*
