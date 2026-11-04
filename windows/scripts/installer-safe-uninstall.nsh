; MyGo 0.7.3 unconditionally follows this hook with RMDir /r "$INSTDIR".
; Remove only known payload files, then point that recursive operation at a
; nonexistent child of NSIS's private plugin directory, never the install path.
; Unrecognized files and nonempty directories are intentionally preserved.
!macro customUnInstall
  Delete "$INSTDIR\Planet Windows.exe"
  Delete "$INSTDIR\ipfs.exe"
  Delete "$INSTDIR\icon.png"
  Delete "$INSTDIR\kubo-LICENSE.txt"
  Delete "$INSTDIR\kubo-LICENSE-MIT.txt"
  Delete "$INSTDIR\kubo-LICENSE-APACHE.txt"
  Delete "$INSTDIR\LICENSE-PLANET.txt"
  Delete "$INSTDIR\THIRD-PARTY-NOTICES.txt"
  Delete "$INSTDIR\KUBO-BUILDINFO.txt"
  Delete "$INSTDIR\KUBO-DEPENDENCY-LICENSES.txt"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
  InitPluginsDir
  StrCpy $INSTDIR "$PLUGINSDIR\planet-uninstall-noop"
!macroend
