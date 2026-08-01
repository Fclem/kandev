import assert from "node:assert/strict";
import { chmod, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import test from "node:test";

const repoRoot = path.resolve(import.meta.dirname, "../..");
const script = path.join(repoRoot, "scripts/release/npm-view-version.sh");

async function runView(mode) {
  const fixtureDir = await mkdtemp(path.join(tmpdir(), "kandev-npm-view-"));
  const npm = path.join(fixtureDir, "npm");
  await writeFile(
    npm,
    `#!/usr/bin/env bash
case "$MOCK_NPM_MODE" in
  found) printf '%s\\n' '1.2.4-nightly.shaabc123def456' ;;
  missing) printf '%s\\n' 'npm error code E404' 'npm error No match found for version nightly' >&2; exit 1 ;;
  failure) printf '%s\\n' 'npm error code EAI_AGAIN' 'registry-secret-detail' >&2; exit 1 ;;
esac
`,
  );
  await chmod(npm, 0o755);
  try {
    return spawnSync("bash", [script, "kandev@nightly"], {
      encoding: "utf8",
      env: {
        ...process.env,
        MOCK_NPM_MODE: mode,
        PATH: `${fixtureDir}${path.delimiter}${process.env.PATH ?? ""}`,
      },
    });
  } finally {
    await rm(fixtureDir, { recursive: true, force: true });
  }
}

test("prints a resolved npm version", async () => {
  const result = await runView("found");
  assert.equal(result.status, 0);
  assert.equal(result.stdout.trim(), "1.2.4-nightly.shaabc123def456");
});

test("treats a missing version or dist-tag as an empty result", async () => {
  const result = await runView("missing");
  assert.equal(result.status, 0);
  assert.equal(result.stdout, "");
});

test("fails closed on registry and network errors", async () => {
  const result = await runView("failure");
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /npm view failed for kandev@nightly/);
  assert.doesNotMatch(result.stderr, /registry-secret-detail/);
});
