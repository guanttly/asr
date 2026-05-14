!include LogicLib.nsh
!include FileFunc.nsh
!ifndef BUILD_UNINSTALLER
!include nsDialogs.nsh
!endif

!define /ifndef INSTALL_REGISTRY_KEY "Software\${APP_GUID}"
!define /ifndef UNINSTALL_REGISTRY_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${UNINSTALL_APP_KEY}"

!ifndef BUILD_UNINSTALLER
Var UpgradePromptDeleteCheckbox
Var UpgradePromptDeleteCheckboxState
Var UpgradePromptShouldShow
Var UpgradePromptContinue
Var UpgradePromptExistingInstallFound
Var UpgradePromptFallbackUninstallString
Var UpgradePromptFallbackInstallDir
Var UpgradePromptFallbackMode
Var UpgradePromptContainsHaystack
Var UpgradePromptContainsNeedle
Var UpgradePromptContainsIndex
Var UpgradePromptContainsNeedleLength
Var UpgradePromptContainsSlice
Var UpgradePromptContainsHaystackLength
Var UpgradePromptContainsResult

Function UpgradePrompt_ResetState
  StrCpy $UpgradePromptShouldShow "0"
  StrCpy $UpgradePromptContinue "0"
  StrCpy $UpgradePromptExistingInstallFound "0"
  StrCpy $UpgradePromptFallbackUninstallString ""
  StrCpy $UpgradePromptFallbackInstallDir ""
  StrCpy $UpgradePromptFallbackMode ""
  StrCpy $UpgradePromptDeleteCheckboxState "0"
FunctionEnd

Function UpgradePrompt_HasOption
  Exch $R0
  Push $R1
  Push $R2

  ${GetParameters} $R1
  ClearErrors
  ${GetOptions} "$R1" "$R0" $R2
  IfErrors 0 +3
    StrCpy $R0 "0"
    Goto done

  StrCpy $R0 "1"

  done:
  Pop $R2
  Pop $R1
  Exch $R0
FunctionEnd

Function UpgradePrompt_IsDeleteDataRequested
  Push "--delete-app-data"
  Call UpgradePrompt_HasOption
FunctionEnd

Function UpgradePrompt_StrContains
  Exch $UpgradePromptContainsNeedle
  Exch 1
  Exch $UpgradePromptContainsHaystack

  StrCpy $UpgradePromptContainsResult ""
  StrCpy $UpgradePromptContainsIndex -1
  StrLen $UpgradePromptContainsNeedleLength $UpgradePromptContainsNeedle
  StrLen $UpgradePromptContainsHaystackLength $UpgradePromptContainsHaystack

  upgrade_prompt_contains_loop:
    IntOp $UpgradePromptContainsIndex $UpgradePromptContainsIndex + 1
    StrCpy $UpgradePromptContainsSlice $UpgradePromptContainsHaystack $UpgradePromptContainsNeedleLength $UpgradePromptContainsIndex
    StrCmp $UpgradePromptContainsSlice $UpgradePromptContainsNeedle upgrade_prompt_contains_found
    StrCmp $UpgradePromptContainsIndex $UpgradePromptContainsHaystackLength upgrade_prompt_contains_done
    Goto upgrade_prompt_contains_loop

  upgrade_prompt_contains_found:
    StrCpy $UpgradePromptContainsResult $UpgradePromptContainsNeedle

  upgrade_prompt_contains_done:
  Pop $UpgradePromptContainsNeedle
  Exch $UpgradePromptContainsResult
FunctionEnd

!macro UpgradePrompt_StrContains OUT NEEDLE HAYSTACK
  Push `${HAYSTACK}`
  Push `${NEEDLE}`
  Call UpgradePrompt_StrContains
  Pop `${OUT}`
!macroend

Function UpgradePrompt_GetInQuotes
  Exch $R0
  Push $R1
  Push $R2
  Push $R3

  StrCpy $R2 -1
  find_open:
    IntOp $R2 $R2 + 1
    StrCpy $R3 $R0 1 $R2
    StrCmp $R3 "" not_found
    StrCmp $R3 '"' 0 find_open

  IntOp $R2 $R2 + 1
  StrCpy $R0 $R0 "" $R2

  StrCpy $R2 0
  find_close:
    IntOp $R2 $R2 + 1
    StrCpy $R3 $R0 1 $R2
    StrCmp $R3 "" not_found
    StrCmp $R3 '"' 0 find_close

  StrCpy $R0 $R0 $R2
  Goto done

  not_found:
    StrCpy $R0 ""

  done:
  Pop $R3
  Pop $R2
  Pop $R1
  Exch $R0
FunctionEnd

Function UpgradePrompt_GetFileParent
  Exch $R0
  Push $R1
  Push $R2
  Push $R3

  StrCpy $R1 0
  StrLen $R2 $R0

  loop:
    IntOp $R1 $R1 + 1
    IntCmp $R1 $R2 get_parent 0 get_parent
    StrCpy $R3 $R0 1 -$R1
    StrCmp $R3 "\\" get_parent
    Goto loop

  get_parent:
    StrCpy $R0 $R0 -$R1

  Pop $R3
  Pop $R2
  Pop $R1
  Exch $R0
FunctionEnd

!macro UpgradePrompt_ScanLegacyRoot ROOT MODE LABEL
  StrCpy $R1 0

  upgrade_prompt_scan_${LABEL}:
    EnumRegKey $R2 ${ROOT} "Software\Microsoft\Windows\CurrentVersion\Uninstall" $R1
    StrCmp $R2 "" upgrade_prompt_done_${LABEL}

    ReadRegStr $R3 ${ROOT} "Software\Microsoft\Windows\CurrentVersion\Uninstall\$R2" "DisplayName"
    ReadRegStr $R4 ${ROOT} "Software\Microsoft\Windows\CurrentVersion\Uninstall\$R2" "UninstallString"
    ReadRegStr $R5 ${ROOT} "Software\Microsoft\Windows\CurrentVersion\Uninstall\$R2" "InstallLocation"
    ReadRegStr $R6 ${ROOT} "Software\Microsoft\Windows\CurrentVersion\Uninstall\$R2" "DisplayIcon"

    StrCmp $R3 "" upgrade_prompt_next_${LABEL}
    StrCmp $R4 "" upgrade_prompt_next_${LABEL}

    StrLen $R7 "${PRODUCT_NAME}"
    StrCpy $R8 $R3 $R7
    StrCmp $R8 "${PRODUCT_NAME}" 0 upgrade_prompt_next_${LABEL}

    !insertmacro UpgradePrompt_StrContains $R8 "JushaVoiceAssistant" "$R4"
    ${If} $R8 == ""
      !insertmacro UpgradePrompt_StrContains $R8 "JushaVoiceAssistant" "$R5"
    ${EndIf}
    ${If} $R8 == ""
      !insertmacro UpgradePrompt_StrContains $R8 "JushaVoiceAssistant" "$R6"
    ${EndIf}
    ${If} $R8 == ""
      Goto upgrade_prompt_next_${LABEL}
    ${EndIf}

    StrCpy $UpgradePromptExistingInstallFound "1"
    StrCpy $UpgradePromptFallbackUninstallString $R4

    ${If} $R5 == ""
      Push $R4
      Call UpgradePrompt_GetInQuotes
      Pop $R5
      ${If} $R5 != ""
        Push $R5
        Call UpgradePrompt_GetFileParent
        Pop $R5
      ${EndIf}
    ${EndIf}

    StrCpy $UpgradePromptFallbackInstallDir $R5
    StrCpy $UpgradePromptFallbackMode "${MODE}"
    Goto upgrade_prompt_done_${LABEL}

  upgrade_prompt_next_${LABEL}:
    IntOp $R1 $R1 + 1
    Goto upgrade_prompt_scan_${LABEL}

  upgrade_prompt_done_${LABEL}:
!macroend

Function UpgradePrompt_CheckStandardInstall
  Push $R0

  ReadRegStr $R0 HKCU "${UNINSTALL_REGISTRY_KEY}" "UninstallString"
  ${If} $R0 != ""
    StrCpy $UpgradePromptExistingInstallFound "1"
    Goto upgrade_prompt_standard_done
  ${EndIf}

  !ifdef UNINSTALL_REGISTRY_KEY_2
    ReadRegStr $R0 HKCU "${UNINSTALL_REGISTRY_KEY_2}" "UninstallString"
    ${If} $R0 != ""
      StrCpy $UpgradePromptExistingInstallFound "1"
      Goto upgrade_prompt_standard_done
    ${EndIf}
  !endif

  ReadRegStr $R0 HKLM "${UNINSTALL_REGISTRY_KEY}" "UninstallString"
  ${If} $R0 != ""
    StrCpy $UpgradePromptExistingInstallFound "1"
    Goto upgrade_prompt_standard_done
  ${EndIf}

  !ifdef UNINSTALL_REGISTRY_KEY_2
    ReadRegStr $R0 HKLM "${UNINSTALL_REGISTRY_KEY_2}" "UninstallString"
    ${If} $R0 != ""
      StrCpy $UpgradePromptExistingInstallFound "1"
      Goto upgrade_prompt_standard_done
    ${EndIf}
  !endif

  upgrade_prompt_standard_done:
  Pop $R0
FunctionEnd

Function UpgradePrompt_CheckLegacyInstall
  Push $R1
  Push $R2
  Push $R3
  Push $R4
  Push $R5
  Push $R6
  Push $R7
  Push $R8

  !insertmacro UpgradePrompt_ScanLegacyRoot HKCU "/currentuser" current_user

  ${If} $UpgradePromptExistingInstallFound == "0"
    !insertmacro UpgradePrompt_ScanLegacyRoot HKLM "/allusers" all_users
  ${EndIf}

  Pop $R8
  Pop $R7
  Pop $R6
  Pop $R5
  Pop $R4
  Pop $R3
  Pop $R2
  Pop $R1
FunctionEnd

Function UpgradePrompt_DetectExistingInstall
  Call UpgradePrompt_ResetState

  Push "--upgrade-continue"
  Call UpgradePrompt_HasOption
  Pop $R9
  StrCmp "$R9" "1" 0 +2
    StrCpy $UpgradePromptContinue "1"

  Call UpgradePrompt_CheckStandardInstall
  ${If} $UpgradePromptExistingInstallFound == "1"
    StrCpy $UpgradePromptShouldShow "1"
    Return
  ${EndIf}

  Call UpgradePrompt_CheckLegacyInstall
  ${If} $UpgradePromptExistingInstallFound == "1"
    StrCpy $UpgradePromptShouldShow "1"
  ${EndIf}
FunctionEnd

Function UpgradePrompt_RunFallbackUninstall
  Push $R0
  Push $R1
  Push $R2
  Push $R3
  Push $R4
  Push $R5
  Push $R6

  ${If} $UpgradePromptFallbackUninstallString == ""
    Goto upgrade_prompt_fallback_done
  ${EndIf}

  Push $UpgradePromptFallbackUninstallString
  Call UpgradePrompt_GetInQuotes
  Pop $R1
  ${If} $R1 == ""
    MessageBox MB_OK|MB_ICONEXCLAMATION "旧版本兼容卸载器路径解析失败，请先手动卸载旧版本后再安装。"
    Quit
  ${EndIf}

  StrCpy $R2 $UpgradePromptFallbackInstallDir
  ${If} $R2 == ""
    Push $R1
    Call UpgradePrompt_GetFileParent
    Pop $R2
  ${EndIf}
  ${If} $R2 == ""
    MessageBox MB_OK|MB_ICONEXCLAMATION "旧版本安装目录解析失败，请先手动卸载旧版本后再安装。"
    Quit
  ${EndIf}

  IfFileExists "$R1" 0 upgrade_prompt_fallback_missing

  StrCpy $R3 "$PLUGINSDIR\legacy-uninstaller.exe"
  CopyFiles /SILENT "$R1" "$R3"
  IfErrors 0 +2
    StrCpy $R3 $R1

  StrCpy $R4 "$UpgradePromptFallbackMode --updated"
  Call UpgradePrompt_IsDeleteDataRequested
  Pop $R5
  ${If} $R5 == "1"
    StrCpy $R4 "$UpgradePromptFallbackMode --delete-app-data"
  ${EndIf}

  ExecWait '"$R3" /S /KEEP_APP_DATA $R4 _?=$R2' $R6
  IfErrors 0 +4
    MessageBox MB_OK|MB_ICONEXCLAMATION "旧版本兼容卸载启动失败，请先手动卸载旧版本后再安装。"
    Quit

  ${If} $R6 != 0
    MessageBox MB_OK|MB_ICONEXCLAMATION "旧版本兼容卸载失败，请先手动卸载旧版本后再安装。"
    Quit
  ${EndIf}

  StrCpy $UpgradePromptFallbackUninstallString ""
  Goto upgrade_prompt_fallback_done

  upgrade_prompt_fallback_missing:
    MessageBox MB_OK|MB_ICONEXCLAMATION "未找到旧版本卸载器，请先手动卸载旧版本后再安装。"
    Quit

  upgrade_prompt_fallback_done:
  Pop $R6
  Pop $R5
  Pop $R4
  Pop $R3
  Pop $R2
  Pop $R1
  Pop $R0
FunctionEnd

Function UpgradePrompt_RelaunchInstaller
  Push $R0

  ${GetParameters} $R0
  ${If} $R0 == ""
    StrCpy $R0 "--upgrade-continue"
  ${Else}
    StrCpy $R0 "$R0 --upgrade-continue"
  ${EndIf}

  ${If} $UpgradePromptDeleteCheckboxState == ${BST_CHECKED}
    StrCpy $R0 "$R0 --delete-app-data"
  ${EndIf}

  Exec '"$EXEPATH" $R0'
  IfErrors 0 +3
    MessageBox MB_OK|MB_ICONEXCLAMATION "无法重新启动安装程序，请重试。"
    Quit

  Quit
FunctionEnd

Function UpgradePrompt_Create
  ${If} ${Silent}
    Abort
  ${EndIf}
  ${If} $UpgradePromptShouldShow != "1"
    Abort
  ${EndIf}
  ${If} $UpgradePromptContinue == "1"
    Abort
  ${EndIf}

  nsDialogs::Create 1018
  Pop $0
  ${If} $0 == error
    Abort
  ${EndIf}

  ${NSD_CreateLabel} 0u 0u 100% 34u "检测到已安装的旧版本。继续安装时，安装程序会先卸载旧版本，再安装新版本。"
  Pop $0

  ${NSD_CreateLabel} 0u 38u 100% 30u "默认会保留本地配置、缓存和历史数据。勾选下面的选项后，升级时会一并清理这些旧数据。"
  Pop $0

  ${If} $UpgradePromptFallbackUninstallString != ""
    ${NSD_CreateLabel} 0u 72u 100% 18u "已启用兼容卸载模式，以处理旧版 Win7 安装记录。"
    Pop $0
    ${NSD_CreateCheckbox} 0u 98u 100% 12u "同时删除旧版本的本地配置和缓存数据"
  ${Else}
    ${NSD_CreateCheckbox} 0u 82u 100% 12u "同时删除旧版本的本地配置和缓存数据"
  ${EndIf}
  Pop $UpgradePromptDeleteCheckbox

  nsDialogs::Show
FunctionEnd

Function UpgradePrompt_Leave
  ${NSD_GetState} $UpgradePromptDeleteCheckbox $UpgradePromptDeleteCheckboxState

  ${If} $UpgradePromptDeleteCheckboxState == ${BST_CHECKED}
    Call UpgradePrompt_IsDeleteDataRequested
    Pop $0
    ${If} $0 != "1"
      Call UpgradePrompt_RelaunchInstaller
    ${EndIf}
  ${EndIf}

  ${If} $UpgradePromptFallbackUninstallString != ""
    Call UpgradePrompt_RunFallbackUninstall
  ${EndIf}
FunctionEnd

!endif

!ifdef BUILD_UNINSTALLER
Function un.UpgradePrompt_HasOption
  Exch $R0
  Push $R1
  Push $R2

  ${GetParameters} $R1
  ClearErrors
  ${GetOptions} "$R1" "$R0" $R2
  IfErrors 0 +3
    StrCpy $R0 "0"
    Goto done

  StrCpy $R0 "1"

  done:
  Pop $R2
  Pop $R1
  Exch $R0
FunctionEnd

Function un.UpgradePrompt_ConfirmManualUninstall
  ${If} ${Silent}
    Return
  ${EndIf}

  Push "--uninstall-confirmed"
  Call un.UpgradePrompt_HasOption
  Pop $R9
  StrCmp "$R9" "1" 0 +2
    Return

  MessageBox MB_ICONQUESTION|MB_YESNOCANCEL "是否同时删除本地配置、缓存和历史数据？$\r$\n$\r$\n选择“是”会在卸载程序时一并清理旧数据；选择“否”只卸载程序文件。" IDYES upgrade_prompt_uninstall_yes IDNO upgrade_prompt_uninstall_no
  Quit

  upgrade_prompt_uninstall_yes:
    Exec '"$EXEPATH" /S --uninstall-confirmed --delete-app-data'
    Quit

  upgrade_prompt_uninstall_no:
    Exec '"$EXEPATH" /S --uninstall-confirmed'
    Quit

FunctionEnd

!endif

!ifndef BUILD_UNINSTALLER
!macro customInit
  Call UpgradePrompt_DetectExistingInstall
  ${If} $UpgradePromptContinue == "1"
  ${AndIf} $UpgradePromptFallbackUninstallString != ""
    Call UpgradePrompt_RunFallbackUninstall
  ${EndIf}
!macroend

!macro customPageAfterChangeDir
  Page custom UpgradePrompt_Create UpgradePrompt_Leave
!macroend

!endif

!ifdef BUILD_UNINSTALLER
!macro customUnInit
  Call un.UpgradePrompt_ConfirmManualUninstall
!macroend

!endif