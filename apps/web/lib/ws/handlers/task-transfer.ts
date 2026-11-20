import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { Task } from "@/lib/types/http";
import { ApiError } from "@/lib/api/client";
import { isTaskDetailPath } from "@/lib/links";
import { fetchTask } from "@/lib/api/domains/kanban-api";
import { sidebarTaskPageCache } from "@/lib/sidebar/sidebar-task-page-cache";
import {
  admitTaskMembership,
  guardTaskMembership,
  isTaskMembershipAdmitted,
  isTaskMembershipGuarded,
  revokeTaskMembershipAdmission,
  taskMembershipAdmissionSequence,
} from "@/lib/state/task-membership-control";
import {
  bumpSidebarTaskQueryRevision,
  removeTaskFromBothKanbans,
  type TaskEventPayload,
} from "./task-archive-cache";
import { removeArchivedTaskFromCache } from "./task-status-summary";
import { redirectAwayFromRemovedTask } from "./task-lifecycle-side-effects";
import { applyTaskOverviewPatch } from "./task-overview";

/**
 * Transfer-aware workspace membership reconciliation.
 *
 * Membership of the active workspace is admitted or removed only from an
 * accepted canonical task read. Transfer notifications and ordinary task
 * events are invalidation hints: their payloads never establish membership
 * and never delete task-owned data (sessions, transcripts, drafts, queues).
 * Operation IDs, source IDs, and `updated_at` do not order placement, because
 * a task can return to its source and completed transfers keep `updated_at`.
 */

const RETRY_DELAYS_MS = [1_000, 4_000, 15_000];

/** Minimum gap between canonical reads of one task; later invalidations trail this window. */
const REFRESH_COOLDOWN_MS = 250;

/** Applies an accepted canonical task row through the ordinary update path. */
export type CanonicalTaskApplier = (store: StoreApi<AppState>, payload: TaskEventPayload) => void;

type ReconcileRecord = {
  apply: CanonicalTaskApplier;
  epoch: number;
  running?: Promise<void>;
  retries: number;
  retryTimer?: ReturnType<typeof setTimeout>;
  lastReadAt?: number;
};

type CanonicalRead = { kind: "task"; task: Task } | { kind: "unavailable" } | { kind: "transient" };

type ReadFence = {
  workspaceId: string | null;
  workspaceContextGeneration: number;
  overviewGeneration: number;
  epoch: number;
  admission: number;
};

const records = new WeakMap<object, Map<string, ReconcileRecord>>();

function recordFor(
  store: StoreApi<AppState>,
  taskId: string,
  apply: CanonicalTaskApplier,
): ReconcileRecord {
  let byTask = records.get(store);
  if (!byTask) {
    byTask = new Map();
    records.set(store, byTask);
  }
  let record = byTask.get(taskId);
  if (!record) {
    record = { apply, epoch: 0, retries: 0 };
    byTask.set(taskId, record);
  }
  return record;
}

function eventTaskId(payload: TaskEventPayload): string | undefined {
  return payload.task_id ?? payload.id;
}

function residentTask(state: AppState, taskId: string) {
  return (
    state.taskOverview?.byId[taskId] ??
    state.kanban.tasks.find((task) => task.id === taskId) ??
    Object.values(state.kanbanMulti.snapshots)
      .flatMap((snapshot) => snapshot.tasks)
      .find((task) => task.id === taskId)
  );
}

function membershipChanges(
  payload: TaskEventPayload & { source_workspace_id?: string },
  resident: { workflowId?: string | null; workspaceId?: string | null },
  activeWorkspaceId: string,
): boolean {
  if (payload.source_workspace_id) return true;
  if (payload.workspace_id && payload.workspace_id !== activeWorkspaceId) return true;
  return Boolean(
    payload.workflow_id && resident.workflowId && payload.workflow_id !== resident.workflowId,
  );
}

/**
 * Decides whether an ordinary task event may be applied directly. Events that
 * would insert a task, change its workspace/workflow membership, or touch a
 * task this store saw leave the workspace are converted into canonical reads.
 */
export function admitTaskEvent(
  store: StoreApi<AppState>,
  payload: TaskEventPayload,
  apply: CanonicalTaskApplier,
): boolean {
  const state = store.getState();
  const taskId = eventTaskId(payload);
  const activeWorkspaceId = state.workspaces?.activeId;
  if (!taskId || !activeWorkspaceId || payload.is_ephemeral) return true;
  const resident = residentTask(state, taskId);
  const foreign = Boolean(payload.workspace_id && payload.workspace_id !== activeWorkspaceId);
  const guarded = isTaskMembershipGuarded(store, activeWorkspaceId, taskId);
  // An archived row only leaves boards and refreshes archive rows already
  // listed; it never establishes active workspace membership.
  if (!guarded && !foreign && payload.archived_at) return true;
  const verified =
    resident &&
    !guarded &&
    isTaskMembershipAdmitted(store, taskId) &&
    !membershipChanges(payload, resident, activeWorkspaceId);
  if (verified) return true;
  // A non-resident explicit-foreign event carries no reconciliation work. A resident
  // one still invalidates, so a departure after a canonical return is confirmed.
  if (foreign && !resident) return false;
  void invalidateTaskMembership(store, taskId, apply);
  return false;
}

/**
 * Invalidates one task's membership and reconciles it from a canonical read.
 * Concurrent invalidations coalesce into one read; a newer invalidation
 * supersedes an in-flight result. The promise settles after reconciliation.
 */
export function invalidateTaskMembership(
  store: StoreApi<AppState>,
  taskId: string,
  apply: CanonicalTaskApplier,
  fromRetry = false,
): Promise<void> {
  const record = recordFor(store, taskId, apply);
  record.apply = apply;
  if (!fromRetry) record.retries = 0;
  record.epoch += 1;
  if (record.retryTimer) {
    clearTimeout(record.retryTimer);
    record.retryTimer = undefined;
  }
  if (!record.running) {
    record.running = reconcileUntilCurrent(store, taskId, record).finally(() => {
      record.running = undefined;
    });
  }
  return record.running;
}

/** Resolves once no canonical membership read is in flight for the task. */
export function taskMembershipSettled(store: StoreApi<AppState>, taskId: string): Promise<void> {
  return records.get(store)?.get(taskId)?.running ?? Promise.resolve();
}

/** Task ids this store has seen really deleted. Task ids are never reused. */
const deletedTaskIds = new WeakMap<StoreApi<AppState>, Set<string>>();

/**
 * Retires a deleted task. A canonical read already in flight observes the
 * retirement and applies nothing, and a read that starts later is fenced, so a
 * stale pre-delete row cannot re-admit the task after a genuine delete.
 */
export function retireTaskMembership(store: StoreApi<AppState>, taskId: string): void {
  let deleted = deletedTaskIds.get(store);
  if (!deleted) {
    deleted = new Set();
    deletedTaskIds.set(store, deleted);
  }
  deleted.add(taskId);
  const byTask = records.get(store);
  const record = byTask?.get(taskId);
  if (!byTask || !record) return;
  clearTimeout(record.retryTimer);
  byTask.delete(taskId);
}

function captureFence(store: StoreApi<AppState>, taskId: string, epoch: number): ReadFence {
  const state = store.getState();
  return {
    workspaceId: state.workspaces?.activeId ?? null,
    workspaceContextGeneration: state.workspaceContextGeneration,
    overviewGeneration: state.taskOverview.generation,
    epoch,
    admission: taskMembershipAdmissionSequence(store, taskId),
  };
}

async function awaitRefreshWindow(record: ReconcileRecord): Promise<void> {
  const elapsed = Date.now() - (record.lastReadAt ?? 0);
  const wait = Math.min(REFRESH_COOLDOWN_MS, REFRESH_COOLDOWN_MS - elapsed);
  if (wait <= 0) return;
  const pause = Promise.withResolvers<void>();
  setTimeout(pause.resolve, wait);
  await pause.promise;
}

async function reconcileUntilCurrent(
  store: StoreApi<AppState>,
  taskId: string,
  record: ReconcileRecord,
): Promise<void> {
  for (;;) {
    await awaitRefreshWindow(record);
    if (records.get(store)?.get(taskId) !== record) return;
    const fence = captureFence(store, taskId, record.epoch);
    record.lastReadAt = Date.now();
    const result = await readCanonicalTask(taskId);
    if (records.get(store)?.get(taskId) !== record) return;
    const state = store.getState();
    if (
      (state.workspaces?.activeId ?? null) !== fence.workspaceId ||
      state.workspaceContextGeneration !== fence.workspaceContextGeneration
    ) {
      return;
    }
    const superseded =
      record.epoch !== fence.epoch ||
      state.taskOverview.generation !== fence.overviewGeneration ||
      taskMembershipAdmissionSequence(store, taskId) !== fence.admission;
    if (superseded) continue;
    applyCanonicalMembership(store, taskId, result, fence.workspaceId, record);
    return;
  }
}

async function readCanonicalTask(taskId: string): Promise<CanonicalRead> {
  try {
    return { kind: "task", task: await fetchTask(taskId, { cache: "no-store" }) };
  } catch (error) {
    if (error instanceof ApiError && (error.status === 403 || error.status === 404)) {
      return { kind: "unavailable" };
    }
    return { kind: "transient" };
  }
}

function applyCanonicalMembership(
  store: StoreApi<AppState>,
  taskId: string,
  result: CanonicalRead,
  workspaceId: string | null,
  record: ReconcileRecord,
): void {
  if (!workspaceId) return;
  if (deletedTaskIds.get(store)?.has(taskId)) return;
  if (result.kind === "transient") {
    scheduleRetry(store, taskId, record);
    return;
  }
  record.retries = 0;
  if (result.kind === "task" && result.task.workspace_id === workspaceId) {
    admitTaskMembership(store, [taskId]);
    record.apply(store, canonicalPayload(store, result.task));
    return;
  }
  departWorkspace(store, taskId, workspaceId, result.kind === "task" ? result.task : null, record);
}

function canonicalPayload(store: StoreApi<AppState>, task: Task): TaskEventPayload {
  const resident = residentTask(store.getState(), task.id);
  const oldWorkflowId =
    resident?.workflowId && resident.workflowId !== task.workflow_id ? resident.workflowId : null;
  return {
    ...(task as unknown as TaskEventPayload),
    task_id: task.id,
    old_workflow_id: oldWorkflowId,
  };
}

function scheduleRetry(store: StoreApi<AppState>, taskId: string, record: ReconcileRecord): void {
  const delay = RETRY_DELAYS_MS[record.retries];
  if (delay === undefined) return;
  record.retries += 1;
  record.retryTimer = setTimeout(() => {
    record.retryTimer = undefined;
    void invalidateTaskMembership(store, taskId, record.apply, true);
  }, delay);
}

/**
 * Removes only the workspace's listing membership: boards, overview, archive
 * list, sidebar pages, and preview (a preview closes when its row leaves).
 * Sessions, transcripts, drafts, queues, walkthroughs, and recent-task
 * identity are task-owned and survive.
 */
function departWorkspace(
  store: StoreApi<AppState>,
  taskId: string,
  workspaceId: string,
  destination: Task | null,
  record: ReconcileRecord,
): void {
  guardTaskMembership(store, workspaceId, taskId);
  revokeTaskMembershipAdmission(store, taskId);
  store.setState((state) =>
    bumpSidebarTaskQueryRevision(
      removeTaskFromBothKanbans(
        removeArchivedTaskFromCache(
          applyTaskOverviewPatch(state, taskId, null, workspaceId),
          taskId,
        ),
        taskId,
      ),
      workspaceId,
    ),
  );
  sidebarTaskPageCache(store).removeTasks(new Set([taskId]));
  store.getState().setOfficeRefetchTrigger?.("tasks");
  const onDetailRoute =
    typeof window !== "undefined" && isTaskDetailPath(window.location.pathname, taskId);
  if (destination) {
    if (onDetailRoute) adoptOpenTaskWorkspace(store, destination, record.apply);
    return;
  }
  redirectAwayFromRemovedTask(taskId);
}

/**
 * An open task keeps its route and selected session; only its displayed
 * workspace context follows the canonical placement.
 */
function adoptOpenTaskWorkspace(
  store: StoreApi<AppState>,
  task: Task,
  apply: CanonicalTaskApplier,
): void {
  const { activeTaskId, activeSessionId } = store.getState().tasks;
  store.getState().setActiveWorkspace(task.workspace_id);
  store.setState((state) =>
    state.tasks.activeTaskId === activeTaskId && state.tasks.activeSessionId === activeSessionId
      ? state
      : { ...state, tasks: { ...state.tasks, activeTaskId, activeSessionId } },
  );
  admitTaskMembership(store, [task.id]);
  apply(store, { ...(task as unknown as TaskEventPayload), task_id: task.id });
}
