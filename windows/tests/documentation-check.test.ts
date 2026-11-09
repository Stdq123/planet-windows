import { afterEach, expect, test } from "bun:test";
import { execFileSync } from "node:child_process";
import { mkdirSync, mkdtempSync, realpathSync, rmSync, unlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join, resolve } from "node:path";
import { checkDocumentation } from "../../scripts/check-docs";

const fixtures: string[] = [];
const backtick = String.fromCharCode(96);
const gitEnvironment = Object.fromEntries(
  Object.entries(process.env).filter(([key]) => !key.toUpperCase().startsWith("GIT_")),
);

function fixture(files: Record<string, string | Uint8Array>) {
  const root = mkdtempSync(join(tmpdir(), "planet-docs-test-"));
  fixtures.push(root);
  for (const [name, content] of Object.entries(files)) {
    const path = join(root, name);
    mkdirSync(dirname(path), { recursive: true });
    writeFileSync(path, content);
  }
  for (const args of [["init", "--quiet"], ["-c", "core.autocrlf=false", "add", "--all"]]) {
    execFileSync("git", args, { cwd: root, env: gitEnvironment, stdio: "pipe" });
  }
  return root;
}

afterEach(() => {
  for (const root of fixtures.splice(0)) {
    const actual = realpathSync(root);
    if (dirname(actual) !== realpathSync(tmpdir()) || !basename(actual).startsWith("planet-docs-test-")
      || resolve(root) !== actual) {
      throw new Error("Refusing to clean an unexpected documentation fixture path: " + root);
    }
    rmSync(root, { recursive: true, force: true });
  }
});

test("checks tracked documentation links to headings, images, source files and directories", () => {
  const root = fixture({
    "README.md": "# Home\n\n[guide](docs/guide.md#同步文章到-xtwitter)\n"
      + "[duplicate](docs/guide.md#same-1)\n![image](docs/media/picture.png)\n"
      + "[code](windows/main.go)\n[directory](windows)\n[license](LICENSE)\n"
      + "[encoded](docs/a%20b.md#mixed-case)\n",
    "docs/guide.md": "# 同步文章到 X（Twitter）\n\n## Same\n\n## Same\n",
    "docs/a b.md": "# Mixed Case\n",
    "docs/media/picture.png": new Uint8Array([137, 80, 78, 71]),
    "windows/main.go": "package main\n",
    "LICENSE": "License\n",
    "NOTICE.md": "Excluded\r\n",
    "AGENTS.md": "Excluded\r\n",
    "windows/internal/templateinstall/builtins/plain/README.md": "Excluded\r\n",
  });
  expect(checkDocumentation(root)).toEqual({ files: 3, links: 7 });
});

test("reads working-tree changes and reports the source line of a missing file", () => {
  const root = fixture({ "README.md": "# Home\n" });
  writeFileSync(join(root, "README.md"), "# Home\n\n[missing](docs/missing.md)\n");
  expect(() => checkDocumentation(root)).toThrow("README.md:3");
  expect(() => checkDocumentation(root)).toThrow("docs/missing.md");
});

test("rejects nonexistent local heading anchors", () => {
  const root = fixture({
    "README.md": "# Home\n\n[guide](docs/guide.md#缺失标题)\n",
    "docs/guide.md": "# 实际标题\n",
  });
  expect(() => checkDocumentation(root)).toThrow("missing heading");
});

test.each(["docs/GUIDE.md#guide", "DOCS/guide.md#guide", "DOCS"])("rejects incorrectly cased tracked paths: %s", target => {
  const root = fixture({
    "README.md": "# Home\n\n[guide](" + target + ")\n",
    "docs/guide.md": "# Guide\n",
  });
  expect(() => checkDocumentation(root)).toThrow("tracked path");
});

test.each(["drafts/guide.md#guide", "drafts"])("does not let untracked local targets hide broken links: %s", target => {
  const root = fixture({ "README.md": "# Home\n\n[guide](" + target + ")\n" });
  mkdirSync(join(root, "drafts"));
  writeFileSync(join(root, "drafts/guide.md"), "# Guide\n");
  expect(() => checkDocumentation(root)).toThrow("tracked path");
});

test("resolves Setext headings and duplicates without confusing seven equals signs with a conflict", () => {
  const root = fixture({
    "README.md": "Guide\n=======\n\n## Guide\n\nDetails\n-------\n\n"
      + "[guide](#guide)\n[duplicate](#guide-1)\n[details](#details)\n",
  });
  expect(checkDocumentation(root)).toEqual({ files: 1, links: 3 });
});

test.each(["docs/guide.md", "docs/image.png"])("finds incoming links to a deleted tracked target: %s", target => {
  const root = fixture({ "README.md": "# Home\n\n[target](" + target + ")\n", [target]: "# Target\n" });
  unlinkSync(join(root, target));
  expect(() => checkDocumentation(root)).toThrow("missing local target");
});

test("allows deleted documents with no remaining references", () => {
  const root = fixture({ "README.md": "# Home\n", "docs/removed.md": "# Removed\n" });
  unlinkSync(join(root, "docs/removed.md"));
  expect(checkDocumentation(root)).toEqual({ files: 1, links: 0 });
});

test("ignores links and headings in fenced or inline code and indented examples", () => {
  const fence = backtick.repeat(3);
  const root = fixture({
    "README.md": "# Real\n\n" + fence + "md\n[missing](absent.md)\n# Fake\n" + fence + "\n\n"
      + "~~~text\n[missing](also-absent.md)\n~~~\n\n"
      + backtick + "[missing](inline.md)" + backtick + "\n\n"
      + "    [missing](indented.md)\n\n[self](#real)\n",
  });
  expect(checkDocumentation(root)).toEqual({ files: 1, links: 1 });
  writeFileSync(join(root, "README.md"), "# Real\n\n" + fence + "md\n# Fake\n" + fence + "\n[fake](#fake)\n");
  expect(() => checkDocumentation(root)).toThrow("missing heading");
});

test("does not inspect external URLs, custom schemes or historical absolute paths", () => {
  const root = fixture({
    "README.md": "# Home\n\n[web](https://example.invalid/a#missing)\n"
      + "[mail](mailto:example@example.invalid)\n[resource](ipfs://example)\n"
      + backtick + "D:\\old-workspace\\README.md" + backtick + "\n",
  });
  expect(checkDocumentation(root)).toEqual({ files: 1, links: 3 });
});

test("checks the nested guide and pull request template whitelist", () => {
  const root = fixture({
    "windows/README.md": "# Windows\n",
    "windows/docs/nested/guide.md": "# Guide\n",
    ".github/PULL_REQUEST_TEMPLATE.md": "# Pull request\n",
    "docs/design/nested.md": "# Design\n",
  });
  expect(checkDocumentation(root)).toEqual({ files: 4, links: 0 });
});

test.each([
  ["invalid UTF-8", new Uint8Array([0xc3, 0x28, 0x0a]), "UTF-8"],
  ["BOM", new Uint8Array([0xef, 0xbb, 0xbf, 0x23, 0x0a]), "BOM"],
  ["CRLF", "# Home\r\n", "LF"],
  ["missing final newline", "# Home", "final newline"],
  ["conflict markers", "# Home\n<<<<<<< HEAD\nleft\n=======\nright\n>>>>>>> topic\n", "conflict marker"],
  ["unclosed fence", "# Home\n" + backtick.repeat(3) + "text\nexample\n", "unclosed code fence"],
  ["short closing fence", backtick.repeat(4) + "text\nexample\n" + backtick.repeat(3) + "\n", "unclosed code fence"],
] as const)("rejects %s", (_name, contents, message) => {
  const root = fixture({ "README.md": contents });
  expect(() => checkDocumentation(root)).toThrow(message);
});
