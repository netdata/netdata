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
        ClearErrors
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

        IfErrors 0 +2
        MessageBox MB_ICONEXCLAMATION|MB_OK "Unable to create an entry in the Control Panel!" IDOK end

        ClearErrors
        ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
        IntFmt $0 "0x%08X" $0
        WriteRegDWORD HKLM "${ND_UININSTALL_REG}" "EstimatedSize" "$0"

        IfErrors 0 +2
        MessageBox MB_ICONEXCLAMATION|MB_OK "Cannot estimate the installation size." IDOK end
        end:
FunctionEnd

Section "Install Netdata"
	SetOutPath $INSTDIR
	SetCompress off

	File /r "C:\msys64\opt\netdata\*.*"

	ClearErrors
        nsExec::ExecToLog '$SYSDIR\sc.exe create Netdata binPath= "$INSTDIR\usr\bin\netdata.exe" start= delayed-auto'
        pop $0
        ${If} $0 != 0
	    DetailPrint "Warning: Failed to create Netdata service."
        ${EndIf}

	ClearErrors
        nsExec::ExecToLog '$SYSDIR\sc.exe description Netdata "Real-time system monitoring service"'
        pop $0
        ${If} $0 != 0
	    DetailPrint "Warning: Failed to add Netdata service description."
        ${EndIf}

	ClearErrors
        nsExec::ExecToLog '$SYSDIR\sc.exe start Netdata'
        pop $0
        ${If} $0 != 0
	    DetailPrint "Warning: Failed to start Netdata service."
        ${EndIf}

	WriteUninstaller "$INSTDIR\Uninstall.exe"

        Call NetdataUninstallRegistry
SectionEnd

Section "Uninstall"
	ClearErrors
        nsExec::ExecToLog '$SYSDIR\sc.exe stop Netdata'
        pop $0
        ${If} $0 != 0
	    DetailPrint "Warning: Failed to stop Netdata service."
        ${EndIf}

	ClearErrors
        nsExec::ExecToLog '$SYSDIR\sc.exe delete Netdata'
        pop $0
        ${If} $0 != 0
	    DetailPrint "Warning: Failed to delete Netdata service."
        ${EndIf}

	RMDir /r "$INSTDIR"

        DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Netdata"
SectionEnd

