#!/bin/sh
set -eu
[ "${PLANET_DISPOSABLE_CONTAINER:-}" = "yes" ] && [ -f /.dockerenv ] || { echo 'Refusing to run outside disposable Docker.' >&2; exit 1; }
for net in /sys/class/net/*; do [ "${net##*/}" = lo ] || { echo 'Network must be disabled.' >&2; exit 1; }; done
[ -f /input/setup.exe ] && [ -f /input/holder.exe ] || exit 1
export WINEPREFIX=/tmp/planet-audit-wine HOME=/tmp/planet-audit-home WINEDEBUG=-all
mkdir -p "$HOME" "$WINEPREFIX"
xvfb-run -a sh -eu <<'WINE_TEST'
wineboot -u >/dev/null 2>&1
wineserver -w
cp /opt/planet-wine-test/bcryptprimitives.dll "$WINEPREFIX/drive_c/windows/system32/bcryptprimitives.dll"
root="$WINEPREFIX/drive_c/audit"
shared="$root/shared"
install="$root/dedicated"
userdata="$WINEPREFIX/drive_c/users/root/AppData/Roaming/Planet Windows"
programs="$WINEPREFIX/drive_c/users/root/AppData/Roaming/Microsoft/Windows/Start Menu/Programs"
shortcut="$programs/Planet Windows.lnk"
mkdir -p "$shared/tools" "$userdata"
mkdir -p "$programs"
printf 'foreign shortcut' > "$shortcut"
printf 'foreign program' > "$shared/Planet Windows.exe"
cp /input/holder.exe "$shared/tools/foreign.exe"
printf 'unrelated' > "$shared/lock.txt"
printf 'user-library' > "$userdata/library.json"
wine "$shared/tools/foreign.exe" 'C:\audit\shared\lock.txt' 'C:\audit\shared\ready.flag' 'C:\audit\shared\release.flag' &
foreign_pid=$!
for i in $(seq 1 60); do [ -f "$shared/ready.flag" ] && break; sleep 1; done
[ -f "$shared/ready.flag" ] && kill -0 "$foreign_pid" || { echo 'FAIL: foreign test process did not start'; exit 1; }
if wine /input/setup.exe /S '/D=C:\audit\shared'; then echo 'FAIL: accepted shared directory with same-name file'; exit 1; fi
kill -0 "$foreign_pid" || { echo 'FAIL: foreign process was stopped'; exit 1; }
[ "$(cat "$shared/Planet Windows.exe")" = 'foreign program' ] && [ -f "$shared/tools/foreign.exe" ] || { echo 'FAIL: same-name or unknown file changed'; exit 1; }
echo 'PASS: shared directory and same-name file rejected; foreign process remains running'
touch "$shared/release.flag"
wait "$foreign_pid"
wineserver -w
wine /input/setup.exe /S '/D=C:\audit\dedicated'
wineserver -w
[ -f "$install/Planet Windows.exe" ] && [ -f "$install/Uninstall.exe" ] || { echo 'FAIL: new install missing files'; exit 1; }
echo 'PASS: empty-directory installation'
[ "$(cat "$shortcut")" = 'foreign shortcut' ] || { echo 'FAIL: pre-existing shortcut was overwritten'; exit 1; }
echo 'PASS: pre-existing Start Menu shortcut preserved at install'
printf 'unknown file' > "$install/keep.txt"
before_reinstall=$(sha256sum "$install/Planet Windows.exe" "$install/ipfs.exe" "$install/Uninstall.exe")
wine /input/setup.exe /S '/D=C:\audit\dedicated' || { echo 'FAIL: identical verified reinstall was rejected'; exit 1; }
wineserver -w
[ "$before_reinstall" = "$(sha256sum "$install/Planet Windows.exe" "$install/ipfs.exe" "$install/Uninstall.exe")" ] && [ "$(cat "$install/keep.txt")" = 'unknown file' ] && [ "$(cat "$userdata/library.json")" = 'user-library' ] && [ "$(cat "$shortcut")" = 'foreign shortcut' ] || { echo 'FAIL: identical reinstall changed files, shortcut, or user data'; exit 1; }
wine reg query 'HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\org.planetable.planetwindows' >/dev/null 2>&1 || { echo 'FAIL: identical reinstall lost uninstall entry'; exit 1; }
wine reg query 'HKCU\Software\PlanetWindows\InstallerState' >/dev/null 2>&1 || { echo 'FAIL: identical reinstall lost installer state'; exit 1; }
echo 'PASS: identical verified reinstall leaves existing installation and user data unchanged'
cp /input/holder.exe "$root/holder.exe"
wine "$root/holder.exe" 'C:\audit\dedicated\ipfs.exe' 'C:\audit\locked.flag' 'C:\audit\release-lock.flag' &
lock_pid=$!
for i in $(seq 1 60); do [ -f "$root/locked.flag" ] && break; sleep 1; done
[ -f "$root/locked.flag" ] && kill -0 "$lock_pid" || { echo 'FAIL: lock holder did not start'; exit 1; }
locked_before=$(sha256sum "$install/Planet Windows.exe" | cut -d' ' -f1)
if wine /input/setup.exe /S '/D=C:\audit\dedicated'; then echo 'FAIL: upgrade accepted while a payload file was locked'; exit 1; fi
kill -0 "$lock_pid" || { echo 'FAIL: upgrade stopped the process locking a payload file'; exit 1; }
[ "$locked_before" = "$(sha256sum "$install/Planet Windows.exe" | cut -d' ' -f1)" ] && [ "$(cat "$install/keep.txt")" = 'unknown file' ] || { echo 'FAIL: refused locked upgrade changed installed or unknown files'; exit 1; }
[ "$(cat "$shortcut")" = 'foreign shortcut' ] && [ "$(cat "$userdata/library.json")" = 'user-library' ] || { echo 'FAIL: refused locked upgrade changed shortcut or user data'; exit 1; }
wine reg query 'HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\org.planetable.planetwindows' >/dev/null 2>&1 || { echo 'FAIL: refused locked upgrade lost uninstall entry'; exit 1; }
wine reg query 'HKCU\Software\PlanetWindows\InstallerState' >/dev/null 2>&1 || { echo 'FAIL: refused locked upgrade lost installer state'; exit 1; }
echo 'PASS: locked registered installation upgrade refused without stopping process or changing files'
wine "$install/Uninstall.exe" /S || true
# NSIS starts a temporary uninstaller copy; Wine's launcher can return zero
# before that child finishes. Keep the lock while the child runs, then check
# the actual files and registry after the Wine server has quiesced.
sleep 8
kill -0 "$lock_pid" || { echo 'FAIL: lock holder exited too early'; exit 1; }
touch "$root/release-lock.flag"
wait "$lock_pid"
wineserver -w
[ -f "$install/Uninstall.exe" ] && [ -f "$install/Planet Windows.exe" ] || { echo 'FAIL: locked uninstall removed files'; exit 1; }
wine reg query 'HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\org.planetable.planetwindows' >/dev/null 2>&1 || { echo 'FAIL: locked uninstall lost its registry entry'; exit 1; }
echo 'PASS: locked file rejects uninstall and preserves uninstall entry'
cp "$install/icon.png" "$root/original-icon"
printf 'user modified file' > "$install/icon.png"
if wine /input/setup.exe /S '/D=C:\audit\dedicated'; then echo 'FAIL: upgrade accepted a modified payload'; exit 1; fi
[ "$(cat "$install/icon.png")" = 'user modified file' ] && [ "$locked_before" = "$(sha256sum "$install/Planet Windows.exe" | cut -d' ' -f1)" ] && [ "$(cat "$install/keep.txt")" = 'unknown file' ] || { echo 'FAIL: refused modified upgrade changed installed or unknown files'; exit 1; }
[ "$(cat "$shortcut")" = 'foreign shortcut' ] && [ "$(cat "$userdata/library.json")" = 'user-library' ] || { echo 'FAIL: refused modified upgrade changed shortcut or user data'; exit 1; }
wine reg query 'HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\org.planetable.planetwindows' >/dev/null 2>&1 || { echo 'FAIL: refused modified upgrade lost uninstall entry'; exit 1; }
wine reg query 'HKCU\Software\PlanetWindows\InstallerState' >/dev/null 2>&1 || { echo 'FAIL: refused modified upgrade lost installer state'; exit 1; }
echo 'PASS: modified registered installation upgrade refused without changing files'
wine "$install/Uninstall.exe" /S || true
wineserver -w
[ "$(cat "$install/icon.png")" = 'user modified file' ] && [ -f "$install/Uninstall.exe" ] && [ -f "$install/Planet Windows.exe" ] || { echo 'FAIL: refused uninstall removed files'; exit 1; }
wine reg query 'HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\org.planetable.planetwindows' >/dev/null 2>&1 || { echo 'FAIL: refused uninstall lost its registry entry'; exit 1; }
echo 'PASS: changed file rejects uninstall and preserves uninstall entry'
cp "$root/original-icon" "$install/icon.png"
wine "$install/Uninstall.exe" /S
wineserver -w
[ "$(cat "$install/keep.txt")" = 'unknown file' ] && [ "$(cat "$userdata/library.json")" = 'user-library' ] || { echo 'FAIL: uninstall removed unrelated data'; exit 1; }
[ "$(cat "$shortcut")" = 'foreign shortcut' ] || { echo 'FAIL: uninstall removed unrelated shortcut'; exit 1; }
[ ! -f "$install/Planet Windows.exe" ] && [ ! -f "$install/Uninstall.exe" ] || { echo 'FAIL: uninstall left owned application files'; exit 1; }
echo 'PASS: verified uninstall preserves unknown files and user data'
rm "$shortcut"
wine /input/setup.exe /S '/D=C:\audit\dedicated2'
wineserver -w
[ -f "$shortcut" ] || { echo 'FAIL: owned Start Menu shortcut missing'; exit 1; }
printf 'user changed shortcut' > "$shortcut"
wine "$root/dedicated2/Uninstall.exe" /S || true
wineserver -w
[ "$(cat "$shortcut")" = 'user changed shortcut' ] && [ ! -f "$root/dedicated2/Uninstall.exe" ] || { echo 'FAIL: modified shortcut was not preserved'; exit 1; }
echo 'PASS: modified Start Menu shortcut preserved at uninstall'
rm "$shortcut"
wine /input/setup.exe /S '/D=C:\audit\dedicated3'
wineserver -w
[ -f "$shortcut" ] || { echo 'FAIL: owned shortcut missing on third install'; exit 1; }
wine "$root/dedicated3/Uninstall.exe" /S || true
wineserver -w
[ ! -e "$shortcut" ] && [ ! -f "$root/dedicated3/Uninstall.exe" ] || { echo 'FAIL: unchanged owned shortcut was not removed'; exit 1; }
echo 'PASS: unchanged owned Start Menu shortcut removed at uninstall'
if [ -f /input/previous.exe ]; then
  old="$root/old"
  wine /input/previous.exe /S '/D=C:\audit\old'
  wineserver -w
  [ -f "$old/Planet Windows.exe" ] && [ -f "$old/Uninstall.exe" ] || { echo 'FAIL: old installation missing'; exit 1; }
  before=$(sha256sum "$old/Planet Windows.exe" | cut -d' ' -f1)
  printf 'old user file' > "$old/keep.txt"
  if wine /input/setup.exe /S '/D=C:\audit\old'; then echo 'FAIL: unverified old installation accepted for upgrade'; exit 1; fi
  wineserver -w
  [ "$before" = "$(sha256sum "$old/Planet Windows.exe" | cut -d' ' -f1)" ] && [ "$(cat "$old/keep.txt")" = 'old user file' ] && [ -f "$old/Uninstall.exe" ] || { echo 'FAIL: rejected old upgrade changed files'; exit 1; }
  wine reg query 'HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\org.planetable.planetwindows' >/dev/null 2>&1 || { echo 'FAIL: rejected old upgrade lost uninstall entry'; exit 1; }
  echo 'PASS: unverified old installation upgrade refused without changing old files or registration'
fi
WINE_TEST
