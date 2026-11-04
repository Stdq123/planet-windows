#!/bin/sh
set -eu

# Run only inside a disposable Docker container: /input is read-only and the
# Wine prefix, Windows registry and installed files live on the container FS.
[ "${PLANET_DISPOSABLE_CONTAINER:-}" = "yes" ] || { echo 'Refusing to run outside the disposable test container.' >&2; exit 1; }
[ -f /.dockerenv ] || { echo 'Refusing to run outside Docker.' >&2; exit 1; }
for net in /sys/class/net/*; do
  [ "${net##*/}" = lo ] || { echo 'Refusing to run with container networking enabled.' >&2; exit 1; }
done
[ -f /input/setup.exe ] || { echo 'Missing /input/setup.exe' >&2; exit 1; }
export WINEPREFIX=/tmp/planet-audit-wine HOME=/tmp/planet-audit-home WINEDEBUG=-all
mkdir -p "$HOME" "$WINEPREFIX"
xvfb-run -a sh -eu <<'WINE_TEST'
wineboot -u >/dev/null 2>&1
wineserver -w
shared="$WINEPREFIX/drive_c/audit/shared"
userdata="$WINEPREFIX/drive_c/users/root/AppData/Roaming/Planet Windows"
mkdir -p "$shared/user-directory" "$userdata"
printf 'unrelated-root' > "$shared/keep.txt"
printf 'unrelated-nested' > "$shared/user-directory/keep.txt"
printf 'user-library' > "$userdata/library.json"
first=/input/setup.exe
if [ -f /input/previous.exe ]; then first=/input/previous.exe; fi
wine "$first" /S '/D=C:\audit\shared'
wineserver -w
[ -f "$shared/Planet Windows.exe" ] && [ -f "$shared/Uninstall.exe" ] || { echo 'FAIL: silent installation did not create payload and uninstaller'; exit 1; }
echo 'PASS: silent install into a pre-existing directory'
wine /input/setup.exe /S '/D=C:\audit\shared'
wineserver -w
[ "$(cat "$shared/keep.txt")" = unrelated-root ] && [ "$(cat "$userdata/library.json")" = user-library ] || { echo 'FAIL: upgrade damaged existing files'; exit 1; }
if [ "$first" = /input/previous.exe ]; then
  echo 'PASS: install over previous version preserved unrelated file and user data'
else
  echo 'PASS: repeat install preserved unrelated file and user data'
fi
wine "$shared/Uninstall.exe" /S
wineserver -w
[ -f "$shared/keep.txt" ] && [ "$(cat "$shared/keep.txt")" = unrelated-root ] || { echo 'FAIL: uninstall removed unrelated file'; exit 1; }
[ -f "$shared/user-directory/keep.txt" ] || { echo 'FAIL: uninstall removed unrelated nested file'; exit 1; }
[ -f "$userdata/library.json" ] && [ "$(cat "$userdata/library.json")" = user-library ] || { echo 'FAIL: uninstall removed user data'; exit 1; }
for owned in 'Planet Windows.exe' 'ipfs.exe' 'icon.png' 'kubo-LICENSE.txt' 'kubo-LICENSE-MIT.txt' 'kubo-LICENSE-APACHE.txt' 'LICENSE-PLANET.txt' 'THIRD-PARTY-NOTICES.txt' 'KUBO-BUILDINFO.txt' 'KUBO-DEPENDENCY-LICENSES.txt' 'Uninstall.exe'; do
  [ ! -e "$shared/$owned" ] || { echo "FAIL: uninstall left owned payload: $owned"; exit 1; }
done
echo 'PASS: uninstall preserved unrelated files and user data while removing owned payload'
WINE_TEST
