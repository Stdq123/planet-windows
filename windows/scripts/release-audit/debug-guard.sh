#!/bin/sh
set -eu
[ "${PLANET_DISPOSABLE_CONTAINER:-}" = yes ] && [ -f /.dockerenv ] || exit 1
export WINEPREFIX=/tmp/wine HOME=/tmp/home WINEDEBUG=-all
mkdir -p "$HOME" "$WINEPREFIX"
xvfb-run -a sh -u <<'WINE_DEBUG'
wineboot -u >/dev/null 2>&1
wineserver -w
cp /opt/planet-wine-test/bcryptprimitives.dll "$WINEPREFIX/drive_c/windows/system32/bcryptprimitives.dll"
mkdir -p "$WINEPREFIX/drive_c/audit"
wine /input/setup.exe /S '/D=C:\audit\dedicated'
echo "install exit=$?"
wineserver -w
ls -la "$WINEPREFIX/drive_c/audit/dedicated"
cp /input/guard.exe /tmp/guard.exe
wine /tmp/guard.exe record 'C:\audit\dedicated'
echo "direct record exit=$?"
wine reg query 'HKCU\Software\PlanetWindows\InstallerState' 2>&1
WINE_DEBUG
