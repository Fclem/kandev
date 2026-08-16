import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";

const { mockClearScrollTargetForSession } = vi.hoisted(() => ({
  mockClearScrollTargetForSession: vi.fn(),
}));

const dockviewState = vi.hoisted(() => ({
  isRestoringLayout: false,
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: Object.assign(() => ({}), {
    getState: () => ({
      clearScrollTargetForSession: mockClearScrollTargetForSession,
      isRestoringLayout: dockviewState.isRestoringLayout,
      // Non-maximized so handleMaximizeExitOnLastClose early-returns without
      // touching the maximize helpers this test does not mock.
      preMaximizeLayout: null,
    }),
  }),
}));

vi.mock("@/lib/layout/panel-portal-manager", () => ({
  panelPortalManager: {
    get: vi.fn(() => undefined),
    release: vi.fn(),
  },
}));

import { setupPortalCleanup } from "./dockview-layout-setup";

type RemoveHandler = (panel: { id: string }) => void;

function makeApi() {
  const handlers: RemoveHandler[] = [];
  return {
    onDidRemovePanel: (handler: RemoveHandler) => {
      handlers.push(handler);
      return { dispose: () => {} };
    },
    panels: [] as Array<{ id: string }>,
    hasMaximizedGroup: () => false,
    fireRemoval(panel: { id: string }) {
      for (const handler of handlers) handler(panel);
    },
  };
}

function makeAppStore(activeSessionId: string): StoreApi<AppState> {
  return {
    getState: () => ({ tasks: { activeSessionId } }) as unknown as AppState,
  } as StoreApi<AppState>;
}

beforeEach(() => {
  vi.clearAllMocks();
  dockviewState.isRestoringLayout = false;
});

describe("setupPortalCleanup — scroll-target teardown", () => {
  it("clears the removed session's target when a session:<id> panel is removed", () => {
    const api = makeApi();
    setupPortalCleanup(api as never, makeAppStore("session-1"));

    api.fireRemoval({ id: "session:session-7" });

    expect(mockClearScrollTargetForSession).toHaveBeenCalledWith("session-7");
  });

  it("clears by the active session when the canonical chat panel is removed", () => {
    const api = makeApi();
    setupPortalCleanup(api as never, makeAppStore("session-9"));

    api.fireRemoval({ id: "chat" });

    expect(mockClearScrollTargetForSession).toHaveBeenCalledWith("session-9");
  });

  it("leaves targets intact when an unrelated panel is removed", () => {
    const api = makeApi();
    setupPortalCleanup(api as never, makeAppStore("session-1"));

    api.fireRemoval({ id: "files" });

    expect(mockClearScrollTargetForSession).not.toHaveBeenCalled();
  });

  it("clears the removed session only, never the other sessions' targets", () => {
    const api = makeApi();
    setupPortalCleanup(api as never, makeAppStore("session-active"));

    api.fireRemoval({ id: "session:session-a" });

    expect(mockClearScrollTargetForSession).toHaveBeenCalledTimes(1);
    expect(mockClearScrollTargetForSession).toHaveBeenCalledWith("session-a");
  });

  it("runs the clear even while a layout restore is in progress (before the restore guard)", () => {
    dockviewState.isRestoringLayout = true;
    const api = makeApi();
    setupPortalCleanup(api as never, makeAppStore("session-1"));

    api.fireRemoval({ id: "session:session-3" });
    api.fireRemoval({ id: "chat" });

    expect(mockClearScrollTargetForSession).toHaveBeenCalledWith("session-3");
    expect(mockClearScrollTargetForSession).toHaveBeenCalledWith("session-1");
  });

  it("does not clear without an active session when the canonical chat is removed", () => {
    const api = makeApi();
    setupPortalCleanup(api as never, makeAppStore(""));

    api.fireRemoval({ id: "chat" });

    expect(mockClearScrollTargetForSession).not.toHaveBeenCalled();
  });
});
