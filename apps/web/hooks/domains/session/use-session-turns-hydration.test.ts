import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  ensureSessionTurnsLoaded,
  clearInFlightTurnsLoadForTest,
} from "./use-session-turns-hydration";

const mockListSessionTurns = vi.fn();

vi.mock("@/lib/api/domains/session-api", () => ({
  listSessionTurns: (...args: unknown[]) => mockListSessionTurns(...args),
}));

type TurnStoreState = {
  loadedBySession: Record<string, boolean>;
  sessions: Record<string, { state: string }>;
};

type TurnStoreMock = {
  getState: () => {
    turns: { loadedBySession: Record<string, boolean> };
    taskSessions: { items: Record<string, { state: string }> };
  };
  addTurn: ReturnType<typeof vi.fn>;
  markTurnsLoaded: ReturnType<typeof vi.fn>;
};

type TurnStoreHarness = TurnStoreMock & {
  setSessions: (v: Record<string, { state: string }>) => void;
};

function makeStore(): TurnStoreHarness {
  const state: TurnStoreState = {
    loadedBySession: {},
    sessions: { "sess-1": { state: "RUNNING" } },
  };
  const addTurn = vi.fn();
  const markTurnsLoaded = vi.fn((sid: string) => {
    state.loadedBySession[sid] = true;
  });
  return {
    getState: () => ({
      turns: { loadedBySession: state.loadedBySession },
      taskSessions: { items: state.sessions },
      // The zustand store exposes actions on getState() too.
      addTurn,
      markTurnsLoaded,
    }),
    addTurn,
    markTurnsLoaded,
    setSessions: (v) => {
      state.sessions = v;
    },
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  mockListSessionTurns.mockResolvedValue({ turns: [], total: 0 });
});

afterEach(() => {
  clearInFlightTurnsLoadForTest();
});

describe("ensureSessionTurnsLoaded", () => {
  it("deduplicates concurrent hydration of the same session", async () => {
    mockListSessionTurns.mockResolvedValue({ turns: [], total: 0 });
    const store = makeStore();

    await Promise.all([
      ensureSessionTurnsLoaded("sess-1", store as never),
      ensureSessionTurnsLoaded("sess-1", store as never),
    ]);

    expect(mockListSessionTurns).toHaveBeenCalledTimes(1);
  });

  it("merges turns and marks the session loaded", async () => {
    mockListSessionTurns.mockResolvedValue({
      turns: [
        {
          id: "turn-1",
          session_id: "sess-1",
          task_id: "task-1",
          started_at: "2026-08-10T10:00:00Z",
          metadata: { runtime_config_snapshot: { model: "mock-fast" } },
          created_at: "2026-08-10T10:00:00Z",
          updated_at: "2026-08-10T10:00:00Z",
        },
      ],
      total: 1,
    });
    const store = makeStore();

    await ensureSessionTurnsLoaded("sess-1", store as never);

    expect(store.addTurn).toHaveBeenCalledWith(expect.objectContaining({ id: "turn-1" }));
    expect(store.markTurnsLoaded).toHaveBeenCalledWith("sess-1");
  });

  it("marks an empty session loaded so it is never refetched", async () => {
    mockListSessionTurns.mockResolvedValue({ turns: [], total: 0 });
    const store = makeStore();

    await ensureSessionTurnsLoaded("sess-1", store as never);
    await ensureSessionTurnsLoaded("sess-1", store as never);

    expect(mockListSessionTurns).toHaveBeenCalledTimes(1);
    expect(store.markTurnsLoaded).toHaveBeenCalledTimes(1);
  });

  it("skips the fetch entirely when the session is already marked loaded", async () => {
    const store = makeStore();
    store.getState().turns.loadedBySession["sess-1"] = true;

    await ensureSessionTurnsLoaded("sess-1", store as never);

    expect(mockListSessionTurns).not.toHaveBeenCalled();
  });

  it("does not resurrect turns after the session was removed mid-flight", async () => {
    let resolveList!: (value: { turns: unknown[]; total: number }) => void;
    mockListSessionTurns.mockReturnValue(
      new Promise<{ turns: unknown[]; total: number }>((resolve) => {
        resolveList = resolve;
      }),
    );
    const store = makeStore();

    const pending = ensureSessionTurnsLoaded("sess-1", store as never);
    // Session deleted while the REST request is in flight (removeTaskSession
    // drops both taskSessions.items[sid] and turns.bySession[sid]).
    store.setSessions({});
    resolveList({ turns: [{ id: "turn-stale" }], total: 1 });
    await pending;

    expect(store.addTurn).not.toHaveBeenCalled();
    expect(store.markTurnsLoaded).not.toHaveBeenCalled();
  });

  it("clears the in-flight entry on failure so the next fetch retries", async () => {
    mockListSessionTurns.mockRejectedValueOnce(new Error("boom"));
    mockListSessionTurns.mockResolvedValueOnce({ turns: [], total: 0 });
    const store = makeStore();

    await ensureSessionTurnsLoaded("sess-1", store as never);
    await ensureSessionTurnsLoaded("sess-1", store as never);

    expect(mockListSessionTurns).toHaveBeenCalledTimes(2);
    expect(store.markTurnsLoaded).toHaveBeenCalledTimes(1);
  });

  it("skips the fetch when the session is absent from the store at entry", async () => {
    mockListSessionTurns.mockResolvedValue({ turns: [], total: 0 });
    const store = makeStore();
    store.setSessions({});

    await ensureSessionTurnsLoaded("sess-1", store as never);

    expect(mockListSessionTurns).not.toHaveBeenCalled();
    expect(store.addTurn).not.toHaveBeenCalled();
  });
});
