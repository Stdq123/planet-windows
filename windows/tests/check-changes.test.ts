import { afterAll, expect, test } from "bun:test";
import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, renameSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";

const suiteRoot = mkdtempSync(join(tmpdir(), "planet-check-changes-"));
const script = resolve(import.meta.dir, "../../scripts/check-changes.ts");
const env = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith("GIT_") && !key.startsWith("GITHUB_")));
afterAll(() => {
  if (dirname(suiteRoot) !== resolve(tmpdir()) || !suiteRoot.split(/[\\/]/).at(-1)?.startsWith("planet-check-changes-")) throw Error("Unexpected fixture path");
  rmSync(suiteRoot, { recursive: true });
});

function fixture() {
  const root = mkdtempSync(join(suiteRoot, "case "));
  const git = (...args: string[]) => {
    const result = Bun.spawnSync(["git", "-C", root, "-c", "core.hooksPath=.test-hooks", "-c", "commit.gpgSign=false", "-c", "user.name=Checks test", "-c", "user.email=checks@example.invalid", ...args], { env });
    if (result.exitCode) throw Error(result.stderr.toString());
    return result.stdout.toString().trim();
  };
  const write = (file: string, text = "# Documentation\n") => {
    mkdirSync(dirname(join(root, file)), { recursive: true });
    writeFileSync(join(root, file), text);
  };
  const commit = () => { git("add", "."); git("commit", "-qm", "fixture change"); return git("rev-parse", "HEAD"); };
  git("init", "-q", "-b", "main");
  write("README.md");
  write("windows/main.go", "package main\n");
  // Replace only the expensive build boundary; exercise real Git and dispatch.
  write("scripts/check.ps1", "Write-Output 'FULL_CHECK_EXECUTED'\n");
  const base = commit();
  git("update-ref", "refs/remotes/origin/main", base);
  const run = (args: string[], input = "", extraEnv = {}) => Bun.spawnSync(["bun", script, ...args], { cwd: root, env: { ...env, ...extraEnv }, stdin: Buffer.from(input) });
  const scope = (from = base, to = "HEAD") => {
    const result = run(["scope", from, to]);
    expect(result.exitCode).toBe(0);
    return JSON.parse(result.stdout.toString());
  };
  return { root, git, write, commit, base, run, scope };
}

test("README and documentation images take the light path, including Unicode filenames", () => {
  const f = fixture();
  f.write("README.md", "# Updated\n");
  f.write("docs/中文 guide.md");
  f.write("docs/screenshots/reader.png", "test image");
  f.commit();
  expect(f.scope().mode).toBe("docs");
});

test("code, release inputs, workflow changes and mixed changes retain full checks", () => {
  const f = fixture();
  for (const file of ["windows/main.go", "CHANGELOG.md", "NOTICE.md", "AGENTS.md", "LICENSE", "windows/internal/templateinstall/builtins/plain/README.md", "windows/docs/kubo-dependency-licenses.txt", ".github/workflows/windows-ci.yml", "scripts/check-changes.ts", "windows/resources/icon.png"]) {
    const before = f.git("rev-parse", "HEAD");
    f.write(file, "changed\n");
    f.write("README.md", `# Change to ${file}\n`);
    f.commit();
    expect(f.scope(before).mode).toBe("full");
  }
}, 20_000);

test("renaming code into a documentation path and changing file mode cannot hide code changes", () => {
  const f = fixture();
  mkdirSync(join(f.root, "docs"));
  renameSync(join(f.root, "windows/main.go"), join(f.root, "docs/example.md"));
  f.commit();
  expect(f.scope().mode).toBe("full");
  const before = f.git("rev-parse", "HEAD");
  f.git("update-index", "--chmod=+x", "README.md");
  f.git("commit", "-qm", "fixture executable mode");
  expect(f.scope(before).mode).toBe("full");
});

test("unknown comparison bases and empty differences use full checks", () => {
  const f = fixture();
  expect(f.scope("missing-base").mode).toBe("full");
  expect(f.scope().mode).toBe("full");
});

test("pre-push checks new documentation branches without invoking the full build", () => {
  const f = fixture();
  f.write("README.md", "# New text\n");
  const head = f.commit();
  const result = f.run(["pre-push", "origin"], `refs/heads/docs ${head} refs/heads/docs ${"0".repeat(40)}\n`);
  expect(result.exitCode).toBe(0);
  expect(result.stdout.toString()).toContain("Documentation checks passed");
  expect(result.stdout.toString()).not.toContain("FULL_CHECK_EXECUTED");
});

test("existing branch updates compare both endpoints, including reverted code", () => {
  const f = fixture();
  f.write("windows/main.go", "package different\n");
  const remote = f.commit();
  f.git("checkout", "-q", f.base);
  f.write("README.md", "# Docs on older code\n");
  const head = f.commit();
  const result = f.run(["pre-push", "origin"], `HEAD ${head} refs/heads/docs ${remote}\n`);
  expect(result.exitCode).toBe(0);
  expect(result.stdout.toString()).toContain("FULL_CHECK_EXECUTED");
});

test("missing new-branch baseline and tag updates run full checks", () => {
  const f = fixture();
  f.write("README.md", "# New docs\n");
  const head = f.commit();
  for (const [remote, ref] of [["unknown", "refs/heads/docs"], ["origin", "refs/tags/v0.0.1"]]) {
    const result = f.run(["pre-push", remote!], `HEAD ${head} ${ref} ${"0".repeat(40)}\n`);
    expect(result.exitCode).toBe(0);
    expect(result.stdout.toString()).toContain("FULL_CHECK_EXECUTED");
  }
});

test("pre-push rejects dirty or different checkout rather than validating the wrong tree", () => {
  const f = fixture();
  f.write("README.md", "# New docs\n");
  const head = f.commit();
  const line = `HEAD ${head} refs/heads/docs ${f.base}\n`;
  f.write("README.md", "# Uncommitted\n");
  let result = f.run(["pre-push", "origin"], line);
  expect(result.exitCode).not.toBe(0);
  expect(result.stderr.toString()).toContain("clean working tree");
  f.git("checkout", "--", "README.md");
  result = f.run(["pre-push", "origin"], `HEAD ${f.base} refs/heads/older ${"0".repeat(40)}\n`);
  expect(result.exitCode).not.toBe(0);
  expect(result.stderr.toString()).toContain("checked-out commit");
});

test("deleted refs do not build, while any non-document update in a multi-ref push does", () => {
  const f = fixture();
  const deletion = `(delete) ${"0".repeat(40)} refs/heads/old ${f.base}\n`;
  for (const input of ["", deletion]) {
    const noUpdate = f.run(["pre-push", "origin"], input);
    expect(noUpdate.exitCode).toBe(0);
    expect(noUpdate.stdout.toString()).toContain("Check scope: none");
    expect(noUpdate.stdout.toString()).not.toContain("FULL_CHECK_EXECUTED");
  }
  f.write("README.md", "# New docs\n");
  const head = f.commit();
  const result = f.run(["pre-push", "origin"], `${deletion}HEAD ${head} refs/heads/docs ${f.base}\nHEAD ${head} refs/tags/v0.0.1 ${"0".repeat(40)}\n`);
  expect(result.exitCode).toBe(0);
  expect(result.stdout.toString()).toContain("FULL_CHECK_EXECUTED");
});

test("CI classifies the actual PR merge tree and preserves full manual checks", () => {
  const f = fixture();
  f.git("checkout", "-qb", "docs");
  f.write("README.md", "# PR documentation\n");
  const head = f.commit();
  f.git("checkout", "-q", "main");
  f.git("merge", "--no-ff", "-m", "test merge", "docs");
  const eventFile = join(suiteRoot, "event.json");
  const outputFile = join(suiteRoot, "outputs.txt");
  writeFileSync(eventFile, JSON.stringify({ pull_request: { base: { sha: f.base }, head: { sha: head } } }));
  writeFileSync(outputFile, "");
  let result = f.run(["ci"], "", { GITHUB_EVENT_NAME: "pull_request", GITHUB_EVENT_PATH: eventFile, GITHUB_OUTPUT: outputFile });
  expect(result.exitCode).toBe(0);
  expect(readFileSync(outputFile, "utf8")).toContain("mode=docs\n");
  writeFileSync(outputFile, "");
  result = f.run(["ci"], "", { GITHUB_EVENT_NAME: "workflow_dispatch", GITHUB_OUTPUT: outputFile });
  expect(result.exitCode).toBe(0);
  expect(readFileSync(outputFile, "utf8")).toContain("mode=full\n");
  f.git("checkout", "-q", "docs");
  writeFileSync(outputFile, "");
  result = f.run(["ci"], "", { GITHUB_EVENT_NAME: "pull_request", GITHUB_EVENT_PATH: eventFile, GITHUB_OUTPUT: outputFile });
  expect(result.exitCode).toBe(0);
  expect(readFileSync(outputFile, "utf8")).toContain("mode=full\n");
});

test("CI uses push before/after commits and falls back for missing or mismatched history", () => {
  const f = fixture();
  f.write("README.md", "# Documentation push\n");
  const head = f.commit();
  const eventFile = join(suiteRoot, "push-event.json");
  const outputFile = join(suiteRoot, "push-output.txt");
  for (const [before, after, expected] of [[f.base, head, "docs"], ["0".repeat(40), head, "full"], ["f".repeat(40), head, "full"], [f.base, f.base, "full"]]) {
    writeFileSync(eventFile, JSON.stringify({ before, after }));
    writeFileSync(outputFile, "");
    const result = f.run(["ci"], "", { GITHUB_EVENT_NAME: "push", GITHUB_EVENT_PATH: eventFile, GITHUB_OUTPUT: outputFile });
    expect(result.exitCode).toBe(0);
    expect(readFileSync(outputFile, "utf8")).toBe(`mode=${expected}\n`);
  }
});

test("pre-push propagates broken documentation and full-check failures", () => {
  const f = fixture();
  f.write("README.md", "# Broken\n\n[missing](missing.md)\n");
  let head = f.commit();
  let result = f.run(["pre-push", "origin"], `HEAD ${head} refs/heads/docs ${f.base}\n`);
  expect(result.exitCode).not.toBe(0);
  expect(result.stderr.toString()).toContain("missing local target");
  f.write("scripts/check.ps1", "exit 7\n");
  head = f.commit();
  result = f.run(["pre-push", "origin"], `HEAD ${head} refs/heads/docs ${f.base}\n`);
  expect(result.exitCode).not.toBe(0);
  expect(result.stderr.toString()).toContain("Full checks failed");
});

test("Git invokes the actual pre-push hook with ref input for a lightweight documentation push", () => {
  const f = fixture();
  const sourceRoot = resolve(import.meta.dir, "../..");
  mkdirSync(join(f.root, ".githooks"));
  for (const name of [".githooks/pre-push", "scripts/check-changes.ts", "scripts/check-docs.ts"]) {
    copyFileSync(join(sourceRoot, name), join(f.root, name));
  }
  const baseline = f.commit();
  f.git("update-ref", "refs/remotes/origin/main", baseline);
  const remote = join(suiteRoot, "remote.git");
  const initialized = Bun.spawnSync(["git", "init", "--bare", "--quiet", remote], { env });
  expect(initialized.exitCode).toBe(0);
  f.git("remote", "add", "origin", remote);
  f.write("README.md", "# Actual hook\n");
  const head = f.commit();
  const result = Bun.spawnSync(["git", "-C", f.root, "-c", "core.hooksPath=.githooks", "push", "origin", "HEAD:refs/heads/docs"], { env });
  expect(result.exitCode).toBe(0);
  expect(result.stdout.toString()).toContain("Documentation checks passed");
  expect(result.stdout.toString()).not.toContain("FULL_CHECK_EXECUTED");
  expect(f.git("ls-remote", "origin", "refs/heads/docs")).toContain(head);
}, 15_000);
