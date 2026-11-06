import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, statSync } from "node:fs";
import { dirname, extname, isAbsolute, relative, resolve, sep } from "node:path";

type Link = { target: string; line: number };
type Markdown = { headings: Set<string>; links: Link[] };

const rootDocuments = new Set([
  "README.md", "CONTRIBUTING.md", "CODE_OF_CONDUCT.md", "CHANGELOG.md", "SECURITY.md",
  "windows/README.md", ".github/PULL_REQUEST_TEMPLATE.md",
]);
const backtick = String.fromCharCode(96);
const fencePattern = new RegExp("^ {0,3}(" + backtick + "{3,}|~{3,})(.*)$");

function isDocumentation(path: string): boolean {
  return rootDocuments.has(path) || /^(?:docs|windows\/docs)\/.*\.md$/.test(path);
}

function maskInlineCode(line: string): string {
  const runs = [...line.matchAll(new RegExp(backtick + "+", "g"))];
  // Work in UTF-16 offsets, matching RegExp.index and String.slice.
  let result = line;
  for (let index = 0; index < runs.length; index++) {
    const opening = runs[index]!;
    const closingIndex = runs.findIndex((run, candidate) => candidate > index && run[0] === opening[0]);
    if (closingIndex < 0) continue;
    const end = runs[closingIndex]!.index! + opening[0].length;
    result = result.slice(0, opening.index) + " ".repeat(end - opening.index!) + result.slice(end);
    index = closingIndex;
  }
  return result;
}

function headingSlug(heading: string): string {
  return heading
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/<[^>]*>/g, "")
    .replace(new RegExp(backtick, "g"), "")
    .toLowerCase()
    .replace(/[^\p{L}\p{M}\p{N}\p{Pc}\s-]/gu, "")
    .replace(/\s/g, "-");
}

// This handles the repository's inline links/images, including angle-bracket
// destinations, escaped punctuation and balanced parentheses; it is not a renderer.
function inlineLinks(line: string, lineNumber: number, report: (message: string) => void): Link[] {
  const links: Link[] = [];
  for (const opening of line.matchAll(/!?\[[^\]\n]*\]\(/g)) {
    if (opening.index! > 0 && line[opening.index! - 1] === "\\") continue;
    let position = opening.index! + opening[0].length;
    while (/\s/.test(line[position] ?? "") && position < line.length) position++;
    let target = "";
    if (line[position] === "<") {
      const end = line.indexOf(">", ++position);
      if (end < 0) { report("unclosed inline link destination"); continue; }
      target = line.slice(position, end);
      position = end + 1;
    } else {
      let depth = 0;
      while (position < line.length) {
        const character = line[position]!;
        if (character === "\\" && position + 1 < line.length) {
          target += line[position + 1];
          position += 2;
          continue;
        }
        if ((character === ")" && depth === 0) || /\s/.test(character)) break;
        if (character === "(") depth++;
        if (character === ")") depth--;
        target += character;
        position++;
      }
    }
    const ending = line.slice(position).match(/^\s*(?:(?:"[^"]*"|'[^']*'|\([^)]*\))\s*)?\)/);
    if (!ending) { report("unclosed inline link"); continue; }
    links.push({ target, line: lineNumber });
  }
  return links;
}

function parseMarkdown(text: string, file: string, issues: string[]): Markdown {
  const headings = new Set<string>();
  const links: Link[] = [];
  const addHeading = (title: string) => {
    const base = headingSlug(title);
    let slug = base;
    let suffix = 0;
    while (headings.has(slug)) slug = base + "-" + ++suffix;
    headings.add(slug);
  };
  let paragraph = "";
  let inConflict = false;
  let fence: { marker: string; length: number; line: number } | undefined;
  for (const [index, line] of text.split("\n").entries()) {
    const lineNumber = index + 1;
    const report = (message: string) => issues.push(file + ":" + lineNumber + ": " + message);
    if (/^<{7}(?: .*)?$/.test(line)) inConflict = true;
    if (/^(?:<{7}(?: .*)?|>{7}(?: .*)?|\|{7}(?: .*)?)$/.test(line)
      || (inConflict && /^={7}$/.test(line))) report("conflict marker");
    if (/^>{7}(?: .*)?$/.test(line)) inConflict = false;
    const marker = line.match(fencePattern);
    if (fence) {
      if (marker && marker[1]![0] === fence.marker && marker[1]!.length >= fence.length && !marker[2]!.trim()) {
        fence = undefined;
      }
      continue;
    }
    if (marker) {
      fence = { marker: marker[1]![0]!, length: marker[1]!.length, line: lineNumber };
      paragraph = "";
      continue;
    }
    if (/^(?: {4}|\t)/.test(line)) { paragraph = ""; continue; }
    const heading = line.match(/^ {0,3}#{1,6}\s+(.+?)(?:\s+#+\s*)?$/);
    if (heading) {
      addHeading(heading[1]!);
      paragraph = "";
    } else if (/^ {0,3}(?:=+|-+)\s*$/.test(line)) {
      if (paragraph) addHeading(paragraph);
      paragraph = "";
    } else if (!line.trim() || /^ {0,3}(?:>|[-+*]\s|\d+[.)]\s)/.test(line)) {
      paragraph = "";
    } else {
      paragraph += (paragraph ? " " : "") + line.trim();
    }
    const visible = maskInlineCode(line);
    for (const id of visible.matchAll(/<[^>]+\b(?:id|name)=["']([^"']+)["'][^>]*>/g)) headings.add(id[1]!);
    links.push(...inlineLinks(visible, lineNumber, report));
  }
  if (fence) issues.push(file + ":" + fence.line + ": unclosed code fence");
  return { headings, links };
}

export function checkDocumentation(root: string): { files: number; links: number } {
  root = resolve(root);
  // A hook may inherit GIT_INDEX_FILE/GIT_DIR; this check always targets root's worktree.
  const env = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.toUpperCase().startsWith("GIT_")));
  const tracked = execFileSync("git", ["-C", root, "ls-files", "-z"], {
    encoding: "utf8", env, maxBuffer: 16 * 1024 * 1024,
  }).split("\0").filter(Boolean);
  const trackedPaths = new Set(["", ...tracked]);
  for (const file of tracked) {
    for (let slash = file.lastIndexOf("/"); slash >= 0; slash = file.lastIndexOf("/", slash - 1)) {
      trackedPaths.add(file.slice(0, slash));
    }
  }
  const files = tracked.filter(file => isDocumentation(file) && existsSync(resolve(root, file)));
  const documents = new Map<string, Markdown>();
  const issues: string[] = [];
  for (const file of files) {
    const bytes = readFileSync(resolve(root, file));
    if (bytes.subarray(0, 3).equals(Buffer.from([0xef, 0xbb, 0xbf]))) issues.push(file + ": UTF-8 BOM is not allowed");
    let text: string;
    try { text = new TextDecoder("utf-8", { fatal: true }).decode(bytes); }
    catch { issues.push(file + ": invalid UTF-8"); continue; }
    if (text.includes("\r")) issues.push(file + ": use LF line endings");
    if (text && !text.endsWith("\n")) issues.push(file + ": missing final newline");
    documents.set(resolve(root, file), parseMarkdown(text, file, issues));
  }
  let linkCount = 0;
  for (const [source, document] of documents) {
    for (const link of document.links) {
      linkCount++;
      if (/^[a-z][a-z\d+.-]*:/i.test(link.target) || link.target.startsWith("//")) continue;
      const label = relative(root, source).split(sep).join("/") + ":" + link.line + ": ";
      let destination: string;
      let anchor: string;
      try {
        const hash = link.target.indexOf("#");
        destination = decodeURIComponent((hash < 0 ? link.target : link.target.slice(0, hash)).split("?")[0]!);
        anchor = hash < 0 ? "" : decodeURIComponent(link.target.slice(hash + 1));
      } catch { issues.push(label + "invalid URL encoding in " + link.target); continue; }
      const target = destination ? resolve(dirname(source), destination) : source;
      const location = relative(root, target);
      if (isAbsolute(location) || location === ".." || location.startsWith(".." + sep)) {
        issues.push(label + "local target is outside the repository: " + link.target);
        continue;
      }
      // GitHub paths are case-sensitive, even when the local filesystem is not.
      if (!trackedPaths.has(location.split(sep).join("/")) || !existsSync(target)) {
        issues.push(label + "missing local target (expected an exact tracked path): " + link.target);
        continue;
      }
      if (!anchor || extname(target).toLowerCase() !== ".md" || !statSync(target).isFile()) continue;
      // Links may target excluded license documents; inspect their headings without
      // applying the human-document formatting policy to packaging inputs.
      const headings = documents.get(target)?.headings
        ?? parseMarkdown(readFileSync(target, "utf8"), location, []).headings;
      if (!headings.has(anchor)) issues.push(label + "missing heading #" + anchor + " in " + (destination || location));
    }
  }
  if (issues.length) throw new Error("Documentation check failed:\n" + issues.join("\n"));
  return { files: files.length, links: linkCount };
}

if (import.meta.main) {
  try {
    const result = checkDocumentation(resolve(import.meta.dir, ".."));
    console.log("Documentation check passed: " + result.files + " files, " + result.links + " links.");
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
