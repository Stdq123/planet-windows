import { appendFileSync, readFileSync } from "node:fs";
import { join } from "node:path";

type Scope = { mode: "docs" | "full" | "none"; reason: string };
const full = (reason: string): Scope => ({ mode: "full", reason });
const objectID = /^(?:[0-9a-f]{40}|[0-9a-f]{64})$/;
const zeroID = /^0+$/;

function git(root: string, ...args: string[]): string {
  const result = Bun.spawnSync(["git", "-C", root, ...args]);
  if (result.exitCode !== 0) throw Error(result.stderr.toString().trim() || "Git comparison failed");
  return result.stdout.toString();
}

function commit(root: string, ref: string): string {
  return git(root, "rev-parse", "--verify", "--end-of-options", `${ref}^{commit}`).trim();
}

function documentationPath(path: string): boolean {
  // Release metadata, license materials, agent instructions and embedded assets
  // are intentionally outside this list, even when their extension is Markdown.
  if (["README.md", "windows/README.md", "CONTRIBUTING.md", "CODE_OF_CONDUCT.md", "SECURITY.md", ".github/PULL_REQUEST_TEMPLATE.md"].includes(path)) return true;
  if (/(?:^|\/)AGENTS\.md$/.test(path)) return false;
  return /^(?:docs|windows\/docs)\/.+\.md$/.test(path)
    || /^docs\/(?:screenshots|uiux)\/.+\.png$/.test(path);
}

export function changeScope(root: string, baseRef: string, headRef: string): Scope {
  try {
    const base = commit(root, baseRef);
    const head = commit(root, headRef);
    // Disabling rename detection preserves both sides of a code-to-doc rename.
    const raw = git(root, "diff", "--no-ext-diff", "--no-textconv", "--raw", "--no-abbrev", "--no-renames", "-z", base, head, "--");
    const fields = raw.split("\0");
    if (fields.pop() !== "" || fields.length === 0 || fields.length % 2) return full("No reliable changed-file list");
    for (let i = 0; i < fields.length; i += 2) {
      const header = fields[i]!.match(/^:(\d{6}) (\d{6}) [0-9a-f]+ [0-9a-f]+ ([A-Z])$/);
      if (!header || ![header[1], header[2]].every(mode => mode === "000000" || mode === "100644")) return full("File type or mode requires full checks");
      if (!documentationPath(fields[i + 1]!)) return full(`Non-documentation input: ${fields[i + 1]}`);
    }
    return { mode: "docs", reason: "Only allowlisted documentation files changed" };
  } catch {
    return full("Comparison base or history is unavailable");
  }
}

function ciScope(root: string): Scope {
  try {
    const event = process.env.GITHUB_EVENT_NAME;
    if (event !== "pull_request" && event !== "push") return full("Manual or unrecognized workflow event");
    const payload = JSON.parse(readFileSync(process.env.GITHUB_EVENT_PATH!, "utf8"));
    const checkedOut = commit(root, "HEAD");
    if (event === "push") {
      if (!objectID.test(payload.before) || !objectID.test(payload.after) || zeroID.test(payload.before) || checkedOut !== payload.after) return full("Push comparison is unavailable or does not match checkout");
      return changeScope(root, payload.before, checkedOut);
    }
    const base = payload.pull_request?.base?.sha;
    const head = payload.pull_request?.head?.sha;
    if (!objectID.test(base) || !objectID.test(head)) return full("PR commit identities are unavailable");
    const parents = git(root, "show", "-s", "--format=%P", checkedOut).trim().split(" ");
    // Actions checks out the proposed merge, so classify exactly that tree.
    if (parents.length !== 2 || parents[0] !== base || parents[1] !== head) return full("Checkout is not the expected PR merge");
    return changeScope(root, base, checkedOut);
  } catch {
    return full("Workflow event or comparison could not be read");
  }
}

function pushScope(root: string, input: string, remote: string): Scope {
  const updates = input.trim() ? input.trim().split(/\r?\n/).map(line => {
    const fields = line.split(/\s+/);
    if (fields.length !== 4 || !objectID.test(fields[1]!) || !objectID.test(fields[3]!)) throw Error("Malformed pre-push ref input");
    return { local: fields[1]!, remote: fields[3]!, ref: fields[2]! };
  }).filter(update => !zeroID.test(update.local)) : [];
  if (!updates.length) return { mode: "none", reason: "No commits are being pushed" };

  // Checking another checkout can hide a broken committed document or code.
  if (git(root, "status", "--porcelain", "--untracked-files=normal").trim()) throw Error("Pre-push checks need a clean working tree. Commit or save pending work before pushing.");
  const checkedOut = commit(root, "HEAD");
  let scope: Scope = { mode: "docs", reason: "Only allowlisted documentation files changed" };
  for (const update of updates) {
    if (commit(root, update.local) !== checkedOut) throw Error("Pre-push checks need the pushed tip to match the checked-out commit. Check out that branch and push it separately.");
    let current: Scope;
    if (!update.ref.startsWith("refs/heads/")) {
      current = full("Tag or non-branch ref update");
    } else if (!zeroID.test(update.remote)) {
      // Compare endpoints for existing branches, including non-fast-forward updates.
      current = changeScope(root, update.remote, update.local);
    } else {
      try {
        if (!/^[A-Za-z0-9_.-]+$/.test(remote)) throw Error("Unknown remote name");
        const base = git(root, "merge-base", `refs/remotes/${remote}/main`, update.local).trim();
        current = changeScope(root, base, update.local);
      } catch {
        current = full("New-branch baseline is unavailable");
      }
    }
    if (current.mode === "full") scope = current;
  }
  return scope;
}

if (import.meta.main) {
  try {
    const root = git(process.cwd(), "rev-parse", "--show-toplevel").trim();
    const command = process.argv[2];
    if (command === "scope") {
      console.log(JSON.stringify(changeScope(root, process.argv[3] ?? "", process.argv[4] ?? "")));
    } else if (command === "ci") {
      const scope = ciScope(root);
      console.log(`Check scope: ${scope.mode} (${scope.reason})`);
      if (!process.env.GITHUB_OUTPUT) throw Error("GITHUB_OUTPUT is required for CI classification");
      appendFileSync(process.env.GITHUB_OUTPUT, `mode=${scope.mode}\n`);
    } else if (command === "pre-push") {
      const scope = pushScope(root, readFileSync(0, "utf8"), process.argv[3] ?? "");
      console.log(`Check scope: ${scope.mode} (${scope.reason})`);
      if (scope.mode === "docs") {
        const { checkDocumentation } = await import("./check-docs.ts");
        const result = checkDocumentation(root);
        console.log(`Documentation checks passed: ${result.files} documents, ${result.links} links.`);
      } else if (scope.mode === "full") {
        const result = Bun.spawnSync(["powershell.exe", "-NoProfile", "-File", join(root, "scripts/check.ps1")], { cwd: root, stdout: "inherit", stderr: "inherit" });
        if (result.exitCode !== 0) throw Error(`Full checks failed (exit ${result.exitCode})`);
      }
    } else {
      throw Error("Usage: check-changes.ts ci | pre-push <remote> | scope <base> <head>");
    }
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
