#!/bin/sh
set -eu
[ "${PLANET_DISPOSABLE_CONTAINER:-}" = yes ] && [ -f /.dockerenv ] || exit 1
for net in /sys/class/net/*; do [ "${net##*/}" = lo ] || exit 1; done
export WINEPREFIX=/tmp/planet-upgrade-wine HOME=/tmp/planet-upgrade-home WINEDEBUG=-all
mkdir -p "$HOME" "$WINEPREFIX"
xvfb-run -a sh -eu <<'WINE_TEST'
wineboot -u >/dev/null 2>&1
wineserver -w
cp /opt/planet-wine-test/bcryptprimitives.dll "$WINEPREFIX/drive_c/windows/system32/bcryptprimitives.dll"
root="$WINEPREFIX/drive_c/audit"
old="$root/old"
programs="$WINEPREFIX/drive_c/users/root/AppData/Roaming/Microsoft/Windows/Start Menu/Programs"
data="$WINEPREFIX/drive_c/users/root/AppData/Roaming/Planet Windows"
mkdir -p "$programs" "$data"
echo 'TEST: install old safe package'
wine /input/old.exe /S '/D=C:\audit\old'
wineserver -w
[ -f "$old/Planet Windows.exe" ] && [ -f "$programs/Planet Windows.lnk" ] || { echo 'FAIL: old installation setup'; exit 1; }
# Wine cannot execute a Go PE directly from the Docker Desktop 9p bind mount.
# Run only the copied test binaries from the container's executable tmpfs.
cp /input/guard.exe "$root/guard.exe"
cp /input/holder.exe "$root/holder.exe"
echo 'TEST: ownership preflight of old installation'
if ! wine "$root/guard.exe" upgrade-check 'C:\audit\old'; then echo 'FAIL: verified prior installation did not pass ownership preflight'; exit 1; fi
printf 'extra-owned' > "$old/extra.dat"
extra_hash=$(sha256sum "$old/extra.dat" | cut -d' ' -f1)
wine reg add 'HKCU\Software\PlanetWindows\InstallerState' /v extra.dat /t REG_SZ /d "$extra_hash" /f >/dev/null
printf 'changed-extra-owned' > "$old/extra.dat"
if wine "$root/guard.exe" upgrade-check 'C:\audit\old'; then echo 'FAIL: additional registered file was not checked'; exit 1; fi
wine reg delete 'HKCU\Software\PlanetWindows\InstallerState' /v extra.dat /f >/dev/null
rm "$old/extra.dat"
icon_hash=$(sha256sum "$old/icon.png" | cut -d' ' -f1)
wine reg delete 'HKCU\Software\PlanetWindows\InstallerState' /v icon.png /f >/dev/null
if wine "$root/guard.exe" upgrade-check 'C:\audit\old'; then echo 'FAIL: missing registered hash was accepted'; exit 1; fi
wine reg add 'HKCU\Software\PlanetWindows\InstallerState' /v icon.png /t REG_SZ /d "$icon_hash" /f >/dev/null
cp "$old/icon.png" "$root/icon-original"
printf 'modified' > "$old/icon.png"
if wine "$root/guard.exe" upgrade-check 'C:\audit\old'; then echo 'FAIL: modified old file passed ownership preflight'; exit 1; fi
cp "$root/icon-original" "$old/icon.png"
mv "$old/icon.png" "$root/icon-original"
if wine "$root/guard.exe" upgrade-check 'C:\audit\old'; then echo 'FAIL: missing old file passed ownership preflight'; exit 1; fi
mv "$root/icon-original" "$old/icon.png"
wine "$root/holder.exe" 'C:\audit\old\ipfs.exe' 'C:\audit\ready.flag' 'C:\audit\release.flag' &
holder=$!
for i in $(seq 1 60); do [ -f "$root/ready.flag" ] && break; sleep 1; done
[ -f "$root/ready.flag" ] && kill -0 "$holder" || { echo 'FAIL: lock test did not start'; exit 1; }
if wine "$root/guard.exe" upgrade-check 'C:\audit\old'; then echo 'FAIL: locked old file passed ownership preflight'; exit 1; fi
kill -0 "$holder" || { echo 'FAIL: preflight terminated lock holder'; exit 1; }
touch "$root/release.flag"
wait "$holder"
echo 'PASS: ownership preflight accepts complete verified files and rejects changed, missing, locked files'
printf 'keep unknown' > "$old/custom.txt"
printf 'keep data' > "$data/library.json"
oldhash=$(sha256sum "$old/Planet Windows.exe" | cut -d' ' -f1)
oldinfo=$(sha256sum "$old/KUBO-BUILDINFO.txt" | cut -d' ' -f1)
shortcut_before=$(sha256sum "$programs/Planet Windows.lnk" | cut -d' ' -f1)
if ! wine /input/new.exe /S '/D=C:\audit\old'; then echo 'FAIL: verified safe installation cannot upgrade'; exit 1; fi
wineserver -w
[ "$oldhash" = "$(sha256sum "$old/Planet Windows.exe" | cut -d' ' -f1)" ] || { echo 'FAIL: old executable changed'; exit 1; }
[ "$oldinfo" = "$(sha256sum "$old/KUBO-BUILDINFO.txt" | cut -d' ' -f1)" ] || { echo 'FAIL: old payload changed'; exit 1; }
[ "$(cat "$old/custom.txt")" = 'keep unknown' ] && [ "$(cat "$data/library.json")" = 'keep data' ] || { echo 'FAIL: upgrade changed user files'; exit 1; }
location=$(wine reg query 'HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\org.planetable.planetwindows' /v InstallLocation | tr -d '\r' | sed -n 's/.*REG_SZ[[:space:]]*//p')
[ -n "$location" ] || { echo 'FAIL: upgraded install location missing'; exit 1; }
new=$(winepath -u "$location")
[ "$new" != "$old" ] && [ -f "$new/Planet Windows.exe" ] && grep -q 'PLANET-UPGRADE-FIXTURE-V2' "$new/KUBO-BUILDINFO.txt" || { echo 'FAIL: registered new payload missing'; exit 1; }
[ "$shortcut_before" != "$(sha256sum "$programs/Planet Windows.lnk" | cut -d' ' -f1)" ] || { echo 'FAIL: launch shortcut did not switch'; exit 1; }
echo 'PASS: registered safe installation upgraded while retaining old executable and user files'
WINE_TEST
