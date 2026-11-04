/* Test-only shim for Wine 8, which lacks the Windows system DLL required by
 * modern Go programs. This DLL is used only inside the disposable container;
 * it must never be distributed with the Windows application. */
#include <windows.h>
#include <bcrypt.h>

__declspec(dllexport) BOOLEAN WINAPI ProcessPrng(PBYTE output, SIZE_T length) {
    if (length > 0xffffffffu) return FALSE;
    return BCryptGenRandom(NULL, output, (ULONG)length, BCRYPT_USE_SYSTEM_PREFERRED_RNG) == 0;
}
