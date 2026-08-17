import { describe, expect, it, vi, type Mock } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "@/lib/state/slices/session/session-slice";
import type { SessionSlice } from "@/lib/state/slices/session/types";
import type { TurnEventPayload } from "@/lib/types/backend";
import { registerTurnsHandlers } from "./turns";

const SESSION_ID = "session-1";
const TASK_ID = "task-1";
const TURN_STARTED = "session.turn.started";
const TURN_COMPLETED = "session.turn.completed";
const NOTIFICATION = "notification";
const TURN_STARTED_AT = "2026-07-23T10:00:00.000Z";
const TURN_COMPLETED_AT = "2026-07-23T10:01:00.000Z";
const WORKSPACE_ID = "workspace-1";

type TurnTestState = SessionSlice & {
  quickChat: {
    isOpen: boolean;
    activeSessionId: string | null;
    sessions: Array<{ sessionId: string; workspaceId: string }>;
  };
  markQuickChatUnseenIdle: Mock;
  recordQuickChatSettled: (sessionId: string, updatedAt: string) => boolean;
};

function makeStore(
  quickChat: TurnTestState["quickChat"] = { isOpen: false, activeSessionId: null, sessions: [] },
) {
  const settledLedger: Record<string, string> = {};
  return create<TurnTestState>()(
    immer((set) => ({
      ...(createSessionSlice as unknown as (storeSet: typeof set) => SessionSlice)(set),
      quickChat,
      markQuickChatUnseenIdle: vi.fn(),
      recordQuickChatSettled: (sessionId: string, updatedAt: string) => {
        if (!updatedAt || settledLedger[sessionId] >= updatedAt) return false;
        settledLedger[sessionId] = updatedAt;
        return true;
      },
      availableCommands: { bySessionId: {} },
    })),
  );
}

function turn(id: string, startedAt: string, completedAt?: string): TurnEventPayload {
  return {
    id,
    session_id: SESSION_ID,
    task_id: TASK_ID,
    started_at: startedAt,
    completed_at: completedAt,
    created_at: startedAt,
    updated_at: completedAt ?? startedAt,
  };
}

function send(
  store: ReturnType<typeof makeStore>,
  action: typeof TURN_STARTED | typeof TURN_COMPLETED,
  payload: TurnEventPayload,
) {
  const handler = registerTurnsHandlers(store as never)[action]!;
  handler({ type: NOTIFICATION, action, payload } as never);
}

describe("session turn WebSocket handlers", () => {
  it("keeps a completed turn inactive when its delayed started event arrives", () => {
    const store = makeStore();
    const completed = TURN_COMPLETED_AT;

    send(store, TURN_COMPLETED, turn("turn-1", TURN_STARTED_AT, completed));
    send(store, TURN_STARTED, turn("turn-1", TURN_STARTED_AT));

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeFalsy();
    expect(store.getState().turns.bySession[SESSION_ID]).toEqual([
      expect.objectContaining({ id: "turn-1", completed_at: completed }),
    ]);
  });

  it("does not clear a newer active turn when an older turn completes", () => {
    const store = makeStore();

    send(store, TURN_STARTED, turn("turn-a", TURN_STARTED_AT));
    send(store, TURN_STARTED, turn("turn-b", TURN_COMPLETED_AT));
    send(store, TURN_COMPLETED, turn("turn-a", TURN_STARTED_AT, "2026-07-23T10:02:00.000Z"));

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBe("turn-b");
  });

  it("tracks a normal turn from start through completion", () => {
    const store = makeStore();
    const completed = TURN_COMPLETED_AT;

    send(store, TURN_STARTED, turn("turn-1", TURN_STARTED_AT));
    expect(store.getState().turns.activeBySession[SESSION_ID]).toBe("turn-1");

    send(store, TURN_COMPLETED, turn("turn-1", TURN_STARTED_AT, completed));
    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeNull();
    expect(store.getState().turns.bySession[SESSION_ID][0].completed_at).toBe(completed);
  });

  it("flushes batched message updates before completing a turn", () => {
    const store = makeStore();
    const flush = vi.fn();
    const handler = registerTurnsHandlers(store as never, { flush })[TURN_COMPLETED]!;

    handler({
      id: "complete-1",
      type: NOTIFICATION,
      action: TURN_COMPLETED,
      payload: turn("turn-1", TURN_STARTED_AT, TURN_COMPLETED_AT),
    } as never);

    expect(flush).toHaveBeenCalledTimes(1);
  });

  it("marks a closed quick chat once and suppresses all dialog-open completions", () => {
    const quickChat = {
      isOpen: false,
      activeSessionId: null,
      sessions: [{ sessionId: SESSION_ID, workspaceId: WORKSPACE_ID }],
    };
    const store = makeStore(quickChat);

    send(store, TURN_COMPLETED, turn("turn-1", TURN_STARTED_AT, TURN_COMPLETED_AT));
    send(store, TURN_COMPLETED, turn("turn-1", TURN_STARTED_AT, TURN_COMPLETED_AT));

    expect(store.getState().markQuickChatUnseenIdle).toHaveBeenCalledTimes(1);
    expect(store.getState().markQuickChatUnseenIdle).toHaveBeenCalledWith(SESSION_ID, WORKSPACE_ID);

    store.setState({
      quickChat: {
        ...quickChat,
        isOpen: true,
        activeSessionId: "other-session",
        sessions: [
          ...quickChat.sessions,
          { sessionId: "other-session", workspaceId: WORKSPACE_ID },
        ],
      },
    });
    send(store, TURN_COMPLETED, turn("turn-2", TURN_STARTED_AT, TURN_COMPLETED_AT));

    expect(store.getState().markQuickChatUnseenIdle).toHaveBeenCalledTimes(1);
  });

  it("does not re-mark an older replay after the settled ledger recorded the completion", () => {
    const store = makeStore({
      isOpen: false,
      activeSessionId: null,
      sessions: [{ sessionId: SESSION_ID, workspaceId: WORKSPACE_ID }],
    });

    send(store, TURN_COMPLETED, turn("turn-1", TURN_STARTED_AT, TURN_COMPLETED_AT));
    // A delayed replay of an older completion arrives with a fresh turn id.
    send(store, TURN_COMPLETED, turn("turn-2", TURN_STARTED_AT, TURN_COMPLETED_AT));

    expect(store.getState().markQuickChatUnseenIdle).toHaveBeenCalledTimes(1);
  });

  it("suppresses a completion in another workspace while the dialog is open", () => {
    const store = makeStore({
      isOpen: true,
      activeSessionId: "session-a",
      sessions: [
        { sessionId: "session-a", workspaceId: "workspace-a" },
        { sessionId: SESSION_ID, workspaceId: "workspace-b" },
      ],
    });

    send(store, TURN_COMPLETED, turn("turn-1", TURN_STARTED_AT, TURN_COMPLETED_AT));

    expect(store.getState().markQuickChatUnseenIdle).not.toHaveBeenCalled();
  });
});
