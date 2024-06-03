Outfile "netdata-installer.exe"
InstallDir "C:\netdata"

RequestExecutionLevel admin

Section
	SetOutPath $INSTDIR
	WriteUninstaller $INSTDIR\uninstaller.exe
SectionEnd

Section "Install Netdata"
	SetOutPath $INSTDIR

	SetCompress off
	File /r "C:\msys64\opt\netdata\*.*"
SectionEnd

Section "Uninstall"
	nsExec::ExecToLog 'cmd.exe /C "$INSTDIR\uninstall.exe" pr --confirm-command'
SectionEnd
