import { Duplex, PassThrough } from "node:stream";

import { describe, expect, it } from "vitest";

import { bindFixtureConnectErrors, bridgeFixtureConnectStreams } from "./plugin-git-credentials";

class TestEndpoint extends Duplex {
  readonly writes: string[] = [];

  _read(): void {}

  _write(chunk: Buffer, _encoding: BufferEncoding, callback: (error?: Error | null) => void): void {
    this.writes.push(chunk.toString());
    callback();
  }

  send(value: string): void {
    this.push(value);
  }
}

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

describe("bridgeFixtureConnectStreams", () => {
  it("pipes both directions and tears down both streams when either leg fails", async () => {
    const client = new TestEndpoint();
    const upstream = new TestEndpoint();
    bridgeFixtureConnectStreams(client, upstream);

    client.send("request");
    upstream.send("response");
    await new Promise<void>((resolve) => setImmediate(resolve));

    expect(upstream.writes).toEqual(["request"]);
    expect(client.writes).toEqual(["response"]);

    client.destroy(new Error("fixture stream failure"));
    await new Promise<void>((resolve) => setImmediate(resolve));

    expect(client.destroyed).toBe(true);
    expect(upstream.destroyed).toBe(true);
  });
});
