import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { useStore, type StoreApi } from "zustand";
import { createAppStore, type AppState } from "@/lib/state/store";
import { agentProfileId, sessionId, taskId } from "@/lib/types/http";
import { sessionCatalogRecordSchema } from "@/lib/types/session-catalog";

const fetchTaskMock = vi.hoisted(() => vi.fn());
const catalogLifecycleRequestMock = vi.fn();
let mockStore: StoreApi<AppState>;

vi.mock("@/components/task/chat/use-composer-prompt-suggestion", () => ({
  useComposerPromptSuggestion: () => ({ promptSuggestion: null }),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: AppState) => unknown) => useStore(mockStore, selector),
  useAppStoreApi: () => mockStore,
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/components/github/pr-status-chip", () => ({
  PRStatusChip: () => null,
}));

vi.mock("@/components/gitlab/mr-status-chip", () => ({
  MRStatusChip: () => null,
}));

vi.mock("@/components/integrations/registered-change-request-status", () => ({
  RegisteredChangeRequestStatus: () => null,
}));

vi.mock("@/components/task/share/share-button", () => ({
  ShareButton: () => null,
  shareableSessionStateClient: () => false,
}));

vi.mock("@/components/task/chat/chat-input-container", () => ({
  ChatInputContainer: () => <textarea aria-label="Draft" />,
}));

vi.mock("@/lib/api/domains/kanban-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/kanban-api")>()),
  fetchTask: fetchTaskMock,
}));

vi.mock("@/components/task/chat/queued-ghost-list", () => ({
  QueueAffordance: ({
    children,
    renderStatusBar,
  }: {
    children: ReactNode;
    renderStatusBar?: (queueChip: ReactNode) => ReactNode;
  }) => (
    <>
      {renderStatusBar?.(null)}
      {children}
    </>
  ),
}));

vi.mock("./composer-agent-start-hint", () => ({
  ComposerAgentStartHint: () => null,
}));

vi.mock("./dynamic-route-recovery", () => ({
  DynamicRouteRecovery: () => null,
}));

vi.mock("./chat-status-bar", () => ({
  ComposerCIStatus: () => null,
  ChatStatusBar: () => null,
  resolveStatusRowTaskId: (sessionTaskId: string | null, statusTaskId: string | null) =>
    sessionTaskId ?? statusTaskId,
}));

vi.mock("@/components/task/chat/todo-indicator", () => ({
  TodoIndicator: () => null,
}));

vi.mock("./pr-archive-banners", () => ({
  PRMergedBanner: () => null,
  PRClosedBanner: () => null,
}));

vi.mock("@/hooks/use-keyboard-shortcut", () => ({
  useKeyboardShortcut: vi.fn(),
}));

vi.mock("@/hooks/use-message-handler", () => ({
  buildTaskMentionsContext: vi.fn(),
  useMessageHandler: () => ({ handleSendMessage: vi.fn().mockResolvedValue(undefined) }),
}));

vi.mock("@/hooks/domains/kanban/use-plan-actions", () => ({
  usePlanActions: () => ({
    implementPlanHandler: vi.fn(),
    proceedStepName: null,
    proceed: vi.fn(),
    isMoving: false,
  }),
}));

vi.mock("@/hooks/domains/session/use-executor-environment-availability", () => ({
  useExecutorEnvironmentAvailability: () => ({
    unavailable: false,
    status: null,
  }),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ send: vi.fn(), request: catalogLifecycleRequestMock }),
}));

import { ChatInputArea } from "./chat-input-area";

beforeEach(() => {
  mockStore = createAppStore();
  fetchTaskMock.mockReset().mockResolvedValue({ workspace_id: "task-workspace" });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function composerPanelState() {
  return {
    resolvedSessionId: "session-1",
    taskId: "task-1",
    sessionModel: null,
    activeModel: null,
    isAgentBusy: false,
    activeDocument: null,
    planComments: [],
    previewFeedback: [],
    pendingPRFeedback: [],
    walkthroughComments: [],
    messageComments: [],
    contextFiles: [],
    prompts: [],
    markCommentsSent: vi.fn(),
    clearSessionPlanComments: vi.fn(),
    handleClearPRFeedback: vi.fn(),
    handleClearWalkthroughComments: vi.fn(),
    clearEphemeral: vi.fn(),
    consumeSubmittedEphemeral: vi.fn(),
    addContextFile: vi.fn(),
    planModeEnabled: false,
    planCommentMigration: {
      status: "complete",
      pendingCount: 0,
      failure: null,
      needsAttention: false,
      isReady: true,
      isBlocking: false,
      retry: vi.fn(),
    },
    hasPendingClarification: true,
    session: { state: "WAITING_FOR_INPUT", pending_action: "clarification" },
    task: { id: "task-1", title: "Task" },
    taskDescription: "Task description",
    needsRecovery: false,
    planModeAvailable: true,
    mcpServers: [],
    mcpAttachmentHistory: [],
    supportsSteering: false,
    isStarting: false,
    isQueueReady: true,
    inputMode: "direct",
    isPreparingEnvironment: false,
    isFailed: false,
    isCompleted: false,
    agentCommands: [],
    todoItems: [],
    pendingCommentsByFile: {},
    handleToggleContextFile: vi.fn(),
    handleAddContextFile: vi.fn(),
    contextItems: [],
    planContextEnabled: false,
    handlePlanModeChange: vi.fn(),
  } as never;
}

function renderComposer() {
  return render(
    <ChatInputArea
      chatInputRef={{ current: null }}
      clarificationKey={0}
      onClarificationResolved={vi.fn()}
      handleSubmit={vi.fn()}
      handleCancelTurn={vi.fn().mockResolvedValue(undefined)}
      showRequestChangesTooltip={false}
      panelState={composerPanelState()}
      isSending={false}
    />,
  );
}

it("exposes refresh outside model hydration and toolbar visibility without touching an unsent draft", () => {
  mockStore.getState().upsertTaskSessionFromEvent("task-1", {
    id: sessionId("session-1"),
    task_id: taskId("task-1"),
    state: "WAITING_FOR_INPUT",
    agent_profile_id: agentProfileId("profile"),
    started_at: "2026-11-16T00:00:00Z",
    updated_at: "2026-11-16T00:00:00Z",
  });
  mockStore.getState().applySessionCatalogRefresh(
    "task-1",
    "session-1",
    sessionCatalogRecordSchema.parse({
      version: 1,
      revision: 1,
      profile_id: "profile",
      disposition: "rediscovery_required",
      reason: "remote_executor",
      owner: { execution_id: "execution", attempt_id: "attempt", source_generation: 1 },
    }),
  );
  renderComposer();
  const editor = screen.getByRole("textbox", { name: "Draft" });
  fireEvent.change(editor, { target: { value: "Unsent work" } });
  const action = screen.getByRole("button", { name: "Refresh session" });
  expect(action.getAttribute("disabled")).toBeNull();
  expect(screen.getByRole("textbox", { name: "Draft" })).toBe(editor);
  expect(editor instanceof HTMLTextAreaElement && editor.value).toBe("Unsent work");
  expect(catalogLifecycleRequestMock).not.toHaveBeenCalled();
});
