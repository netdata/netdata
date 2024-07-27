!include "MUI2.nsh"
!include "FileFunc.nsh"

Name "Netdata"
Outfile "netdata-installer.exe"
InstallDir "$PROGRAMFILES\Netdata"
RequestExecutionLevel admin

!define MUI_ICON "NetdataWhite.ico"
!define MUI_UNICON "NetdataWhite.ico"

!define ND_UININSTALL_REG "Software\Microsoft\Windows\CurrentVersion\Uninstall\Netdata"

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

Function NetdataUninstallRegistry
        WriteRegStr HKLM "${ND_UININSTALL_REG}" \
                         "DisplayName" "Netdata - Real-time system monitoring."
        WriteRegStr HKLM "${ND_UININSTALL_REG}" \
                         "DisplayIcon" "$INSTDIR\Uninstall.exe,0"
        WriteRegStr HKLM "${ND_UININSTALL_REG}" \
                         "UninstallString" "$INSTDIR\Uninstall.exe"
        WriteRegStr HKLM "${ND_UININSTALL_REG}" \
                         "RegOwner" "Netdata Inc."
        WriteRegStr HKLM "${ND_UININSTALL_REG}" \
                         "RegCompany" "Netdata Inc."
        WriteRegStr HKLM "${ND_UININSTALL_REG}" \
                         "Publisher" "Netdata Inc."
        WriteRegStr HKLM "${ND_UININSTALL_REG}" \
                         "HelpLink" "https://learn.netdata.cloud/"
        WriteRegStr HKLM "${ND_UININSTALL_REG}" \
                         "URLInfoAbout" "https://www.netdata.cloud/"
        WriteRegStr HKLM "${ND_UININSTALL_REG}" \
                         "DisplayVersion" "${CURRVERSION}"
        WriteRegStr HKLM "${ND_UININSTALL_REG}" \
                         "VersionMajor" "${MAJORVERSION}"
        WriteRegStr HKLM "${ND_UININSTALL_REG}" \
                         "VersionMinor" "${MINORVERSION}"

        ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
        IntFmt $0 "0x%08X" $0
        WriteRegDWORD HKLM "${ND_UININSTALL_REG}" "EstimatedSize" "$0"
FunctionEnd

Section "Install Netdata"
	SetOutPath $INSTDIR
	SetCompress off

	File /r "C:\msys64\opt\netdata\*.*"

	ClearErrors
	ExecWait '"$SYSDIR\sc.exe" create Netdata binPath= "$INSTDIR\usr\bin\netdata.exe" start= delayed-auto'
	IfErrors 0 +2
	DetailPrint "Warning: Failed to create Netdata service."

	ClearErrors
	ExecWait '"$SYSDIR\sc.exe" description Netdata "Real-time system monitoring service"'
	IfErrors 0 +2
	DetailPrint "Warning: Failed to add Netdata service description."

	ClearErrors
	ExecWait '"$SYSDIR\sc.exe" start Netdata'
	IfErrors 0 +2
	DetailPrint "Warning: Failed to start Netdata service."

	WriteUninstaller "$INSTDIR\Uninstall.exe"

        Call NetdataUninstallRegistry
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

        DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Netdata"
SectionEnd

