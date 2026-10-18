import { act, cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

const state = vi.hoisted(() => ({
  activePanel: "changes-a",
  sendBeat: vi.fn(),
  app: {
    tasks: { activeSessionId: "session-a", activeTaskId: "task-a" },
    features: { sessionUserActivity: true },
  },
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof state.app) => unknown) => selector(state.app),
  useAppStoreApi: () => ({ getState: () => state.app }),
}));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ sendSessionUserActivityBeat: state.sendBeat }),
}));
vi.mock("@/hooks/use-panel-active", () => ({
  usePanelActive: (id: string) => state.activePanel === id,
}));
vi.mock("@/hooks/domains/session/use-session-git-refresh", () => ({
  useSessionGitRefresh: () => undefined,
}));
vi.mock("@/hooks/use-file-editors", () => ({
  useFileEditors: () => ({ openFile: vi.fn() }),
}));
vi.mock("./task-changes-panel", () => ({ TaskChangesPanel: () => null }));
vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (select: (value: object) => unknown) =>
    select({ selectedDiff: null, setSelectedDiff: vi.fn() }),
}));

import { renderPanel } from "./dockview-panel-content";

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(1000);
  state.activePanel = "changes-a";
  state.sendBeat.mockReset().mockResolvedValue({ enabled: true });
  vi.spyOn(document, "hasFocus").mockReturnValue(true);
  Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

it("AC-TASKS-USER-ACTIVITY-001.4: only the focused non-chat session panel reports effort", async () => {
  const panels = () => (
    <>
      {renderPanel("changes-a", "diff-viewer", { sessionId: "session-a" })}
      {renderPanel("changes-b", "diff-viewer", { sessionId: "session-b" })}
    </>
  );
  const view = render(panels());
  act(() => document.dispatchEvent(new Event("pointerdown")));
  await act(async () => Promise.resolve());
  expect(state.sendBeat).toHaveBeenCalledExactlyOnceWith("session-a");
  state.activePanel = "changes-b";
  view.rerender(panels());
  act(() => document.dispatchEvent(new Event("wheel")));
  await act(async () => vi.advanceTimersByTimeAsync(15_000));
  expect(state.sendBeat.mock.calls.map(([id]) => id)).toEqual([
    "session-a",
    "session-b",
    "session-b",
  ]);
});
