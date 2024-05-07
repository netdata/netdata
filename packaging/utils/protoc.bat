@echo off
::
:: The problem with /usr/bin/protoc is that it accepts colon separated (:) paths at its parameters.
:: This makes C:/ being parsed as 2 paths: C and /, which of course both fail.
:: To overcome this problem, we use bash_execute.sh, which replaces all occurences of C: with /c.
::
set "batch_dir=%~dp0"
set "batch_dir=%batch_dir:\=/%"
C:\msys64\usr\bin\bash.exe %batch_dir%/bash_execute.sh protoc %*
