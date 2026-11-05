import { expect, test } from "bun:test";
import { releaseMetadata } from "../../scripts/release-metadata";

const versionFile = JSON.stringify({ version: "0.3.0" });
const changelog = `# Changelog

## [Unreleased]
### Added
- Draft improvement.

## [0.3.0] - 2026-11-05
### Fixed
- Preserves saved articles after refresh failures.

## [0.2.0] - 2026-10-01
### Added
- Older feature.
`;
const options = { release: true, tag: "v0.3.0", today: "2026-11-05" };

test("release notes select only the version matching the tag", () => {
  const result = releaseMetadata(versionFile, changelog, options);
  expect(result.version).toBe("0.3.0");
  expect(result.notes).toContain("Preserves saved articles");
  expect(result.notes).not.toContain("Draft improvement");
  expect(result.notes).not.toContain("Older feature");
});

test("draft output uses Unreleased and clearly identifies its status", () => {
  const result = releaseMetadata(versionFile, changelog);
  expect(result.mode).toBe("draft");
  expect(result.notes).toContain("DRAFT");
  expect(result.notes).toContain("Draft improvement");
  expect(result.notes).not.toContain("Preserves saved articles");
});

test("app-compatible numeric versions are required", () => {
  for (const version of ["01.2.3", "1.2", "1.2.3-beta", "4294967296.0.0", 3, null]) {
    expect(() => releaseMetadata(JSON.stringify({ version }), changelog)).toThrow("version");
  }
});

test("release mode requires an exact matching version tag", () => {
  for (const tag of [undefined, "0.3.0", "v0.4.0", "v0.3.0-other"]) {
    expect(() => releaseMetadata(versionFile, changelog, { ...options, tag })).toThrow("tag");
  }
});

test("missing, duplicated or undated version entries block release", () => {
  for (const text of [
    changelog.replace("[0.3.0]", "[0.3.1]"),
    changelog + "\n## [0.3.0] - 2026-11-05\n- Duplicate.\n",
    changelog.replace("[0.3.0] - 2026-11-05", "[0.3.0]"),
  ]) expect(() => releaseMetadata(versionFile, text, options)).toThrow();
});

test("invalid and future release dates are rejected", () => {
  for (const date of ["2026-02-30", "2026-13-05", "2026-11-06"]) {
    expect(() => releaseMetadata(versionFile, changelog.replace("2026-11-05", date), options)).toThrow("date");
  }
});

test("empty release entries and duplicate Unreleased entries are rejected", () => {
  expect(() => releaseMetadata(versionFile, changelog.replace("- Preserves saved articles after refresh failures.", ""), options)).toThrow("entries");
  expect(() => releaseMetadata(versionFile, changelog + "\n## [Unreleased]\n- Duplicate.\n")).toThrow("Unreleased");
});
