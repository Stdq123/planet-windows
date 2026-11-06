import { expect, test } from "bun:test";
import { resolve } from "node:path";

const installer = resolve(import.meta.dir, "../scripts/build-safe-installer.ps1");
const publishTest = resolve(import.meta.dir, "../scripts/test-publish-build.ps1");

test("PowerShell packaging rejects quote-stripped versions before reading payloads", () => {
  // PowerShell 5.1 removes embedded quotes in native arguments. The validator
  // must prove that Bun accepted the original value, not its altered spelling.
  const result = Bun.spawnSync(["powershell", "-NoProfile", "-Command", `
$ErrorActionPreference = 'Stop'
$candidate = '0.0.1-alpha.1' + [char]34
& $env:PLANET_INSTALLER_SCRIPT -PayloadDir 'missing-payload' -OutputFile 'missing-output.exe' -ReleaseVersion $candidate
`], { env: { ...process.env, PLANET_INSTALLER_SCRIPT: installer } });
  expect(result.exitCode).not.toBe(0);
  expect(result.stderr.toString()).toContain("Invalid release version");
  expect(result.stderr.toString()).not.toContain("Cannot find path");
});

for (const version of ["0.3.0", "0.0.1-alpha.1"]) {
  test(`publishing ${version} build output preserves old files and refuses partial payloads`, () => {
    const result = Bun.spawnSync(["powershell", "-NoProfile", "-File", publishTest, "-ReleaseVersion", version]);
    if (result.exitCode !== 0) throw new Error(result.stderr.toString());
    expect(result.stdout.toString()).toContain("Build output publishing checks passed");
  });
}
