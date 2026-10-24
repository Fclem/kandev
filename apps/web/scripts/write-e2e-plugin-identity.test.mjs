import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, writeFile, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";

test("fixture identity records the actual manifest minimum rather than a pinned version", async () => {
  const dir = await mkdtemp(path.join(tmpdir(), "effort-identity-"));
  try {
    const source = path.join(dir, "source.js"),
      generated = path.join(dir, "bundle.js"),
      manifest = path.join(dir, "manifest.yaml"),
      archive = path.join(dir, "plugin.tar.gz"),
      identity = path.join(dir, "identity.json");
    await writeFile(source, "source");
    await writeFile(generated, 'id: "prompt-history-plugin"');
    await writeFile(archive, "archive");
    await writeFile(
      manifest,
      'id: "kandev-plugin-e2e"\nversion: "1.0.0"\nmin_kandev_version: "0.98.0"\ncapabilities:\n  api_read: ["messages", "session_activity"]\n',
    );
    const result = spawnSync(
      process.execPath,
      ["scripts/write-e2e-plugin-identity.mjs", source, generated, manifest, archive, identity],
      { encoding: "utf8" },
    );
    assert.equal(result.status, 0, result.stderr);
    assert.equal(JSON.parse(await readFile(identity, "utf8")).minKandevVersion, "0.98.0");
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});
