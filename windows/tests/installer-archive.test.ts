import { expect, test } from "bun:test";
import { resolve } from "node:path";

const helper = resolve(import.meta.dir, "../scripts/release-audit/archive-entries.ps1");
const payload = [
  "Planet Windows.exe", "icon.png", "ipfs.exe", "kubo-LICENSE.txt",
  "kubo-LICENSE-MIT.txt", "kubo-LICENSE-APACHE.txt", "LICENSE-PLANET.txt",
  "THIRD-PARTY-NOTICES.txt", "KUBO-BUILDINFO.txt", "KUBO-DEPENDENCY-LICENSES.txt",
];
const rootUninstaller = "Uninstall.exe";
const stagedUninstaller = "$PLUGINSDIR\\update-payload\\Uninstall.exe";
const legacyEntries = [
  "$PLUGINSDIR\\System.dll", "$PLUGINSDIR\\planet-installer-guard.exe", "$PLUGINSDIR\\nsExec.dll",
  ...payload.flatMap(name => [name, `$PLUGINSDIR\\update-payload\\${name}`]),
];
const uninstallerHash = "a".repeat(64);

function audit(entries: string[], stagedHash = uninstallerHash) {
  const command = `
$ErrorActionPreference = 'Stop'
. $env:PLANET_ARCHIVE_HELPER
$fixture = $env:PLANET_ARCHIVE_FIXTURE | ConvertFrom-Json
$reads = [Collections.Generic.List[string]]::new()
$readHash = {
  param([string]$Name)
  $reads.Add($Name)
  if ($Name -eq 'Uninstall.exe') { return $fixture.rootHash }
  return $fixture.stagedHash
}
$result = Assert-InstallerArchiveEntries -Entries $fixture.entries -PayloadFiles $fixture.payload -GetEntryHash $readHash
@{ audit = $result; reads = @($reads.ToArray()) } | ConvertTo-Json -Depth 4
`;
  return Bun.spawnSync(["powershell", "-NoProfile", "-Command", command], {
    env: {
      ...process.env,
      PLANET_ARCHIVE_HELPER: helper,
      PLANET_ARCHIVE_FIXTURE: JSON.stringify({ entries, payload, rootHash: uninstallerHash, stagedHash }),
    },
  });
}

test("legacy NSIS listing audits without claiming an unseen uninstaller hash", () => {
  const result = audit(legacyEntries);
  if (result.exitCode !== 0) throw new Error(result.stderr.toString());
  expect(JSON.parse(result.stdout.toString())).toEqual({
    audit: { status: "not-listed", sha256: null },
    reads: [],
  });
});

test("new NSIS listing requires matching generated uninstallers and records their hash", () => {
  const result = audit([...legacyEntries, rootUninstaller, stagedUninstaller]);
  if (result.exitCode !== 0) throw new Error(result.stderr.toString());
  expect(JSON.parse(result.stdout.toString())).toEqual({
    audit: { status: "matched", sha256: uninstallerHash },
    reads: [rootUninstaller, stagedUninstaller],
  });
});

for (const [name, generated] of [
  ["missing staged copy", [rootUninstaller]],
  ["missing root copy", [stagedUninstaller]],
  ["duplicate root copy", [rootUninstaller, rootUninstaller, stagedUninstaller]],
  ["duplicate staged copy", [rootUninstaller, stagedUninstaller, stagedUninstaller]],
  ["case-variant duplicate", [rootUninstaller, "uninstall.exe", stagedUninstaller]],
] as const) {
  test(`NSIS listing rejects an uninstaller pair with ${name}`, () => {
    const result = audit([...legacyEntries, ...generated]);
    expect(result.exitCode).not.toBe(0);
    expect(result.stderr.toString()).toContain("exactly one root and one staged Uninstall.exe");
  });
}

test("NSIS audit rejects different generated uninstaller bytes", () => {
  const result = audit([...legacyEntries, rootUninstaller, stagedUninstaller], "b".repeat(64));
  expect(result.exitCode).not.toBe(0);
  expect(result.stderr.toString()).toContain("Generated root and staged uninstallers differ");
});

for (const unexpected of ["unexpected.exe", "Uninstall-extra.exe", "nested\\Uninstall.exe"]) {
  test(`NSIS listing keeps rejecting unexpected payload ${unexpected}`, () => {
    const result = audit([...legacyEntries, rootUninstaller, stagedUninstaller, unexpected]);
    expect(result.exitCode).not.toBe(0);
    expect(result.stderr.toString()).toContain("Installer has unexpected payload entries");
    expect(result.stderr.toString()).toContain(JSON.stringify(unexpected));
  });
}
