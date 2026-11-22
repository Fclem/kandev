import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useStore } from "zustand";
import { ApiError } from "@/lib/api/client";
import { createAppStore, type AppState } from "@/lib/state/store";
import type { Task, TaskTransferReceipt, TaskTransferRequest } from "@/lib/types/http";
import { useTaskWorkspaceMove } from "./use-task-workspace-move";
import { useTaskWorkspaceMoveOperation } from "./use-task-workspace-move-submit";

// Covers AC-TASKS-CROSS-WORKSPACE-TRANSFER-004.2 and 005.3.

const api = vi.hoisted(() => ({
  fetchTask: vi.fn(),
  listWorkflows: vi.fn(),
  fetchWorkflowSnapshot: vi.fn(),
  transferTask: vi.fn(),
}));
vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api")>()),
  ...api,
}));
const reconcile = vi.hoisted(() => vi.fn(async () => undefined));
vi.mock("@/lib/ws/handlers/tasks", () => ({ reconcileTaskMembership: reconcile }));

let store: ReturnType<typeof createAppStore>;
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: AppState) => unknown) => useStore(store, selector),
  useAppStoreApi: () => store,
}));

const networkLost = () => new TypeError("network lost");
const GENERATION = "2026-11-01T10:00:00.123456789Z";
const task = {
  id: "task-1",
  workspace_id: "ws-a",
  workflow_id: "wf-a",
  workflow_step_id: "step-a",
  title: "Move me",
  updated_at: GENERATION,
} as Task;
const receipt = { operation_id: "op-1", task_id: "task-1" } as TaskTransferReceipt;

function workspace(id: string, name: string) {
  return { id, name, owner_id: "owner", created_at: "", updated_at: "" };
}

function conflict(code: string) {
  return new ApiError("conflict", 409, { error: "task transfer conflict", code });
}

function snapshotFor(workflowId: string) {
  return {
    workflow: { id: workflowId, name: `${workflowId} name`, workspace_id: "ws" },
    steps: [
      { id: `${workflowId}-later`, name: "Later", position: 2, color: "" },
      { id: `${workflowId}-first`, name: "First", position: 0, color: "" },
    ],
    tasks: [],
  };
}

function render() {
  return renderHook(
    ({ open }: { open: boolean }) =>
      useTaskWorkspaceMove({ open, taskId: "task-1", workspaceId: "ws-a" }),
    { initialProps: { open: true } },
  );
}

type Hook = ReturnType<typeof render>;

async function choose(hook: Hook, workspaceId = "ws-b", workflowId = "wf-b") {
  await waitFor(() => expect(hook.result.current.taskStatus).toBe("success"));
  act(() => hook.result.current.changeWorkspace(workspaceId));
  await waitFor(() => expect(hook.result.current.workflowsStatus).toBe("success"));
  act(() => hook.result.current.changeWorkflow(workflowId));
  await waitFor(() => expect(hook.result.current.stepsStatus).toBe("success"));
  act(() => hook.result.current.chooseStep(`${workflowId}-first`));
}

function sentBodies(): TaskTransferRequest[] {
  return api.transferTask.mock.calls.map((call) => call[1] as TaskTransferRequest);
}

beforeEach(() => {
  vi.clearAllMocks();
  window.sessionStorage.clear();
  store = createAppStore({
    workspaces: {
      activeId: "ws-a",
      items: [
        workspace("ws-a", "Personal"),
        workspace("ws-b", "Product"),
        workspace("ws-c", "Ops"),
      ],
    },
  });
  api.fetchTask.mockResolvedValue(task);
  api.fetchWorkflowSnapshot.mockImplementation(async (id: string) => snapshotFor(id));
  api.listWorkflows.mockImplementation(async (workspaceId: string) => ({
    workflows: [{ id: `wf-${workspaceId.slice(3)}`, workspace_id: workspaceId, name: "Flow" }],
  }));
});

describe("destination choices", () => {
  it("orders steps, resets dependent choices, and never switches the active workspace", async () => {
    const hook = render();
    await choose(hook);
    expect(hook.result.current.steps.map((step) => step.id)).toEqual(["wf-b-first", "wf-b-later"]);
    expect(hook.result.current.canSubmit).toBe(true);
    expect(hook.result.current.workspaces.map((item) => item.id)).toEqual(["ws-b", "ws-c"]);

    act(() => hook.result.current.changeWorkspace("ws-c"));
    expect(hook.result.current.selectedWorkflowId).toBe("");
    expect(hook.result.current.selectedStepId).toBe("");
    expect(hook.result.current.canSubmit).toBe(false);
    expect(store.getState().workspaces.activeId).toBe("ws-a");
  });

  it("ignores a late workflow catalog for a previously selected workspace", async () => {
    const late = Promise.withResolvers<{ workflows: unknown[] }>();
    api.listWorkflows.mockImplementation(async (workspaceId: string) =>
      workspaceId === "ws-b"
        ? late.promise
        : { workflows: [{ id: "wf-c", workspace_id: "ws-c", name: "Ops flow" }] },
    );
    const hook = render();
    await waitFor(() => expect(hook.result.current.taskStatus).toBe("success"));
    act(() => hook.result.current.changeWorkspace("ws-b"));
    act(() => hook.result.current.changeWorkspace("ws-c"));
    await waitFor(() => expect(hook.result.current.workflowsStatus).toBe("success"));
    await act(async () =>
      late.resolve({ workflows: [{ id: "wf-b", workspace_id: "ws-b", name: "Late" }] }),
    );
    expect(hook.result.current.workflows.map((workflow) => workflow.id)).toEqual(["wf-c"]);
  });

  it("never names the previous task's source workflow while the next task's loads", async () => {
    const other = { ...task, id: "task-2", workflow_id: "wf-x", workflow_step_id: "wf-x-first" };
    api.fetchTask.mockImplementation(async (id: string) => (id === "task-2" ? other : task));
    const hook = renderHook(
      ({ taskId }: { taskId: string }) =>
        useTaskWorkspaceMove({ open: true, taskId, workspaceId: "ws-a" }),
      { initialProps: { taskId: "task-1" } },
    );
    await waitFor(() => expect(hook.result.current.sourceWorkflowName).toBe("wf-a name"));
    api.fetchWorkflowSnapshot.mockImplementation((id: string) =>
      id === "wf-x" ? new Promise(() => undefined) : Promise.resolve(snapshotFor(id)),
    );
    hook.rerender({ taskId: "task-2" });
    await waitFor(() => expect(hook.result.current.task?.id).toBe("task-2"));
    expect(hook.result.current.sourceWorkflowName).toBeNull();
    expect(hook.result.current.sourceStepName).toBeNull();
  });
});

describe("submission", () => {
  it("binds the exact generation string and admits one request per click burst", async () => {
    const pending = Promise.withResolvers<{ receipt: TaskTransferReceipt }>();
    api.transferTask.mockReturnValue(pending.promise);
    const hook = render();
    await choose(hook);
    act(() => {
      void hook.result.current.submit();
      void hook.result.current.submit();
    });
    expect(api.transferTask).toHaveBeenCalledTimes(1);
    expect(sentBodies()[0]).toMatchObject({
      expected_task_updated_at: GENERATION,
      destination_workflow_step_id: "wf-b-first",
    });
    await act(async () => pending.resolve({ receipt }));
    expect(hook.result.current.operation?.status).toBe("committed");
    expect(reconcile).toHaveBeenCalledWith(store, "task-1");
  });

  it("reveals the accepted receipt only after canonical membership is reconciled", async () => {
    const gate = Promise.withResolvers<undefined>();
    reconcile.mockReturnValueOnce(gate.promise);
    api.transferTask.mockResolvedValueOnce({ receipt });
    const hook = render();
    await choose(hook);
    act(() => {
      void hook.result.current.submit();
    });
    await waitFor(() => expect(reconcile).toHaveBeenCalledWith(store, "task-1"));
    expect(hook.result.current.operation?.status).toBe("submitting");
    await act(() => hook.result.current.submit());
    expect(api.transferTask).toHaveBeenCalledTimes(1);
    await act(async () => gate.resolve(undefined));
    expect(hook.result.current.operation?.status).toBe("committed");
  });

  it("keeps choices after an original rejection and uses a fresh key next time", async () => {
    api.transferTask.mockRejectedValueOnce(conflict("step_incompatible"));
    const hook = render();
    await choose(hook);
    await act(() => hook.result.current.submit());
    expect(hook.result.current.operation).toMatchObject({
      status: "rejected",
      code: "step_incompatible",
    });
    expect(hook.result.current.selectedStepId).toBe("wf-b-first");
    expect(hook.result.current.canSubmit).toBe(true);

    api.transferTask.mockResolvedValueOnce({ receipt });
    await act(() => hook.result.current.submit());
    const [first, second] = sentBodies();
    expect(second.idempotency_key).not.toBe(first.idempotency_key);
  });

  it("settles a lost domain rejection only through an authoritative check", async () => {
    api.transferTask.mockRejectedValueOnce(networkLost());
    const hook = render();
    await choose(hook);
    await act(() => hook.result.current.submit());
    expect(hook.result.current.operation?.status).toBe("uncertain");
    expect(hook.result.current.canSubmit).toBe(false);

    // A generic conflict on the check is not terminal evidence.
    api.transferTask.mockRejectedValueOnce(conflict("source_changed"));
    await act(() => hook.result.current.check());
    expect(hook.result.current.operation?.status).toBe("check-unavailable");

    api.transferTask.mockRejectedValueOnce(conflict("operation_rejected"));
    await act(() => hook.result.current.check());
    expect(hook.result.current.operation?.status).toBe("operation-rejected");
    const keys = sentBodies().map((body) => body.idempotency_key);
    expect(new Set(keys).size).toBe(1);

    await act(() => hook.result.current.refreshSource());
    expect(hook.result.current.canSubmit).toBe(true);
    api.transferTask.mockResolvedValueOnce({ receipt });
    await act(() => hook.result.current.submit());
    expect(sentBodies().at(-1)?.idempotency_key).not.toBe(keys[0]);
  });

  it("keeps an uncertain request across denied checks, reopen, and access restoration", async () => {
    api.transferTask.mockRejectedValueOnce(new ApiError("bad gateway", 502, null));
    const hook = render();
    await choose(hook);
    await act(() => hook.result.current.submit());
    const original = sentBodies()[0];

    for (const status of [403, 404]) {
      api.transferTask.mockRejectedValueOnce(new ApiError("denied", status, null));
      await act(() => hook.result.current.check());
      expect(hook.result.current.operation?.status).toBe("check-unavailable");
      expect(hook.result.current.canSubmit).toBe(false);
    }

    hook.rerender({ open: false });
    hook.rerender({ open: true });
    await waitFor(() =>
      expect(hook.result.current.selectedStepId).toBe(original.destination_workflow_step_id),
    );
    expect(hook.result.current.operation?.body).toEqual(original);
    // A canonical placement refresh does not resolve the historical operation.
    await act(() => hook.result.current.refreshSource());
    expect(hook.result.current.operation?.status).toBe("check-unavailable");

    api.transferTask.mockResolvedValueOnce({ receipt });
    await act(() => hook.result.current.check());
    expect(hook.result.current.operation?.status).toBe("committed");
    expect(sentBodies().every((body) => body.idempotency_key === original.idempotency_key)).toBe(
      true,
    );
  });

  it("forgets an unresolved operation when the authenticated identity changes", async () => {
    api.transferTask.mockRejectedValueOnce(networkLost());
    const hook = render();
    await choose(hook);
    await act(() => hook.result.current.submit());
    expect(hook.result.current.operation?.status).toBe("uncertain");
    act(() => {
      store.setState((state) => ({
        ...state,
        auth: { ...state.auth, user: { ...(state.auth.user ?? {}), id: "someone-else" } as never },
      }));
    });
    await waitFor(() => expect(hook.result.current.operation).toBeNull());
  });
});

// A full reload creates a new app store (and registry) in the same tab.
function reload() {
  const auth = store.getState().auth;
  store = createAppStore({
    auth,
    workspaces: {
      activeId: "ws-a",
      items: [workspace("ws-a", "Personal"), workspace("ws-b", "Product")],
    },
  });
}

function signIn(id: string) {
  act(() =>
    store.getState().setAuthState({
      mode: "enabled",
      authenticated: true,
      user: {
        id,
        email: `${id}@example.test`,
        display_name: id,
        role: "member",
        status: "active",
      },
    }),
  );
}

function overrideSessionStorage(read: () => Storage): () => void {
  const original = Object.getOwnPropertyDescriptor(window, "sessionStorage");
  Object.defineProperty(window, "sessionStorage", { configurable: true, get: read });
  return () => {
    if (original) Object.defineProperty(window, "sessionStorage", original);
    else Reflect.deleteProperty(window, "sessionStorage");
  };
}

function blockSessionStorage(): () => void {
  return overrideSessionStorage(() => {
    throw new DOMException("blocked", "SecurityError");
  });
}

function refuseWrites(): () => void {
  const real = window.sessionStorage;
  const refusing = new Proxy(real, {
    get(target, prop) {
      if (prop === "setItem") {
        return () => {
          throw new DOMException("quota", "QuotaExceededError");
        };
      }
      const value = Reflect.get(target, prop, target);
      return typeof value === "function" ? value.bind(target) : value;
    },
  });
  return overrideSessionStorage(() => refusing);
}

describe("reload restore", () => {
  it("restores an unresolved request so Check result re-sends the same body and key", async () => {
    api.transferTask.mockRejectedValueOnce(networkLost());
    const first = render();
    await choose(first);
    await act(() => first.result.current.submit());
    const original = sentBodies()[0];
    first.unmount();

    reload();
    const restored = render();
    expect(restored.result.current.operation).toMatchObject({
      status: "uncertain",
      body: original,
    });
    expect(restored.result.current.canSubmit).toBe(false);
    // Nothing is replayed automatically.
    expect(api.transferTask).toHaveBeenCalledTimes(1);

    api.transferTask.mockResolvedValueOnce({ receipt });
    await act(() => restored.result.current.check());
    expect(api.transferTask).toHaveBeenCalledTimes(2);
    expect(sentBodies()[1]).toEqual(original);
    expect(restored.result.current.operation?.status).toBe("committed");

    // A resolved operation is no longer restored.
    restored.unmount();
    reload();
    expect(render().result.current.operation).toBeNull();
  });

  it("reports a tampered record as uncheckable without sending it", async () => {
    api.transferTask.mockRejectedValueOnce(networkLost());
    const first = render();
    await choose(first);
    await act(() => first.result.current.submit());
    first.unmount();
    const [key] = Object.keys(window.sessionStorage);
    const stored = JSON.parse(window.sessionStorage.getItem(key)!);
    stored.body.destination_workflow_step_id = "tampered";
    window.sessionStorage.setItem(key, JSON.stringify(stored));

    reload();
    const tampered = render();
    expect(tampered.result.current.operation).toBeNull();
    expect(tampered.result.current.restoreUnverifiable).toBe(true);
    await choose(tampered);
    expect(tampered.result.current.canSubmit).toBe(false);
    await act(() => tampered.result.current.submit());
    await act(() => tampered.result.current.check());
    expect(api.transferTask).toHaveBeenCalledTimes(1);
  });
});

describe("storage unavailable", () => {
  it("refuses a fresh submission when sessionStorage cannot be read", async () => {
    const restore = blockSessionStorage();
    try {
      const hook = render();
      await choose(hook);
      expect(hook.result.current.storageUnavailable).toBe(true);
      // No earlier move exists, so the reload-specific notice must not be claimed.
      expect(hook.result.current.restoreUnverifiable).toBe(false);
      expect(hook.result.current.canSubmit).toBe(false);
      await act(() => hook.result.current.submit());
      expect(api.transferTask).not.toHaveBeenCalled();
    } finally {
      restore();
    }
  });

  it("sends nothing and says why when the immutable record cannot be persisted", async () => {
    const restore = refuseWrites();
    try {
      const hook = render();
      await choose(hook);
      expect(hook.result.current.canSubmit).toBe(true);
      await act(() => hook.result.current.submit());
      expect(api.transferTask).not.toHaveBeenCalled();
      expect(hook.result.current.operation).toBeNull();
      expect(hook.result.current.storageUnavailable).toBe(true);
      expect(hook.result.current.canSubmit).toBe(false);
    } finally {
      restore();
    }
  });

  it("still checks an already stored request when new writes are refused", async () => {
    api.transferTask.mockRejectedValueOnce(networkLost());
    const first = render();
    await choose(first);
    await act(() => first.result.current.submit());
    const original = sentBodies()[0];
    first.unmount();
    reload();
    const restore = refuseWrites();
    try {
      const restored = render();
      expect(restored.result.current.operation?.status).toBe("uncertain");
      api.transferTask.mockResolvedValueOnce({ receipt });
      await act(() => restored.result.current.check());
      expect(sentBodies()[1]).toEqual(original);
      expect(restored.result.current.operation?.status).toBe("committed");
    } finally {
      restore();
    }
  });

  it("replaces a stale rejection with the storage notice when a resubmission cannot be saved", async () => {
    api.transferTask.mockRejectedValueOnce(conflict("destination_full"));
    const hook = render();
    await choose(hook);
    await act(() => hook.result.current.submit());
    expect(hook.result.current.operation?.status).toBe("rejected");
    const restore = refuseWrites();
    try {
      await act(() => hook.result.current.submit());
      expect(api.transferTask).toHaveBeenCalledTimes(1);
      expect(hook.result.current.storageUnavailable).toBe(true);
      expect(hook.result.current.operation).toBeNull();
    } finally {
      restore();
    }
  });
});

describe("caller-partitioned restore", () => {
  it("hides a caller's unresolved request from another caller and restores it on switching back", async () => {
    signIn("user-a");
    api.transferTask.mockRejectedValueOnce(networkLost());
    const hook = render();
    await choose(hook);
    await act(() => hook.result.current.submit());
    const original = sentBodies()[0];

    signIn("user-b");
    await waitFor(() => expect(hook.result.current.operation).toBeNull());
    await act(() => hook.result.current.check());
    expect(api.transferTask).toHaveBeenCalledTimes(1);
    // The other caller's record stays stored while hidden.
    expect(Object.keys(window.sessionStorage)).toHaveLength(1);

    signIn("user-a");
    await waitFor(() =>
      expect(hook.result.current.operation).toMatchObject({ status: "uncertain", body: original }),
    );
    expect(hook.result.current.canSubmit).toBe(false);
    api.transferTask.mockResolvedValueOnce({ receipt });
    await act(() => hook.result.current.check());
    expect(sentBodies()).toEqual([original, original]);
    expect(hook.result.current.operation?.status).toBe("committed");
    expect(Object.keys(window.sessionStorage)).toHaveLength(0);
  });
});

describe("identity switch with several consumers", () => {
  it("adopts the new caller without updating another consumer during render", async () => {
    signIn("user-a");
    api.transferTask.mockRejectedValueOnce(networkLost());
    const hook = render();
    const other = renderHook(() => useTaskWorkspaceMoveOperation("task-1"));
    await choose(hook);
    await act(() => hook.result.current.submit());
    expect(other.result.current.operation?.status).toBe("uncertain");
    const errors = vi.spyOn(console, "error").mockImplementation(() => undefined);
    try {
      signIn("user-b");
      await waitFor(() => expect(other.result.current.operation).toBeNull());
      expect(hook.result.current.operation).toBeNull();
      const renderPhaseUpdates = errors.mock.calls.filter((call) =>
        String(call[0]).includes("Cannot update a component"),
      );
      expect(renderPhaseUpdates).toEqual([]);
    } finally {
      errors.mockRestore();
    }
  });
});

describe("superseded responses across caller switches", () => {
  it("ignores a late original failure after a successful check that followed a caller switch", async () => {
    signIn("user-a");
    const original = Promise.withResolvers<{ receipt: TaskTransferReceipt }>();
    api.transferTask.mockReturnValueOnce(original.promise);
    const hook = render();
    await choose(hook);
    act(() => {
      void hook.result.current.submit();
    });
    signIn("user-b");
    signIn("user-a");
    await waitFor(() => expect(hook.result.current.operation?.status).toBe("uncertain"));
    api.transferTask.mockResolvedValueOnce({ receipt });
    await act(() => hook.result.current.check());
    expect(hook.result.current.operation?.status).toBe("committed");
    await act(async () => original.reject(networkLost()));
    expect(hook.result.current.operation?.status).toBe("committed");
    expect(Object.keys(window.sessionStorage)).toHaveLength(0);
  });

  it("keeps a check in flight when the superseded original fails before it resolves", async () => {
    signIn("user-a");
    const original = Promise.withResolvers<{ receipt: TaskTransferReceipt }>();
    const check = Promise.withResolvers<{ receipt: TaskTransferReceipt }>();
    api.transferTask.mockReturnValueOnce(original.promise).mockReturnValueOnce(check.promise);
    const hook = render();
    await choose(hook);
    act(() => {
      void hook.result.current.submit();
    });
    signIn("user-b");
    signIn("user-a");
    await waitFor(() => expect(hook.result.current.operation?.status).toBe("uncertain"));
    act(() => {
      void hook.result.current.check();
    });
    await act(async () => original.reject(networkLost()));
    expect(hook.result.current.operation?.status).toBe("submitting");
    await act(async () => check.resolve({ receipt }));
    expect(hook.result.current.operation?.status).toBe("committed");
  });

  it("keeps an accepted receipt committed when a caller switch lands during reconciliation", async () => {
    signIn("user-a");
    const gate = Promise.withResolvers<undefined>();
    reconcile.mockReturnValueOnce(gate.promise);
    api.transferTask.mockResolvedValueOnce({ receipt });
    const hook = render();
    await choose(hook);
    act(() => {
      void hook.result.current.submit();
    });
    await waitFor(() => expect(reconcile).toHaveBeenCalled());
    signIn("user-b");
    signIn("user-a");
    await waitFor(() => expect(Object.keys(window.sessionStorage)).toHaveLength(0));
    expect(hook.result.current.operation?.status).not.toBe("uncertain");
    await act(async () => gate.resolve(undefined));
    expect(Object.keys(window.sessionStorage)).toHaveLength(0);
    expect(hook.result.current.operation?.status).not.toBe("uncertain");
  });
});

describe("caller-partitioned restore", () => {
  it("keeps an unresolved request through logout and login", async () => {
    signIn("user-a");
    api.transferTask.mockRejectedValueOnce(networkLost());
    const hook = render();
    await choose(hook);
    await act(() => hook.result.current.submit());
    const original = sentBodies()[0];

    act(() => store.getState().setAuthState({ mode: "enabled", authenticated: false, user: null }));
    await waitFor(() => expect(hook.result.current.operation).toBeNull());
    expect(Object.keys(window.sessionStorage)).toHaveLength(1);

    hook.unmount();
    signIn("user-a");
    reload();
    const restored = render();
    expect(restored.result.current.operation).toMatchObject({
      status: "uncertain",
      body: original,
    });
    expect(api.transferTask).toHaveBeenCalledTimes(1);
  });
});
