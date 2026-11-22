import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { TaskWorkspaceMoveState } from "@/hooks/domains/kanban/use-task-workspace-move";
import type { WorkspaceMoveOperation } from "@/lib/kanban/task-workspace-move-operation";
import type { Task, TaskTransferRequest } from "@/lib/types/http";
import { TaskWorkspaceMoveDialog } from "./task-workspace-move-dialog";

// Covers AC-TASKS-CROSS-WORKSPACE-TRANSFER-004.2, 004.3, 004.5 and 005.3.
const SUBMIT_TEST_ID = "workspace-move-submit";
const OUTCOME_TEST_ID = "workspace-move-outcome";

const { useTaskWorkspaceMoveMock, responsiveMock } = vi.hoisted(() => ({
  useTaskWorkspaceMoveMock: vi.fn(),
  responsiveMock: { isMobile: false, isFinePointer: true },
}));
vi.mock("@/hooks/domains/kanban/use-task-workspace-move", () => ({
  useTaskWorkspaceMove: useTaskWorkspaceMoveMock,
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsiveMock,
}));

const body: TaskTransferRequest = {
  expected_source_workspace_id: "ws-a",
  expected_source_workflow_id: "wf-a",
  expected_source_workflow_step_id: "step-a",
  expected_task_updated_at: "2026-11-01T10:00:00.123456789Z",
  destination_workspace_id: "ws-b",
  destination_workflow_id: "wf-b",
  destination_workflow_step_id: "step-b",
  idempotency_key: "key-1",
};

function state(overrides: Partial<TaskWorkspaceMoveState> = {}): TaskWorkspaceMoveState {
  return {
    task: { id: "task-1", title: "Keep the thread", workspace_id: "ws-a" } as Task,
    taskStatus: "success",
    retryTask: vi.fn(),
    sourceWorkspaceName: "Personal",
    sourceWorkflowName: "Kanban",
    sourceStepName: "In progress",
    workspaces: [],
    selectedWorkspaceId: "",
    changeWorkspace: vi.fn(),
    workflows: [],
    workflowsStatus: "idle",
    retryWorkflows: vi.fn(),
    selectedWorkflowId: "",
    changeWorkflow: vi.fn(),
    steps: [],
    stepsStatus: "idle",
    retrySteps: vi.fn(),
    selectedStepId: "",
    chooseStep: vi.fn(),
    operation: null,
    restoreUnverifiable: false,
    storageUnavailable: false,
    frozen: false,
    sourceStale: false,
    canSubmit: false,
    submit: vi.fn(async () => undefined),
    check: vi.fn(async () => undefined),
    refreshSource: vi.fn(async () => undefined),
    clearResolved: vi.fn(),
    ...overrides,
  } as TaskWorkspaceMoveState;
}

function renderDialog(onOpenChange = vi.fn()) {
  render(
    <StateProvider>
      <TaskWorkspaceMoveDialog
        open
        onOpenChange={onOpenChange}
        taskId="task-1"
        workspaceId="ws-a"
      />
    </StateProvider>,
  );
  return onOpenChange;
}

function operation(status: WorkspaceMoveOperation["status"], extra = {}): WorkspaceMoveOperation {
  return { status, taskId: "task-1", body, ...extra } as WorkspaceMoveOperation;
}

function outcomeStatus(): string | null {
  return screen.getByTestId(OUTCOME_TEST_ID).getAttribute("data-status");
}

beforeEach(() => {
  responsiveMock.isMobile = false;
  responsiveMock.isFinePointer = true;
});
afterEach(cleanup);

describe("TaskWorkspaceMoveDialog", () => {
  it("shows the current placement and preservation notice, and cancels without submitting", () => {
    const hook = state();
    useTaskWorkspaceMoveMock.mockReturnValue(hook);
    const onOpenChange = renderDialog();

    expect(screen.getByTestId("workspace-move-current").textContent).toContain(
      "Personal / Kanban / In progress",
    );
    expect(screen.getByTestId("workspace-move-notice").textContent).toContain(
      "History and sessions",
    );
    expect(screen.getByTestId(SUBMIT_TEST_ID).hasAttribute("disabled")).toBe(true);
    fireEvent.click(screen.getByTestId("workspace-move-cancel"));
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(hook.submit).not.toHaveBeenCalled();
  });

  it("distinguishes empty and failed workflow catalogs and offers retry", () => {
    useTaskWorkspaceMoveMock.mockReturnValue(
      state({ selectedWorkspaceId: "ws-b", workflowsStatus: "success", workflows: [] }),
    );
    renderDialog();
    expect(screen.getByTestId("workspace-move-no-workflows")).toBeTruthy();
    cleanup();

    const hook = state({ selectedWorkspaceId: "ws-b", workflowsStatus: "error" });
    useTaskWorkspaceMoveMock.mockReturnValue(hook);
    renderDialog();
    const failure = screen.getByTestId("workspace-move-workflows-error");
    fireEvent.click(within(failure).getByRole("button"));
    expect(hook.retryWorkflows).toHaveBeenCalledOnce();
  });

  it("explains an original rejection while keeping the move available", () => {
    useTaskWorkspaceMoveMock.mockReturnValue(
      state({ operation: operation("rejected", { code: "step_incompatible" }), canSubmit: true }),
    );
    renderDialog();
    const outcome = screen.getByTestId(OUTCOME_TEST_ID);
    expect(outcome.getAttribute("role")).toBe("alert");
    expect(screen.getByTestId("workspace-move-rejection").getAttribute("data-code")).toBe(
      "step_incompatible",
    );
    expect(screen.getByTestId(SUBMIT_TEST_ID).hasAttribute("disabled")).toBe(false);
  });
});

describe("TaskWorkspaceMoveDialog outcomes", () => {
  it("keeps an uncertain or unverifiable request checkable without offering a new move", () => {
    for (const status of ["uncertain", "check-unavailable"] as const) {
      const hook = state({ operation: operation(status), frozen: true });
      useTaskWorkspaceMoveMock.mockReturnValue(hook);
      renderDialog();
      expect(outcomeStatus()).toBe(status);
      expect(screen.getByTestId(SUBMIT_TEST_ID).hasAttribute("disabled")).toBe(true);
      fireEvent.click(screen.getByTestId("workspace-move-check-result"));
      expect(hook.check).toHaveBeenCalledOnce();
      cleanup();
    }
  });

  it("names unavailable tab storage separately from an uncheckable earlier move", () => {
    for (const [overrides, status] of [
      [{ restoreUnverifiable: true }, "restore-unverifiable"],
      [{ storageUnavailable: true }, "storage-unavailable"],
    ] as const) {
      useTaskWorkspaceMoveMock.mockReturnValue(state(overrides));
      renderDialog();
      expect(outcomeStatus()).toBe(status);
      cleanup();
    }
  });

  it("shows the storage notice over a resolved rejection but keeps an unresolved one checkable", () => {
    for (const resolved of [
      operation("rejected", { code: "destination_full" }),
      operation("operation-rejected"),
    ]) {
      useTaskWorkspaceMoveMock.mockReturnValue(
        state({ operation: resolved, storageUnavailable: true, sourceStale: true }),
      );
      renderDialog();
      expect(outcomeStatus()).toBe("storage-unavailable");
      cleanup();
    }
    useTaskWorkspaceMoveMock.mockReturnValue(
      state({ operation: operation("uncertain"), frozen: true, storageUnavailable: true }),
    );
    renderDialog();
    expect(outcomeStatus()).toBe("uncertain");
    expect(screen.getByTestId("workspace-move-check-result")).toBeTruthy();
  });

  it("requires a refreshed review after a stale source or terminal rejection", () => {
    const hook = state({ operation: operation("operation-rejected"), sourceStale: true });
    useTaskWorkspaceMoveMock.mockReturnValue(hook);
    renderDialog();
    fireEvent.click(screen.getByTestId("workspace-move-refresh-task"));
    expect(hook.refreshSource).toHaveBeenCalledOnce();
  });

  it("offers Open task after a listing-initiated move", () => {
    useTaskWorkspaceMoveMock.mockReturnValue(state({ operation: operation("committed") }));
    renderDialog();
    expect(screen.getByTestId("workspace-move-open-task").getAttribute("href")).toContain("task-1");
    expect(screen.queryByTestId(SUBMIT_TEST_ID)).toBeNull();
  });

  it("uses an inset drawer with touch-sized actions on phones", () => {
    responsiveMock.isMobile = true;
    useTaskWorkspaceMoveMock.mockReturnValue(state());
    renderDialog();
    expect(screen.getByTestId("workspace-move-drawer")).toBeTruthy();
    expect(screen.queryByTestId("workspace-move-dialog")).toBeNull();
    expect(screen.getByTestId(SUBMIT_TEST_ID).className).toContain("min-h-12");
  });
});
