import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TaskItemWithContextMenu } from "./task-switcher-context-menu";
import type { TaskSwitcherItem } from "./task-switcher-types";
import { readMoveInstructions, setMoveInstructions } from "./workflow-move-instructions-test-helpers";

const WORKFLOW_1_ID = "workflow-1";
const WORKFLOW_1_NAME = "Workflow 1";
const MOVE_TO_TEST_ID = "task-context-move-to";
const STEP_2_TEST_ID = "task-context-step-step-2";
const INSTRUCTIONS_TEST_ID = "workflow-move-instructions";
const SUBMIT_TEST_ID = "workflow-move-submit";
const workflowMoveMock = vi.hoisted(() => ({
  move: vi.fn(),
  isMoving: false,
}));
const touchDrawerMock = vi.hoisted(() => ({ enabled: false }));

vi.mock("@/hooks/domains/kanban/use-workflow-move", () => ({
  useWorkflowMove: () => workflowMoveMock,
}));
vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => touchDrawerMock.enabled,
}));

afterEach(() => {
  cleanup();
});

beforeEach(() => {
  workflowMoveMock.move.mockReset();
  workflowMoveMock.move.mockResolvedValue({ disposition: "committed", response: {} });
  touchDrawerMock.enabled = false;
});

function task(overrides: Partial<TaskSwitcherItem> = {}): TaskSwitcherItem {
  return { id: "task-1", title: "Task 1", state: "IN_PROGRESS", ...overrides };
}

function renderWorkflowMoveMenu({
  selectedTaskIds,
  onMoveToStep,
  onBulkMove,
}: {
  selectedTaskIds?: Set<string>;
  onMoveToStep?: (taskId: string, workflowId: string, targetStepId: string) => void;
  onBulkMove?: (taskIds: string[], targetWorkflowId: string, targetStepId: string) => void;
} = {}) {
  const bulkMove = onBulkMove ?? vi.fn();
  render(
    <StateProvider>
      <ToastProvider>
        <TooltipProvider>
          <TaskItemWithContextMenu
            task={task({
              workflowId: WORKFLOW_1_ID,
              workflowStepId: "step-1",
            })}
            workflows={[{ id: WORKFLOW_1_ID, name: WORKFLOW_1_NAME }]}
            stepsByWorkflowId={{
              [WORKFLOW_1_ID]: [
                { id: "step-1", title: "Todo" },
                { id: "step-2", title: "Review" },
              ],
            }}
            selectedTaskIds={selectedTaskIds}
            onMoveToStep={onMoveToStep}
            onBulkMove={bulkMove}
          >
            <div data-testid="task-row">Task 1</div>
          </TaskItemWithContextMenu>
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
  return { bulkMove };
}

/** One task's step moves live under Move to > Workflow step. */
async function hoverSingleTaskStepList() {
  fireEvent.pointerMove(screen.getByTestId(MOVE_TO_TEST_ID), { pointerType: "mouse" });
  fireEvent.pointerMove(await screen.findByTestId("task-context-move-step"), {
    pointerType: "mouse",
  });
}

async function openMoveSubmenu() {
  fireEvent.contextMenu(screen.getByTestId("task-row"));
  await screen.findByTestId(MOVE_TO_TEST_ID);
  await hoverSingleTaskStepList();
  const targetStep = await screen.findByTestId(STEP_2_TEST_ID);
  fireEvent.pointerMove(targetStep, {
    pointerType: "mouse",
  });
}

describe("TaskItemWithContextMenu — single-task move options", () => {
  it("keeps the existing direct move as a one-selection action", async () => {
    const onMoveToStep = vi.fn();
    renderWorkflowMoveMenu({ onMoveToStep });

    fireEvent.contextMenu(screen.getByTestId("task-row"));
    await hoverSingleTaskStepList();
    fireEvent.click(await screen.findByTestId(STEP_2_TEST_ID));

    expect(onMoveToStep).toHaveBeenCalledOnce();
    expect(onMoveToStep).toHaveBeenCalledWith("task-1", WORKFLOW_1_ID, "step-2");
    expect(workflowMoveMock.move).not.toHaveBeenCalled();
  });

  it("submits one-shot options through the single-task move payload", async () => {
    renderWorkflowMoveMenu();
    await openMoveSubmenu();

    const instructions = await screen.findByTestId(INSTRUCTIONS_TEST_ID);
    setMoveInstructions(instructions, "  start review with tests  ");
    fireEvent.click(screen.getByTestId(SUBMIT_TEST_ID));

    await waitFor(() =>
      expect(workflowMoveMock.move).toHaveBeenCalledWith("task-1", {
        workflow_id: WORKFLOW_1_ID,
        workflow_step_id: "step-2",
        position: 0,
        entry_options: { instructions: "start review with tests" },
      }),
    );
  });

  it("keeps desktop one-shot options when the move is rejected", async () => {
    workflowMoveMock.move.mockResolvedValueOnce({
      disposition: "failed",
      error: new Error("move rejected"),
      response: {},
    });
    renderWorkflowMoveMenu();
    await openMoveSubmenu();

    const instructions = await screen.findByTestId(INSTRUCTIONS_TEST_ID);
    setMoveInstructions(instructions, "  retry review later  ");
    fireEvent.click(screen.getByTestId(SUBMIT_TEST_ID));

    await waitFor(() => expect(workflowMoveMock.move).toHaveBeenCalledOnce());
    expect(readMoveInstructions(instructions)).toBe("  retry review later  ");
  });

  it("exposes inline move options under a one-task selection step", async () => {
    renderWorkflowMoveMenu({ selectedTaskIds: new Set(["task-1"]) });

    fireEvent.contextMenu(screen.getByTestId("task-row"));
    await screen.findByTestId(MOVE_TO_TEST_ID);

    expect(screen.queryByTestId("task-context-move-with-options")).toBeNull();
    await hoverSingleTaskStepList();
    fireEvent.pointerMove(await screen.findByTestId(STEP_2_TEST_ID), {
      pointerType: "mouse",
    });
    expect(await screen.findByTestId("workflow-move-skip-step-prompt")).toBeTruthy();
    expect(screen.getByTestId(INSTRUCTIONS_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(SUBMIT_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId("task-context-step-options-step-2")).toBeNull();
  });
});

describe("TaskItemWithContextMenu — single-task move options (workspace and bulk)", () => {
  it("offers workspace transfer for a one-task selection in a one-step workflow", async () => {
    render(
      <StateProvider>
        <ToastProvider>
          <TooltipProvider>
            <TaskItemWithContextMenu
              task={task({
                workflowId: WORKFLOW_1_ID,
                workflowStepId: "step-1",
                workspaceId: "workspace-1",
              })}
              workflows={[{ id: WORKFLOW_1_ID, name: WORKFLOW_1_NAME }]}
              stepsByWorkflowId={{ [WORKFLOW_1_ID]: [{ id: "step-1", title: "Todo" }] }}
              selectedTaskIds={new Set(["task-1"])}
            >
              <div data-testid="task-row">Task 1</div>
            </TaskItemWithContextMenu>
          </TooltipProvider>
        </ToastProvider>
      </StateProvider>,
    );

    fireEvent.contextMenu(screen.getByTestId("task-row"));
    fireEvent.pointerMove(await screen.findByTestId(MOVE_TO_TEST_ID), { pointerType: "mouse" });
    expect(await screen.findByTestId("task-context-change-workspace")).toBeTruthy();
  });

  it("does not expose an options submenu for bulk selection moves", async () => {
    const { bulkMove } = renderWorkflowMoveMenu({
      selectedTaskIds: new Set(["task-1", "task-2"]),
    });
    fireEvent.contextMenu(screen.getByTestId("task-row"));
    await screen.findByTestId(MOVE_TO_TEST_ID);

    expect(screen.queryByTestId("task-context-move-with-options")).toBeNull();
    expect(screen.getByTestId(MOVE_TO_TEST_ID)).toBeTruthy();
    expect(workflowMoveMock.move).not.toHaveBeenCalled();

    fireEvent.pointerMove(screen.getByTestId(MOVE_TO_TEST_ID), {
      pointerType: "mouse",
    });
    fireEvent.click(await screen.findByTestId(STEP_2_TEST_ID));
    expect(bulkMove).toHaveBeenCalledWith(["task-1", "task-2"], WORKFLOW_1_ID, "step-2");
  });

  it("uses the touch Drawer presentation for the shared form", async () => {
    touchDrawerMock.enabled = true;
    renderWorkflowMoveMenu();
    await openMoveSubmenu();

    // Coarse/touch pointers keep the "Move with options" item that opens the
    // shared form in a Drawer instead of rendering it inline.
    fireEvent.click(await screen.findByTestId("task-context-step-options-step-2"));
    expect(await screen.findByTestId("workflow-move-options")).toBeTruthy();
  });
});
