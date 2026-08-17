import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ListQuickTerminalTabsResponse } from "@/lib/api/domains/quick-terminal-api";
import type { ListQuickChatSessionsResponse } from "@/lib/api/domains/workspace-api";
import type { TaskSession } from "@/lib/types/http";
const apiMock = vi.hoisted(() => ({
  listQuickChatSessions: vi.fn(),
  listQuickTerminalTabs: vi.fn(),
}));

const syncMock = vi.hoisted(() => ({
  migrateStoredQuickChatNames: vi.fn(),
  toQuickChatSessions: vi.fn((sessions: unknown[]) => sessions),
  toQuickTerminalTab: vi.fn((tab: unknown) => tab),
}));

const storeApiMock = vi.hoisted(() => ({
  getState: () => mockState,
}));

type MockState = {
  connection: { status: string };
  quickChat: { syncRevisionByWorkspace: Record<string, number> };
  taskSessions: { items: Record<string, TaskSession> };
  setTaskSession: (session: TaskSession) => void;
  syncQuickChatSessions: (...args: unknown[]) => void;
  syncQuickTerminalTabs: (...args: unknown[]) => void;
};

let mockState: MockState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) => selector(mockState),
  useAppStoreApi: () => storeApiMock,
}));
vi.mock("@/lib/api/domains/workspace-api", () => ({
  listQuickChatSessions: apiMock.listQuickChatSessions,
}));
vi.mock("@/lib/api/domains/quick-terminal-api", () => ({
  listQuickTerminalTabs: apiMock.listQuickTerminalTabs,
  toQuickTerminalTab: syncMock.toQuickTerminalTab,
}));
vi.mock("@/lib/local-storage", () => ({ getStoredQuickChatNames: vi.fn(() => ({})) }));
vi.mock("@/lib/quick-chat/map-sessions", () => ({
  toQuickChatSessions: syncMock.toQuickChatSessions,
}));
vi.mock("@/lib/quick-chat/rename", () => ({
  migrateStoredQuickChatNames: syncMock.migrateStoredQuickChatNames,
}));

import { useQuickChatResync } from "./use-quick-chat-resync";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((innerResolve) => {
    resolve = innerResolve;
  });
  return { promise, resolve };
}

function taskSession(state: TaskSession["state"], updatedAt: string): TaskSession {
  // The resync boundary only reads the stable identity, state, and update timestamp.
  return {
    id: "session-1",
    task_id: "task-1",
    state,
    updated_at: updatedAt,
  } as unknown as TaskSession;
}

describe("useQuickChatResync", () => {
  beforeEach(() => {
    mockState = {
      connection: { status: "connected" },
      quickChat: { syncRevisionByWorkspace: { "workspace-1": 0 } },
      setTaskSession: vi.fn(),
      taskSessions: { items: {} },
      syncQuickChatSessions: vi.fn(),
      syncQuickTerminalTabs: vi.fn(),
    };
    vi.clearAllMocks();
  });

  afterEach(cleanup);

  it("discards a superseded response and retries the latest workspace state", async () => {
    const firstSessions = deferred<ListQuickChatSessionsResponse>();
    const firstTerminals = deferred<ListQuickTerminalTabsResponse>();
    const latestSessions = deferred<ListQuickChatSessionsResponse>();
    const latestTerminals = deferred<ListQuickTerminalTabsResponse>();
    apiMock.listQuickChatSessions
      .mockReturnValueOnce(firstSessions.promise)
      .mockReturnValueOnce(latestSessions.promise);
    apiMock.listQuickTerminalTabs
      .mockReturnValueOnce(firstTerminals.promise)
      .mockReturnValueOnce(latestTerminals.promise);

    renderHook(() => useQuickChatResync("workspace-1"));

    await act(async () => {
      mockState.quickChat.syncRevisionByWorkspace["workspace-1"] = 1;
      firstSessions.resolve({ sessions: [], task_sessions: [] });
      firstTerminals.resolve({ tabs: [] });
      await Promise.resolve();
    });

    await waitFor(() => expect(apiMock.listQuickChatSessions).toHaveBeenCalledTimes(2));

    await act(async () => {
      latestSessions.resolve({ sessions: [], task_sessions: [] });
      latestTerminals.resolve({ tabs: [] });
      await Promise.resolve();
    });

    await waitFor(() => expect(mockState.syncQuickChatSessions).toHaveBeenCalledTimes(1));
    expect(mockState.syncQuickTerminalTabs).toHaveBeenCalledTimes(1);
  });

  it("does not overwrite a newer live task session with a stale resync row", async () => {
    const stale = taskSession("RUNNING", "2099-01-01T00:00:00Z");
    mockState.taskSessions.items[stale.id] = taskSession("IDLE", "2099-01-01T00:01:00Z");
    apiMock.listQuickChatSessions.mockResolvedValueOnce({
      sessions: [],
      task_sessions: [stale],
    });
    apiMock.listQuickTerminalTabs.mockResolvedValueOnce({ tabs: [] });

    renderHook(() => useQuickChatResync("workspace-1"));

    await waitFor(() => expect(mockState.syncQuickChatSessions).toHaveBeenCalledTimes(1));

    expect(mockState.setTaskSession).not.toHaveBeenCalled();
  });
});
