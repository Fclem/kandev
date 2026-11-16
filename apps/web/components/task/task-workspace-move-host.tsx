"use client";

import { useCallback, useMemo, useSyncExternalStore } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import { TaskWorkspaceMoveDialog } from "@/components/task/task-workspace-move-dialog";
import { taskWorkspaceMoveRequests } from "@/lib/kanban/task-workspace-move-request";

/** The single owner of the workspace-move form for every task surface. */
export function TaskWorkspaceMoveHost() {
  const store = useAppStoreApi();
  const requests = taskWorkspaceMoveRequests(store);
  const subject = useSyncExternalStore(
    useCallback((listener: () => void) => requests.subscribe(listener), [requests]),
    () => requests.current(),
    () => null,
  );
  const focusReturnRef = useMemo(() => ({ current: subject?.returnFocus ?? null }), [subject]);
  return (
    <TaskWorkspaceMoveDialog
      open={subject !== null}
      onOpenChange={(open) => {
        if (!open) requests.close();
      }}
      taskId={subject?.taskId ?? null}
      workspaceId={subject?.workspaceId ?? null}
      focusReturnRef={focusReturnRef}
    />
  );
}
