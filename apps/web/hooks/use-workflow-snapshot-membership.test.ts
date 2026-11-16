import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { useStore } from "zustand";
import { createAppStore, type AppState } from "@/lib/state/store";
import type { TaskOverview } from "@/lib/state/slices/task-overview-types";
import { taskMembershipSettled } from "@/lib/ws/handlers/task-transfer";
import { useWorkflowSnapshot } from "./use-workflow-snapshot";

// Covers AC-TASKS-CROSS-WORKSPACE-TRANSFER-005.4: absence from a recovery
// listing is a reason to confirm membership, not proof of departure.

const api = vi.hoisted(() => ({ fetchWorkflowSnapshot: vi.fn(), fetchTask: vi.fn() }));
vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api")>()),
  fetchWorkflowSnapshot: api.fetchWorkflowSnapshot,
}));
vi.mock("@/lib/api/domains/kanban-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/kanban-api")>()),
  fetchTask: api.fetchTask,
}));

let store: ReturnType<typeof createAppStore>;
const MOVED_TASK = "task-moved";
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: AppState) => unknown) => useStore(store, selector),
  useAppStoreApi: () => store,
}));

const row = (workflowId: string) =>
  ({
    id: MOVED_TASK,
    workspaceId: "ws",
    workflowId,
    workflowStepId: `${workflowId}-step`,
    title: "Moved while offline",
    position: 0,
    updatedAt: "2026-11-01T10:00:00Z",
  }) as TaskOverview;

// The boot-hydrated board is reused first; the recovery listing follows a reconnect.
function reconnect() {
  for (const status of ["disconnected", "connected"] as const) {
    act(() => {
      store.setState((state) => ({ ...state, connection: { ...state.connection, status } }));
    });
  }
}

beforeEach(() => {
  api.fetchWorkflowSnapshot.mockReset();
  api.fetchTask.mockReset();
  store = createAppStore({
    workspaces: { items: [], activeId: "ws" },
    kanban: { workflowId: "wf-1", steps: [], tasks: [row("wf-1")] },
  });
  store.setState((state) => ({
    ...state,
    connection: { ...state.connection, status: "connected" },
  }));
});

it("confirms a resident omitted from a complete workflow listing instead of dropping it", async () => {
  api.fetchWorkflowSnapshot.mockResolvedValue({
    workflow: { id: "wf-1", workspace_id: "ws", name: "One" },
    steps: [],
    tasks: [],
  });
  api.fetchTask.mockResolvedValue({
    id: MOVED_TASK,
    workspace_id: "ws",
    workflow_id: "wf-2",
    workflow_step_id: "wf-2-step",
    title: "Moved while offline",
    updated_at: "2026-11-01T10:00:01Z",
  });

  renderHook(() => useWorkflowSnapshot("wf-1"));
  reconnect();
  await waitFor(() => expect(api.fetchTask).toHaveBeenCalledWith(MOVED_TASK, expect.anything()));
  await taskMembershipSettled(store, MOVED_TASK);

  expect(store.getState().kanban.tasks).toEqual([]);
  expect(store.getState().taskOverview.byId[MOVED_TASK]?.workflowId).toBe("wf-2");
});

it("removes an omitted resident only after its departure is confirmed", async () => {
  api.fetchWorkflowSnapshot.mockResolvedValue({
    workflow: { id: "wf-1", workspace_id: "ws", name: "One" },
    steps: [],
    tasks: [],
  });
  api.fetchTask.mockResolvedValue({
    id: MOVED_TASK,
    workspace_id: "ws-other",
    workflow_id: "wf-other",
    workflow_step_id: "other-step",
    title: "Moved while offline",
  });

  renderHook(() => useWorkflowSnapshot("wf-1"));
  reconnect();
  await waitFor(() => expect(api.fetchTask).toHaveBeenCalled());
  await taskMembershipSettled(store, MOVED_TASK);
  expect(store.getState().taskOverview.byId[MOVED_TASK]).toBeUndefined();
});
