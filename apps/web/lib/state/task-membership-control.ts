import type { StoreApi } from "zustand";
import type { AppState } from "./app-state-types";

/**
 * Workspace-membership control metadata for one authenticated app store.
 *
 * - `admitted` records the task-overview generation in which an accepted
 *   canonical task or list read admitted a task. Reconnects and workspace
 *   switches advance that generation, so earlier admissions stop counting.
 * - `guards` records workspaces a task was confirmed to have left (or lost
 *   access to). Guards live for the store's authenticated lifetime: a socket
 *   replacement does not drain events already queued upstream.
 *
 * This is control metadata only, never a second copy of task rows.
 */
type TaskMembershipControl = {
  identity: string;
  admitted: Map<string, { generation: number; sequence: number }>;
  guards: Map<string, Set<string>>;
  sequence: number;
};

const controls = new WeakMap<object, TaskMembershipControl>();

function controlIdentity(state: AppState): string {
  return JSON.stringify([state.auth?.mode, state.auth?.authenticated, state.auth?.user?.id]);
}

function membershipControl(store: StoreApi<AppState>): TaskMembershipControl {
  const identity = controlIdentity(store.getState());
  let control = controls.get(store);
  if (!control || control.identity !== identity) {
    control = { identity, admitted: new Map(), guards: new Map(), sequence: 0 };
    controls.set(store, control);
  }
  return control;
}

/** Records canonical admission for tasks returned by an accepted read. */
export function admitTaskMembership(store: StoreApi<AppState>, taskIds: Iterable<string>): void {
  const control = membershipControl(store);
  const generation = store.getState().taskOverview.generation;
  for (const taskId of taskIds) {
    control.sequence += 1;
    control.admitted.set(taskId, { generation, sequence: control.sequence });
  }
}

export function revokeTaskMembershipAdmission(store: StoreApi<AppState>, taskId: string): void {
  membershipControl(store).admitted.delete(taskId);
}

export function isTaskMembershipAdmitted(store: StoreApi<AppState>, taskId: string): boolean {
  const admitted = membershipControl(store).admitted.get(taskId);
  return admitted?.generation === store.getState().taskOverview.generation;
}

/** Monotonic admission marker: a later accepted read supersedes an older one. */
export function taskMembershipAdmissionSequence(store: StoreApi<AppState>, taskId: string): number {
  return membershipControl(store).admitted.get(taskId)?.sequence ?? 0;
}

export function guardTaskMembership(
  store: StoreApi<AppState>,
  workspaceId: string,
  taskId: string,
): void {
  const guards = membershipControl(store).guards;
  const workspaces = guards.get(taskId) ?? new Set<string>();
  workspaces.add(workspaceId);
  guards.set(taskId, workspaces);
}

export function isTaskMembershipGuarded(
  store: StoreApi<AppState>,
  workspaceId: string,
  taskId: string,
): boolean {
  return membershipControl(store).guards.get(taskId)?.has(workspaceId) ?? false;
}
