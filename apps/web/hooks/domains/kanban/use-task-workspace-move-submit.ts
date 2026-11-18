"use client";

import { useCallback, useSyncExternalStore } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { transferTask } from "@/lib/api";
import {
  settleWorkspaceMove,
  workspaceMoveOperations,
  type WorkspaceMoveOperation,
} from "@/lib/kanban/task-workspace-move-operation";
import type { TaskTransferRequest } from "@/lib/types/http";
import { reconcileTaskMembership } from "@/lib/ws/handlers/tasks";

function useAuthIdentity(): string {
  return useAppStore((state) =>
    JSON.stringify([state.auth?.mode, state.auth?.authenticated, state.auth?.user?.id]),
  );
}

/**
 * Owns one task's workspace-move operation for the page lifetime. Submission
 * admission is synchronous (a second click while a request is outstanding is
 * refused), and an unresolved operation survives closing and reopening the
 * form. Only a committed receipt or an authoritative `operation_rejected`
 * check resolves it.
 */
export function useTaskWorkspaceMoveOperation(taskId: string | null) {
  const store = useAppStoreApi();
  const identity = useAuthIdentity();
  const registry = workspaceMoveOperations(store);
  const operation = useSyncExternalStore(
    useCallback((listener: () => void) => registry.subscribe(listener), [registry]),
    () => (taskId ? registry.get(identity, taskId) : null),
    () => null,
  );
  const unverifiable = useSyncExternalStore(
    useCallback((listener: () => void) => registry.subscribe(listener), [registry]),
    () => (taskId ? registry.isUnverifiable(identity, taskId) : false),
    () => false,
  );

  const run = useCallback(
    async (attempt: "submit" | "check", body: TaskTransferRequest): Promise<void> => {
      if (!taskId) return;
      const pending = registry.begin(identity, taskId, body, attempt);
      if (!pending) return;
      let settled: WorkspaceMoveOperation;
      try {
        settled = settleWorkspaceMove(pending, await transferTask(taskId, body));
      } catch (error) {
        settled = settleWorkspaceMove(pending, { error });
      }
      // Durable settlement comes first: a caller switch during the reconciliation
      // read must not discard an accepted receipt.
      registry.settle(identity, pending, settled);
      if (settled.status === "committed") {
        // Canonical placement is reconciled before success is announced. A
        // failed reconciliation read leaves the operation committed.
        await reconcileTaskMembership(store, taskId).catch(() => undefined);
      }
    },
    [identity, registry, store, taskId],
  );

  const submit = useCallback((body: TaskTransferRequest) => run("submit", body), [run]);
  const check = useCallback(() => {
    if (!taskId) return Promise.resolve();
    const current = registry.get(identity, taskId);
    return current ? run("check", current.body) : Promise.resolve();
  }, [identity, registry, run, taskId]);
  const clearResolved = useCallback(() => {
    if (taskId) registry.clearResolved(identity, taskId);
  }, [identity, registry, taskId]);

  return { operation, unverifiable, submit, check, clearResolved };
}
