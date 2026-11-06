import { afterAll, expect, test } from "bun:test";
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";

const testRoot = mkdtempSync(join(tmpdir(), "planet-release-tests-"));
const repo = resolve(import.meta.dir, "../..");
// Git hooks export repository selectors; -C alone cannot isolate fixture commands.
const fixtureEnv = Object.fromEntries(
  Object.entries(process.env).filter(([name]) => !name.toUpperCase().startsWith("GIT_")),
);
afterAll(() => {
  if (dirname(testRoot) !== resolve(tmpdir()) || !testRoot.split(/[\\/]/).at(-1)?.startsWith("planet-release-tests-")) {
    throw new Error("Unexpected fixture cleanup path");
  }
  rmSync(testRoot, { recursive: true });
});

function fixture(version = "0.3.0") {
  const folder = mkdtempSync(join(testRoot, "case "));
  const root = join(folder, "repository");
  const bundle = join(folder, "bundle");
  mkdirSync(join(root, "scripts"), { recursive: true });
  mkdirSync(join(root, "windows/scripts"), { recursive: true });
  mkdirSync(bundle);
  for (const name of ["prepare-release.ps1", "release-metadata.ts", "release-version.ts"]) {
    copyFileSync(join(repo, "scripts", name), join(root, "scripts", name));
  }
  writeFileSync(join(root, ".gitignore"), "windows/.mygo/\n");
  writeFileSync(join(root, "windows/release.json"), JSON.stringify({ version }) + "\n");
  writeFileSync(join(root, "CHANGELOG.md"), `# Changelog\n\n## [Unreleased]\n- Draft fix.\n\n## [${version}] - 2020-01-01\n- Released fix.\n`);
  writeFileSync(join(bundle, `planet-windows-safe-setup-${version}-amd64.exe`), "TEST TEXT ONLY, NOT EXECUTABLE");
  // Stub only the existing binary audit. Exercise real git, Bun, PowerShell,
  // metadata checks, copy and hashing without a certificate or installer execution.
  writeFileSync(join(root, "windows/scripts/audit-release.ps1"), `param([string]$BundleDir)
$name = 'planet-windows-safe-setup-${version}-amd64.exe'
$hash = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $BundleDir $name)).Hash.ToLowerInvariant()
$signature = if ($env:PLANET_RELEASE_FIXTURE -eq 'unsigned') { 'NotSigned' } else { 'Valid' }
$reviews = if ($env:PLANET_RELEASE_FIXTURE -eq 'license') { 1 } else { 0 }
if ($env:PLANET_RELEASE_FIXTURE -eq 'hash') { $hash = '0' * 64 }
@{installer=$name; installerSha256=$hash; signatures=@(@{file='Planet Windows.exe';status=$signature},@{file=$name;status=$signature}); kuboModulesRequiringManualLicenseReview=$reviews} | ConvertTo-Json -Depth 4
`);
  const git = (...args: string[]) => {
    const result = Bun.spawnSync(["git", "-C", root, "-c", "core.hooksPath=.fixture-hooks", "-c", "commit.gpgSign=false", "-c", "tag.gpgSign=false", "-c", "user.name=Release test", "-c", "user.email=fixture@example.invalid", ...args], { env: fixtureEnv });
    if (result.exitCode !== 0) throw new Error(result.stderr.toString());
  };
  git("init", "--quiet");
  git("add", ".");
  git("commit", "--quiet", "-m", "test: release fixture");
  git("tag", "-a", `v${version}`, "-m", "Test only");
  const run = (release = true, mode = "") => Bun.spawnSync([
    "powershell", "-NoProfile", "-File", join(root, "scripts/prepare-release.ps1"), "-BundleDir", bundle,
    ...(release ? ["-Release", "-Tag", `v${version}`] : []),
  ], { cwd: folder, env: { ...fixtureEnv, PLANET_RELEASE_FIXTURE: mode } });
  const outputs = () => {
    const path = join(root, "windows/.mygo/release-drafts");
    return existsSync(path) ? readdirSync(path).map(name => join(path, name)) : [];
  };
  return { root, git, run, outputs };
}

test("draft materials copy bytes, record dirtiness and retain unresolved audit state", () => {
  const f = fixture();
  writeFileSync(join(f.root, "uncommitted.txt"), "draft work");
  const result = f.run(false, "unsigned");
  if (result.exitCode !== 0) throw new Error(result.stderr.toString());
  expect(result.exitCode).toBe(0);
  const output = f.outputs()[0]!;
  const manifest = JSON.parse(readFileSync(join(output, "release-manifest.json"), "utf8"));
  expect(manifest.mode).toBe("draft");
  expect(manifest.sourceCheckoutDirty).toBe(true);
  expect(manifest.signatures[0].status).toBe("NotSigned");
  expect(readFileSync(join(output, "SHA256SUMS.txt"), "utf8")).toContain(manifest.installerSha256);
  expect(readFileSync(join(output, "release-notes.md"), "utf8")).toContain("DRAFT");
});

test("release materials require a clean annotated tag and accepted audit", () => {
  const f = fixture();
  const result = f.run();
  if (result.exitCode !== 0) throw new Error(result.stderr.toString());
  expect(result.exitCode).toBe(0);
  expect(JSON.parse(readFileSync(join(f.outputs()[0]!, "release-manifest.json"), "utf8")).mode).toBe("release");
});

test("prerelease materials preserve the channel and require the same audit gates", () => {
  const f = fixture("0.0.1-alpha.1");
  const result = f.run();
  if (result.exitCode !== 0) throw new Error(result.stderr.toString());
  const manifest = JSON.parse(readFileSync(join(f.outputs()[0]!, "release-manifest.json"), "utf8"));
  expect(manifest.version).toBe("0.0.1-alpha.1");
  expect(manifest.tag).toBe("v0.0.1-alpha.1");
  expect(manifest.prerelease).toBe(true);
  for (const mode of ["unsigned", "license"]) {
    const rejected = f.run(true, mode);
    expect(rejected.exitCode).not.toBe(0);
    expect(rejected.stderr.toString()).toContain("Release blocked");
  }
});

for (const mode of ["unsigned", "license", "hash"]) {
  test(`release preparation rejects ${mode} failures`, () => {
    const result = fixture().run(true, mode);
    expect(result.exitCode).not.toBe(0);
    expect(result.stderr.toString()).toContain(mode === "hash" ? "Installer changed after auditing" : "Release blocked");
  });
}

test("dirty checkouts are blocked before producing release materials", () => {
  const dirty = fixture();
  writeFileSync(join(dirty.root, "uncommitted.txt"), "unfinished");
  const result = dirty.run();
  expect(result.exitCode).not.toBe(0);
  expect(result.stderr.toString()).toContain("requires a clean checkout");
  expect(dirty.outputs()).toEqual([]);
});

test("lightweight tags are blocked before producing release materials", () => {
  const lightweight = fixture();
  lightweight.git("tag", "-d", "v0.3.0");
  lightweight.git("tag", "v0.3.0");
  const result = lightweight.run();
  expect(result.exitCode).not.toBe(0);
  expect(result.stderr.toString()).toContain("requires an existing annotated tag");
  expect(lightweight.outputs()).toEqual([]);
});

test("mismatched tags are blocked before producing release materials", () => {
  const mismatch = fixture();
  mismatch.git("commit", "--allow-empty", "--quiet", "-m", "test: later commit");
  const result = mismatch.run();
  expect(result.exitCode).not.toBe(0);
  expect(result.stderr.toString()).toContain("Release tag must point");
  expect(mismatch.outputs()).toEqual([]);
});
