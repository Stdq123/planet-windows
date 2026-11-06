import { afterAll, expect, test } from "bun:test";
import { chmodSync, copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { pathToFileURL } from "node:url";

const testRoot = mkdtempSync(join(tmpdir(), "planet-source-tests-"));
const script = resolve(import.meta.dir, "../../scripts/prepare-source.ps1");
const cleanEnv = Object.fromEntries(Object.entries(process.env).filter(([name]) => !name.toUpperCase().startsWith("GIT_")));
afterAll(() => {
  if (dirname(testRoot) !== resolve(tmpdir()) || !testRoot.split(/[\\/]/).at(-1)?.startsWith("planet-source-tests-")) throw new Error("Unexpected cleanup path");
  rmSync(testRoot, { recursive: true, force: true });
});

function run(command: string[], cwd: string, env = cleanEnv) {
  const result = Bun.spawnSync(command, { cwd, env });
  if (result.exitCode !== 0) throw new Error(result.stderr.toString() + result.stdout.toString());
  return result.stdout.toString().trim();
}

const shortTestRoot = run(["powershell", "-NoProfile", "-Command", `
Add-Type -TypeDefinition 'using System.Text; using System.Runtime.InteropServices; public static class SourceTestPaths { [DllImport("kernel32.dll", CharSet=CharSet.Unicode, SetLastError=true)] public static extern uint GetShortPathName(string path, StringBuilder output, uint count); }'
$buffer = New-Object Text.StringBuilder 32768
if ([SourceTestPaths]::GetShortPathName($env:SOURCE_TEST_ROOT, $buffer, $buffer.Capacity) -eq 0) { throw 'Cannot resolve test root short path.' }
$buffer.ToString()
`], testRoot, { ...cleanEnv, SOURCE_TEST_ROOT: testRoot });

function fixture(shortPaths = false) {
  const folder = mkdtempSync(join(shortPaths ? shortTestRoot : testRoot, "case-"));
  const root = join(folder, "repo");
  const app = join(root, "windows");
  const module = "github.com/ethereum/go-ethereum";
  const version = "v0.0.1";
  const proxy = join(folder, "proxy");
  const endpoint = join(proxy, module, "@v");
  const zipRoot = join(folder, "module-zip");
  const library = join(zipRoot, `${module}@${version}`);
  const cache = join(folder, "cache");
  const output = join(folder, "output");
  for (const path of [app, join(root, "docs"), join(root, "scripts"), endpoint, join(library, "common")]) mkdirSync(path, { recursive: true });
  copyFileSync(resolve(import.meta.dir, "../../scripts/release-version.ts"), join(root, "scripts/release-version.ts"));
  copyFileSync(script, join(root, "scripts/prepare-source.ps1"));
  const moduleFile = `module ${module}\n\ngo 1.23\n`;
  writeFileSync(join(library, "go.mod"), moduleFile);
  writeFileSync(join(library, "COPYING"), "TEST FIXTURE license text\n");
  writeFileSync(join(library, "COPYING.LESSER"), "TEST FIXTURE library license text\n");
  writeFileSync(join(library, "common", "value.go"), 'package common\nfunc Value() string { return "fixture" }\n');
  writeFileSync(join(library, "common", "value_test.go"), "package common\n// Full module source retains dependency tests.\n");
  writeFileSync(join(endpoint, `${version}.mod`), moduleFile);
  writeFileSync(join(endpoint, `${version}.info`), JSON.stringify({ Version: version, Time: "2020-01-01T00:00:00Z" }));
  writeFileSync(join(endpoint, "list"), version + "\n");
  run(["powershell", "-NoProfile", "-Command", `Add-Type -AssemblyName System.IO.Compression, System.IO.Compression.FileSystem
$zipRoot = (Get-Item -LiteralPath $env:SOURCE_FIXTURE_ZIP_ROOT).FullName
$archive = [IO.Compression.ZipFile]::Open($env:SOURCE_FIXTURE_ZIP_PATH, [IO.Compression.ZipArchiveMode]::Create)
try { foreach ($file in Get-ChildItem -LiteralPath $zipRoot -Recurse -File) {
  $name = $file.FullName.Substring($zipRoot.Length + 1).Replace('\\', '/')
  [IO.Compression.ZipFileExtensions]::CreateEntryFromFile($archive, $file.FullName, $name) | Out-Null
} } finally { $archive.Dispose() }`], folder,
    { ...cleanEnv, SOURCE_FIXTURE_ZIP_ROOT: zipRoot, SOURCE_FIXTURE_ZIP_PATH: join(endpoint, `${version}.zip`) });
  const env = { ...cleanEnv, GOTOOLCHAIN: "local", GOPROXY: pathToFileURL(proxy).href, GOSUMDB: "off", GOMODCACHE: cache, GOWORK: "off", GOFLAGS: "" };
  writeFileSync(join(app, "go.mod"), `module example.test/sourcefixture\n\ngo 1.23\n\nrequire ${module} ${version}\n`);
  writeFileSync(join(app, "main.go"), `package main\nimport ("fmt"; "${module}/common")\nfunc main() { fmt.Println(common.Value()) }\n`);
  run(["go", "mod", "download", module], app, env);
  writeFileSync(join(app, "release.json"), '{"version":"0.0.1-alpha.1"}\n');
  writeFileSync(join(app, "bun.lock"), "fixture locked frontend dependencies\n");
  writeFileSync(join(root, "LICENSE"), "TEST FIXTURE app license\n");
  writeFileSync(join(root, "docs", "rebuilding-release.md"), "# Fixture rebuilding instructions\n");
  writeFileSync(join(root, ".gitignore"), ".env\nwindows/build/\nwindows/node_modules/\n");
  const git = (...args: string[]) => run(["git", "-c", "core.hooksPath=.fixture-hooks", "-c", "commit.gpgSign=false", "-c", "user.name=Source fixture", "-c", "user.email=fixture@example.invalid", ...args], root, env);
  git("init", "--quiet"); git("add", "."); git("commit", "--quiet", "-m", "test: source fixture");
  const commit = git("rev-parse", "HEAD");
  const prepare = (defaultRoot = false) => Bun.spawnSync([
    "powershell", "-NoProfile", "-File", defaultRoot ? join(root, "scripts/prepare-source.ps1") : script,
    ...(defaultRoot ? [] : ["-RepositoryRoot", root]), "-OutputDirectory", output,
  ], { cwd: folder, env });
  return { root, app, cache, output, env, commit, git, prepare };
}

test("default source path resolves from the script and archives exact tracked source with vendored libraries", () => {
  const f = fixture();
  writeFileSync(join(f.root, ".env"), "PRIVATE_FIXTURE_DO_NOT_ARCHIVE=secret\n");
  for (const path of [join(f.app, "build"), join(f.app, "node_modules")]) mkdirSync(path);
  writeFileSync(join(f.app, "build", "private.exe"), "not source");
  const result = f.prepare(true);
  if (result.exitCode !== 0) throw new Error(result.stderr.toString() + result.stdout.toString());
  const manifest = JSON.parse(readFileSync(join(f.output, "source-manifest.json"), "utf8"));
  expect(manifest.schemaVersion).toBe(1);
  expect(manifest.sourceCommit).toBe(f.commit);
  expect(manifest.version).toBe("0.0.1-alpha.1");
  const archive = readFileSync(join(f.output, manifest.archive));
  expect(new Bun.CryptoHasher("sha256").update(archive).digest("hex")).toBe(manifest.archiveSha256);
  expect(archive.length).toBe(manifest.archiveBytes);
  const extracted = join(f.output, "inspection");
  run(["powershell", "-NoProfile", "-Command", "Add-Type -AssemblyName System.IO.Compression.FileSystem; [IO.Compression.ZipFile]::ExtractToDirectory($env:SOURCE_FIXTURE_ARCHIVE, $env:SOURCE_FIXTURE_EXTRACT)"], f.root,
    { ...f.env, SOURCE_FIXTURE_ARCHIVE: join(f.output, manifest.archive), SOURCE_FIXTURE_EXTRACT: extracted });
  const library = join(extracted, "windows/vendor/github.com/ethereum/go-ethereum");
  expect(readFileSync(join(extracted, "windows/main.go"), "utf8")).toBe(f.git("show", "HEAD:windows/main.go") + "\n");
  expect(readFileSync(join(library, "common/value_test.go"), "utf8")).toContain("Full module source");
  expect(existsSync(join(library, "COPYING.LESSER"))).toBe(true);
  expect(existsSync(join(extracted, "windows/vendor/modules.txt"))).toBe(true);
  expect(JSON.parse(readFileSync(join(extracted, "SOURCE-PROVENANCE.json"), "utf8")).sourceCommit).toBe(f.commit);
  for (const name of [".git", ".env", "windows/build", "windows/node_modules"]) expect(existsSync(join(extracted, name))).toBe(false);
});

test.skipIf(!shortTestRoot.includes("~"))("short Windows paths preserve exact source archive entry names", () => {
  const f = fixture(true);
  const result = f.prepare();
  if (result.exitCode !== 0) throw new Error(result.stderr.toString() + result.stdout.toString());
  const manifest = JSON.parse(readFileSync(join(f.output, "source-manifest.json"), "utf8"));
  expect(manifest.sourceCommit).toBe(f.commit);
  const entries = JSON.parse(run(["powershell", "-NoProfile", "-Command", `
Add-Type -AssemblyName System.IO.Compression, System.IO.Compression.FileSystem
$archive = [IO.Compression.ZipFile]::OpenRead($env:SOURCE_FIXTURE_ARCHIVE)
try { @($archive.Entries | ForEach-Object { $_.FullName }) | ConvertTo-Json } finally { $archive.Dispose() }
`], f.root, { ...f.env, SOURCE_FIXTURE_ARCHIVE: join(f.output, manifest.archive) })) as string[];
  expect(entries).toContain("SOURCE-PROVENANCE.json");
  expect(entries).toContain("windows/main.go");
  expect(entries).toContain("windows/vendor/github.com/ethereum/go-ethereum/COPYING.LESSER");
  expect(entries).toContain("windows/vendor/github.com/ethereum/go-ethereum/common/value_test.go");
  expect(entries.every(entry => !entry.startsWith("/") && !entry.includes("\\") && !entry.startsWith("ip/"))).toBe(true);
});

test("dirty tracked source is rejected before any source artifact is emitted", () => {
  const f = fixture();
  writeFileSync(join(f.app, "main.go"), "package main\nfunc main() {}\n");
  const result = f.prepare();
  expect(result.exitCode).not.toBe(0);
  expect(result.stderr.toString()).toContain("clean checkout");
  expect(existsSync(join(f.output, "source-manifest.json"))).toBe(false);
});

test("tracked credentials are rejected instead of being hidden in a source archive", () => {
  const f = fixture();
  writeFileSync(join(f.root, ".env"), "PRIVATE_FIXTURE_DO_NOT_ARCHIVE=secret\n");
  f.git("add", "-f", ".env"); f.git("commit", "--quiet", "-m", "test: unsafe tracked input");
  const result = f.prepare();
  expect(result.exitCode).not.toBe(0);
  expect(result.stderr.toString()).toContain("Unsafe tracked source path");
  expect(existsSync(join(f.output, "source-manifest.json"))).toBe(false);
});

test("modified dependency cache is rejected rather than attested as pinned source", () => {
  const f = fixture();
  const cached = join(f.cache, "github.com/ethereum/go-ethereum@v0.0.1/common/value.go");
  chmodSync(cached, 0o666);
  writeFileSync(cached, 'package common\nfunc Value() string { return "tampered" }\n');
  const result = f.prepare();
  expect(result.exitCode).not.toBe(0);
  expect(result.stderr.toString()).toContain("verification");
  expect(existsSync(join(f.output, "source-manifest.json"))).toBe(false);
});

test("module replacement is rejected instead of archiving different library source", () => {
  const f = fixture();
  f.git("config", "core.autocrlf", "false");
  writeFileSync(join(f.app, "go.mod"), readFileSync(join(f.app, "go.mod"), "utf8") + "\nreplace github.com/ethereum/go-ethereum => ../unreviewed-library\n");
  f.git("add", "windows/go.mod");
  f.git("commit", "--quiet", "-m", "test: replaced source");
  const result = f.prepare();
  expect(result.exitCode).not.toBe(0);
  expect(result.stderr.toString()).toContain("does not permit module replacements");
  expect(existsSync(join(f.output, "source-manifest.json"))).toBe(false);
});
