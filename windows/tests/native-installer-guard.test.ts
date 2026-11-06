import { expect, test } from "bun:test";
import { resolve } from "node:path";

const script = resolve(import.meta.dir, "../scripts/test-installer-native.ps1");
const baseEnv = Object.fromEntries(Object.entries(process.env).filter(([name]) => !/^(GITHUB_|RUNNER_|PLANET_NATIVE_INSTALLER_SMOKE$)/i.test(name)));

test("native installer harness is valid PowerShell without executing it", () => {
  const command = "$taskTokens=$null; $taskErrors=$null; [System.Management.Automation.Language.Parser]::ParseFile($env:PLANET_SMOKE_SCRIPT,[ref]$taskTokens,[ref]$taskErrors) | Out-Null; if ($taskErrors.Count) { $taskErrors | ForEach-Object { Write-Error $_ }; exit 1 }";
  const result = Bun.spawnSync(["powershell", "-NoProfile", "-Command", command], { env: { ...baseEnv, PLANET_SMOKE_SCRIPT: script } });
  if (result.exitCode !== 0) throw new Error(result.stderr.toString());
  expect(result.exitCode).toBe(0);
});

test("native installer harness refuses local and insufficient CI markers before reading inputs", () => {
  for (const markers of [
    {},
    { GITHUB_ACTIONS: "true", RUNNER_OS: "Windows" },
    { GITHUB_ACTIONS: "true", RUNNER_OS: "Windows", RUNNER_ENVIRONMENT: "self-hosted", PLANET_NATIVE_INSTALLER_SMOKE: "1" },
    { GITHUB_ACTIONS: "true", RUNNER_OS: "Windows", RUNNER_ENVIRONMENT: "github-hosted" },
  ]) {
    const result = Bun.spawnSync(["powershell", "-NoProfile", "-File", script,
      "-Installer", "C:\\nonexistent-planet-native-smoke\\DO-NOT-EXECUTE.exe",
      "-ExpectedSHA256", "0".repeat(64), "-ExpectedVersion", "0.0.1-alpha.1",
      "-EvidenceDir", "C:\\nonexistent-planet-native-smoke\\DO-NOT-CREATE",
    ], { env: { ...baseEnv, ...markers } });
    expect(result.exitCode).not.toBe(0);
    expect(result.stderr.toString()).toContain("Refusing normal host execution");
  }
});
