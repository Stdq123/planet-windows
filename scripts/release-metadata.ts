import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { parseArgs } from "node:util";
import { parseReleaseVersion } from "./release-version";

export function releaseMetadata(
  versionFile: string,
  changelog: string,
  options: { release?: boolean; tag?: string; today?: string } = {},
) {
  const { version, prerelease } = parseReleaseVersion(JSON.parse(versionFile)?.version);
  const tag = `v${version}`;
  if (options.release && options.tag !== tag) throw new Error(`Release tag must be exactly ${tag}.`);

  const text = changelog.replace(/\r\n/g, "\n");
  const headings = [...text.matchAll(/^## .*$/gm)];
  const matching = (label: string) => headings.filter(heading => heading[0] === `## [${label}]`
    || heading[0].startsWith(`## [${label}] - `));
  if (matching("Unreleased").length !== 1) throw new Error("Changelog must have exactly one Unreleased section.");
  const selected = matching(options.release ? version : "Unreleased");
  if (selected.length !== 1) throw new Error(`Changelog must have exactly one entry for ${version}.`);
  const heading = selected[0]!;
  const index = headings.indexOf(heading);
  const body = text.slice(heading.index! + heading[0].length, headings[index + 1]?.index ?? text.length).trim();
  if (!/^[-*] \S/m.test(body)) throw new Error("Changelog section has no change entries.");
  let date: string | null = null;
  if (options.release) {
    date = heading[0].match(/^## \[[^\]]+\] - (\d{4}-\d{2}-\d{2})$/)?.[1] ?? null;
    const parsed = date ? new Date(`${date}T00:00:00Z`) : null;
    const today = options.today ?? new Date().toLocaleDateString("en-CA", { timeZone: "Asia/Shanghai" });
    if (!date || !parsed || Number.isNaN(parsed.getTime()) || parsed.toISOString().slice(0, 10) !== date || date > today) {
      throw new Error("Release date must be a real YYYY-MM-DD date, no later than today.");
    }
  }
  return {
    version, tag, date, prerelease, mode: options.release ? "release" : "draft",
    notes: options.release ? `# ${tag} — ${date}\n\n${body}\n`
      : `# DRAFT — ${tag}\n\nUnreleased working-tree snapshot; not an approved release.\n\n${body}\n`,
  };
}

if (import.meta.main) {
  try {
    const { values } = parseArgs({ options: { release: { type: "boolean" }, tag: { type: "string" } } });
    const root = resolve(import.meta.dir, "..");
    const result = releaseMetadata(
      readFileSync(resolve(root, "windows/release.json"), "utf8"),
      readFileSync(resolve(root, "CHANGELOG.md"), "utf8"),
      { release: values.release, tag: values.tag },
    );
    console.log(JSON.stringify(result));
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
