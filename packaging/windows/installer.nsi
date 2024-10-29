!include "MUI2.nsh"
!include "nsDialogs.nsh"
!include "FileFunc.nsh"

Name "Netdata"
Outfile "netdata-installer-x64.exe"
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
Page Custom NetdataConfigPage NetdataConfigLeave
!insertmacro MUI_PAGE_INSTFILES
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
	Quit
    launch:
!macroend

var hCtrlButton
var hStartMsys
var startMsys

var hCloudURL
var cloudURL
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
                    Quit
                ${EndIf}
        goahead:
FunctionEnd

Function un.onInit
!insertmacro SingleInstanceFile
FunctionEnd

Function ShowHelp
Pop $0
        MessageBox MB_ICONQUESTION|MB_OK "$\"Cloud URL$\" The Netdata Cloud base URL.$\n$\n$\"Proxy URL$\" set the proxy server address to use if your network requires one.$\n$\n$\"Insecure connection$\" disable verification of the server's certificate chain and host name.$\n$\n$\"Open Terminal$\" open MSYS2 terminal to run additional commands after installation." IDOK endHelp
        endHelp:
FunctionEnd

Function NetdataConfigPage
        !insertmacro MUI_HEADER_TEXT "Netdata configuration" "Connect your Agent to your Netdata Cloud Space"

        nsDialogs::Create 1018
        Pop $0
        ${If} $0 == error
            Abort
        ${EndIf}

        IfFileExists "$INSTDIR\etc\netdata\claim.conf" NotNeeded

        ${NSD_CreateLabel} 0 0 100% 12u "Enter your Space's Claim Token and the Room IDs where you want to add the Agent."
        ${NSD_CreateLabel} 0 12% 100% 12u "If no Room IDs are specified, the Agent will be added to the $\"All nodes$\" Room."

        ${NSD_CreateLabel} 0 30% 20% 10% "Claim Token"
        Pop $0
        ${NSD_CreateText} 21% 30% 79% 10% ""
        Pop $hCloudToken

        ${NSD_CreateLabel} 0 45% 20% 10% "Room ID(s)"
        Pop $0
        ${NSD_CreateText} 21% 45% 79% 10% ""
        Pop $hCloudRooms

        ${NSD_CreateLabel} 0 60% 20% 10% "Proxy URL"
        Pop $0
        ${NSD_CreateText} 21% 60% 79% 10% ""
        Pop $hProxy

        ${NSD_CreateLabel} 0 75% 20% 10% "Cloud URL"
        Pop $0
        ${NSD_CreateText} 21% 75% 79% 10% "https://app.netdata.cloud"
        Pop $hCloudURL

        ${NSD_CreateCheckbox} 0 92% 25% 10u "Insecure connection"
        Pop $hInsecure

        ${NSD_CreateCheckbox} 50% 92% 25% 10u "Open terminal"
        Pop $hStartMsys

        ${NSD_CreateButton} 90% 90% 30u 15u "&Help"
        Pop $hCtrlButton
        ${NSD_OnClick} $hCtrlButton ShowHelp

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
                ${NSD_GetText} $hCloudURL $cloudURL
                ${NSD_GetText} $hCloudRooms $cloudRooms
                ${NSD_GetText} $hProxy $proxy
                ${NSD_GetState} $hStartMsys $startMsys
                ${NSD_GetState} $hInsecure $insecure
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

Function InstallDLL
        ; Check if certutil is available
        nsExec::ExecToStack 'where certutil'
        Pop $R0
        StrCmp $R0 "" NoCertUtil FoundCertUtil

    NoCertUtil:
        DetailPrint "certutil not found, assuming files are different."
        Goto CopyDLL

    FoundCertUtil:
        ; Calculate hash of the existing DLL
        nsExec::ExecToStack 'certutil -hashfile "$SYSDIR\wevt_netdata.dll" MD5'
        Pop $R0

        ; Calculate hash of the new DLL
        nsExec::ExecToStack 'certutil -hashfile "$INSTDIR\usr\bin\wevt_netdata.dll" MD5'
        Pop $R1

        StrCmp $R0 $R1 SetPermissions

    CopyDLL:
        ClearErrors
        CopyFiles /SILENT "$INSTDIR\usr\bin\wevt_netdata.dll" "$SYSDIR"
        IfErrors RetryPrompt SetPermissions

    RetryPrompt:
        MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "Failed to copy wevt_netdata.dll probably because it is in use. Please close the Event Viewer (or other Event Log applications) and press Retry."
        StrCmp $R0 IDRETRY CopyDLL
        StrCmp $R0 IDCANCEL ExitInstall

        Goto End

    SetPermissions:
        nsExec::ExecToLog 'icacls "$SYSDIR\wevt_netdata.dll" /grant "NT SERVICE\EventLog":R'
        Goto End

    ExitInstall:
        Abort

    End:
FunctionEnd

Function InstallManifest
    IfFileExists "$INSTDIR\usr\bin\wevt_netdata_manifest.xml" CopyManifest End

    CopyManifest:
        ClearErrors
        CopyFiles /SILENT "$INSTDIR\usr\bin\wevt_netdata_manifest.xml" "$SYSDIR"
        IfErrors RetryPrompt InstallManifest

    RetryPrompt:
        MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "Failed to copy wevt_netdata_manifest.xml."
        StrCmp $R0 IDRETRY CopyManifest
        StrCmp $R0 IDCANCEL ExitInstall

    InstallManifest:
        nsExec::ExecToLog 'wevtutil im "$SYSDIR\wevt_netdata_manifest.xml" "/mf:$SYSDIR\wevt_netdata.dll" "/rf:$SYSDIR\wevt_netdata.dll"'
        Goto End

    ExitInstall:
        Abort

    End:
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
        Call InstallDLL
        Call InstallManifest

        StrLen $0 $cloudToken
        StrLen $1 $cloudRooms
        ${If} $0 == 0
        ${OrIf} $1 == 0
                Goto runCmds
        ${EndIf}

        ${If} $0 == 135
        ${AndIf} $1 >= 36
                nsExec::ExecToLog '$INSTDIR\usr\bin\NetdataClaim.exe /T $cloudToken /R $cloudRooms /P $proxy /I $insecure /U $cloudURL'
                pop $0
        ${Else}
                MessageBox MB_OK "The Cloud information does not have the expected length."
        ${EndIf}

        runCmds:
        ClearErrors
        nsExec::ExecToLog '$SYSDIR\sc.exe start Netdata'
        pop $0
        ${If} $0 != 0
	        MessageBox MB_OK "Warning: Failed to start Netdata service."
        ${EndIf}

        ${If} $startMsys == ${BST_CHECKED}
                nsExec::ExecToLog '$INSTDIR\msys2.exe'
                pop $0
        ${EndIf}
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

        ; Check if the manifest exists before uninstalling it
        IfFileExists "$SYSDIR\wevt_netdata_manifest.xml" ManifestExistsForUninstall ManifestNotExistsForUninstall

ManifestExistsForUninstall:
        nsExec::ExecToLog 'wevtutil um "$SYSDIR\wevt_netdata_manifest.xml"'
        pop $0
        ${If} $0 != 0
            DetailPrint "Warning: Failed to uninstall the event manifest."
        ${EndIf}
        Delete "$SYSDIR\wevt_netdata_manifest.xml"
        Delete "$SYSDIR\wevt_netdata.dll"
        Goto DoneUninstall

ManifestNotExistsForUninstall:
        DetailPrint "Manifest not found, skipping manifest uninstall."

DoneUninstall:

        ; Remove files
        SetOutPath "$PROGRAMFILES"
        RMDir /r /REBOOTOK "$INSTDIR"

        DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Netdata"
SectionEnd
