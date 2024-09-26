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

!define INSTALLERLOCKFILEGUID "f787d5ef-5c41-4dc0-a115-a1fb654fad1c"

# https://nsis.sourceforge.io/Allow_only_one_installer_instance
!macro SingleInstanceFile
    !if "${NSIS_PTR_SIZE}" > 4
    !include "Util.nsh"
    !else ifndef IntPtrCmp
    !define IntPtrCmp IntCmp
    !endif

    !ifndef NSIS_PTR_SIZE & SYSTYPE_PTR
    !define SYSTYPE_PTR i ; NSIS v2.x
    !else
    !define /ifndef SYSTYPE_PTR p ; NSIS v3.0+
    !endif

    !if "${NSIS_CHAR_SIZE}" < 2
    Push "$TEMP\${INSTALLERLOCKFILEGUID}.lock"
    !else
    Push "$APPDATA\${INSTALLERLOCKFILEGUID}.lock"
    !endif

    System::Call 'KERNEL32::CreateFile(ts,i0x40000000,i0,${SYSTYPE_PTR}0,i4,i0x04000000,${SYSTYPE_PTR}0)${SYSTYPE_PTR}.r0'
    ${IntPtrCmp} $0 -1 "" launch launch
        System::Call 'kernel32::AttachConsole(i -1)i.r0'
        ${If} $0 != 0
            System::Call 'kernel32::GetStdHandle(i -11)i.r0'
            FileWrite $0 "The installer is already running.$\r$\n"
        ${EndIf}
	Abort
    launch:
!macroend

var hStartMsys
var startMsys

var hCloudToken
var cloudToken
var hCloudRooms
var cloudRooms
var hProxy
var proxy
var hInsecure
var insecure
var accepted

var avoidClaim

Function .onInit
        !insertmacro SingleInstanceFile

        nsExec::ExecToLog '$SYSDIR\sc.exe stop Netdata'
        pop $0
        ${If} $0 == 0
            nsExec::ExecToLog '$SYSDIR\sc.exe delete Netdata'
            pop $0
        ${EndIf}

        StrCpy $startMsys ${BST_UNCHECKED}
        StrCpy $insecure ${BST_UNCHECKED}
        StrCpy $avoidClaim ${BST_UNCHECKED}
        StrCpy $accepted ${BST_UNCHECKED}
        
        ${GetParameters} $R0
        ${GetOptions} $R0 "/s" $0
        IfErrors +2 0
            SetSilent silent
        ClearErrors

        ${GetOptions} $R0 "/t" $0
        IfErrors +2 0
            StrCpy $startMsys ${BST_CHECKED}
        ClearErrors

        ${GetOptions} $R0 "/i" $0
        IfErrors +2 0
            StrCpy $insecure ${BST_CHECKED}
        ClearErrors

        ${GetOptions} $R0 "/a" $0
        IfErrors +2 0
            StrCpy $accepted ${BST_CHECKED}
        ClearErrors

        ${GetOptions} $R0 "/token=" $0
        IfErrors +2 0
            StrCpy $cloudToken $0
        ClearErrors

        ${GetOptions} $R0 "/rooms=" $0
        IfErrors +2 0
            StrCpy $cloudRooms $0
        ClearErrors

        ${GetOptions} $R0 "/proxy=" $0
        IfErrors +2 0
            StrCpy $proxy $0
        ClearErrors

        IfSilent checklicense goahead
        checklicense:
                ${If} $accepted == ${BST_UNCHECKED}
                    System::Call 'kernel32::AttachConsole(i -1)i.r0'
                    ${If} $0 != 0
                        System::Call 'kernel32::GetStdHandle(i -11)i.r0'
                        FileWrite $0 "You must accept the licenses (/A) to continue.$\r$\n"
                    ${EndIf}
                    Abort
                ${EndIf}
        goahead:
FunctionEnd

Function un.onInit
!insertmacro SingleInstanceFile
FunctionEnd

Function NetdataConfigPage
        !insertmacro MUI_HEADER_TEXT "Netdata configuration" "Claim your agent on Netdata Cloud"

        nsDialogs::Create 1018
        Pop $0
        ${If} $0 == error
            Abort
        ${EndIf}

        IfFileExists "$INSTDIR\etc\netdata\claim.conf" NotNeeded

        ${NSD_CreateLabel} 0 0 100% 12u "Enter your Token and Cloud Room(s)."
        ${NSD_CreateLabel} 0 15% 100% 12u "Optionally, you can open a terminal to execute additional commands."

        ${NSD_CreateLabel} 0 30% 20% 10% "Token"
        Pop $0
        ${NSD_CreateText} 21% 30% 79% 10% ""
        Pop $hCloudToken

        ${NSD_CreateLabel} 0 45% 20% 10% "Room(s)"
        Pop $0
        ${NSD_CreateText} 21% 45% 79% 10% ""
        Pop $hCloudRooms

        ${NSD_CreateLabel} 0 60% 20% 10% "Proxy"
        Pop $0
        ${NSD_CreateText} 21% 60% 79% 10% ""
        Pop $hProxy

        ${NSD_CreateCheckbox} 0 75% 100% 10u "Insecure connection"
        Pop $hInsecure

        ${NSD_CreateCheckbox} 0 90% 100% 10u "Open terminal"
        Pop $hStartMsys
        Goto EndDialogDraw

        NotNeeded:
        StrCpy $avoidClaim ${BST_CHECKED}
        ${NSD_CreateLabel} 0 0 100% 12u "Your host has already been claimed. You can proceed with the update."

        EndDialogDraw:
        nsDialogs::Show
FunctionEnd

Function NetdataConfigLeave
        ${If} $avoidClaim == ${BST_UNCHECKED}
                ${NSD_GetText} $hCloudToken $cloudToken
                ${NSD_GetText} $hCloudRooms $cloudRooms
                ${NSD_GetText} $hProxy $proxy
                ${NSD_GetState} $hStartMsys $startMsys
                ${NSD_GetState} $hInsecure $insecure

                StrLen $0 $cloudToken
                StrLen $1 $cloudRooms
                ${If} $0 == 0
                ${OrIf} $1 == 0
                        Goto runMsys
                ${EndIf}

                ${If} $0 == 135
                ${AndIf} $1 >= 36
                        nsExec::ExecToLog '$INSTDIR\usr\bin\NetdataClaim.exe /T $cloudToken /R $cloudRooms /P $proxy /I $insecure'
                        pop $0
                ${Else}
                        MessageBox MB_OK "The Cloud information does not have the expected length."
                ${EndIf}

                runMsys:
                ${If} $startMsys == ${BST_CHECKED}
                        nsExec::ExecToLog '$INSTDIR\msys2.exe'
                        pop $0
                ${EndIf}
        ${EndIf}

        ClearErrors
        nsExec::ExecToLog '$SYSDIR\sc.exe start Netdata'
        pop $0
        ${If} $0 != 0
	        MessageBox MB_OK "Warning: Failed to start Netdata service."
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

	WriteUninstaller "$INSTDIR\Uninstall.exe"

        Call NetdataUninstallRegistry

        IfSilent runcmds goodbye
        runcmds:
           nsExec::ExecToLog '$SYSDIR\sc.exe start Netdata'
           pop $0

           System::Call 'kernel32::AttachConsole(i -1)i.r0'
           ${If} $0 != 0
                System::Call 'kernel32::GetStdHandle(i -11)i.r0'
                FileWrite $0 "Netdata installed with success.$\r$\n"
           ${EndIf}
           ${If} $startMsys == ${BST_CHECKED}
                   nsExec::ExecToLog '$INSTDIR\msys2.exe'
                   pop $0
           ${EndIf}

           StrLen $0 $cloudToken
           StrLen $1 $cloudRooms
           ${If} $0 == 0
           ${OrIf} $1 == 0
                   Goto goodbye
           ${EndIf}

           ${If} $0 == 135
           ${AndIf} $1 >= 36
                    nsExec::ExecToLog '$INSTDIR\usr\bin\NetdataClaim.exe /T $cloudToken /R $cloudRooms /P $proxy /I $insecure'
                    pop $0
           ${Else}         
                    System::Call 'kernel32::AttachConsole(i -1)i.r0'
                    ${If} $0 != 0
                        System::Call 'kernel32::GetStdHandle(i -11)i.r0'
                        FileWrite $0 "Room(s) or Token invalid.$\r$\n"
                    ${EndIf}
           ${EndIf}
        goodbye:
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

