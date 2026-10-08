; MyGO-Clash Windows installer.
;
; Built by tools/winsetup, which passes the build's facts as defines and
; writes the file lists next to it (see the list below). It replaces the
; installer that `mygo build` makes, and keeps what that one does, so that
; installs of both kinds update, repair and uninstall each other:
;
;   - a per-user install in %LOCALAPPDATA%\Programs\MyGO-Clash, no
;     administrator needed, where the app updates itself in place;
;   - the same uninstall entry (HKCU\...\Uninstall\<identifier>), Start menu
;     shortcut, URL schemes and Uninstall.exe.
;
; On top of that it asks for the language first, finds an existing install
; and offers to repair, upgrade or remove it, closes a running app before
; copying files, and its uninstaller can remove the privileged service and
; the app's data.
;
; Defines (all required unless noted):
;   PRODUCT_NAME   MyGO-Clash
;   IDENTIFIER     io.mygo.clash        the uninstall entry's name
;   VERSION        0.2.0-beta
;   VI_VERSION     0.2.0.0              numeric, for the file properties
;   ARCH           amd64 or arm64
;   MAIN_EXE       MyGO-Clash.exe
;   SERVICE_NAME   mygo-clash-service   the privileged service
;   SERVICE_SLUG   mygo-clash           its --name argument
;   COPYRIGHT, PUBLISHER, ESTIMATED_KB
;   OUTFILE        the installer to write
;   WORKDIR        holds files.nsh and schemes.nsh, written by the tool
;   ICON, HEADER_BMP, WELCOME_BMP        branding
;   SIGN_CMD       (optional) signs the uninstaller; %1 is the file

Unicode true
ManifestDPIAware true
SetCompressor /SOLID lzma
RequestExecutionLevel user

!define UNINSTKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${IDENTIFIER}"
!define APPKEY "Software\${PRODUCT_NAME}"
!define RUNKEY "Software\Microsoft\Windows\CurrentVersion\Run"
!define APPROVEDKEY "Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run"

Name "${PRODUCT_NAME}"
OutFile "${OUTFILE}"
InstallDir "$LOCALAPPDATA\Programs\${PRODUCT_NAME}"
BrandingText "${PRODUCT_NAME} ${VERSION}"

VIProductVersion "${VI_VERSION}"
VIAddVersionKey "ProductName" "${PRODUCT_NAME}"
VIAddVersionKey "FileDescription" "${PRODUCT_NAME} Setup (${ARCH})"
VIAddVersionKey "LegalCopyright" "${COPYRIGHT}"
VIAddVersionKey "CompanyName" "${PUBLISHER}"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "ProductVersion" "${VERSION}"

; makensis writes the uninstaller to a temporary file, runs the command
; with it as %1 and puts the signed file into the installer. "= 0" stops
; the build when signing fails.
!ifdef SIGN_CMD
  !uninstfinalize '${SIGN_CMD}' = 0
!endif

!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"
!include "nsDialogs.nsh"
!include "WinMessages.nsh"

!insertmacro un.GetParameters
!insertmacro un.GetOptions

Var /GLOBAL OldVer        ; the installed version, "" when unknown
Var /GLOBAL HasOld        ; 1 when an install was found
Var /GLOBAL CmpResult     ; this version against the installed one: 1, 0, -1
Var /GLOBAL CleanInstall  ; 1 to remove the old program files first
Var /GLOBAL ReinstChoice  ; 1 or 2: the radio button of the reinstall page
Var /GLOBAL RadioA
Var /GLOBAL RadioB
Var /GLOBAL DeleteDataBox
Var /GLOBAL DeleteData    ; 1 to delete the app's data when uninstalling

!include "version.nsh"

; ---- Look ----
!define MUI_ICON "${ICON}"
!define MUI_UNICON "${ICON}"
!define MUI_ABORTWARNING
!define MUI_UNABORTWARNING
!define MUI_HEADERIMAGE
!define MUI_HEADERIMAGE_BITMAP "${HEADER_BMP}"
!define MUI_HEADERIMAGE_UNBITMAP "${HEADER_BMP}"
!define MUI_WELCOMEFINISHPAGE_BITMAP "${WELCOME_BMP}"

; ---- Language: the first thing that is asked, remembered for the uninstaller ----
!define MUI_LANGDLL_REGISTRY_ROOT "HKCU"
!define MUI_LANGDLL_REGISTRY_KEY "${APPKEY}"
!define MUI_LANGDLL_REGISTRY_VALUENAME "Installer Language"
!define MUI_LANGDLL_ALWAYSSHOW
!define MUI_LANGDLL_ALLLANGUAGES

; ---- Install pages ----
!insertmacro MUI_PAGE_WELCOME
Page custom PageReinstall PageReinstallLeave

!define MUI_PAGE_CUSTOMFUNCTION_PRE SkipDirectoryPage
!insertmacro MUI_PAGE_DIRECTORY

Page custom PageCloseApp

!insertmacro MUI_PAGE_INSTFILES

!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_FUNCTION LaunchApp
!define MUI_FINISHPAGE_SHOWREADME
!define MUI_FINISHPAGE_SHOWREADME_TEXT "$(DesktopShortcut)"
!define MUI_FINISHPAGE_SHOWREADME_FUNCTION CreateDesktopShortcut
!insertmacro MUI_PAGE_FINISH

; ---- Uninstall pages ----
!define MUI_PAGE_CUSTOMFUNCTION_SHOW un.ConfirmShow
!define MUI_PAGE_CUSTOMFUNCTION_LEAVE un.ConfirmLeave
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

; ---- Languages: English first, the fallback ----
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "TradChinese"
!insertmacro MUI_LANGUAGE "Japanese"
!insertmacro MUI_LANGUAGE "Korean"
!insertmacro MUI_LANGUAGE "Russian"
!insertmacro MUI_LANGUAGE "German"
!insertmacro MUI_LANGUAGE "French"
!insertmacro MUI_LANGUAGE "Spanish"
!insertmacro MUI_LANGUAGE "PortugueseBR"
!insertmacro MUI_LANGUAGE "Italian"
!insertmacro MUI_LANGUAGE "Turkish"
!insertmacro MUI_LANGUAGE "Vietnamese"
!insertmacro MUI_LANGUAGE "Indonesian"
!insertmacro MUI_LANGUAGE "Farsi"
!insertmacro MUI_LANGUAGE "Arabic"

!include "strings.nsh"
!include "${WORKDIR}\schemes.nsh"

; ---------------------------------------------------------------------------
; Closing the app. The app and its core are the same executable, and nothing
; of the app runs from anywhere else: the privileged service runs a copy in
; Program Files, so it never locks these files.
;
; The app is asked to quit first, so that it puts the system proxy back; a
; version that does not understand the request is killed after a few seconds.
; Only processes of the current user are killed, which is all a user-level
; process can kill anyway.

!macro CloseAppFunctions un

; $0 = 1 when the app's executable is in use, which is how to tell that
; the app of this folder is running, whoever's it is.
Function ${un}AppRunning
  StrCpy $0 0
  ${IfNot} ${FileExists} "$INSTDIR\${MAIN_EXE}"
    Return
  ${EndIf}
  ClearErrors
  FileOpen $1 "$INSTDIR\${MAIN_EXE}" a
  ${If} ${Errors}
    StrCpy $0 1
  ${Else}
    FileClose $1
  ${EndIf}
FunctionEnd

; Waits up to $2 half seconds for the app to be gone; $0 = 1 if it is not.
Function ${un}WaitAppGone
  StrCpy $3 0
  wag_loop:
    Call ${un}AppRunning
    StrCmp $0 0 wag_done
    IntOp $3 $3 + 1
    IntCmp $3 $2 wag_done 0 wag_done
    Sleep 500
    Goto wag_loop
  wag_done:
FunctionEnd

Function ${un}EnsureAppClosed
  Push $0
  Push $1
  Push $2
  Push $3
  Call ${un}AppRunning
  StrCmp $0 0 eac_done
  ${IfNot} ${Silent}
    MessageBox MB_OKCANCEL|MB_ICONEXCLAMATION "$(${un}RunningMsg)" IDOK eac_close
    Quit
  ${EndIf}
  eac_close:
  ; Ask it to quit, which makes a second start hand over `--quit`.
  Exec '"$INSTDIR\${MAIN_EXE}" --quit'
  StrCpy $2 20
  Call ${un}WaitAppGone
  StrCmp $0 0 eac_done
  eac_kill:
    ; /T takes the core with it. The filter keeps to the user's own processes.
    ExpandEnvStrings $1 "%USERDOMAIN%\%USERNAME%"
    nsExec::Exec '"$SYSDIR\taskkill.exe" /F /T /IM "${MAIN_EXE}" /FI "USERNAME eq $1"'
    Pop $1
    StrCpy $2 10
    Call ${un}WaitAppGone
    StrCmp $0 0 eac_done
    ${If} ${Silent}
      SetErrorLevel 5
      Quit
    ${EndIf}
    MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "$(${un}CloseFailed)" IDRETRY eac_kill
    Quit
  eac_done:
  Pop $3
  Pop $2
  Pop $1
  Pop $0
FunctionEnd

!macroend

!insertmacro CloseAppFunctions ""
!insertmacro CloseAppFunctions "un."

; ---------------------------------------------------------------------------
; Installer

Function .onInit
  StrCpy $HasOld 0
  StrCpy $CleanInstall 0
  StrCpy $ReinstChoice 1
  StrCpy $OldVer ""

  ; Silent installs (/S) skip the dialog.
  !insertmacro MUI_LANGDLL_DISPLAY

  ; An install of ours, from this installer or from mygo's: update it where
  ; it is, so that the app keeps updating itself there.
  ReadRegStr $0 HKCU "${UNINSTKEY}" "InstallLocation"
  ${If} $0 != ""
    StrCpy $INSTDIR $0
    ${If} ${FileExists} "$0\${MAIN_EXE}"
      StrCpy $HasOld 1
      ; The app updates itself without the installer, so the registry may
      ; be behind: the executable knows its version.
      StrCpy $VStr "$0\${MAIN_EXE}"
      Call ReadExeVersion
      StrCpy $OldVer $VStr
      ${If} $OldVer == ""
        ReadRegStr $OldVer HKCU "${UNINSTKEY}" "DisplayVersion"
      ${EndIf}
    ${EndIf}
  ${EndIf}
FunctionEnd

Function SkipDirectoryPage
  ${If} $HasOld = 1
    Abort
  ${EndIf}
FunctionEnd

; The page of an existing install: repair or uninstall for the same
; version, upgrade for an older one, downgrade for a newer one. Silent
; installs skip it and install over it.
Function PageReinstall
  ${If} $HasOld = 0
  ${OrIf} ${Silent}
    Abort
  ${EndIf}

  ${If} $OldVer == ""
    StrCpy $CmpResult 1
  ${Else}
    StrCpy $CmpA "${VERSION}"
    StrCpy $CmpB $OldVer
    Call SemverCompare
    StrCpy $CmpResult $VCmp
  ${EndIf}

  ${If} $CmpResult = 0
    StrCpy $1 "$(ReinstSame)"
    StrCpy $2 "$(OptRepair)"
    StrCpy $3 "$(OptUninstall)"
  ${ElseIf} $CmpResult = -1
    StrCpy $1 "$(ReinstNewer)"
    StrCpy $2 "$(OptDowngrade)"
    StrCpy $3 "$(OptClean)"
  ${Else}
    ${If} $OldVer == ""
      StrCpy $1 "$(ReinstUnknown)"
    ${Else}
      StrCpy $1 "$(ReinstOlder)"
    ${EndIf}
    StrCpy $2 "$(OptUpgrade)"
    StrCpy $3 "$(OptClean)"
  ${EndIf}

  !insertmacro MUI_HEADER_TEXT "$(ReinstTitle)" "$(ReinstSubtitle)"
  nsDialogs::Create 1018
  Pop $0
  ${If} $(^RTL) = 1
    nsDialogs::SetRTL $(^RTL)
  ${EndIf}
  ${NSD_CreateLabel} 0 0 100% 44u $1
  Pop $1
  ${NSD_CreateRadioButton} 12u 58u -12u 12u $2
  Pop $RadioA
  ${NSD_CreateRadioButton} 12u 76u -12u 12u $3
  Pop $RadioB
  ${If} $ReinstChoice = 2
    SendMessage $RadioB ${BM_SETCHECK} ${BST_CHECKED} 0
  ${Else}
    SendMessage $RadioA ${BM_SETCHECK} ${BST_CHECKED} 0
  ${EndIf}
  ${NSD_SetFocus} $RadioA
  nsDialogs::Show
FunctionEnd

Function PageReinstallLeave
  ${NSD_GetState} $RadioB $0
  ${If} $0 = ${BST_CHECKED}
    StrCpy $ReinstChoice 2
  ${Else}
    StrCpy $ReinstChoice 1
  ${EndIf}

  ${If} $CmpResult = 0
    ${If} $ReinstChoice = 2
      Call RunExistingUninstaller
    ${EndIf}
  ${ElseIf} $ReinstChoice = 2
    StrCpy $CleanInstall 1
  ${Else}
    StrCpy $CleanInstall 0
  ${EndIf}
FunctionEnd

; Same version, "uninstall" chosen: run the uninstaller of the install,
; which asks about the data and the service itself, and leave Setup, unless
; the user cancelled it. _?= keeps it from copying itself, so that this
; waits for it.
Function RunExistingUninstaller
  ${IfNot} ${FileExists} "$INSTDIR\Uninstall.exe"
    MessageBox MB_OK|MB_ICONEXCLAMATION "$(UninstallFailed)"
    Abort
  ${EndIf}
  HideWindow
  ClearErrors
  ExecWait '"$INSTDIR\Uninstall.exe" _?=$INSTDIR' $0
  BringToFront
  ${If} ${FileExists} "$INSTDIR\${MAIN_EXE}"
    Abort ; cancelled: back to the page
  ${EndIf}
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
  Quit
FunctionEnd

; The app must be closed before the files change; a page of no content that
; asks it to be, when the user is there to ask.
Function PageCloseApp
  Call EnsureAppClosed
  Abort
FunctionEnd

Function LaunchApp
  SetOutPath "$INSTDIR"
  Exec '"$INSTDIR\${MAIN_EXE}"'
FunctionEnd

; The finish page offers it, so silent installs (/S) make none.
Function CreateDesktopShortcut
  SetOutPath "$INSTDIR"
  CreateShortCut "$DESKTOP\${PRODUCT_NAME}.lnk" "$INSTDIR\${MAIN_EXE}"
FunctionEnd

Section "Install"
  ; Silent installs, and anything that got here without the page.
  Call EnsureAppClosed

  ${If} $CleanInstall = 1
  ${AndIf} ${FileExists} "$INSTDIR\${MAIN_EXE}"
    ; Nothing but the program lives here; the settings are elsewhere. The
    ; current directory must be elsewhere too, for the folder to go.
    SetOutPath "$TEMP"
    RMDir /r "$INSTDIR"
  ${EndIf}

  SetOutPath "$INSTDIR"
  !include "${WORKDIR}\files.nsh"
  WriteUninstaller "$INSTDIR\Uninstall.exe"

  CreateShortCut "$SMPROGRAMS\${PRODUCT_NAME}.lnk" "$INSTDIR\${MAIN_EXE}"

  WriteRegStr HKCU "${UNINSTKEY}" "DisplayName" "${PRODUCT_NAME}"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINSTKEY}" "Publisher" "${PUBLISHER}"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayIcon" "$INSTDIR\${MAIN_EXE}"
  WriteRegStr HKCU "${UNINSTKEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTKEY}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKCU "${UNINSTKEY}" "QuietUninstallString" '"$INSTDIR\Uninstall.exe" /S'
  WriteRegDWORD HKCU "${UNINSTKEY}" "EstimatedSize" ${ESTIMATED_KB}
  WriteRegDWORD HKCU "${UNINSTKEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTKEY}" "NoRepair" 1

  !insertmacro RegisterSchemes
SectionEnd

; ---------------------------------------------------------------------------
; Uninstaller

Function un.onInit
  StrCpy $DeleteData 0
  ; Silent: the language is the remembered one, and the data goes only on
  ; request (/DELETEDATA).
  !insertmacro MUI_UNGETLANGUAGE
  ${un.GetParameters} $0
  ClearErrors
  ${un.GetOptions} $0 "/DELETEDATA" $1
  ${IfNot} ${Errors}
    StrCpy $DeleteData 1
  ${EndIf}
FunctionEnd

; Adds the "Also delete app data" check box to the confirmation page, below
; its text. The page is the standard one, so the box is made by hand, in the
; page's font and the window's DPI.
Function un.ConfirmShow
  ; $1 the page, $2 DPI, $3..$6 position and size in pixels
  FindWindow $1 "#32770" "" $HWNDPARENT
  System::Call 'user32::GetDpiForWindow(p r1) i .r2'
  ${If} $2 = 0
    StrCpy $2 96
  ${EndIf}
  ${If} $(^RTL) = 1
    IntOp $3 ${__NSD_CheckBox_EXSTYLE} | 0x00400000 ; WS_EX_LAYOUTRTL
    IntOp $4 50 * $2
  ${Else}
    StrCpy $3 ${__NSD_CheckBox_EXSTYLE}
    StrCpy $4 0
  ${EndIf}
  IntOp $4 $4 / 96
  IntOp $5 100 * $2
  IntOp $5 $5 / 96
  IntOp $6 380 * $2
  IntOp $6 $6 / 96
  IntOp $7 22 * $2
  IntOp $7 $7 / 96
  System::Call 'user32::CreateWindowEx(i r3, w "${__NSD_CheckBox_CLASS}", w "$(un.DeleteData)", i ${__NSD_CheckBox_STYLE}, i r4, i r5, i r6, i r7, p r1, i 0, i 0, i 0) p .s'
  Pop $DeleteDataBox
  SendMessage $HWNDPARENT ${WM_GETFONT} 0 0 $1
  SendMessage $DeleteDataBox ${WM_SETFONT} $1 1
  ${If} $DeleteData = 1
    SendMessage $DeleteDataBox ${BM_SETCHECK} ${BST_CHECKED} 0
  ${EndIf}
FunctionEnd

Function un.ConfirmLeave
  ${NSD_GetState} $DeleteDataBox $0
  ${If} $0 = ${BST_CHECKED}
    StrCpy $DeleteData 1
  ${Else}
    StrCpy $DeleteData 0
  ${EndIf}
FunctionEnd

; Deletes the shortcut on the stack, trying again for 5 s while it is in
; use: Explorer opens a new shortcut a few seconds after it appears, not
; sharing it for deletion, and Delete fails meanwhile. It succeeds when
; there is no shortcut.
Function un.DeleteShortcut
  Exch $0
  Push $1
  StrCpy $1 50
  retry:
    ClearErrors
    Delete $0
    IfErrors 0 done
    IntOp $1 $1 - 1
    IntCmp $1 0 done
    Sleep 100
    Goto retry
  done:
  Pop $1
  Pop $0
FunctionEnd

; $0 = 1 when the privileged service is installed (a query needs no rights).
Function un.ServiceInstalled
  nsExec::ExecToStack '"$SYSDIR\sc.exe" query ${SERVICE_NAME}'
  Pop $0
  Pop $1
  ${If} $0 = 0
    StrCpy $0 1
  ${Else}
    StrCpy $0 0
  ${EndIf}
FunctionEnd

; Removes the privileged service that TUN mode uses: stops it, deletes it and
; its copy of the program in Program Files. That takes an administrator, which
; this uninstaller is not: the app's own `service uninstall` is run through
; UAC, as the app does it. Silent uninstalls never prompt; they leave the
; service, unless they were started elevated.
Function un.RemoveService
  Call un.ServiceInstalled
  StrCmp $0 1 0 rs_done
  ${IfNot} ${FileExists} "$INSTDIR\${MAIN_EXE}"
    Goto rs_failed
  ${EndIf}
  System::Call 'shell32::IsUserAnAdmin() i .r2'
  ${If} $2 = 1
    nsExec::Exec '"$INSTDIR\${MAIN_EXE}" service uninstall --name ${SERVICE_SLUG}'
    Pop $0
  ${Else}
    ${If} ${Silent}
      Goto rs_done
    ${EndIf}
    MessageBox MB_YESNO|MB_ICONQUESTION "$(un.ServiceAsk)" IDYES rs_ask
    Goto rs_failed
    rs_ask:
    ClearErrors
    ExecShellWait "runas" "$INSTDIR\${MAIN_EXE}" "service uninstall --name ${SERVICE_SLUG}" SW_HIDE
  ${EndIf}
  ; The service goes a moment after the command returns.
  StrCpy $3 0
  rs_wait:
    Call un.ServiceInstalled
    StrCmp $0 0 rs_done
    IntOp $3 $3 + 1
    IntCmp $3 10 rs_failed 0 rs_failed
    Sleep 500
    Goto rs_wait
  rs_failed:
  ${IfNot} ${Silent}
    MessageBox MB_OK|MB_ICONINFORMATION "$(un.ServiceLeft)"
  ${EndIf}
  rs_done:
FunctionEnd

Section "Uninstall"
  Call un.EnsureAppClosed
  Call un.RemoveService

  Push "$SMPROGRAMS\${PRODUCT_NAME}.lnk"
  Call un.DeleteShortcut
  Push "$DESKTOP\${PRODUCT_NAME}.lnk"
  Call un.DeleteShortcut

  ; Starting at login is a value of the Run key, named by the identifier.
  DeleteRegValue HKCU "${RUNKEY}" "${IDENTIFIER}"
  DeleteRegValue HKCU "${APPROVEDKEY}" "${IDENTIFIER}"

  DeleteRegKey HKCU "${UNINSTKEY}"
  !insertmacro UnregisterSchemes
  DeleteRegValue HKCU "${APPKEY}" "Installer Language"
  DeleteRegKey /ifempty HKCU "${APPKEY}"

  ${If} $DeleteData = 1
    ; Settings, profiles and secrets in the roaming data folder, the cache
    ; and the web view's files next to it. The program's folder is another.
    ${If} "$APPDATA\${PRODUCT_NAME}" != $INSTDIR
      RMDir /r "$APPDATA\${PRODUCT_NAME}"
    ${EndIf}
    ${If} "$LOCALAPPDATA\${PRODUCT_NAME}" != $INSTDIR
      RMDir /r "$LOCALAPPDATA\${PRODUCT_NAME}"
    ${EndIf}
  ${EndIf}

  ; Last, so that the app's folder is gone once the uninstall is done: the
  ; uninstaller runs from a copy of itself that nothing waits for.
  SetOutPath "$TEMP"
  ${If} ${FileExists} "$INSTDIR\${MAIN_EXE}"
    RMDir /r "$INSTDIR"
  ${Else}
    ; Not our folder, or half an install: take only what we put there.
    Delete "$INSTDIR\Uninstall.exe"
    RMDir "$INSTDIR"
  ${EndIf}
SectionEnd
