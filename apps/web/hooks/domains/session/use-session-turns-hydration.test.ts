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
  bySession: Record<string, unknown[]>;
};

type TurnStoreMock = {
  getState: () => {
    turns: { loadedBySession: Record<string, boolean>; bySession: Record<string, unknown[]> };
    taskSessions: { items: Record<string, { state: string }> };
  };
  addTurn: ReturnType<typeof vi.fn>;
  markTurnsLoaded: ReturnType<typeof vi.fn>;
};

type TurnStoreHarness = TurnStoreMock & {
  setSessions: (v: Record<string, { state: string }>) => void;
};

const SESSION_ID = "sess-1";
const EMPTY_TURNS = { turns: [], total: 0 };

/** Builds a REST-shaped turn row; overrides win for race-specific fields. */
function makeTurn(id: string, overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id,
    session_id: SESSION_ID,
    task_id: "task-1",
    started_at: "2026-08-10T10:00:00Z",
    metadata: { runtime_config_snapshot: { model: "mock-fast" } },
    created_at: "2026-08-10T10:00:00Z",
    updated_at: "2026-08-10T10:00:00Z",
    ...overrides,
  };
}

function makeStore(): TurnStoreHarness {
  const state: TurnStoreState = {
    loadedBySession: {},
    sessions: { [SESSION_ID]: { state: "RUNNING" } },
    bySession: {},
  };
  const addTurn = vi.fn();
  const markTurnsLoaded = vi.fn((sid: string) => {
    state.loadedBySession[sid] = true;
  });
  return {
    getState: () => ({
      turns: { loadedBySession: state.loadedBySession, bySession: state.bySession },
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
  mockListSessionTurns.mockResolvedValue(EMPTY_TURNS);
});

afterEach(() => {
  clearInFlightTurnsLoadForTest();
});

describe("ensureSessionTurnsLoaded — hydration and marker", () => {
  it("deduplicates concurrent hydration of the same session", async () => {
    mockListSessionTurns.mockResolvedValue(EMPTY_TURNS);
    const store = makeStore();

    await Promise.all([
      ensureSessionTurnsLoaded(SESSION_ID, store as never),
      ensureSessionTurnsLoaded(SESSION_ID, store as never),
    ]);

    expect(mockListSessionTurns).toHaveBeenCalledTimes(1);
  });

  it("merges unseen history turns and marks the session loaded", async () => {
    mockListSessionTurns.mockResolvedValue({ turns: [makeTurn("turn-1")], total: 1 });
    const store = makeStore();

    await ensureSessionTurnsLoaded(SESSION_ID, store as never);

    expect(store.addTurn).toHaveBeenCalledWith(expect.objectContaining({ id: "turn-1" }));
    expect(store.markTurnsLoaded).toHaveBeenCalledWith(SESSION_ID);
  });

  it("marks an empty session loaded so it is never refetched", async () => {
    mockListSessionTurns.mockResolvedValue(EMPTY_TURNS);
    const store = makeStore();

    await ensureSessionTurnsLoaded(SESSION_ID, store as never);
    await ensureSessionTurnsLoaded(SESSION_ID, store as never);

    expect(mockListSessionTurns).toHaveBeenCalledTimes(1);
    expect(store.markTurnsLoaded).toHaveBeenCalledTimes(1);
  });

  it("skips the fetch entirely when the session is already marked loaded", async () => {
    const store = makeStore();
    store.getState().turns.loadedBySession[SESSION_ID] = true;

    await ensureSessionTurnsLoaded(SESSION_ID, store as never);

    expect(mockListSessionTurns).not.toHaveBeenCalled();
  });
});

describe("ensureSessionTurnsLoaded — guards and races", () => {
  it("does not resurrect turns after the session was removed mid-flight", async () => {
    let resolveList!: (value: { turns: unknown[]; total: number }) => void;
    mockListSessionTurns.mockReturnValue(
      new Promise<{ turns: unknown[]; total: number }>((resolve) => {
        resolveList = resolve;
      }),
    );
    const store = makeStore();

    const pending = ensureSessionTurnsLoaded(SESSION_ID, store as never);
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

    await ensureSessionTurnsLoaded(SESSION_ID, store as never);
    await ensureSessionTurnsLoaded(SESSION_ID, store as never);

    expect(mockListSessionTurns).toHaveBeenCalledTimes(2);
    expect(store.markTurnsLoaded).toHaveBeenCalledTimes(1);
  });

  it("skips the fetch when the session is absent from the store at entry", async () => {
    mockListSessionTurns.mockResolvedValue(EMPTY_TURNS);
    const store = makeStore();
    store.setSessions({});

    await ensureSessionTurnsLoaded(SESSION_ID, store as never);

    expect(mockListSessionTurns).not.toHaveBeenCalled();
    expect(store.addTurn).not.toHaveBeenCalled();
  });

  it("does not clobber live WS turn data with a stale REST snapshot", async () => {
    // WS session.turn.started/completed seeded turn-1 with the completion
    // metadata while the REST request was in flight; the REST response is an
    // older snapshot of the same turn plus the pre-WS history (turn-2).
    // addTurn's Object.assign would overwrite the newer live metadata, so the
    // hydration must only merge turns the store has not seen yet.
    mockListSessionTurns.mockResolvedValue({
      turns: [
        makeTurn("turn-1", { metadata: { runtime_config_snapshot: { model: "older" } } }),
        makeTurn("turn-2", { started_at: "2026-08-11T10:00:00Z" }),
      ],
      total: 2,
    });
    const store = makeStore();
    // The live WS turn already in the store: newer metadata than the REST
    // snapshot for the same id.
    store.getState().turns.bySession[SESSION_ID] = [
      {
        id: "turn-1",
        metadata: {
          runtime_config_snapshot: { model: "newer" },
          prompt_usage: { total_tokens: 9 },
        },
      },
    ] as never;

    await ensureSessionTurnsLoaded(SESSION_ID, store as never);

    // Only the unseen history turn may be merged.
    expect(store.addTurn).toHaveBeenCalledTimes(1);
    expect(store.addTurn).toHaveBeenCalledWith(expect.objectContaining({ id: "turn-2" }));
  });
});
