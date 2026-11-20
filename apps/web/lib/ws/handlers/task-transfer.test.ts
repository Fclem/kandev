import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { defaultKanbanState } from "@/lib/state/slices/kanban/kanban-slice";
import type { TaskOverview } from "@/lib/state/slices/task-overview-types";
import type { TaskSession } from "@/lib/types/http";
import { ApiError } from "@/lib/api/client";
import { reconcileTaskMembership, registerTasksHandlers } from "./tasks";
import { taskMembershipSettled } from "./task-transfer";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "./types";

// Covers AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.5 / 004.4 / 005.4: transfer
// notifications change workspace membership, never task-owned data, and
// membership is admitted only from accepted canonical reads.

const fetchTask = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/kanban-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/kanban-api")>()),
  fetchTask,
}));

const SOURCE = "ws-a";
const DESTINATION = "ws-b";
const TASK = "task-1";
const SESSION = "session-1";
const OPERATION = "op-1";
const UPDATED_AT = "2026-11-01T10:00:00.123456789Z";
const RETURNED_STEP = "step-a-returned";
const TASK_UPDATED = "task.updated" as const;
const TASK_STATE_CHANGED = "task.state_changed" as const;

type CanonicalTask = {
  id: string;
  workspace_id: string;
  workflow_id: string;
  workflow_step_id: string;
  title: string;
  state: string;
  updated_at: string;
  position: number;
};

function canonical(fields: Partial<CanonicalTask> = {}): CanonicalTask {
  return {
    id: TASK,
    workspace_id: SOURCE,
    workflow_id: "wf-a",
    workflow_step_id: "step-a",
    title: "Transferable",
    state: "COMPLETED",
    updated_at: UPDATED_AT,
    position: 0,
    ...fields,
  };
}

function overview(fields: Partial<TaskOverview> = {}): TaskOverview {
  return {
    id: TASK,
    workspaceId: SOURCE,
    workflowId: "wf-a",
    workflowStepId: "step-a",
    title: "Transferable",
    state: "COMPLETED",
    position: 0,
    updatedAt: UPDATED_AT,
    ...fields,
  } as TaskOverview;
}

function sourceStore() {
  const store = createAppStore({
    workspaces: { items: [], activeId: SOURCE },
    kanban: { workflowId: "wf-a", steps: [], tasks: [overview()] },
    kanbanMulti: {
      ...defaultKanbanState.kanbanMulti,
      snapshots: {
        "wf-a": { workflowId: "wf-a", workflowName: "Kanban", steps: [], tasks: [overview()] },
      },
    },
  });
  store.setState((state) => ({
    ...state,
    connection: { ...state.connection, status: "connected" },
    taskSessionsByTask: {
      ...state.taskSessionsByTask,
      itemsByTaskId: { [TASK]: [{ id: SESSION, task_id: TASK } as unknown as TaskSession] },
    },
    tasks: { ...state.tasks, activeTaskId: null, activeSessionId: null },
  }));
  return { store, handlers: registerTasksHandlers(store) };
}

type Handlers = WsHandlers;
type Store = StoreApi<AppState>;

function deliver(handlers: Handlers, action: keyof Handlers, payload: Record<string, unknown>) {
  const handler = handlers[action] as (message: unknown) => void;
  handler({ id: "m", type: "notification", action, payload });
}

function resident(store: Store): boolean {
  const state = store.getState();
  return (
    state.kanban.tasks.some((task) => task.id === TASK) ||
    Boolean(state.kanbanMulti.snapshots["wf-a"]?.tasks.some((task) => task.id === TASK)) ||
    Boolean(state.taskOverview.byId[TASK])
  );
}

function sessionsKept(store: Store): boolean {
  return store.getState().taskSessionsByTask.itemsByTaskId[TASK]?.[0]?.id === SESSION;
}

function departureMarker(handlers: Handlers) {
  deliver(handlers, "task.deleted", {
    task_id: TASK,
    workspace_id: SOURCE,
    workflow_id: "wf-a",
    transfer_operation_id: OPERATION,
  });
}

function oldSourceEvent(
  handlers: Handlers,
  action: typeof TASK_UPDATED | typeof TASK_STATE_CHANGED,
) {
  deliver(handlers, action, {
    task_id: TASK,
    workspace_id: SOURCE,
    workflow_id: "wf-a",
    workflow_step_id: "step-a-stale",
    title: "Transferable",
    state: "COMPLETED",
    updated_at: UPDATED_AT,
  });
}

function reconnect(store: Store) {
  for (const status of ["disconnected", "connected"] as const) {
    store.setState((state) => ({ ...state, connection: { ...state.connection, status } }));
  }
}

beforeEach(() => {
  fetchTask.mockReset();
});

describe("transfer departure marker", () => {
  it("removes source membership only after a canonical read confirms departure", async () => {
    const { store, handlers } = sourceStore();
    const read = Promise.withResolvers<CanonicalTask>();
    fetchTask.mockReturnValueOnce(read.promise);

    departureMarker(handlers);
    expect(resident(store)).toBe(true);
    expect(sessionsKept(store)).toBe(true);

    read.resolve(canonical({ workspace_id: DESTINATION, workflow_id: "wf-b" }));
    await vi.waitFor(() => expect(resident(store)).toBe(false));
    expect(sessionsKept(store)).toBe(true);
  });

  it("keeps the row through a transient read failure and departs on the retry", async () => {
    vi.useFakeTimers();
    try {
      const { store, handlers } = sourceStore();
      fetchTask
        .mockRejectedValueOnce(new ApiError("unavailable", 503, null))
        .mockResolvedValue(canonical({ workspace_id: DESTINATION, workflow_id: "wf-b" }));

      departureMarker(handlers);
      await vi.advanceTimersByTimeAsync(300);
      await taskMembershipSettled(store, TASK);
      expect(resident(store)).toBe(true);

      await vi.advanceTimersByTimeAsync(1_000);
      await taskMembershipSettled(store, TASK);
      expect(fetchTask).toHaveBeenCalledTimes(2);
      expect(resident(store)).toBe(false);
      expect(sessionsKept(store)).toBe(true);
    } finally {
      vi.useRealTimers();
    }
  });

  it("gives a fresh invalidation its own retry after the earlier budget was spent", async () => {
    vi.useFakeTimers();
    try {
      const { store, handlers } = sourceStore();
      fetchTask.mockRejectedValue(new ApiError("unavailable", 503, null));
      departureMarker(handlers);
      await vi.advanceTimersByTimeAsync(600_000);
      await taskMembershipSettled(store, TASK);
      expect(resident(store)).toBe(true);
      expect(fetchTask).toHaveBeenCalledTimes(4);

      fetchTask.mockReset();
      fetchTask
        .mockRejectedValueOnce(new ApiError("unavailable", 503, null))
        .mockResolvedValue(canonical({ workspace_id: DESTINATION, workflow_id: "wf-b" }));
      departureMarker(handlers);
      await vi.advanceTimersByTimeAsync(1_300);
      await taskMembershipSettled(store, TASK);
      expect(fetchTask).toHaveBeenCalledTimes(2);
      expect(resident(store)).toBe(false);
    } finally {
      vi.useRealTimers();
    }
  });

  it("removes the row when access is definitively unavailable without deleting task data", async () => {
    const { store, handlers } = sourceStore();
    fetchTask.mockRejectedValue(new ApiError("not found", 404, null));

    departureMarker(handlers);
    await vi.waitFor(() => expect(resident(store)).toBe(false));
    expect(sessionsKept(store)).toBe(true);
  });

  it("keeps a returned completed task when an old or duplicated marker arrives", async () => {
    const { store, handlers } = sourceStore();
    // A -> B -> A already happened; updated_at is unchanged for completed tasks.
    fetchTask.mockResolvedValue(canonical());

    departureMarker(handlers);
    departureMarker(handlers);
    await vi.waitFor(() => expect(fetchTask).toHaveBeenCalled());
    await taskMembershipSettled(store, TASK);
    expect(resident(store)).toBe(true);
  });

  it("does not let a reconciliation result superseded by a newer invalidation erase membership", async () => {
    const { store, handlers } = sourceStore();
    const stale = Promise.withResolvers<CanonicalTask>();
    fetchTask
      .mockReturnValueOnce(stale.promise)
      .mockResolvedValueOnce(canonical({ workflow_step_id: RETURNED_STEP }));

    departureMarker(handlers);
    await vi.waitFor(() => expect(fetchTask).toHaveBeenCalledTimes(1));
    // A newer source event invalidates while the departure read is in flight.
    oldSourceEvent(handlers, TASK_UPDATED);
    stale.resolve(canonical({ workspace_id: DESTINATION, workflow_id: "wf-b" }));

    await vi.waitFor(() => expect(fetchTask).toHaveBeenCalledTimes(2));
    await vi.waitFor(() =>
      expect(store.getState().taskOverview.byId[TASK]?.workflowStepId).toBe(RETURNED_STEP),
    );
    expect(resident(store)).toBe(true);
  });

  it("keeps the normal cleanup for a real deletion", () => {
    const { store, handlers } = sourceStore();
    deliver(handlers, "task.deleted", { task_id: TASK, workspace_id: SOURCE, workflow_id: "wf-a" });
    expect(resident(store)).toBe(false);
    expect(store.getState().taskSessionsByTask.itemsByTaskId[TASK] ?? []).toHaveLength(0);
    expect(fetchTask).not.toHaveBeenCalled();
  });
});

describe("untagged source events after a confirmed departure", () => {
  for (const action of [TASK_UPDATED, TASK_STATE_CHANGED] as const) {
    it(`does not resurrect the departed task from a delayed ${action}`, async () => {
      const { store, handlers } = sourceStore();
      fetchTask.mockResolvedValue(canonical({ workspace_id: DESTINATION, workflow_id: "wf-b" }));
      departureMarker(handlers);
      await vi.waitFor(() => expect(resident(store)).toBe(false));

      oldSourceEvent(handlers, action);
      expect(resident(store)).toBe(false);
      await vi.waitFor(() => expect(fetchTask.mock.calls.length).toBeGreaterThanOrEqual(2));
      await taskMembershipSettled(store, TASK);
      expect(resident(store)).toBe(false);
    });
  }

  it("delivers the old event before the marker without leaving a resurrected row", async () => {
    const { store, handlers } = sourceStore();
    fetchTask.mockResolvedValue(canonical({ workspace_id: DESTINATION, workflow_id: "wf-b" }));
    await reconcileTaskMembership(store, TASK);
    expect(resident(store)).toBe(false);
    oldSourceEvent(handlers, TASK_STATE_CHANGED);
    departureMarker(handlers);
    await taskMembershipSettled(store, TASK);
    await vi.waitFor(() => expect(fetchTask.mock.calls.length).toBeGreaterThanOrEqual(2));
    expect(resident(store)).toBe(false);
  });

  it("admits a genuine canonical return and never replays an older payload afterwards", async () => {
    const { store, handlers } = sourceStore();
    fetchTask.mockResolvedValueOnce(canonical({ workspace_id: DESTINATION, workflow_id: "wf-b" }));
    departureMarker(handlers);
    await vi.waitFor(() => expect(resident(store)).toBe(false));

    // The task genuinely returns; an ordinary event is the discovery hint.
    fetchTask.mockResolvedValue(canonical({ workflow_step_id: RETURNED_STEP }));
    oldSourceEvent(handlers, TASK_UPDATED);
    await vi.waitFor(() =>
      expect(store.getState().taskOverview.byId[TASK]?.workflowStepId).toBe(RETURNED_STEP),
    );

    // An old event released after the return invalidates but is never applied.
    oldSourceEvent(handlers, TASK_STATE_CHANGED);
    await vi.waitFor(() => expect(fetchTask).toHaveBeenCalledTimes(3));
    await taskMembershipSettled(store, TASK);
    expect(store.getState().taskOverview.byId[TASK]?.workflowStepId).toBe(RETURNED_STEP);
  });

  it("keeps the departure guard across a reconnect of the same store", async () => {
    const { store, handlers } = sourceStore();
    fetchTask.mockResolvedValue(canonical({ workspace_id: DESTINATION, workflow_id: "wf-b" }));
    departureMarker(handlers);
    await vi.waitFor(() => expect(resident(store)).toBe(false));

    reconnect(store);
    oldSourceEvent(handlers, TASK_UPDATED);
    expect(resident(store)).toBe(false);
    await vi.waitFor(() => expect(fetchTask).toHaveBeenCalledTimes(2));
    await taskMembershipSettled(store, TASK);
    expect(resident(store)).toBe(false);
  });
});

describe("membership admission without any transfer notification", () => {
  it("requires canonical admission for a resident event after reconnect", async () => {
    const { store, handlers } = sourceStore();
    fetchTask.mockResolvedValue(canonical());
    await reconcileTaskMembership(store, TASK);
    // Transfer happens while disconnected; no marker reaches this store.
    reconnect(store);
    fetchTask.mockResolvedValue(canonical({ workspace_id: DESTINATION, workflow_id: "wf-b" }));

    oldSourceEvent(handlers, TASK_STATE_CHANGED);
    expect(store.getState().taskOverview.byId[TASK]?.workflowStepId).toBe("step-a");
    await vi.waitFor(() => expect(resident(store)).toBe(false));
  });

  it("never inserts an unknown event candidate without a canonical read", async () => {
    const { store, handlers } = sourceStore();
    store.setState((state) => ({
      ...state,
      kanban: { ...state.kanban, tasks: [] },
      kanbanMulti: { ...state.kanbanMulti, snapshots: {} },
    }));
    expect(resident(store)).toBe(false);
    fetchTask.mockResolvedValue(canonical({ workspace_id: DESTINATION, workflow_id: "wf-b" }));

    oldSourceEvent(handlers, TASK_UPDATED);
    expect(resident(store)).toBe(false);
    await vi.waitFor(() => expect(fetchTask).toHaveBeenCalledTimes(1));
    await taskMembershipSettled(store, TASK);
    expect(resident(store)).toBe(false);
  });

  it("admits an ordinary new task through the same canonical read", async () => {
    const { store, handlers } = sourceStore();
    fetchTask.mockResolvedValue(canonical({ id: "task-new", title: "New" }));

    deliver(handlers, "task.created", {
      task_id: "task-new",
      workspace_id: SOURCE,
      workflow_id: "wf-a",
      workflow_step_id: "step-a",
      title: "New",
    });
    await vi.waitFor(() =>
      expect(store.getState().kanban.tasks.map((task) => task.id)).toContain("task-new"),
    );
  });

  it("does not treat a partial sidebar page omission as a departure", async () => {
    const { store, handlers } = sourceStore();
    fetchTask.mockResolvedValue(canonical());
    await reconcileTaskMembership(store, TASK);
    // A filtered page read that does not contain the task is accepted.
    const readId = store.getState().beginTaskOverviewRead();
    store.getState().retainTaskOverviews("sidebar:filtered", [], readId);
    store.getState().finishTaskOverviewRead(readId, []);

    expect(resident(store)).toBe(true);
    deliver(handlers, TASK_STATE_CHANGED, {
      task_id: TASK,
      workspace_id: SOURCE,
      workflow_id: "wf-a",
      workflow_step_id: "step-a-next",
      updated_at: "2026-11-01T10:00:01Z",
    });
    expect(store.getState().taskOverview.byId[TASK]?.workflowStepId).toBe("step-a-next");
  });
});

describe("foreign events never establish membership", () => {
  it("does not insert an unknown foreign task.created on the active board", async () => {
    const { store, handlers } = sourceStore();
    store.setState((state) => ({ ...state, taskOverview: { ...state.taskOverview, byId: {} } }));
    store.setState((state) => ({ ...state, kanban: { ...state.kanban, tasks: [] } }));
    fetchTask.mockResolvedValue(canonical({ workspace_id: DESTINATION, workflow_id: "wf-b" }));

    deliver(handlers, "task.created", {
      task_id: TASK,
      workspace_id: DESTINATION,
      workflow_id: "wf-b",
      workflow_step_id: "step-b",
      title: "Foreign",
      state: "TODO",
      updated_at: UPDATED_AT,
    });
    await taskMembershipSettled(store, TASK);

    expect(fetchTask).not.toHaveBeenCalled();
    expect(resident(store)).toBe(false);
  });

  it("ignores a departed task's old source event on the current socket", async () => {
    const { store, handlers } = sourceStore();
    fetchTask.mockResolvedValue(canonical({ workspace_id: DESTINATION, workflow_id: "wf-b" }));

    departureMarker(handlers);
    await taskMembershipSettled(store, TASK);
    expect(resident(store)).toBe(false);

    fetchTask.mockClear();
    oldSourceEvent(handlers, TASK_UPDATED);
    await taskMembershipSettled(store, TASK);

    expect(resident(store)).toBe(false);
  });

  it("does not re-read the canonical task for repeated foreign events after departure", async () => {
    const { store, handlers } = sourceStore();
    fetchTask.mockResolvedValue(canonical({ workspace_id: DESTINATION, workflow_id: "wf-b" }));

    departureMarker(handlers);
    await taskMembershipSettled(store, TASK);
    expect(resident(store)).toBe(false);
    const reads = fetchTask.mock.calls.length;

    for (let i = 0; i < 3; i++) {
      deliver(handlers, TASK_UPDATED, {
        task_id: TASK,
        workspace_id: DESTINATION,
        workflow_id: "wf-b",
        workflow_step_id: "step-b",
        title: "Foreign",
        state: "TODO",
        updated_at: UPDATED_AT,
      });
    }
    await taskMembershipSettled(store, TASK);

    expect(fetchTask.mock.calls.length).toBe(reads);
    expect(resident(store)).toBe(false);
  });
});

describe("departure guard persists across canonical return", () => {
  it("removes the task again when it returns and then departs to a third workspace", async () => {
    const { store, handlers } = sourceStore();
    fetchTask.mockResolvedValueOnce(canonical({ workspace_id: DESTINATION, workflow_id: "wf-b" }));
    departureMarker(handlers);
    await taskMembershipSettled(store, TASK);
    expect(resident(store)).toBe(false);

    fetchTask.mockResolvedValueOnce(canonical({ workspace_id: SOURCE }));
    oldSourceEvent(handlers, TASK_UPDATED);
    await taskMembershipSettled(store, TASK);
    expect(resident(store)).toBe(true);

    fetchTask.mockResolvedValueOnce(canonical({ workspace_id: "ws-c", workflow_id: "wf-c" }));
    deliver(handlers, TASK_UPDATED, {
      task_id: TASK,
      workspace_id: "ws-c",
      workflow_id: "wf-c",
      workflow_step_id: "step-c",
      title: "Moved",
      state: "TODO",
      updated_at: UPDATED_AT,
    });
    await taskMembershipSettled(store, TASK);

    expect(resident(store)).toBe(false);
  });

  it("keeps the row when the canonical read for a third-workspace event is transient", async () => {
    vi.useFakeTimers();
    try {
      const { store, handlers } = sourceStore();
      const settle = async () => {
        await vi.advanceTimersByTimeAsync(300);
        await taskMembershipSettled(store, TASK);
      };
      fetchTask.mockResolvedValueOnce(
        canonical({ workspace_id: DESTINATION, workflow_id: "wf-b" }),
      );
      departureMarker(handlers);
      await settle();

      fetchTask.mockResolvedValueOnce(canonical({ workspace_id: SOURCE }));
      oldSourceEvent(handlers, TASK_UPDATED);
      await settle();
      expect(resident(store)).toBe(true);

      fetchTask.mockRejectedValueOnce(new Error("503"));
      deliver(handlers, TASK_UPDATED, {
        task_id: TASK,
        workspace_id: "ws-c",
        workflow_id: "wf-c",
        workflow_step_id: "step-c",
        title: "Moved",
        state: "TODO",
        updated_at: UPDATED_AT,
      });
      await settle();

      expect(resident(store)).toBe(true);
      vi.clearAllTimers();
    } finally {
      vi.useRealTimers();
    }
  });
});

describe("reads for a returned resident task stay bounded", () => {
  it("bounds canonical reads for a stream of untagged events inside one window", async () => {
    vi.useFakeTimers();
    try {
      const { store, handlers } = sourceStore();
      fetchTask.mockResolvedValueOnce(
        canonical({ workspace_id: DESTINATION, workflow_id: "wf-b" }),
      );
      departureMarker(handlers);
      await taskMembershipSettled(store, TASK);

      fetchTask.mockResolvedValue(canonical({ workspace_id: SOURCE }));
      oldSourceEvent(handlers, TASK_UPDATED);
      await vi.advanceTimersByTimeAsync(300);
      await taskMembershipSettled(store, TASK);
      expect(resident(store)).toBe(true);
      const reads = fetchTask.mock.calls.length;

      // Five events 20ms apart all fall inside the refresh window opened by the read above.
      for (let i = 0; i < 5; i++) {
        await vi.advanceTimersByTimeAsync(20);
        oldSourceEvent(handlers, TASK_UPDATED);
      }
      await vi.advanceTimersByTimeAsync(300);
      await taskMembershipSettled(store, TASK);

      expect(fetchTask.mock.calls.length - reads).toBeLessThanOrEqual(2);
      expect(resident(store)).toBe(true);
    } finally {
      vi.useRealTimers();
    }
  });
});

describe("departure adoption follows the detail route, not a preview", () => {
  afterEach(() => window.history.pushState({}, "", "/"));

  it("keeps the listing workspace when only a preview of the departed task is open", async () => {
    const { store, handlers } = sourceStore();
    store.setState((state) => ({
      ...state,
      tasks: { ...state.tasks, activeTaskId: TASK, activeSessionId: SESSION },
    }));
    fetchTask.mockResolvedValue(canonical({ workspace_id: DESTINATION, workflow_id: "wf-b" }));
    departureMarker(handlers);
    await taskMembershipSettled(store, TASK);
    expect(store.getState().workspaces.activeId).toBe(SOURCE);
  });

  it("adopts the destination workspace when the departed task detail route is open", async () => {
    window.history.pushState({}, "", `/t/${TASK}`);
    const { store, handlers } = sourceStore();
    store.setState((state) => ({
      ...state,
      tasks: { ...state.tasks, activeTaskId: TASK, activeSessionId: SESSION },
    }));
    fetchTask.mockResolvedValue(canonical({ workspace_id: DESTINATION, workflow_id: "wf-b" }));
    departureMarker(handlers);
    await taskMembershipSettled(store, TASK);
    expect(store.getState().workspaces.activeId).toBe(DESTINATION);
    expect(store.getState().tasks.activeTaskId).toBe(TASK);
    expect(store.getState().tasks.activeSessionId).toBe(SESSION);
  });
});

describe("transfer departure read across a delete", () => {
  it("does not resurrect a task deleted while its departure read is in flight", async () => {
    const { store, handlers } = sourceStore();
    const read = Promise.withResolvers<CanonicalTask>();
    fetchTask.mockReturnValueOnce(read.promise);

    departureMarker(handlers);
    expect(resident(store)).toBe(true);
    await vi.waitFor(() => expect(fetchTask).toHaveBeenCalledTimes(1));
    const settled = taskMembershipSettled(store, TASK);
    deliver(handlers, "task.deleted", { task_id: TASK, workspace_id: SOURCE, workflow_id: "wf-a" });
    expect(resident(store)).toBe(false);

    read.resolve(canonical({ workspace_id: SOURCE, workflow_id: "wf-a" }));
    await settled;
    expect(resident(store)).toBe(false);
    expect(fetchTask).toHaveBeenCalledTimes(1);
  });
});
