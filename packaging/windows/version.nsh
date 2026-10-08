; Version helpers for the installer, written for plain NSIS (no plugins
; beyond the System plugin that ships with it).
;
;   StrCpy $CmpA "0.2.0"
;   StrCpy $CmpB "0.2.0-beta"
;   Call SemverCompare       ; $VCmp = 1 (A newer), 0 (same) or -1 (A older)
;
;   StrCpy $VStr "C:\...\MyGO-Clash.exe"
;   Call ReadExeVersion      ; $VStr = its ProductVersion, or "" without one
;
; Everything goes through global variables so that no call has to juggle the
; stack. A version is major.minor.patch[-prerelease][+build]; a pre-release
; is older than its release, and its identifiers are ranked
; dev < alpha < beta < pre < rc < anything else, each followed by a number
; ("beta.2", "rc1"), plain numbers ranking below all of them.

Var /GLOBAL VStr
Var /GLOBAL VTok
Var /GLOBAL VNum
Var /GLOBAL VCmp
Var /GLOBAL V1
Var /GLOBAL V2
Var /GLOBAL V3
Var /GLOBAL CmpA
Var /GLOBAL CmpB
Var /GLOBAL AMaj
Var /GLOBAL AMin
Var /GLOBAL APat
Var /GLOBAL APre
Var /GLOBAL PMaj
Var /GLOBAL PMin
Var /GLOBAL PPat
Var /GLOBAL PPre
Var /GLOBAL S1
Var /GLOBAL S2
Var /GLOBAL RA
Var /GLOBAL NA
Var /GLOBAL IRank
Var /GLOBAL INum

; NextTok: $VStr = "a.b.c" -> $VTok = "a", $VStr = "b.c".
Function NextTok
  StrCpy $V1 0
  nt_loop:
    StrCpy $V2 $VStr 1 $V1
    StrCmp $V2 "" nt_end
    StrCmp $V2 "." nt_dot
    IntOp $V1 $V1 + 1
    Goto nt_loop
  nt_dot:
    StrCpy $VTok $VStr $V1
    IntOp $V1 $V1 + 1
    StrCpy $VStr $VStr "" $V1
    Return
  nt_end:
    StrCpy $VTok $VStr
    StrCpy $VStr ""
FunctionEnd

; NextNum: like NextTok, with the token as a number in $VNum (0 when it is
; not one).
Function NextNum
  Call NextTok
  IntOp $VNum $VTok + 0
FunctionEnd

; ParseVer: $VStr = a version -> $PMaj, $PMin, $PPat and $PPre.
Function ParseVer
  StrCpy $V1 $VStr 1
  StrCmp $V1 "v" 0 +2
    StrCpy $VStr $VStr "" 1
  ; Drop the build metadata after "+".
  StrCpy $V1 0
  pv_plus:
    StrCpy $V2 $VStr 1 $V1
    StrCmp $V2 "" pv_plus_done
    StrCmp $V2 "+" pv_plus_cut
    IntOp $V1 $V1 + 1
    Goto pv_plus
  pv_plus_cut:
    StrCpy $VStr $VStr $V1
  pv_plus_done:
  ; Split the pre-release off at the first "-".
  StrCpy $PPre ""
  StrCpy $V1 0
  pv_dash:
    StrCpy $V2 $VStr 1 $V1
    StrCmp $V2 "" pv_dash_done
    StrCmp $V2 "-" pv_dash_cut
    IntOp $V1 $V1 + 1
    Goto pv_dash
  pv_dash_cut:
    IntOp $V2 $V1 + 1
    StrCpy $PPre $VStr "" $V2
    StrCpy $VStr $VStr $V1
  pv_dash_done:
  Call NextNum
  StrCpy $PMaj $VNum
  Call NextNum
  StrCpy $PMin $VNum
  Call NextNum
  StrCpy $PPat $VNum
FunctionEnd

; NextIdent: $VStr = "beta.2" -> $IRank, $INum of "beta", $VStr = "2".
Function NextIdent
  Call NextTok
  IntOp $V3 $VTok + 0
  StrCmp $V3 $VTok 0 id_alpha
    StrCpy $IRank -1
    StrCpy $INum $V3
    Return
  id_alpha:
  StrCpy $V3 $VTok 3
  StrCmp $V3 "dev" id_dev
  StrCmp $V3 "pre" id_pre
  StrCpy $V3 $VTok 5
  StrCmp $V3 "alpha" id_alpha5
  StrCpy $V3 $VTok 4
  StrCmp $V3 "beta" id_beta
  StrCpy $V3 $VTok 2
  StrCmp $V3 "rc" id_rc
  StrCpy $IRank 5
  StrCpy $INum 0
  Return
  id_dev:
    StrCpy $IRank 0
    StrCpy $V3 $VTok "" 3
    Goto id_num
  id_alpha5:
    StrCpy $IRank 1
    StrCpy $V3 $VTok "" 5
    Goto id_num
  id_beta:
    StrCpy $IRank 2
    StrCpy $V3 $VTok "" 4
    Goto id_num
  id_pre:
    StrCpy $IRank 3
    StrCpy $V3 $VTok "" 3
    Goto id_num
  id_rc:
    StrCpy $IRank 4
    StrCpy $V3 $VTok "" 2
  id_num:
    IntOp $INum $V3 + 0
FunctionEnd

; ComparePre: $APre against $PPre -> $VCmp.
Function ComparePre
  StrCmp $APre $PPre 0 cp_differ
    StrCpy $VCmp 0
    Return
  cp_differ:
  ; A release is newer than any pre-release of it.
  StrCmp $APre "" 0 cp_a_pre
    StrCpy $VCmp 1
    Return
  cp_a_pre:
  StrCmp $PPre "" 0 cp_both
    StrCpy $VCmp -1
    Return
  cp_both:
  StrCpy $S1 $APre
  StrCpy $S2 $PPre
  cp_next:
    StrCmp $S1 "" cp_a_done
    StrCmp $S2 "" cp_b_done
    StrCpy $VStr $S1
    Call NextIdent
    StrCpy $S1 $VStr
    StrCpy $RA $IRank
    StrCpy $NA $INum
    StrCpy $VStr $S2
    Call NextIdent
    StrCpy $S2 $VStr
    IntCmp $RA $IRank 0 cp_less cp_greater
    IntCmp $NA $INum cp_next cp_less cp_greater
  cp_a_done:
    ; A ran out: the longer one is newer.
    StrCmp $S2 "" cp_same cp_less
  cp_b_done:
    Goto cp_greater
  cp_same:
    StrCpy $VCmp 0
    Return
  cp_less:
    StrCpy $VCmp -1
    Return
  cp_greater:
    StrCpy $VCmp 1
FunctionEnd

Function SemverCompare
  StrCpy $VStr $CmpA
  Call ParseVer
  StrCpy $AMaj $PMaj
  StrCpy $AMin $PMin
  StrCpy $APat $PPat
  StrCpy $APre $PPre
  StrCpy $VStr $CmpB
  Call ParseVer
  IntCmp $AMaj $PMaj 0 sc_less sc_greater
  IntCmp $AMin $PMin 0 sc_less sc_greater
  IntCmp $APat $PPat 0 sc_less sc_greater
  Call ComparePre
  Return
  sc_less:
    StrCpy $VCmp -1
    Return
  sc_greater:
    StrCpy $VCmp 1
FunctionEnd

; ReadExeVersion: $VStr = a path -> $VStr = the ProductVersion in the
; version resource of that file, "" when it has none.
Function ReadExeVersion
  Push $0
  Push $1
  Push $2
  Push $3
  Push $4
  Push $5
  Push $6
  StrCpy $0 $VStr
  StrCpy $6 ""
  System::Call 'version::GetFileVersionInfoSizeW(w r0, *i 0) i .r1'
  IntCmp $1 0 rv_done
  System::Alloc $1
  Pop $2
  System::Call 'version::GetFileVersionInfoW(w r0, i 0, i r1, i r2) i .r3'
  IntCmp $3 0 rv_free
  ; The language and code page of the first translation, such as 0409 04B0.
  System::Call 'version::VerQueryValueW(i r2, w "\VarFileInfo\Translation", *i .r4, *i .r5) i .r3'
  IntCmp $3 0 rv_free
  IntCmp $5 4 rv_have rv_free rv_have
  rv_have:
  System::Call '*$4(&i2 .r3, &i2 .r5)'
  IntFmt $3 "%04X" $3
  IntFmt $5 "%04X" $5
  System::Call 'version::VerQueryValueW(i r2, w "\StringFileInfo\$3$5\ProductVersion", *i .r4, *i .r1) i .r0'
  IntCmp $0 0 rv_free
  IntCmp $1 0 rv_free
  System::Call 'kernel32::lstrcpyW(w .r6, i r4)'
  rv_free:
  System::Free $2
  rv_done:
  StrCpy $VStr $6
  Pop $6
  Pop $5
  Pop $4
  Pop $3
  Pop $2
  Pop $1
  Pop $0
FunctionEnd
