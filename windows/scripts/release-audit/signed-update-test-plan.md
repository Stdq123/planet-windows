# Signed update independent test plan

This is an acceptance plan, not a statement that any test has passed. Do not
create a publisher identity or use production keys, feeds, sites or installs.

## Trust and network

Use test-only Ed25519 keys generated in memory and loopback TLS fixtures. Each
fixture needs a known test CA injected only into its test client; production
TLS verification must remain enabled. Check valid signature and exact signed
bytes, malformed base64, duplicate JSON keys, truncated/oversized bodies,
wrong keys, forged signatures and changes to every signed field. Check invalid
app/platform, version ordering and equal-version conflicts, expiry, future dates,
maximum validity and replay policy across restart and trust-source changes.

Exercise redirects on every hop, including same origin, different host, port,
scheme downgrade and credentials. Check connection timeout, cancellation,
stalled transfer, decompression limits and excessive redirects. A deliberately
configured local test endpoint is not evidence of public network reachability.

## Settings, download and identity

Change or disable the source/key while a controlled request is blocked. Release
the response and assert it cannot replace newer UI state or authorize download.
After a verified check, change trust settings before download and verify refusal.
Exercise concurrent checks and downloads with distinct responses and assert each
result remains bound to the settings used for verification.

Use separate temporary directories with known foreign files, same-name targets,
links and locked files. Verify streamed exact size and SHA-256, private staging,
no overwrite, bounded disk usage, and preservation of existing files on errors,
cancellation or restart. Replace or modify a completed installer after checking
and assert any later use revalidates its identity, size and digest. No installer
must execute as a side effect of a check or download.

## Close-and-install, if implemented

If no handoff is implemented, verify the UI says so and provides only the
verified file without claiming installation. If implemented, use an isolated
Windows VM or disposable Wine environment with a real protocol-2 test install
and a test-signed local feed. The app, helper and installer must all operate on
temporary data, private HKCU and a temporary install path. Do not run them on
the host user's real installation or registry. Tests must demonstrate:

- The UI requires explicit action after verification; changes to feed, key or
  downloaded file before action stop the handoff.
- The existing app exits and releases the protocol mutex before the installer
  attempts its transaction. An active app or another installer is never killed.
- A detached helper survives application shutdown, and a launch failure or
  nonzero installer result is surfaced or leaves a documented recovery route.
- Terminate the app, helper, installer parent and guard separately at controlled
  transitions; restart and check that the original app or a recoverable journal
  remains, with user files and trust settings intact. Repeat after registry
  anchor creation, partial download and before/after final installer activation.
- The child does not use a different same-named mutex after parent termination;
  reproduce the parent/child handoff with controlled process synchronization.
- A modified, missing or locked owned file and an untrusted legacy installation
  fail without deleting or replacing unknown files or the user's library.

An in-process injected error is not a process-kill test, and Wine is not an
isolated native Windows installation. Power-loss and cache durability require a
separate VM hard-power-off experiment and must not be claimed from either.

## Final evidence

Record commands, input installer and fixture hashes, environment, each actual
result, failing/skipped tests, digital-signature status and cleanup/isolation
boundaries. Use `.mygo/releases/<GUID>/windows-amd64/` for an actual final
package, not an old MyGo default installer or a test-only fixture. Keep test
secrets and Wine compatibility DLLs out of release payloads.
