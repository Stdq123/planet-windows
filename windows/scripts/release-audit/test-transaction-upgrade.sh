#!/bin/sh
set -eu
[ "${PLANET_DISPOSABLE_CONTAINER:-}" = yes ] && [ -f /.dockerenv ] || { echo 'Refusing to run outside disposable Docker.' >&2; exit 1; }
for net in /sys/class/net/*; do [ "${net##*/}" = lo ] || { echo 'Network must be disabled.' >&2; exit 1; }; done
[ -f /input/old.exe ] && [ -f /input/new.exe ] && [ -f /input/legacy-safe.exe ] && [ -f /input/legacy.exe ] && [ -n "${EXPECTED_EXE_SHA256:-}" ] || exit 1
export WINEPREFIX=/tmp/planet-transaction-wine HOME=/tmp/planet-transaction-home WINEDEBUG=-all
mkdir -p "$HOME" "$WINEPREFIX"
cat > /tmp/mutex-holder.c <<'C_CODE'
#include <windows.h>
int main(int argc, char **argv) {
    if (argc != 3) return 2;
    HANDLE mutex = CreateMutexW(NULL, FALSE, L"Global\\PlanetWindows-Installer-Transaction-v2");
    if (!mutex || GetLastError() == ERROR_ALREADY_EXISTS) return 3;
    HANDLE ready = CreateFileA(argv[1], GENERIC_WRITE, 0, NULL, CREATE_NEW, FILE_ATTRIBUTE_NORMAL, NULL);
    if (ready == INVALID_HANDLE_VALUE) return 4;
    CloseHandle(ready);
    while (GetFileAttributesA(argv[2]) == INVALID_FILE_ATTRIBUTES) Sleep(100);
    CloseHandle(mutex);
    return 0;
}
C_CODE
x86_64-w64-mingw32-gcc -O2 -o /tmp/mutex-holder.exe /tmp/mutex-holder.c
xvfb-run -a sh -eu <<'WINE_TEST'
wineboot -u >/dev/null 2>&1
wineserver -w
cp /opt/planet-wine-test/bcryptprimitives.dll "$WINEPREFIX/drive_c/windows/system32/bcryptprimitives.dll"
root="$WINEPREFIX/drive_c/audit"
install="$root/planet"
data="$WINEPREFIX/drive_c/users/root/AppData/Roaming/Planet Windows"
shortcut="$WINEPREFIX/drive_c/users/root/AppData/Roaming/Microsoft/Windows/Start Menu/Programs/Planet Windows.lnk"
state='HKCU\Software\PlanetWindows\InstallerState'
entry='HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\org.planetable.planetwindows'
anchor='HKCU\Software\PlanetWindows\UpdateTransaction'
mkdir -p "$root" "$data"
wine /input/old.exe /S '/D=C:\audit\planet'
wineserver -w
[ -f "$install/Planet Windows.exe" ] && [ -f "$install/Uninstall.exe" ] && [ -f "$shortcut" ] || { echo 'FAIL: protocol-2 old installation missing'; exit 1; }
wine reg query "$state" /v TransactionProtocol | tr -d '\r' | grep -q 'REG_SZ[[:space:]]*2' || { echo 'FAIL: old installer is not protocol 2'; exit 1; }
wine reg query "$entry" /v DisplayVersion | tr -d '\r' | grep -q 'REG_SZ[[:space:]]*0\.2\.0' || { echo 'FAIL: old version is not 0.2.0'; exit 1; }
printf 'preserve unknown' > "$install/custom.txt"
printf '{"version":4,"sites":[],"articles":[]}' > "$data/library.json"
oldhash=$(sha256sum "$install/Planet Windows.exe" | cut -d' ' -f1)
files() { find "$install" -type f -print0 | sort -z | xargs -0 sha256sum; }
registry_snapshot() { wine reg query "$state" /s | tr -d '\r'; wine reg query "$entry" /s | tr -d '\r'; }
before_files=$(files)
before_registry=$(registry_snapshot)
before_shortcut=$(sha256sum "$shortcut")
wine /tmp/mutex-holder.exe 'C:\audit\mutex-ready' 'C:\audit\mutex-release' &
holder=$!
for i in $(seq 1 60); do [ -f "$root/mutex-ready" ] && break; sleep 1; done
[ -f "$root/mutex-ready" ] && kill -0 "$holder" || { echo 'FAIL: test mutex holder did not start'; exit 1; }
if wine /input/new.exe /S '/D=C:\audit\planet'; then echo 'FAIL: upgrade ignored running-app mutex'; exit 1; fi
kill -0 "$holder" || { echo 'FAIL: upgrade terminated mutex holder'; exit 1; }
[ "$before_files" = "$(files)" ] && [ "$before_registry" = "$(registry_snapshot)" ] || { echo 'FAIL: rejected upgrade changed installed files or registry'; exit 1; }
touch "$root/mutex-release"
wait "$holder"
wineserver -w
echo 'PASS: application-style global mutex rejects upgrade without stopping holder or changing installation'
wine /input/new.exe /S '/D=C:\audit\planet' || { echo 'FAIL: protocol-2 upgrade was rejected'; exit 1; }
wineserver -w
wine reg query "$entry" /v DisplayVersion | tr -d '\r' | grep -q 'REG_SZ[[:space:]]*0\.3\.0' || { echo 'FAIL: version not advanced to 0.3.0'; exit 1; }
newhash=$(sha256sum "$install/Planet Windows.exe" | cut -d' ' -f1)
[ "$newhash" = "$EXPECTED_EXE_SHA256" ] || { echo 'FAIL: installed executable does not match 0.3 payload'; exit 1; }
set -- "$install"/.planet-update-completed-*/old-Planet\ Windows.exe
[ "$#" -eq 1 ] && [ -f "$1" ] && [ "$(sha256sum "$1" | cut -d' ' -f1)" = "$oldhash" ] || { echo 'FAIL: original executable not preserved as transaction evidence'; exit 1; }
[ ! -e "$install/.planet-update" ] && ! wine reg query "$anchor" >/dev/null 2>&1 || { echo 'FAIL: completed upgrade has pending state'; exit 1; }
[ "$(cat "$install/custom.txt")" = 'preserve unknown' ] && [ "$(cat "$data/library.json")" = '{"version":4,"sites":[],"articles":[]}' ] && [ "$before_shortcut" = "$(sha256sum "$shortcut")" ] || { echo 'FAIL: upgrade changed user content or shortcut'; exit 1; }
echo 'PASS: protocol-2 0.2 to 0.3 upgrade installed exact payload and preserved user content'
after_files=$(files)
after_registry=$(registry_snapshot)
wine /input/new.exe /S '/D=C:\audit\planet' || { echo 'FAIL: identical reinstallation rejected'; exit 1; }
wineserver -w
[ "$after_files" = "$(files)" ] && [ "$after_registry" = "$(registry_snapshot)" ] || { echo 'FAIL: identical reinstallation changed files or registry'; exit 1; }
echo 'PASS: identical 0.3 reinstall is a no-op'
if wine /input/old.exe /S '/D=C:\audit\planet'; then echo 'FAIL: downgrade accepted'; exit 1; fi
wineserver -w
[ "$after_files" = "$(files)" ] && [ "$after_registry" = "$(registry_snapshot)" ] || { echo 'FAIL: refused downgrade changed files or registry'; exit 1; }
echo 'PASS: downgrade refused without changing installation'
wine "$install/Uninstall.exe" /S || true
wineserver -w
[ ! -f "$install/Planet Windows.exe" ] && [ ! -f "$install/Uninstall.exe" ] || { echo 'FAIL: uninstall left active payload'; exit 1; }
[ "$(cat "$install/custom.txt")" = 'preserve unknown' ] && [ "$(cat "$data/library.json")" = '{"version":4,"sites":[],"articles":[]}' ] || { echo 'FAIL: uninstall changed user content'; exit 1; }
if wine reg query "$entry" >/dev/null 2>&1; then echo 'FAIL: uninstall entry remains'; exit 1; fi
echo 'PASS: upgraded version uninstalled while retaining unknown files and user data'
legacy_safe="$root/legacy-safe"
wine /input/legacy-safe.exe /S '/D=C:\audit\legacy-safe'
wineserver -w
[ -f "$legacy_safe/Planet Windows.exe" ] && [ -f "$legacy_safe/Uninstall.exe" ] || { echo 'FAIL: legacy safe fixture installation missing'; exit 1; }
if wine reg query "$state" /v TransactionProtocol >/dev/null 2>&1; then echo 'FAIL: legacy safe fixture unexpectedly has protocol 2'; exit 1; fi
legacy_safe_before=$(find "$legacy_safe" -type f -print0 | sort -z | xargs -0 sha256sum)
legacy_safe_registry=$(registry_snapshot)
if wine /input/new.exe /S '/D=C:\audit\legacy-safe'; then echo 'FAIL: unlocked legacy safe installation accepted'; exit 1; fi
wineserver -w
[ "$legacy_safe_before" = "$(find "$legacy_safe" -type f -print0 | sort -z | xargs -0 sha256sum)" ] && [ "$legacy_safe_registry" = "$(registry_snapshot)" ] || { echo 'FAIL: refused legacy safe update changed files or registration'; exit 1; }
wine "$legacy_safe/Uninstall.exe" /S || true
wineserver -w
[ ! -f "$legacy_safe/Uninstall.exe" ] || { echo 'FAIL: legacy safe fixture did not uninstall'; exit 1; }
echo 'PASS: legacy safe installation without protocol lock refused without modification'
legacy="$root/legacy"
wine /input/legacy.exe /S '/D=C:\audit\legacy'
wineserver -w
[ -f "$legacy/Planet Windows.exe" ] && [ -f "$legacy/Uninstall.exe" ] || { echo 'FAIL: legacy fixture installation missing'; exit 1; }
legacy_before=$(find "$legacy" -type f -print0 | sort -z | xargs -0 sha256sum)
if wine /input/new.exe /S '/D=C:\audit\legacy'; then echo 'FAIL: untrusted legacy installation accepted'; exit 1; fi
wineserver -w
[ "$legacy_before" = "$(find "$legacy" -type f -print0 | sort -z | xargs -0 sha256sum)" ] && wine reg query "$entry" >/dev/null 2>&1 || { echo 'FAIL: rejected legacy upgrade changed files or registration'; exit 1; }
echo 'PASS: untrusted legacy installation refused without changing files or registration'
WINE_TEST
