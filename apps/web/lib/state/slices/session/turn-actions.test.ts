import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "@/lib/state/slices/session/session-slice";
import type { SessionSlice } from "@/lib/state/slices/session/types";
import { sessionId, taskId } from "@/lib/types/ids";
import type { Turn } from "@/lib/types/http";
import { parseTurnTimestamp, shouldApplyTurnUpdate } from "./turn-actions";

const SESSION_ID = sessionId("session-1");
const TASK_ID = taskId("task-1");
const STARTED_AT = "2026-07-23T10:00:00.000Z";
const COMPLETED_AT = "2026-07-23T10:01:00.000Z";
const LATER_AT = "2026-07-23T10:02:00.000Z";

function makeStore() {
  return create<SessionSlice>()(
    immer((set) => ({
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...(createSessionSlice as any)(set),
      quickChat: { sessions: [] },
      availableCommands: { bySessionId: {} },
    })),
  );
}

function turn(
  id: string,
  overrides: Partial<{
    started_at: string;
    completed_at: string | undefined;
    updated_at: string | undefined;
    metadata: Record<string, unknown>;
  }> = {},
): Turn {
  const completedAt = overrides.completed_at ?? undefined;
  const updatedAt = overrides.updated_at ?? completedAt ?? overrides.started_at ?? STARTED_AT;
  return {
    id,
    session_id: SESSION_ID,
    task_id: TASK_ID,
    started_at: overrides.started_at ?? STARTED_AT,
    completed_at: completedAt,
    created_at: STARTED_AT,
    updated_at: updatedAt,
    metadata: overrides.metadata,
  };
}

describe("parseTurnTimestamp", () => {
  it("accepts RFC3339 with explicit offset or UTC marker", () => {
    expect(parseTurnTimestamp("2026-07-23T10:00:00Z")).toBeGreaterThan(0);
    expect(parseTurnTimestamp("2026-07-23T10:00:00.123Z")).toBeGreaterThan(0);
    expect(parseTurnTimestamp("2026-07-23T10:00:00+02:00")).toBeGreaterThan(0);
  });

  it.each([
    ["0", "Date.parse parses this as 2000-01-01"],
    ["2026-08-31", "partial date (midnight)"],
    ["2026-08-31T23:00:00", "timezone-less (local zone)"],
    ["not-a-timestamp", "garbage"],
    ["", "empty"],
    [undefined, "absent"],
  ])("treats %s as stale", (_value, _reason) => {
    expect(parseTurnTimestamp(_value)).toBe(-Infinity);
  });
});

describe("addTurn reconciliation", () => {
  it("applies a newer completed row over an incomplete WS row", () => {
    const store = makeStore();
    store.getState().addTurn(turn("turn-1", { started_at: STARTED_AT }));
    store
      .getState()
      .addTurn(turn("turn-1", { completed_at: COMPLETED_AT, metadata: { model: "new" } }));
    expect(store.getState().turns.bySession[SESSION_ID][0]).toEqual(
      expect.objectContaining({ completed_at: COMPLETED_AT, metadata: { model: "new" } }),
    );
  });

  it("rejects a stale WS started event after a completed row was hydrated", () => {
    // The reviewer-reported race: REST hydration merged the completed row,
    // then a delayed session.turn.started upsert arrives via addTurn. The
    // stale started payload must not regress metadata or updated_at.
    const store = makeStore();
    store.getState().addTurn(
      turn("turn-1", {
        completed_at: COMPLETED_AT,
        updated_at: COMPLETED_AT,
        metadata: { prompt_usage: { total_tokens: 9 }, model: "newer" },
      }),
    );

    store.getState().addTurn(
      turn("turn-1", {
        started_at: STARTED_AT,
        updated_at: STARTED_AT,
        metadata: { model: "stale-start" },
      }),
    );

    const stored = store.getState().turns.bySession[SESSION_ID][0];
    expect(stored.completed_at).toBe(COMPLETED_AT);
    expect(stored.updated_at).toBe(COMPLETED_AT);
    expect(stored.metadata).toEqual({ prompt_usage: { total_tokens: 9 }, model: "newer" });
  });

  it("keeps the existing row when timestamps are equal", () => {
    const store = makeStore();
    store
      .getState()
      .addTurn(turn("turn-1", { completed_at: COMPLETED_AT, metadata: { model: "existing" } }));
    store
      .getState()
      .addTurn(turn("turn-1", { completed_at: COMPLETED_AT, metadata: { model: "same-ts" } }));
    expect(store.getState().turns.bySession[SESSION_ID][0].metadata).toEqual({
      model: "existing",
    });
  });
});

describe("addTurn malformed timestamps", () => {
  it("treats a malformed incoming timestamp as stale", () => {
    const store = makeStore();
    store
      .getState()
      .addTurn(turn("turn-1", { completed_at: COMPLETED_AT, metadata: { model: "valid" } }));
    store.getState().addTurn(
      turn("turn-1", {
        completed_at: COMPLETED_AT,
        updated_at: "0",
        metadata: { model: "malformed" },
      }),
    );
    expect(store.getState().turns.bySession[SESSION_ID][0].metadata).toEqual({ model: "valid" });
  });

  it("applies a valid incoming row when the existing timestamp is malformed", () => {
    const store = makeStore();
    store
      .getState()
      .addTurn(
        turn("turn-1", { completed_at: COMPLETED_AT, updated_at: "0", metadata: { model: "old" } }),
      );
    store
      .getState()
      .addTurn(turn("turn-1", { completed_at: LATER_AT, metadata: { model: "fresh" } }));
    expect(store.getState().turns.bySession[SESSION_ID][0].metadata).toEqual({ model: "fresh" });
  });

  it("treats a partial-date incoming timestamp as stale", () => {
    const store = makeStore();
    store
      .getState()
      .addTurn(turn("turn-1", { completed_at: COMPLETED_AT, metadata: { model: "valid" } }));
    store.getState().addTurn(
      turn("turn-1", {
        completed_at: COMPLETED_AT,
        updated_at: "2026-08-31",
        metadata: { model: "partial" },
      }),
    );
    expect(store.getState().turns.bySession[SESSION_ID][0].metadata).toEqual({ model: "valid" });
  });

  it("treats a timezone-less incoming timestamp as stale", () => {
    const store = makeStore();
    store
      .getState()
      .addTurn(turn("turn-1", { completed_at: COMPLETED_AT, metadata: { model: "valid" } }));
    store.getState().addTurn(
      turn("turn-1", {
        completed_at: COMPLETED_AT,
        updated_at: "2026-08-31T23:00:00",
        metadata: { model: "no-zone" },
      }),
    );
    expect(store.getState().turns.bySession[SESSION_ID][0].metadata).toEqual({ model: "valid" });
  });

  it("applies a valid incoming row when the existing timestamp is timezone-less", () => {
    const store = makeStore();
    store.getState().addTurn(
      turn("turn-1", {
        completed_at: COMPLETED_AT,
        updated_at: "2026-08-31T23:00:00",
        metadata: { model: "old" },
      }),
    );
    store
      .getState()
      .addTurn(turn("turn-1", { completed_at: LATER_AT, metadata: { model: "fresh" } }));
    expect(store.getState().turns.bySession[SESSION_ID][0].metadata).toEqual({ model: "fresh" });
  });
});

describe("completeTurn stale guard", () => {
  it("applies a genuine completion and clears the active marker", () => {
    const store = makeStore();
    store.getState().addTurn(turn("turn-1", { started_at: STARTED_AT }));
    store.getState().setActiveTurn(SESSION_ID, "turn-1");

    store
      .getState()
      .completeTurn(SESSION_ID, "turn-1", COMPLETED_AT, { model: "new" }, COMPLETED_AT);

    const stored = store.getState().turns.bySession[SESSION_ID][0];
    expect(stored.completed_at).toBe(COMPLETED_AT);
    expect(stored.metadata).toEqual({ model: "new" });
    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeNull();
  });

  it("ignores a stale re-delivered completion", () => {
    const store = makeStore();
    store.getState().addTurn(
      turn("turn-1", {
        completed_at: LATER_AT,
        updated_at: LATER_AT,
        metadata: { model: "newer" },
      }),
    );

    store
      .getState()
      .completeTurn(SESSION_ID, "turn-1", COMPLETED_AT, { model: "stale" }, COMPLETED_AT);

    const stored = store.getState().turns.bySession[SESSION_ID][0];
    expect(stored.completed_at).toBe(LATER_AT);
    expect(stored.metadata).toEqual({ model: "newer" });
  });
});

describe("shouldApplyTurnUpdate", () => {
  it("completion precedence beats equal or older timestamps", () => {
    const existing = turn("t", { started_at: STARTED_AT, updated_at: STARTED_AT });
    const incoming = turn("t", {
      completed_at: COMPLETED_AT,
      updated_at: STARTED_AT,
    });
    expect(shouldApplyTurnUpdate(existing, incoming)).toBe(true);
  });

  it("never rolls back an existing completion", () => {
    const existing = turn("t", { completed_at: COMPLETED_AT, updated_at: COMPLETED_AT });
    const incoming = turn("t", { started_at: LATER_AT, updated_at: LATER_AT });
    expect(shouldApplyTurnUpdate(existing, incoming)).toBe(false);
  });
});
