#!/bin/sh
set -eu
[ "${PLANET_DISPOSABLE_CONTAINER:-}" = yes ] && [ -f /.dockerenv ] || exit 1
for net in /sys/class/net/*; do [ "${net##*/}" = lo ] || exit 1; done
export WINEPREFIX=/tmp/planet-rename-wine HOME=/tmp/planet-rename-home WINEDEBUG=-all
mkdir -p "$HOME" "$WINEPREFIX"
xvfb-run -a sh -eu <<'WINE_TEST'
wineboot -u >/dev/null 2>&1
wineserver -w
cp /opt/planet-wine-test/bcryptprimitives.dll "$WINEPREFIX/drive_c/windows/system32/bcryptprimitives.dll"
cp /input/renameprobe.exe /tmp/renameprobe.exe
wine /tmp/renameprobe.exe -test.v
wineserver -w
WINE_TEST
