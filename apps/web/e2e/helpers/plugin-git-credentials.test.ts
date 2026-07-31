import { PassThrough } from "node:stream";

import { describe, expect, it } from "vitest";

import { bindFixtureConnectErrors } from "./plugin-git-credentials";

describe("bindFixtureConnectErrors", () => {
  it.each(["client", "upstream"] as const)("destroys both streams on a %s error", (source) => {
    const client = new PassThrough();
    const upstream = new PassThrough();
    bindFixtureConnectErrors(client, upstream);

    const failed = source === "client" ? client : upstream;
    failed.emit("error", new Error("fixture stream failure"));

    expect(client.destroyed).toBe(true);
    expect(upstream.destroyed).toBe(true);
  });
});
