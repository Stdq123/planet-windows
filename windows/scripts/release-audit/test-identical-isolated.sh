#!/bin/sh
set -eu
[ "${PLANET_DISPOSABLE_CONTAINER:-}" = yes ] && [ -f /.dockerenv ] || exit 1
for net in /sys/class/net/*; do [ "${net##*/}" = lo ] || exit 1; done
[ -f /input/old.exe ] && [ -f /input/new.exe ] && [ -f /input/different.exe ] || exit 1
export WINEPREFIX=/tmp/planet-identical-wine HOME=/tmp/planet-identical-home WINEDEBUG=-all
mkdir -p "$HOME" "$WINEPREFIX"
xvfb-run -a sh -eu <<'WINE_TEST'
wineboot -u >/dev/null 2>&1
wineserver -w
cp /opt/planet-wine-test/bcryptprimitives.dll "$WINEPREFIX/drive_c/windows/system32/bcryptprimitives.dll"
root="$WINEPREFIX/drive_c/audit"
old="$root/old"
data="$WINEPREFIX/drive_c/users/root/AppData/Roaming/Planet Windows"
shortcut="$WINEPREFIX/drive_c/users/root/AppData/Roaming/Microsoft/Windows/Start Menu/Programs/Planet Windows.lnk"
mkdir -p "$data"
wine /input/old.exe /S '/D=C:\audit\old'
wineserver -w
[ -f "$old/Planet Windows.exe" ] && [ -f "$shortcut" ] || { echo 'FAIL: old safe package was not installed'; exit 1; }
printf 'preserve custom' > "$old/custom.txt"
printf 'preserve library' > "$data/library.json"
snapshot_files() { find "$old" -maxdepth 1 -type f -print0 | sort -z | xargs -0 sha256sum; }
snapshot_registry() {
  wine reg query 'HKCU\Software\PlanetWindows\InstallerState' /s | tr -d '\r'
  wine reg query 'HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\org.planetable.planetwindows' /s | tr -d '\r'
}
before_files=$(snapshot_files)
before_registry=$(snapshot_registry)
before_shortcut=$(sha256sum "$shortcut")
wine /input/new.exe /S '/D=C:\audit\old' || { echo 'FAIL: identical payload from the new installer was refused'; exit 1; }
wineserver -w
[ "$before_files" = "$(snapshot_files)" ] && [ "$before_registry" = "$(snapshot_registry)" ] && [ "$before_shortcut" = "$(sha256sum "$shortcut")" ] && [ "$(cat "$data/library.json")" = 'preserve library' ] || { echo 'FAIL: identical reinstall changed old files, shortcut, registry or data'; exit 1; }
echo 'PASS: prior safe installation accepted identical new package without mutations'
if wine /input/different.exe /S '/D=C:\audit\old'; then echo 'FAIL: a different payload was accepted'; exit 1; fi
wineserver -w
[ "$before_files" = "$(snapshot_files)" ] && [ "$before_registry" = "$(snapshot_registry)" ] && [ "$before_shortcut" = "$(sha256sum "$shortcut")" ] && [ "$(cat "$data/library.json")" = 'preserve library' ] || { echo 'FAIL: refused different payload changed files, shortcut, registry or data'; exit 1; }
echo 'PASS: different payload refused without mutations'
WINE_TEST
