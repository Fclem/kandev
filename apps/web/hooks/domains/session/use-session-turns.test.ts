import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Turn } from "@/lib/types/http";

const mockListSessionTurns = vi.fn();
const state = {
  tasks: { activeSessionId: "session-a" as string | null },
  turns: {
    bySession: { "session-a": [] as Turn[], "session-b": [] as Turn[] } as Record<string, Turn[]>,
    hydratedBySession: {} as Record<string, boolean>,
  },
  replaceSessionTurns: vi.fn((sessionId: string, turns: Turn[]) => {
    state.turns.bySession[sessionId] = turns;
    state.turns.hydratedBySession[sessionId] = true;
  }),
};

vi.mock("@/lib/api/domains/session-api", () => ({
  listSessionTurns: (...args: unknown[]) => mockListSessionTurns(...args),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (store: typeof state) => unknown) => selector(state),
  useAppStoreApi: () => ({ getState: () => state }),
}));

import { useSessionTurns } from "./use-session-turns";

function turn(id: string): Turn {
  return {
    id,
    session_id: "session-a" as Turn["session_id"],
    task_id: "task-1" as Turn["task_id"],
    started_at: "2026-01-01T00:00:00.000Z",
    created_at: "2026-01-01T00:00:00.000Z",
    updated_at: "2026-01-01T00:00:00.000Z",
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  state.tasks.activeSessionId = "session-a";
  state.turns.bySession = { "session-a": [], "session-b": [] };
  state.turns.hydratedBySession = {};
  mockListSessionTurns.mockResolvedValue({ turns: [turn("turn-a")], total: 1 });
});

describe("useSessionTurns", () => {
  it("fetches when the marker is absent even if a live partial turn exists", async () => {
    state.turns.bySession["session-a"] = [turn("live")];
    const { result } = renderHook(() => useSessionTurns("session-a"));

    await waitFor(() =>
      expect(state.replaceSessionTurns).toHaveBeenCalledWith("session-a", [turn("turn-a")]),
    );

    expect(result.current).toEqual([turn("live")]);
  });

  it("does not refetch a hydrated session", () => {
    state.turns.hydratedBySession["session-a"] = true;
    renderHook(() => useSessionTurns("session-a"));

    expect(mockListSessionTurns).not.toHaveBeenCalled();
  });

  it("discards a delayed response after active session changes", async () => {
    let resolveA!: (value: { turns: Turn[]; total: number }) => void;
    mockListSessionTurns.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveA = resolve;
      }),
    );
    const { rerender } = renderHook(({ sessionId }) => useSessionTurns(sessionId), {
      initialProps: { sessionId: "session-a" },
    });

    await waitFor(() => expect(mockListSessionTurns).toHaveBeenCalledWith("session-a"));
    state.tasks.activeSessionId = "session-b";
    rerender({ sessionId: "session-b" });
    resolveA({ turns: [turn("stale")], total: 1 });

    await act(async () => {});
    expect(state.replaceSessionTurns).not.toHaveBeenCalledWith("session-a", [turn("stale")]);
  });
});
