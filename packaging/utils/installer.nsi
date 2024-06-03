!include "MUI2.nsh"

Outfile "netdata-installer.exe"
InstallDir "$PROGRAMFILES\netdata"
RequestExecutionLevel admin

!define MUI_ABORTWARNING
!define MUI_UNABORTWARNING

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH

!insertmacro MUI_LANGUAGE "English"

Section "Install Netdata"
	SetOutPath $INSTDIR
	SetCompress off

	File /r "C:\msys64\opt\netdata\*.*"

	ClearErrors
	ExecWait '"$SYSDIR\sc.exe" create Netdata binPath="$INSTDIR\netdata.exe" start= auto'
	IfErrors 0 +2
	DetailPrint "Warning: Failed to create Netdata service."

	ClearErrors
	ExecWait '"$SYSDIR\sc.exe" start Netdata'
	IfErrors 0 +2
	DetailPrint "Warning: Failed to start Netdata service."

	WriteUninstaller "$INSTDIR\Uninstall.exe"
SectionEnd

Section "Uninstall"
	ClearErrors
	ExecWait '"$SYSDIR\sc.exe" stop Netdata'
	IfErrors 0 +2
	DetailPrint "Warning: Failed to stop Netdata service."

	ClearErrors
	ExecWait '"$SYSDIR\sc.exe" delete Netdata'
	IfErrors 0 +2
	DetailPrint "Warning: Failed to delete Netdata service."

	RMDir /r "$INSTDIR"
SectionEnd
