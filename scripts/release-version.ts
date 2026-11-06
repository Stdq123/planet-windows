// Release identities omit build metadata: each public version names one artifact.
export function parseReleaseVersion(value: unknown) {
  const match = typeof value === "string"
    ? /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$/.exec(value)
    : null;
  if (!match || match[0] !== value || match.slice(1, 4).some(part => Number(part) > 4294967295)
    || match[4]?.split(".").some(part => /^\d+$/.test(part) && part.length > 1 && part.startsWith("0"))) {
    throw new Error("Invalid app version: expected three uint32 numbers and an optional SemVer prerelease, without leading zeroes or build metadata.");
  }
  return { version: value as string, prerelease: match[4] !== undefined };
}

if (import.meta.main) {
  try {
    if (process.argv.length !== 3) throw new Error("Expected one release version.");
    console.log(JSON.stringify(parseReleaseVersion(process.argv[2])));
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
