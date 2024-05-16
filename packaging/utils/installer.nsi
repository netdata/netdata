Outfile "netdata-installer.exe"
InstallDir "C:\netdata"

RequestExecutionLevel admin

Section
	SetOutPath $INSTDIR
	WriteUninstaller $INSTDIR\uninstaller.exe
SectionEnd

Section "Install MSYS2 environment"
	SetOutPath $TEMP

	SetCompress off
	File "C:\msys64\msys2-installer.exe"
    nsExec::ExecToLog 'cmd.exe /C "$TEMP\msys2-installer.exe" in --confirm-command --accept-messages --root $INSTDIR'

	Delete "$TEMP\msys2-installer.exe"
SectionEnd

Section "Install MSYS2 packages"
	ExecWait '"$INSTDIR\usr\bin\bash.exe" -lc "pacman -S --noconfirm msys/libuv msys/protobuf"'
SectionEnd

Section "Install Netdata"
	SetOutPath $INSTDIR\opt\netdata

	SetCompress off
	File /r "C:\msys64\opt\netdata\*.*"
SectionEnd

Section "Uninstall"
	nsExec::ExecToLog 'cmd.exe /C "$INSTDIR\uninstall.exe" pr --confirm-command'
SectionEnd
