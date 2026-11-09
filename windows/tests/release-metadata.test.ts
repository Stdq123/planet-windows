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

test("release versions reject ambiguous or unsupported identifiers", () => {
  for (const version of ["01.2.3", "1.2", "1.2.3-alpha.01", "1.2.3-", "1.2.3-a..1", "1.2.3+build", "1.2.3-测试", "1.2.3-alpha\n", "4294967296.0.0", 3, null]) {
    expect(() => releaseMetadata(JSON.stringify({ version }), changelog)).toThrow("version");
  }
});

test("prerelease metadata retains the full version in the tag and release notes", () => {
  const version = "0.0.1-alpha.1";
  const text = changelog.replaceAll("0.3.0", version);
  const result = releaseMetadata(JSON.stringify({ version }), text, { ...options, tag: `v${version}` });
  expect(result.version).toBe(version);
  expect(result.tag).toBe(`v${version}`);
  expect(result.prerelease).toBe(true);
  expect(result.notes).toContain(`# v${version}`);
  expect(() => releaseMetadata(JSON.stringify({ version }), text, { ...options, tag: "v0.0.1" })).toThrow("tag");
});

test("stable and prerelease draft metadata explicitly distinguish release channels", () => {
  expect(releaseMetadata(versionFile, changelog).prerelease).toBe(false);
  for (const version of ["0.0.1-alpha.1", "1.2.3-beta", "1.2.3-rc-1", "1.2.3-alpha.99999999999999999999"]) {
    const result = releaseMetadata(JSON.stringify({ version }), changelog);
    expect(result.prerelease).toBe(true);
    expect(result.version).toBe(version);
    expect(result.mode).toBe("draft");
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
