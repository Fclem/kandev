// @vitest-environment node
import { expect, test } from "vitest";
import { startHTTPGitFixture } from "../e2e/helpers/http-git-server";
import { execFile, execFileSync } from "node:child_process";
import { type Server } from "node:http";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { promisify } from "node:util";

test("reads the trusted GitLab origin through backend and executor rewrites", async () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-http-git-test-"));
  const git = promisify(execFile);
  try {
    const fixture = await startHTTPGitFixture(root, "rewrite-source", {
      bridgeGateway: "127.0.0.1",
    });
    try {
      expect(
        execFileSync("git", ["remote", "get-url", "origin"], {
          cwd: fixture.checkoutPath,
          encoding: "utf8",
        }).trim(),
      ).toBe("https://gitlab.com/fixture/rewrite-source.git");
      const head = execFileSync("git", ["rev-parse", "HEAD"], {
        cwd: fixture.checkoutPath,
        encoding: "utf8",
      }).trim();
      const environments = [
        fixture.backendEnv,
        Object.fromEntries(fixture.gitConfigEnvVars.map(({ key, value }) => [key, value])),
      ];
      for (const environment of environments) {
        const result = await git("git", ["ls-remote", "origin", "refs/heads/main"], {
          cwd: fixture.checkoutPath,
          env: { ...process.env, ...environment },
          encoding: "utf8",
        });
        expect(result.stdout.trim().split(/\s+/)).toEqual([head, "refs/heads/main"]);
      }
    } finally {
      await fixture.close();
    }
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("closes its listener when backend config setup fails after listening", async () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-http-git-test-"));
  let server: Server | undefined;
  const setupError = new Error("config write failed");
  try {
    await expect(
      startHTTPGitFixture(root, "config-write-failure", {
        bridgeGateway: "127.0.0.1",
        onListening: (listeningServer) => {
          server = listeningServer;
        },
        writeBackendGitConfig: () => {
          throw setupError;
        },
      }),
    ).rejects.toBe(setupError);

    expect(server).toBeDefined();
    expect(server?.listening).toBe(false);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("aggregates setup and listener-close failures", async () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-http-git-test-"));
  const setupError = new Error("config write failed");
  const closeError = new Error("listener close failed");
  try {
    await expect(
      startHTTPGitFixture(root, "close-failure", {
        bridgeGateway: "127.0.0.1",
        writeBackendGitConfig: () => {
          throw setupError;
        },
        closeServer: async (server) => {
          await closeTestServer(server);
          throw closeError;
        },
      }),
    ).rejects.toMatchObject({
      errors: [setupError, closeError],
    });
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

function closeTestServer(server: Server): Promise<void> {
  return new Promise((resolve, reject) =>
    server.close((error) => (error ? reject(error) : resolve())),
  );
}
