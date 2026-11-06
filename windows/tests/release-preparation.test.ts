import { afterAll, expect, test } from "bun:test";
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { createHash } from "node:crypto";

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
$signature = if ($env:PLANET_RELEASE_FIXTURE -in @('unsigned', 'unsigned-license')) { 'NotSigned' } else { 'Valid' }
$reviews = if ($env:PLANET_RELEASE_FIXTURE -in @('license', 'unsigned-license')) { 1 } else { 0 }
if ($env:PLANET_RELEASE_FIXTURE -eq 'hash') { $hash = '0' * 64 }
$signatures = @(@{file='Planet Windows.exe';status=$signature},@{file=$name;status=$signature})
switch ($env:PLANET_RELEASE_FIXTURE) {
  'app-unsigned' { $signatures[0].status = 'NotSigned' }
  'installer-unsigned' { $signatures[1].status = 'NotSigned' }
  'invalid-signature' { $signatures[0].status = 'HashMismatch' }
  'untrusted-signature' { $signatures[1].status = 'NotTrusted' }
  'unknown-signature' { $signatures[0].status = 'UnknownError' }
  'missing-signature' { $signatures = @($signatures[0]) }
  'empty-signatures' { $signatures = @() }
  'duplicate-signature' { $signatures += $signatures[0] }
  'case-duplicate-signature' { $signatures += @{file='planet windows.exe';status='Valid'} }
  'missing-signature-status' { $signatures[0].Remove('status') }
  'null-signature' { $signatures[0] = $null }
}
@{installer=$name; installerSha256=$hash; signatures=$signatures; kuboModulesRequiringManualLicenseReview=$reviews} | ConvertTo-Json -Depth 4
`);
  const git = (...args: string[]) => {
    const result = Bun.spawnSync(["git", "-C", root, "-c", "core.hooksPath=.fixture-hooks", "-c", "commit.gpgSign=false", "-c", "tag.gpgSign=false", "-c", "user.name=Release test", "-c", "user.email=fixture@example.invalid", ...args], { env: fixtureEnv });
    if (result.exitCode !== 0) throw new Error(result.stderr.toString());
  };
  git("init", "--quiet");
  git("add", ".");
  git("commit", "--quiet", "-m", "test: release fixture");
  git("tag", "-a", `v${version}`, "-m", "Test only");
  const sourceArchive = join(folder, `planet-windows-source-${version}.zip`);
  const sourceManifest = join(folder, "source-manifest.json");
  const sourceBytes = Buffer.from("TEST SOURCE ARCHIVE BYTES");
  writeFileSync(sourceArchive, sourceBytes);
  const source = {
    schemaVersion: 1,
    sourceCommit: Bun.spawnSync(["git", "-C", root, "rev-parse", "HEAD"], { env: fixtureEnv }).stdout.toString().trim(),
    version,
    archive: `planet-windows-source-${version}.zip`,
    archiveSha256: createHash("sha256").update(sourceBytes).digest("hex"),
    archiveBytes: sourceBytes.length,
  };
  writeFileSync(sourceManifest, JSON.stringify(source));
  const run = (release = true, mode = "", allowUnsigned = false, includeSource = true) => Bun.spawnSync([
    "powershell", "-NoProfile", "-File", join(root, "scripts/prepare-release.ps1"), "-BundleDir", bundle,
    ...(release ? ["-Release", "-Tag", `v${version}`] : []),
    ...(allowUnsigned ? ["-AllowUnsignedPrerelease"] : []),
    ...(includeSource ? ["-SourceManifest", sourceManifest] : []),
  ], { cwd: folder, env: { ...fixtureEnv, PLANET_RELEASE_FIXTURE: mode } });
  const outputs = () => {
    const path = join(root, "windows/.mygo/release-drafts");
    return existsSync(path) ? readdirSync(path).map(name => join(path, name)) : [];
  };
  return { root, git, run, outputs, source, sourceArchive, sourceManifest };
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
  const output = f.outputs()[0]!;
  const manifest = JSON.parse(readFileSync(join(output, "release-manifest.json"), "utf8"));
  expect(manifest.mode).toBe("release");
  expect(manifest.correspondingSource).toEqual(f.source);
  expect(readFileSync(join(output, f.source.archive))).toEqual(readFileSync(f.sourceArchive));
  expect(readFileSync(join(output, "SHA256SUMS.txt"), "utf8")).toContain(`${f.source.archiveSha256}  ${f.source.archive}`);
  expect(JSON.parse(readFileSync(join(output, "source-manifest.json"), "utf8"))).toEqual(f.source);
});

test("release requires source while a draft can omit it", () => {
  const f = fixture();
  const release = f.run(true, "", false, false);
  expect(release.exitCode).not.toBe(0);
  expect(release.stderr.toString()).toContain("Release requires a corresponding source archive");
  expect(f.outputs()).toEqual([]);
  expect(f.run(false, "unsigned", false, false).exitCode).toBe(0);
});

for (const changed of ["commit", "version", "path", "hash", "bytes", "tampered"] as const) {
  test(`source archive ${changed} mismatch blocks release before copying`, () => {
    const f = fixture();
    const source = { ...f.source };
    if (changed === "commit") source.sourceCommit = "0".repeat(40);
    if (changed === "version") source.version = "9.9.9";
    if (changed === "path") source.archive = "../planet-windows-source-other.zip";
    if (changed === "hash") source.archiveSha256 = "0".repeat(64);
    if (changed === "bytes") source.archiveBytes++;
    if (changed === "tampered") writeFileSync(f.sourceArchive, "CHANGED SOURCE BYTES");
    writeFileSync(f.sourceManifest, JSON.stringify(source));
    const result = f.run();
    expect(result.exitCode).not.toBe(0);
    expect(result.stderr.toString()).toContain("Source");
    expect(f.outputs()).toEqual([]);
  });
}

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

test("explicit unsigned prerelease records its policy and prominent distribution notice", () => {
  const f = fixture("0.0.1-alpha.1");
  const result = f.run(true, "unsigned", true);
  if (result.exitCode !== 0) throw new Error(result.stderr.toString());
  const output = f.outputs()[0]!;
  const manifest = JSON.parse(readFileSync(join(output, "release-manifest.json"), "utf8"));
  expect(manifest.mode).toBe("release");
  expect(manifest.prerelease).toBe(true);
  expect(manifest.unsignedPrerelease).toBe(true);
  expect(manifest.signingPolicy).toBe("allow-unsigned-prerelease");
  expect(manifest.firstPartySigningStatus).toBe("unsigned");
  expect(manifest.signatures.every((entry: { status: string }) => entry.status === "NotSigned")).toBe(true);
  const notes = readFileSync(join(output, "release-notes.md"), "utf8");
  expect(notes).toContain("UNSIGNED PRERELEASE");
  expect(notes.split("\n").slice(0, 3)).toEqual(["# v0.0.1-alpha.1 — 2020-01-01", "", "## UNSIGNED PRERELEASE"]);
  expect(notes).toContain("SHA-256");
  expect(notes).toContain("publisher identity");
  expect(notes).toContain("Windows");
  expect(notes).toContain("security prompts");
  expect(notes).toContain("Released fix.");
  expect(notes.indexOf("security prompts")).toBeLessThan(notes.indexOf("Released fix."));
});

test("unsigned prerelease opt-in distinguishes signed and partially unsigned payloads", () => {
  for (const mode of ["", "app-unsigned", "installer-unsigned"]) {
    const f = fixture("0.0.1-alpha.1");
    const result = f.run(true, mode, true);
    if (result.exitCode !== 0) throw new Error(result.stderr.toString());
    const output = f.outputs()[0]!;
    const manifest = JSON.parse(readFileSync(join(output, "release-manifest.json"), "utf8"));
    expect(manifest.unsignedPrerelease).toBe(mode !== "");
    expect(manifest.firstPartySigningStatus).toBe(mode ? "unsigned" : "valid");
    expect(readFileSync(join(output, "release-notes.md"), "utf8").includes("UNSIGNED PRERELEASE")).toBe(mode !== "");
  }
});

test("unsigned exception rejects stable releases and draft preparation", () => {
  for (const [version, release] of [["0.3.0", true], ["0.0.1-alpha.1", false]] as const) {
    const f = fixture(version);
    const result = f.run(release, "unsigned", true);
    expect(result.exitCode).not.toBe(0);
    expect(result.stderr.toString()).toContain("AllowUnsignedPrerelease requires -Release and a prerelease version");
    expect(f.outputs()).toEqual([]);
  }
});

for (const mode of ["unsigned-license", "invalid-signature", "untrusted-signature", "unknown-signature", "missing-signature", "empty-signatures", "duplicate-signature", "case-duplicate-signature", "missing-signature-status", "null-signature"]) {
  test(`unsigned prerelease exception cannot bypass ${mode}`, () => {
    const f = fixture("0.0.1-alpha.1");
    const result = f.run(true, mode, true);
    expect(result.exitCode).not.toBe(0);
    expect(result.stderr.toString()).toContain("Release blocked");
    expect(f.outputs()).toEqual([]);
  });
}

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
