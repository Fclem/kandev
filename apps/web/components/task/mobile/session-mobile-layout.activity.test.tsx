import { act, cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

const state = vi.hoisted(() => ({
  sendBeat: vi.fn(),
  layout: {
    activeTaskId: "task-a",
    effectiveSessionId: "session-a",
    isPassthroughMode: false,
    selectedDiff: null,
    handleClearSelectedDiff: vi.fn(),
    totalChangesCount: 0,
    hasUnseenPlanUpdate: false,
    showApproveButton: false,
    handleApprove: vi.fn(),
    currentMobilePanel: "changes",
    handlePanelChange: vi.fn(),
    isTaskSwitcherOpen: false,
    handleMenuClick: vi.fn(),
  },
  app: {
    features: { sessionUserActivity: true },
    workflowSessionFocus: { request: null },
    acknowledgeWorkflowSessionFocus: vi.fn(),
    mobileSession: { reviewItemIdBySessionId: {} },
    setMobileSessionReview: vi.fn(),
    taskSessions: { items: {} },
  },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof state.app) => unknown) => selector(state.app),
  useAppStoreApi: () => ({ getState: () => state.app }),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ sendSessionUserActivityBeat: state.sendBeat }),
}));

vi.mock("@/hooks/use-session-layout-state", () => ({
  useSessionLayoutState: () => state.layout,
}));

vi.mock("@/hooks/use-visual-viewport-offset", () => ({
  useVisualViewportOffset: () => ({ keyboardOpen: false, bottomOffset: 0 }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/components/app-status-bar/app-status-surface-provider", () => ({
  useAppStatusDrawer: () => ({ enabled: false, issueSeverity: null, openStatusDrawer: vi.fn() }),
}));

vi.mock("../review-panel-provider", () => ({
  useNormalizedTaskReviewsState: () => ({ reviews: [], loading: false }),
}));

vi.mock("../review-selection", () => ({
  reviewItemId: () => "",
  useReviewItemSelection: () => ({ selectedReview: null, selectReview: vi.fn() }),
}));

vi.mock("../task-chat-panel", () => ({ TaskChatPanel: () => null }));
vi.mock("../task-plan-panel", () => ({ TaskPlanPanel: () => null }));
vi.mock("../task-files-panel", () => ({ TaskFilesPanel: () => null }));
vi.mock("../passthrough-toolbar", () => ({ PassthroughToolbar: () => null }));
vi.mock("../plugin-task-panel", () => ({ PluginTaskPanel: () => null }));
vi.mock("../launch-queue-status", () => ({ LaunchQueueStatus: () => null }));
vi.mock("../wip-queue-status", () => ({ WipQueueStatus: () => null }));
vi.mock("../file-browser-hooks", () => ({ fetchAndOpenFile: vi.fn() }));
vi.mock("./mobile-changes-panel", () => ({ MobileChangesPanel: () => null }));
vi.mock("./mobile-file-viewer-panel", () => ({ MobileFileViewerPanel: () => null }));
vi.mock("./mobile-review-panel", () => ({ MobileReviewPanel: () => null }));
vi.mock("./mobile-terminal-keybar", () => ({
  MobileTerminalKeybar: () => null,
  KEYBAR_HEIGHT_PX: 0,
}));
vi.mock("./mobile-terminal-pane", () => ({ MobileTerminalPane: () => null }));
vi.mock("./mobile-sessions-section", () => ({ MobileSessionsPicker: () => null }));
vi.mock("./session-mobile-top-bar", () => ({ SessionMobileTopBar: () => null }));
vi.mock("./session-mobile-bottom-nav", () => ({ SessionMobileBottomNav: () => null }));
vi.mock("./session-mobile-review-dialog", () => ({ SessionMobileReviewDialog: () => null }));
vi.mock("@/hooks/domains/task/use-task-status-summary", () => ({
  useTaskStatusSummary: () => null,
}));
vi.mock("@kandev/ui/pannel-session", () => ({ SessionPanelContent: () => null }));

import { SessionMobileLayout } from "./session-mobile-layout";

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(1000);
  state.sendBeat.mockReset().mockResolvedValue({ enabled: true });
  state.layout.currentMobilePanel = "changes";
  state.layout.isTaskSwitcherOpen = false;
  vi.spyOn(document, "hasFocus").mockReturnValue(true);
  Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

// @covers AC-TASKS-USER-ACTIVITY-001.4
it("reports the session while a non-chat mobile panel is shown", () => {
  render(<SessionMobileLayout workspaceId="workspace-a" workflowId="workflow-a" />);
  act(() => document.dispatchEvent(new Event("pointerdown")));
  expect(state.sendBeat).toHaveBeenCalledExactlyOnceWith("session-a");
});

// @covers AC-TASKS-USER-ACTIVITY-001.4
it("pauses reporting while the task switcher covers the session", () => {
  state.layout.isTaskSwitcherOpen = true;
  render(<SessionMobileLayout workspaceId="workspace-a" workflowId="workflow-a" />);
  act(() => document.dispatchEvent(new Event("pointerdown")));
  expect(state.sendBeat).not.toHaveBeenCalled();
});
