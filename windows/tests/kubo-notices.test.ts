import { expect, test } from "bun:test";
import { resolve } from "node:path";

test("Kubo supplemental license evidence rejects unreviewed identities and bytes", () => {
  // The standalone collector uses //go:build ignore; explicit files run its
  // offline tests without adding a Go package beside the Wine C fixture.
  const result = Bun.spawnSync([
    "go", "test", "-count=1",
    "scripts/release-audit/kubo-notices.go",
    "scripts/release-audit/kubo-notices_test.go",
  ], { cwd: resolve(import.meta.dir, ".."), stdout: "pipe", stderr: "pipe" });
  expect(result.exitCode, new TextDecoder().decode(result.stdout) + new TextDecoder().decode(result.stderr)).toBe(0);
}, 60_000);
