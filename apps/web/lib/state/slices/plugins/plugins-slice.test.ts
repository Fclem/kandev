import { describe, expect, it } from "vitest";
import { create, type StoreApi } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createPluginsSlice } from "./plugins-slice";
import type { PluginsSlice } from "./types";
import type { GitHubStarConnectionStatus } from "@/lib/types/github";
import type { PluginRecord } from "@/lib/types/plugins";
import { pluginStarKey } from "@/lib/plugins/plugin-star-connection";

function makeStore() {
  return create<PluginsSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createPluginsSlice as any)(...a) })),
  );
}

function plugin(id: string, overrides: Partial<PluginRecord> = {}): PluginRecord {
  return {
    id,
    api_version: 1,
    version: "1.0.0",
    display_name: `Plugin ${id}`,
    description: "",
    author: "",
    categories: [],
    capabilities: {},
    status: "registered",
    install_path: `/home/user/.kandev/plugins/${id}/1.0.0`,
    signed: true,
    installed_at: "2026-01-01T00:00:00Z",
    restart_count: 0,
    ...overrides,
  };
}

const connection = (userId: string) => ({
  user_id: userId,
  credential_generation: 1,
  active: true,
  capable: true,
  reconnect_required: false,
  available: true,
});

describe("plugins slice", () => {
  it("starts empty, not loading, not loaded, no error", () => {
    const store = makeStore();
    const s = store.getState();
    expect(s.plugins.items).toEqual([]);
    expect(s.plugins.loading).toBe(false);
    expect(s.plugins.loaded).toBe(false);
    expect(s.plugins.error).toBeNull();
  });

  it("setPlugins replaces items, flips loaded=true, and clears error", () => {
    const store = makeStore();
    store.getState().setPluginsError("boom");
    store.getState().setPlugins([plugin("a"), plugin("b")]);
    const s = store.getState();
    expect(s.plugins.items.map((p) => p.id)).toEqual(["a", "b"]);
    expect(s.plugins.loaded).toBe(true);
    expect(s.plugins.error).toBeNull();
  });

  it("setPluginsLoading toggles the loading flag", () => {
    const store = makeStore();
    store.getState().setPluginsLoading(true);
    expect(store.getState().plugins.loading).toBe(true);
    store.getState().setPluginsLoading(false);
    expect(store.getState().plugins.loading).toBe(false);
  });

  it("setPluginsError sets the error message", () => {
    const store = makeStore();
    store.getState().setPluginsError("network error");
    expect(store.getState().plugins.error).toBe("network error");
    store.getState().setPluginsError(null);
    expect(store.getState().plugins.error).toBeNull();
  });

  it("upsertPlugin appends a new plugin", () => {
    const store = makeStore();
    store.getState().setPlugins([plugin("a")]);
    store.getState().upsertPlugin(plugin("b"));
    expect(store.getState().plugins.items.map((p) => p.id)).toEqual(["a", "b"]);
  });

  it("upsertPlugin replaces an existing plugin by id in place", () => {
    const store = makeStore();
    store.getState().setPlugins([plugin("a", { status: "registered" }), plugin("b")]);
    store.getState().upsertPlugin(plugin("a", { status: "active" }));
    const s = store.getState();
    expect(s.plugins.items.map((p) => p.id)).toEqual(["a", "b"]);
    expect(s.plugins.items[0].status).toBe("active");
  });

  it("removePlugin filters by id", () => {
    const store = makeStore();
    store.getState().setPlugins([plugin("a"), plugin("b"), plugin("c")]);
    store.getState().removePlugin("b");
    expect(store.getState().plugins.items.map((p) => p.id)).toEqual(["a", "c"]);
  });
});

describe("plugin star connection identity", () => {
  it("clears the tuple and visible states when the authenticated user changes", () => {
    const store = makeStore();
    const firstEpoch = store.getState().beginPluginStarConnectionRequest("ws-1", "user-a");
    expect(
      store
        .getState()
        .applyPluginStarConnection("ws-1", firstEpoch, connection("user-a"), "user-a"),
    ).toBe(true);
    const scopeKey = pluginStarKey("ws-1", connection("user-a"), "owner/repo");
    const mutationEpoch = store.getState().beginPluginStarRequest(scopeKey, "mutation");
    expect(mutationEpoch).not.toBeNull();
    store.getState().applyPluginStarResult(scopeKey, mutationEpoch as number, true);
    expect(store.getState().pluginStars.values[scopeKey]).toBe(true);

    const secondEpoch = store.getState().beginPluginStarConnectionRequest("ws-1", "user-b");
    const entry = store.getState().pluginStars.connections["ws-1"];
    expect(entry.userId).toBe("user-b");
    expect(entry.status).toBeNull();
    expect(entry.loaded).toBe(false);
    expect(store.getState().pluginStars.values[scopeKey]).toBeUndefined();
    expect(secondEpoch).toBeGreaterThan(firstEpoch);
  });

  it("keeps the tuple when the same user reconnects", () => {
    const store = makeStore();
    const firstEpoch = store.getState().beginPluginStarConnectionRequest("ws-1", "user-a");
    store.getState().applyPluginStarConnection("ws-1", firstEpoch, connection("user-a"), "user-a");

    store.getState().beginPluginStarConnectionRequest("ws-1", "user-a");
    const entry = store.getState().pluginStars.connections["ws-1"];
    expect(entry.status?.user_id).toBe("user-a");
    expect(entry.loaded).toBe(true);
  });

  it("refuses a response that answers for another account", () => {
    const store = makeStore();
    const epoch = store.getState().beginPluginStarConnectionRequest("ws-1", "user-a");
    expect(
      store.getState().applyPluginStarConnection("ws-1", epoch, connection("user-b"), "user-a"),
    ).toBe(false);
    expect(store.getState().pluginStars.connections["ws-1"].status).toBeNull();
  });

  it("refuses a response whose epoch was superseded", () => {
    const store = makeStore();
    const staleEpoch = store.getState().beginPluginStarConnectionRequest("ws-1", "user-a");
    store.getState().beginPluginStarConnectionRequest("ws-1", "user-a");
    expect(
      store
        .getState()
        .applyPluginStarConnection("ws-1", staleEpoch, connection("user-a"), "user-a"),
    ).toBe(false);
  });
  it("rejects a deferred connection response after reset and same-user reload", async () => {
    const store = makeStore();
    const staleEpoch = store.getState().beginPluginStarConnectionRequest("ws-1", "user-a");
    let resolveStaleStatus!: (status: GitHubStarConnectionStatus) => void;
    const staleStatusResponse = new Promise<GitHubStarConnectionStatus>((resolve) => {
      resolveStaleStatus = resolve;
    });
    const staleApplied = staleStatusResponse.then((status) =>
      store.getState().applyPluginStarConnection("ws-1", staleEpoch, status, "user-a"),
    );

    store.getState().resetPluginStarConnection("ws-1");
    const currentEpoch = store.getState().beginPluginStarConnectionRequest("ws-1", "user-a");
    const currentStatus = { ...connection("user-a"), credential_generation: 2 };
    expect(
      store.getState().applyPluginStarConnection("ws-1", currentEpoch, currentStatus, "user-a"),
    ).toBe(true);

    resolveStaleStatus(connection("user-a"));
    await expect(staleApplied).resolves.toBe(false);
    expect(store.getState().pluginStars.connections["ws-1"].status?.credential_generation).toBe(2);
  });

  it("rejects star responses after a scoped cache is cleared and recreated", () => {
    const store = makeStore();
    const firstStatusEpoch = store.getState().beginPluginStarConnectionRequest("ws-1", "user-a");
    store
      .getState()
      .applyPluginStarConnection("ws-1", firstStatusEpoch, connection("user-a"), "user-a");
    const key = pluginStarKey("ws-1", connection("user-a"), "owner/repo");
    const staleEpoch = store.getState().beginPluginStarRequest(key, "mutation");

    const secondStatusEpoch = store.getState().beginPluginStarConnectionRequest("ws-1", "user-b");
    store
      .getState()
      .applyPluginStarConnection("ws-1", secondStatusEpoch, connection("user-b"), "user-b");
    const thirdStatusEpoch = store.getState().beginPluginStarConnectionRequest("ws-1", "user-a");
    store
      .getState()
      .applyPluginStarConnection("ws-1", thirdStatusEpoch, connection("user-a"), "user-a");
    const currentEpoch = store.getState().beginPluginStarRequest(key, "mutation");

    expect(store.getState().applyPluginStarResult(key, currentEpoch as number, false)).toBe(true);
    expect(store.getState().applyPluginStarResult(key, staleEpoch as number, true)).toBe(false);
    store.getState().failPluginStarRequest(key, staleEpoch as number);
    expect(store.getState().pluginStars.values[key]).toBe(false);
    expect(store.getState().pluginStars.busy[key]).toBeUndefined();
  });
});

// @covers AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.13
describe("confirmed repository star counts", () => {
  const repository = "owner/repo";
  const key = pluginStarKey("ws-1", connection("user-a"), repository);

  function hydrate(store: StoreApi<PluginsSlice>, value: boolean) {
    const epoch = store.getState().beginPluginStarRequest(key, "status")!;
    store.getState().applyPluginStarResult(key, epoch, value);
  }

  it("adjusts a known count once per confirmed transition, never during hydration or failure", () => {
    const store = makeStore();
    const actions = store.getState();
    actions.seedPluginStarCount(repository, 12);
    hydrate(store, false);
    expect(store.getState().pluginStars.repositoryCounts[repository]).toBe(12);

    const failed = actions.beginPluginStarRequest(key, "mutation")!;
    expect(store.getState().pluginStars.repositoryCounts[repository]).toBe(12);
    expect(actions.beginPluginStarRequest(key, "mutation")).toBeNull();
    actions.failPluginStarRequest(key, failed);
    expect(store.getState().pluginStars.repositoryCounts[repository]).toBe(12);

    const starred = actions.beginPluginStarRequest(key, "mutation")!;
    expect(actions.applyPluginStarMutationResult(key, starred, repository, true)).toBe(true);
    expect(store.getState().pluginStars.repositoryCounts[repository]).toBe(13);
    expect(actions.applyPluginStarMutationResult(key, starred, repository, false)).toBe(false);
    expect(store.getState().pluginStars.values[key]).toBe(true);
    expect(store.getState().pluginStars.repositoryCounts[repository]).toBe(13);

    // A confirmed no-op is idempotent, even with a new request epoch.
    const unchanged = actions.beginPluginStarRequest(key, "mutation")!;
    actions.applyPluginStarMutationResult(key, unchanged, repository, true);
    expect(store.getState().pluginStars.repositoryCounts[repository]).toBe(13);

    const unstarred = actions.beginPluginStarRequest(key, "mutation")!;
    actions.applyPluginStarMutationResult(key, unstarred, repository, false);
    expect(store.getState().pluginStars.repositoryCounts[repository]).toBe(12);
  });

  it("preserves aggregate counts across actor changes and rejects the previous actor's result", () => {
    const store = makeStore();
    const actions = store.getState();
    const first = {
      ...connection("user-a"),
      actor: { source: "gh_cli", login: "alice", shared: true },
    };
    const firstEpoch = actions.beginPluginStarConnectionRequest("ws-1", "user-a");
    actions.applyPluginStarConnection("ws-1", firstEpoch, first, "user-a");
    const actorKey = pluginStarKey("ws-1", first, repository);
    actions.seedPluginStarCount(repository, 12);
    const statusEpoch = actions.beginPluginStarRequest(actorKey, "status")!;
    actions.applyPluginStarResult(actorKey, statusEpoch, false);
    const success = actions.beginPluginStarRequest(actorKey, "mutation")!;
    actions.applyPluginStarMutationResult(actorKey, success, repository, true);
    const stale = actions.beginPluginStarRequest(actorKey, "mutation")!;

    const second = { ...first, actor: { ...first.actor, login: "bob" } };
    const secondEpoch = actions.beginPluginStarConnectionRequest("ws-1", "user-a");
    actions.applyPluginStarConnection("ws-1", secondEpoch, second, "user-a");
    expect(actions.applyPluginStarMutationResult(actorKey, stale, repository, false)).toBe(false);
    expect(store.getState().pluginStars.values[actorKey]).toBeUndefined();
    expect(store.getState().pluginStars.repositoryCounts[repository]).toBe(13);
    actions.seedPluginStarCount(repository, 12);
    actions.resetPluginStarConnection("ws-1");
    expect(store.getState().pluginStars.repositoryCounts[repository]).toBe(13);
  });

  it("keeps unknown counts unknown and never lets an unstar decrement below zero", () => {
    const store = makeStore();
    const actions = store.getState();
    actions.seedPluginStarCount(repository, null);
    hydrate(store, false);
    const starred = actions.beginPluginStarRequest(key, "mutation")!;
    actions.applyPluginStarMutationResult(key, starred, repository, true);
    expect(store.getState().pluginStars.repositoryCounts[repository]).toBeUndefined();
    expect(store.getState().pluginStars.values[key]).toBe(true);

    // A later known catalog count may seed an unknown projection, including zero.
    actions.seedPluginStarCount(repository, 0);
    const unstarred = actions.beginPluginStarRequest(key, "mutation")!;
    actions.applyPluginStarMutationResult(key, unstarred, repository, false);
    expect(store.getState().pluginStars.repositoryCounts[repository]).toBe(0);
  });
});
