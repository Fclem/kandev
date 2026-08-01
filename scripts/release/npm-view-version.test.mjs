import assert from "node:assert/strict";
import { chmod, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import test from "node:test";

const repoRoot = path.resolve(import.meta.dirname, "../..");
const script = path.join(repoRoot, "scripts/release/npm-view-version.sh");

async function runView(mode) {
  const fixtureDir = await mkdtemp(path.join(tmpdir(), "kandev-npm-view-"));
  const npm = path.join(fixtureDir, "npm");
  const sleep = path.join(fixtureDir, "sleep");
  const countFile = path.join(fixtureDir, "attempts");
  await writeFile(
    npm,
    `#!/usr/bin/env bash
count=0
if [[ -f "$MOCK_NPM_COUNT_FILE" ]]; then read -r count < "$MOCK_NPM_COUNT_FILE"; fi
count=$((count + 1))
printf '%s\n' "$count" > "$MOCK_NPM_COUNT_FILE"
case "$MOCK_NPM_MODE" in
  found) printf '%s\\n' '1.2.4-nightly.shaabc123def456' ;;
  missing) printf '%s\\n' 'npm error code E404' 'npm error No match found for version nightly' >&2; exit 1 ;;
  transient)
    if (( count < 3 )); then
      printf '%s\\n' 'npm error code EAI_AGAIN' >&2
      exit 1
    fi
    printf '%s\\n' '1.2.4-nightly.shaabc123def456'
    ;;
  failure) printf '%s\\n' 'npm error code EAI_AGAIN' 'registry-secret-detail' >&2; exit 1 ;;
esac
`,
  );
  await writeFile(sleep, "#!/usr/bin/env bash\nexit 0\n");
  await chmod(npm, 0o755);
  await chmod(sleep, 0o755);
  try {
    const result = spawnSync("bash", [script, "kandev@nightly"], {
      encoding: "utf8",
      env: {
        ...process.env,
        MOCK_NPM_COUNT_FILE: countFile,
        MOCK_NPM_MODE: mode,
        PATH: `${fixtureDir}${path.delimiter}${process.env.PATH ?? ""}`,
      },
    });
    result.attempts = Number(await readFile(countFile, "utf8"));
    return result;
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

test("retries transient registry failures before succeeding", async () => {
  const result = await runView("transient");
  assert.equal(result.status, 0);
  assert.equal(result.stdout.trim(), "1.2.4-nightly.shaabc123def456");
  assert.equal(result.attempts, 3);
});

test("fails closed on registry and network errors", async () => {
  const result = await runView("failure");
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /npm view failed for kandev@nightly/);
  assert.doesNotMatch(result.stderr, /registry-secret-detail/);
  assert.equal(result.attempts, 3);
});
