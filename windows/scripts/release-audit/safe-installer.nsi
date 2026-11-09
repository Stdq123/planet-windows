; Standalone NSIS distribution. This does not use MyGo's CloseApp or RMDir /r.
Unicode true
!ifndef PAYLOAD
!error "PAYLOAD is required"
!endif
!ifndef OUTPUT
!error "OUTPUT is required"
!endif
!ifndef GUARD
!error "GUARD is required"
!endif
!ifndef VERSION
!error "VERSION is required"
!endif
!include "MUI2.nsh"
!define MUI_ICON "${__FILEDIR__}\..\..\resources\icon.ico"
!define MUI_UNICON "${__FILEDIR__}\..\..\resources\icon.ico"
Name "Planet Windows"
OutFile "${OUTPUT}"
InstallDir "$LOCALAPPDATA\Programs\Planet Windows"
InstallDirRegKey HKCU "Software\PlanetWindows\InstallerState" "InstallLocation"
RequestExecutionLevel user
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Function .onInit
  ; Shared by every protocol-2 installer and uninstaller for the entire process.
  System::Call 'kernel32::CreateMutexW(p0,i1,w "Global\PlanetWindows-Installer-Transaction-v2")p.r9?e'
  Pop $8
  StrCmp $9 0 0 +4
  MessageBox MB_OK|MB_ICONSTOP "Cannot obtain the Planet installer lock." /SD IDOK
  SetErrorLevel 2
  Abort
  StrCmp $8 183 0 +4
  MessageBox MB_OK|MB_ICONSTOP "Another Planet installer or uninstaller is running." /SD IDOK
  SetErrorLevel 2
  Abort
  InitPluginsDir
  SetOutPath "$PLUGINSDIR"
  File /oname=planet-installer-guard.exe "${GUARD}"
FunctionEnd
Function un.onInit
  System::Call 'kernel32::CreateMutexW(p0,i1,w "Global\PlanetWindows-Installer-Transaction-v2")p.r9?e'
  Pop $8
  StrCmp $9 0 0 +4
  MessageBox MB_OK|MB_ICONSTOP "Cannot obtain the Planet installer lock." /SD IDOK
  SetErrorLevel 2
  Abort
  StrCmp $8 183 0 +4
  MessageBox MB_OK|MB_ICONSTOP "Another Planet installer or uninstaller is running." /SD IDOK
  SetErrorLevel 2
  Abort
  InitPluginsDir
  SetOutPath "$PLUGINSDIR"
  File /oname=planet-installer-guard.exe "${GUARD}"
FunctionEnd

!macro CheckFileError
  IfErrors install_failed
!macroend
!macro WriteUninstallRegistry
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.planetable.planetwindows" "DisplayName" "Planet Windows"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.planetable.planetwindows" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.planetable.planetwindows" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.planetable.planetwindows" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.planetable.planetwindows" "QuietUninstallString" '"$INSTDIR\Uninstall.exe" /S'
!macroend
Section
  nsExec::ExecToStack '"$PLUGINSDIR\planet-installer-guard.exe" install-check "$INSTDIR"'
  Pop $0
  Pop $1
  StrCmp $0 "0" install_allowed
  ; An identical, verified existing payload needs no filesystem or registry
  ; changes. A different release goes through the transaction guard.
  nsExec::ExecToStack '"$PLUGINSDIR\planet-installer-guard.exe" identical-install-check "$INSTDIR"'
  Pop $0
  Pop $2
  StrCmp $0 "0" install_identical
  ; Stage in the private plugin directory. The guard alone owns the durable
  ; upgrade transaction; NSIS never overwrites an existing payload directly.
  SetOutPath "$PLUGINSDIR\update-payload"
  File "${PAYLOAD}\Planet Windows.exe"
  File "${PAYLOAD}\ipfs.exe"
  File "${PAYLOAD}\icon.png"
  File "${PAYLOAD}\kubo-LICENSE.txt"
  File "${PAYLOAD}\kubo-LICENSE-MIT.txt"
  File "${PAYLOAD}\kubo-LICENSE-APACHE.txt"
  File "${PAYLOAD}\LICENSE-PLANET.txt"
  File "${PAYLOAD}\THIRD-PARTY-NOTICES.txt"
  File "${PAYLOAD}\KUBO-BUILDINFO.txt"
  File "${PAYLOAD}\KUBO-DEPENDENCY-LICENSES.txt"
  WriteUninstaller "$PLUGINSDIR\update-payload\Uninstall.exe"
  System::Call 'kernel32::GetCurrentProcessId()i.r7'
  nsExec::ExecToStack '"$PLUGINSDIR\planet-installer-guard.exe" upgrade-apply "$INSTDIR" "$PLUGINSDIR\update-payload" "$7" "$9"'
  Pop $0
  Pop $1
  StrCmp $0 "0" install_done
  MessageBox MB_OK|MB_ICONSTOP "Update could not complete: $1. If an update is pending, close the app and rerun this same installer to resume." /SD IDOK
  SetErrorLevel 2
  Abort
install_identical:
  MessageBox MB_OK|MB_ICONINFORMATION "The identical verified version is already installed. No files were changed." /SD IDOK
  SetErrorLevel 0
  Goto install_done
install_allowed:
  ClearErrors
  SetOutPath "$INSTDIR"
  !insertmacro CheckFileError
  SetOverwrite off
  File "${PAYLOAD}\Planet Windows.exe"
  !insertmacro CheckFileError
  File "${PAYLOAD}\ipfs.exe"
  !insertmacro CheckFileError
  File "${PAYLOAD}\icon.png"
  !insertmacro CheckFileError
  File "${PAYLOAD}\kubo-LICENSE.txt"
  !insertmacro CheckFileError
  File "${PAYLOAD}\kubo-LICENSE-MIT.txt"
  !insertmacro CheckFileError
  File "${PAYLOAD}\kubo-LICENSE-APACHE.txt"
  !insertmacro CheckFileError
  File "${PAYLOAD}\LICENSE-PLANET.txt"
  !insertmacro CheckFileError
  File "${PAYLOAD}\THIRD-PARTY-NOTICES.txt"
  !insertmacro CheckFileError
  File "${PAYLOAD}\KUBO-BUILDINFO.txt"
  !insertmacro CheckFileError
  File "${PAYLOAD}\KUBO-DEPENDENCY-LICENSES.txt"
  !insertmacro CheckFileError
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  !insertmacro CheckFileError
  nsExec::ExecToStack '"$PLUGINSDIR\planet-installer-guard.exe" record "$INSTDIR"'
  Pop $0
  Pop $1
  StrCmp $0 "0" record_done
  MessageBox MB_OK|MB_ICONSTOP "Unable to verify installed files: $1" /SD IDOK
  SetErrorLevel 3
  Abort
record_done:
  ClearErrors
  !insertmacro WriteUninstallRegistry
  IfErrors install_failed
  ; Generate privately; the guard atomically refuses an existing destination.
  ClearErrors
  CreateShortCut "$PLUGINSDIR\planet-start.lnk" "$INSTDIR\Planet Windows.exe"
  IfErrors shortcut_failed
  nsExec::ExecToStack '"$PLUGINSDIR\planet-installer-guard.exe" shortcut-create "$INSTDIR" "$PLUGINSDIR\planet-start.lnk" "$SMPROGRAMS\Planet Windows.lnk"'
  Pop $0
  Pop $1
  StrCmp $0 "0" install_done
shortcut_failed:
  MessageBox MB_OK|MB_ICONEXCLAMATION "The app was installed, but its Start Menu shortcut could not be created. Existing shortcuts were not replaced." /SD IDOK
  Goto install_done
install_failed:
  MessageBox MB_OK|MB_ICONSTOP "Installation failed. Existing files were not intentionally removed; inspect the chosen directory before retrying." /SD IDOK
  SetErrorLevel 3
  Abort
install_done:
SectionEnd

!macro DeleteOwned NAME
  IfFileExists "$INSTDIR\${NAME}" 0 +4
  ClearErrors
  Delete "$INSTDIR\${NAME}"
  IfErrors uninstall_failed
!macroend
Section "Uninstall"
  nsExec::ExecToStack '"$PLUGINSDIR\planet-installer-guard.exe" uninstall-check "$INSTDIR"'
  Pop $0
  Pop $1
  StrCmp $0 "0" uninstall_allowed
  MessageBox MB_OK|MB_ICONSTOP "Uninstall refused: $1" /SD IDOK
  SetErrorLevel 2
  Abort
uninstall_allowed:
  nsExec::ExecToStack '"$PLUGINSDIR\planet-installer-guard.exe" shortcut-remove "$INSTDIR" "$SMPROGRAMS\Planet Windows.lnk"'
  Pop $0
  Pop $1
  StrCmp $0 "0" shortcut_checked
  MessageBox MB_OK|MB_ICONSTOP "Uninstall refused: shortcut cannot be checked: $1" /SD IDOK
  SetErrorLevel 2
  Abort
shortcut_checked:
  !insertmacro DeleteOwned "Planet Windows.exe"
  !insertmacro DeleteOwned "ipfs.exe"
  !insertmacro DeleteOwned "icon.png"
  !insertmacro DeleteOwned "kubo-LICENSE.txt"
  !insertmacro DeleteOwned "kubo-LICENSE-MIT.txt"
  !insertmacro DeleteOwned "kubo-LICENSE-APACHE.txt"
  !insertmacro DeleteOwned "LICENSE-PLANET.txt"
  !insertmacro DeleteOwned "THIRD-PARTY-NOTICES.txt"
  !insertmacro DeleteOwned "KUBO-BUILDINFO.txt"
  !insertmacro DeleteOwned "KUBO-DEPENDENCY-LICENSES.txt"
  ClearErrors
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.planetable.planetwindows"
  IfErrors uninstall_failed
  ClearErrors
  Delete "$INSTDIR\Uninstall.exe"
  IfErrors restore_uninstall_entry
  ClearErrors
  DeleteRegKey HKCU "Software\PlanetWindows\InstallerState"
  IfErrors uninstall_failed
  RMDir "$INSTDIR"
  Goto uninstall_done
restore_uninstall_entry:
  ClearErrors
  !insertmacro WriteUninstallRegistry
  Goto uninstall_failed
uninstall_failed:
  MessageBox MB_OK|MB_ICONSTOP "Uninstall failed. The uninstall entry has been kept where possible; close open files and retry." /SD IDOK
  SetErrorLevel 3
  Abort
uninstall_done:
SectionEnd
