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
const BASE_TIMESTAMP = "2026-08-10T10:00:00Z";
const COMPLETION_AT = "2026-08-10T10:55:00Z";
const LIVE_UPDATED_AT = "2026-08-10T11:00:00Z";
const SAME_SECOND_COMPLETION = "2026-08-10T10:05:00Z";

/** Builds a REST-shaped turn row; overrides win for race-specific fields. */
function makeTurn(id: string, overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id,
    session_id: SESSION_ID,
    task_id: "task-1",
    started_at: BASE_TIMESTAMP,
    metadata: { runtime_config_snapshot: { model: "mock-fast" } },
    created_at: BASE_TIMESTAMP,
    updated_at: BASE_TIMESTAMP,
    ...overrides,
  };
}

/** Returns a REST promise the test resolves manually (for race timing). */
function deferredTurnsResponse(): {
  promise: Promise<{ turns: unknown[]; total: number }>;
  resolve: (value: { turns: unknown[]; total: number }) => void;
} {
  let resolve!: (value: { turns: unknown[]; total: number }) => void;
  const promise = new Promise<{ turns: unknown[]; total: number }>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

/** Mirrors the store's addTurn upsert (Object.assign, completed_at preserved). */
function upsertTurn(bySession: Record<string, unknown[]>, turn: Record<string, unknown>): void {
  const sessionId = turn.session_id as string;
  const turns = (bySession[sessionId] ??= []);
  const existing = turns.find((item) => (item as Record<string, unknown>).id === turn.id);
  if (!existing) {
    turns.push(turn);
    return;
  }
  const existingRecord = existing as Record<string, unknown>;
  Object.assign(existingRecord, turn, {
    completed_at: existingRecord.completed_at ?? turn.completed_at,
  });
}

function makeStore(): TurnStoreHarness {
  const state: TurnStoreState = {
    loadedBySession: {},
    sessions: { [SESSION_ID]: { state: "RUNNING" } },
    bySession: {},
  };
  const addTurn = vi.fn((turn: Record<string, unknown>) => {
    upsertTurn(state.bySession, turn);
  });
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
    const { promise, resolve: resolveList } = deferredTurnsResponse();
    mockListSessionTurns.mockReturnValue(promise);
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
});

describe("ensureSessionTurnsLoaded — REST/WS reconciliation", () => {
  it("reconciles an older WS start-snapshot with the newer REST completion", async () => {
    // A `session.turn.started` was delivered but the matching
    // `session.turn.completed` was missed (disconnect window), leaving the
    // store with an incomplete row for a turn the REST full history has as
    // completed. The hydration must merge the newer REST row instead of
    // skipping the ID as "already present".
    const { promise, resolve: resolveList } = deferredTurnsResponse();
    mockListSessionTurns.mockReturnValue(promise);
    const store = makeStore();
    // WS start snapshot: no completed_at, older updated_at.
    store.getState().turns.bySession[SESSION_ID] = [
      makeTurn("turn-1", {
        updated_at: BASE_TIMESTAMP,
        completed_at: undefined,
      }),
    ];

    const pending = ensureSessionTurnsLoaded(SESSION_ID, store as never);
    // REST resolves with the completed row (newer updated_at).
    resolveList({
      turns: [
        makeTurn("turn-1", {
          completed_at: SAME_SECOND_COMPLETION,
          updated_at: SAME_SECOND_COMPLETION,
        }),
      ],
      total: 1,
    });
    await pending;

    const stored = store.getState().turns.bySession[SESSION_ID][0] as Record<string, unknown>;
    expect(stored.completed_at).toBe(SAME_SECOND_COMPLETION);
    expect(store.addTurn).toHaveBeenCalledWith(expect.objectContaining({ id: "turn-1" }));
  });
});

describe("ensureSessionTurnsLoaded — timestamp edge cases", () => {
  it("applies a completed REST row when timestamps collide after precision truncation", async () => {
    // WS rows carry RFC3339Nano fractions; the REST DTO truncates to whole
    // seconds (time.RFC3339). A completion in the same second as the WS start
    // therefore has a REST updated_at <= the WS row's fractional timestamp.
    // The completion state, not the colliding timestamp, must decide.
    const { promise, resolve: resolveList } = deferredTurnsResponse();
    mockListSessionTurns.mockReturnValue(promise);
    const store = makeStore();
    // WS start snapshot with fractional updated_at, no completion.
    store.getState().turns.bySession[SESSION_ID] = [
      makeTurn("turn-1", {
        updated_at: "2026-08-10T10:05:00.123Z",
        completed_at: undefined,
      }),
    ];

    const pending = ensureSessionTurnsLoaded(SESSION_ID, store as never);
    // REST completion row, updated_at truncated to the same second (<= WS).
    resolveList({
      turns: [
        makeTurn("turn-1", {
          completed_at: "2026-08-10T10:05:00.456Z",
          updated_at: SAME_SECOND_COMPLETION,
        }),
      ],
      total: 1,
    });
    await pending;

    const stored = store.getState().turns.bySession[SESSION_ID][0] as Record<string, unknown>;
    expect(stored.completed_at).toBe("2026-08-10T10:05:00.456Z");
  });

  it("treats a REST row without a timestamp as stale, never newest", async () => {
    const { promise, resolve: resolveList } = deferredTurnsResponse();
    mockListSessionTurns.mockReturnValue(promise);
    const store = makeStore();
    // Newer live WS row.
    store.getState().turns.bySession[SESSION_ID] = [
      makeTurn("turn-1", {
        updated_at: LIVE_UPDATED_AT,
        completed_at: COMPLETION_AT,
        metadata: { runtime_config_snapshot: { model: "newer" } },
      }),
    ];

    const pending = ensureSessionTurnsLoaded(SESSION_ID, store as never);
    // Malformed REST row: no updated_at at all.
    resolveList({
      turns: [
        makeTurn("turn-1", {
          updated_at: undefined,
          metadata: { runtime_config_snapshot: { model: "stale" } },
        }),
      ],
      total: 1,
    });
    await pending;

    expect(store.addTurn).not.toHaveBeenCalled();
    const stored = store.getState().turns.bySession[SESSION_ID][0] as Record<string, unknown>;
    expect(stored.updated_at).toBe(LIVE_UPDATED_AT);
    expect(stored.completed_at).toBe(COMPLETION_AT);
  });

  it("does not clobber live WS turn data with a stale REST snapshot", async () => {
    // WS `session.turn.completed` for turn-1 arrives WHILE the REST request
    // is in flight; the REST response is an older snapshot of the same turn
    // plus the pre-WS history (turn-2). The newer live metadata must survive.
    const { promise, resolve: resolveList } = deferredTurnsResponse();
    mockListSessionTurns.mockReturnValue(promise);
    const store = makeStore();

    const pending = ensureSessionTurnsLoaded(SESSION_ID, store as never);
    // Live WS completion lands while the request is pending: newer row.
    store.getState().turns.bySession[SESSION_ID] = [
      makeTurn("turn-1", {
        updated_at: LIVE_UPDATED_AT,
        completed_at: COMPLETION_AT,
        metadata: {
          runtime_config_snapshot: { model: "newer" },
          prompt_usage: { total_tokens: 9 },
        },
      }),
    ];
    resolveList({
      turns: [
        makeTurn("turn-1", { metadata: { runtime_config_snapshot: { model: "older" } } }),
        makeTurn("turn-2", { started_at: "2026-08-11T10:00:00Z" }),
      ],
      total: 2,
    });
    await pending;

    // Only the unseen history turn may be merged; the live row is untouched.
    expect(store.addTurn).toHaveBeenCalledTimes(1);
    expect(store.addTurn).toHaveBeenCalledWith(expect.objectContaining({ id: "turn-2" }));
    const stored = store
      .getState()
      .turns.bySession[
        SESSION_ID
      ].find((turn) => (turn as Record<string, unknown>).id === "turn-1") as Record<
      string,
      unknown
    >;
    expect(stored.updated_at).toBe(LIVE_UPDATED_AT);
    expect(stored.completed_at).toBe(COMPLETION_AT);
    expect(stored.metadata).toEqual({
      runtime_config_snapshot: { model: "newer" },
      prompt_usage: { total_tokens: 9 },
    });
  });
});
