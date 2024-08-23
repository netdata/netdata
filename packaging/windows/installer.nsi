!include "MUI2.nsh"
!include "nsDialogs.nsh"
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
!insertmacro MUI_PAGE_LICENSE "C:\msys64\cloud.txt"
!insertmacro MUI_PAGE_LICENSE "C:\msys64\gpl-3.0.txt"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
Page Custom NetdataConfigPage NetdataConfigLeave
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH

!insertmacro MUI_LANGUAGE "English"

var hStartMsys
var startMsys

var hCloudToken
var cloudToken
var hCloudRoom
var cloudRoom

Function .onInit
        nsExec::ExecToLog '$SYSDIR\sc.exe stop Netdata'
        pop $0
        ${If} $0 == 0
            nsExec::ExecToLog '$SYSDIR\sc.exe delete Netdata'
            pop $0
        ${EndIf}

        StrCpy $startMsys ${BST_UNCHECKED}
FunctionEnd

Function NetdataConfigPage
        !insertmacro MUI_HEADER_TEXT "Netdata configuration" "Claim your agent on Netdata Cloud"

        nsDialogs::Create 1018
        Pop $0
        ${If} $0 == error
            Abort
        ${EndIf}

        ${NSD_CreateLabel} 0 0 100% 12u "Enter your Token and Cloud Room."
        ${NSD_CreateLabel} 0 15% 100% 12u "Optionally, you can open a terminal to execute additional commands."

        ${NSD_CreateLabel} 0 35% 20% 10% "Token"
        Pop $0
        ${NSD_CreateText} 21% 35% 79% 10% ""
        Pop $hCloudToken

        ${NSD_CreateLabel} 0 55% 20% 10% "Room"
        Pop $0
        ${NSD_CreateText} 21% 55% 79% 10% ""
        Pop $hCloudRoom

        ${NSD_CreateCheckbox} 0 70% 100% 10u "Open terminal"
        Pop $hStartMsys
        nsDialogs::Show
FunctionEnd

Function NetdataConfigLeave
        ${NSD_GetText} $hCloudToken $cloudToken
        ${NSD_GetText} $hCloudRoom $cloudRoom
        ${NSD_GetState} $hStartMsys $startMsys

        StrLen $0 $cloudToken
        StrLen $1 $cloudRoom
        ${If} $0 == 125
        ${AndIf} $0 == 36
                # We should start our new claiming software here
                MessageBox MB_OK "$cloudToken | $cloudRoom | $startMsys"
        ${EndIf}

        ${If} $startMsys == 1
            nsExec::ExecToLog '$INSTDIR\msys2.exe'
            pop $0
        ${EndIf}
FunctionEnd

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

