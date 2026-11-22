"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { fetchWorkflowSnapshot } from "@/lib/api";
import {
  isUnresolvedWorkspaceMove,
  type WorkspaceMoveOperation,
} from "@/lib/kanban/task-workspace-move-operation";
import type {
  Task,
  TaskTransferRequest,
  WorkflowSnapshot,
  WorkflowStepDTO,
} from "@/lib/types/http";
import { generateUUID } from "@/lib/utils";
import {
  useChangeWorkflowCatalog,
  useChangeWorkflowDestination,
  useChangeWorkflowTask,
} from "./use-change-workflow-data";
import { useTaskWorkspaceMoveOperation } from "./use-task-workspace-move-submit";

type Args = {
  open: boolean;
  taskId: string | null;
  /** The task's workspace when the form was opened; the captured subject. */
  workspaceId: string | null;
};

const NO_STEPS: WorkflowStepDTO[] = [];

function useSourceSnapshot(workflowId: string | null | undefined) {
  const [snapshot, setSnapshot] = useState<WorkflowSnapshot | null>(null);
  useEffect(() => {
    if (!workflowId) return;
    let active = true;
    void fetchWorkflowSnapshot(workflowId, { cache: "no-store" }).then(
      (result) => active && setSnapshot(result),
      () => active && setSnapshot(null),
    );
    return () => {
      active = false;
    };
  }, [workflowId]);
  // A snapshot is shown only for the workflow it was loaded for, so a reused form
  // never names the previous task's workflow while the next one loads or fails.
  return snapshot?.workflow.id === workflowId ? snapshot : null;
}

function orderedSteps(snapshot: WorkflowSnapshot | null): WorkflowStepDTO[] {
  if (!snapshot) return NO_STEPS;
  return [...snapshot.steps].sort(
    (left, right) => left.position - right.position || left.id.localeCompare(right.id),
  );
}

/** Restores the frozen choices of an unresolved operation into the selectors. */
function useFrozenChoices(
  open: boolean,
  operation: WorkspaceMoveOperation | null,
  setWorkspace: (workspaceId: string) => void,
  destination: ReturnType<typeof useChangeWorkflowDestination>,
) {
  const restoredRef = useRef<string | null>(null);
  const body = isUnresolvedWorkspaceMove(operation) ? operation?.body : undefined;
  const { changeWorkflow, chooseStep } = destination;
  useEffect(() => {
    if (!open) {
      restoredRef.current = null;
      return;
    }
    if (!body || restoredRef.current === body.idempotency_key) return;
    restoredRef.current = body.idempotency_key;
    setWorkspace(body.destination_workspace_id);
    changeWorkflow(body.destination_workflow_id);
    chooseStep(body.destination_workflow_step_id);
  }, [open, body, setWorkspace, changeWorkflow, chooseStep]);
}

/**
 * A stale source or a durably barred operation needs a reviewed placement
 * before another submission (which always receives a fresh key). Closing the
 * form forgets a resolved outcome; an unresolved operation and its immutable
 * request stay with the task for the page lifetime.
 */
function useOperationLifecycle(
  open: boolean,
  operation: WorkspaceMoveOperation | null,
  clearResolved: () => void,
  setSourceStale: (stale: boolean) => void,
) {
  useEffect(() => {
    const stale =
      operation?.status === "operation-rejected" ||
      (operation?.status === "rejected" && operation.code === "source_changed");
    if (stale) setSourceStale(true);
  }, [operation, setSourceStale]);
  useEffect(() => () => clearResolved(), [clearResolved]);
  useEffect(() => {
    if (!open) clearResolved();
  }, [open, clearResolved]);
}

function canSubmitWorkspaceMove(args: {
  taskReady: boolean;
  blocked: boolean;
  selectedWorkspaceId: string;
  catalogReady: boolean;
  destination: ReturnType<typeof useChangeWorkflowDestination>;
  steps: WorkflowStepDTO[];
}): boolean {
  const { destination } = args;
  if (!args.taskReady || args.blocked || !args.selectedWorkspaceId || !args.catalogReady)
    return false;
  if (!destination.selectedWorkflowId || destination.status !== "success") return false;
  return args.steps.some((step) => step.id === destination.selectedStepId);
}

/** Binds the exact source placement and generation string with a fresh key. */
function transferRequest(
  task: Task,
  workspaceId: string,
  workflowId: string,
  stepId: string,
): TaskTransferRequest {
  return {
    expected_source_workspace_id: task.workspace_id,
    expected_source_workflow_id: task.workflow_id,
    expected_source_workflow_step_id: task.workflow_step_id,
    expected_task_updated_at: task.updated_at,
    destination_workspace_id: workspaceId,
    destination_workflow_id: workflowId,
    destination_workflow_step_id: stepId,
    idempotency_key: generateUUID(),
  };
}

function sourcePlacementNames(
  task: Task | null,
  workspaceName: string | undefined,
  snapshot: WorkflowSnapshot | null,
) {
  return {
    sourceWorkspaceName: workspaceName ?? null,
    sourceWorkflowName: snapshot?.workflow.name ?? null,
    sourceStepName:
      snapshot?.steps.find((step) => step.id === task?.workflow_step_id)?.name ?? null,
  };
}

/**
 * Domain state for moving one task to another workspace: explicit
 * workspace -> workflow -> step choice, catalog loading, and the immutable
 * submission/check lifecycle. Browsing never changes the active workspace and
 * the browser never evaluates step equivalence; the server decides.
 */
export function useTaskWorkspaceMove({ open, taskId, workspaceId }: Args) {
  const taskData = useChangeWorkflowTask({ open, taskId, workspaceId });
  const task = taskData.task;
  const sourceSnapshot = useSourceSnapshot(open ? task?.workflow_id : null);
  const workspaces = useAppStore((state) => state.workspaces.items);
  const [selectedWorkspaceId, setSelectedWorkspaceId] = useState("");
  const catalog = useChangeWorkflowCatalog(open, selectedWorkspaceId || null);
  const destination = useChangeWorkflowDestination(open);
  const { operation, unverifiable, storageUnavailable, submit, check, clearResolved } =
    useTaskWorkspaceMoveOperation(taskId);
  const frozen = isUnresolvedWorkspaceMove(operation);
  const [sourceStale, setSourceStale] = useState(false);

  useEffect(() => {
    if (!open) return;
    setSelectedWorkspaceId("");
    setSourceStale(false);
  }, [open]);
  useFrozenChoices(open, operation, setSelectedWorkspaceId, destination);
  useOperationLifecycle(open, operation, clearResolved, setSourceStale);
  const { changeWorkflow } = destination;
  const changeWorkspace = useCallback(
    (nextWorkspaceId: string) => {
      setSelectedWorkspaceId(nextWorkspaceId);
      changeWorkflow("");
    },
    [changeWorkflow],
  );

  const steps = orderedSteps(destination.snapshot);
  const sourceWorkspace = workspaces.find((item) => item.id === task?.workspace_id);
  const destinations = useMemo(
    () => workspaces.filter((item) => item.id !== (task?.workspace_id ?? workspaceId)),
    [workspaces, task?.workspace_id, workspaceId],
  );
  const canSubmit = canSubmitWorkspaceMove({
    taskReady: Boolean(task) && taskData.status === "success",
    blocked: frozen || sourceStale || unverifiable || storageUnavailable,
    selectedWorkspaceId,
    catalogReady: catalog.status === "success",
    destination,
    steps,
  });

  const submitMove = useCallback(() => {
    if (!task || !canSubmit) return Promise.resolve();
    return submit(
      transferRequest(
        task,
        selectedWorkspaceId,
        destination.selectedWorkflowId,
        destination.selectedStepId,
      ),
    );
  }, [
    canSubmit,
    destination.selectedStepId,
    destination.selectedWorkflowId,
    selectedWorkspaceId,
    submit,
    task,
  ]);

  const { refreshTask } = taskData;
  const refreshSource = useCallback(async () => {
    const latest = await refreshTask();
    if (!latest) return;
    setSourceStale(false);
    clearResolved();
  }, [clearResolved, refreshTask]);

  return {
    task,
    taskStatus: taskData.status,
    retryTask: refreshSource,
    ...sourcePlacementNames(task, sourceWorkspace?.name, sourceSnapshot),
    workspaces: destinations,
    selectedWorkspaceId,
    changeWorkspace,
    workflows: catalog.workflows,
    workflowsStatus: catalog.status,
    retryWorkflows: catalog.retry,
    selectedWorkflowId: destination.selectedWorkflowId,
    changeWorkflow: destination.changeWorkflow,
    steps,
    stepsStatus: destination.status,
    retrySteps: destination.retry,
    selectedStepId: destination.selectedStepId,
    chooseStep: destination.chooseStep,
    operation,
    /** A prior operation from before a reload cannot be checked from this browser. */
    restoreUnverifiable: unverifiable,
    /** Tab storage is unavailable, so no move can be recorded durably from this browser. */
    storageUnavailable,
    frozen,
    sourceStale,
    canSubmit,
    submit: submitMove,
    check,
    refreshSource,
    clearResolved,
  };
}

export type TaskWorkspaceMoveState = ReturnType<typeof useTaskWorkspaceMove>;
