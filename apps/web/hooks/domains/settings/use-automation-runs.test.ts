import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import type { AutomationRun } from "@/lib/types/automation";

type MockState = {
  automationRuns: {
    byAutomationId: Record<string, AutomationRun[]>;
    loading: Record<string, boolean>;
    mutationEpoch: Record<string, number>;
    deleting: Record<string, number | false>;
  };
};

let mockState: MockState = {
  automationRuns: { byAutomationId: {}, loading: {}, mutationEpoch: {}, deleting: {} },
};

function setRuns(automationId: string, runs: AutomationRun[]) {
  mockState = {
    automationRuns: {
      ...mockState.automationRuns,
      byAutomationId: { ...mockState.automationRuns.byAutomationId, [automationId]: runs },
    },
  };
}

const storeActions = {
  setAutomationRuns: (automationId: string, runs: AutomationRun[]) => setRuns(automationId, runs),
  setAutomationRunsLoading: (automationId: string, loading: boolean) => {
    mockState = {
      automationRuns: {
        ...mockState.automationRuns,
        loading: { ...mockState.automationRuns.loading, [automationId]: loading },
      },
    };
  },
  removeAutomationRun: (automationId: string, runId: string) => {
    const runs = mockState.automationRuns.byAutomationId[automationId] ?? [];
    setRuns(
      automationId,
      runs.filter((r) => r.id !== runId),
    );
  },
  clearAutomationRuns: (automationId: string) => setRuns(automationId, []),
  restoreAutomationRun: (automationId: string, run: AutomationRun) => {
    const runs = mockState.automationRuns.byAutomationId[automationId] ?? [];
    if (runs.some((r) => r.id === run.id)) return;
    setRuns(automationId, [...runs, run]);
  },
  beginAutomationRunDelete: (automationId: string) => {
    if ((mockState.automationRuns.deleting[automationId] ?? false) !== false) return null;
    const next = (mockState.automationRuns.mutationEpoch[automationId] ?? 0) + 1;
    mockState = {
      automationRuns: {
        ...mockState.automationRuns,
        mutationEpoch: { ...mockState.automationRuns.mutationEpoch, [automationId]: next },
        deleting: { ...mockState.automationRuns.deleting, [automationId]: next },
      },
    };
    return next;
  },
  endAutomationRunDelete: (automationId: string, generation: number) => {
    if (mockState.automationRuns.deleting[automationId] === generation) {
      mockState = {
        automationRuns: {
          ...mockState.automationRuns,
          deleting: { ...mockState.automationRuns.deleting, [automationId]: false },
        },
      };
    }
  },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockState & typeof storeActions) => unknown) =>
    selector({ ...mockState, ...storeActions }),
  useAppStoreApi: () => ({
    getState: () => ({ ...mockState, ...storeActions }),
  }),
}));

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

vi.mock("@/lib/api/domains/automation-api", () => ({
  listAutomationRuns: vi.fn(),
  deleteAutomationRun: vi.fn(),
  deleteAllAutomationRuns: vi.fn(),
}));

import { toast } from "sonner";
import {
  listAutomationRuns,
  deleteAutomationRun,
  deleteAllAutomationRuns,
} from "@/lib/api/domains/automation-api";
import { useAutomationRuns } from "./use-automation-runs";

const AUTOMATION_ID = "auto-1";
const BATCH_DELETE_ERROR = "batch delete failed";
const WORKSPACE_ID = "ws-1";

function mkRun(id: string): AutomationRun {
  return {
    id,
    automation_id: AUTOMATION_ID,
    trigger_id: "trig-1",
    trigger_type: "scheduled",
    task_id: "",
    status: "skipped",
    dedup_key: "",
    trigger_data: {},
    error_message: "",
    created_at: new Date().toISOString(),
  };
}

/** A promise plus its resolve function, for controlling when a mocked async
 * call settles relative to other events in a test. */
function deferred<T>() {
  const { promise, resolve } = Promise.withResolvers<T>();
  return { promise, resolve };
}

beforeEach(() => {
  mockState = {
    automationRuns: { byAutomationId: {}, loading: {}, mutationEpoch: {}, deleting: {} },
  };
  vi.mocked(listAutomationRuns).mockReset();
  vi.mocked(deleteAutomationRun).mockReset();
  vi.mocked(deleteAllAutomationRuns).mockReset();
  vi.mocked(toast.error).mockReset();
});

describe("useAutomationRuns", () => {
  it("re-applies the optimistic removal if an in-flight refresh resurrects the row before delete confirms", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    // The delete request stays pending until we manually resolve it below,
    // so we can interleave a "stale refresh" in between.
    const del = deferred<{ deleted: boolean }>();
    vi.mocked(deleteAutomationRun).mockReturnValue(del.promise);
    // A concurrent refresh() resolves with the pre-delete list — as if the
    // list request was already in flight when the delete was fired. The
    // hook's own mount-effect fetch (first call) is left pending so it
    // doesn't confound the explicit refresh() below.
    vi.mocked(listAutomationRuns)
      .mockReturnValueOnce(Promise.withResolvers<AutomationRun[]>().promise)
      .mockResolvedValue([runX, runY]);

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteRun("run-x");
    });
    rerender();
    expect(result.current.runs.map((r) => r.id)).toEqual(["run-y"]);

    // The in-flight refresh resolves and overwrites the store with the
    // stale full list, resurrecting run-x — this is the race being guarded
    // against, reproduced here explicitly.
    await act(async () => {
      await result.current.refresh();
    });
    rerender();
    expect(result.current.runs.map((r) => r.id).sort()).toEqual(["run-x", "run-y"]);

    // The delete now confirms server-side. Without re-applying the removal
    // on success, run-x would stay resurrected until the next full refresh.
    await act(async () => {
      del.resolve({ deleted: true });
      await del.promise;
    });
    rerender();
    expect(result.current.runs.map((r) => r.id)).toEqual(["run-y"]);
  });

  it("keeps a run created after delete-all completed when reconciling the full list", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    const runNew = mkRun("run-new");
    setRuns(AUTOMATION_ID, [runX, runY]);

    const del = deferred<{ deleted: boolean }>();
    vi.mocked(deleteAllAutomationRuns).mockReturnValue(del.promise);
    vi.mocked(listAutomationRuns)
      .mockReturnValueOnce(Promise.withResolvers<AutomationRun[]>().promise) // mount
      .mockResolvedValueOnce([runX, runY]) // in-flight refresh, pre-delete list
      .mockResolvedValue([runNew]); // authoritative post-delete refresh

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteAllRuns();
    });
    rerender();
    expect(result.current.runs).toEqual([]);

    await act(async () => {
      await result.current.refresh();
    });
    rerender();
    expect(result.current.runs).toHaveLength(2);

    // The success path must end with exactly the authoritative post-delete
    // list: a blanket clear would drop run-new, which was created after the
    // backend delete completed.
    await act(async () => {
      del.resolve({ deleted: true });
      await del.promise;
    });
    rerender();
    expect(result.current.runs.map((r) => r.id)).toEqual(["run-new"]);
  });

  it("passes workspaceId through to the delete-run and delete-all-runs API calls", async () => {
    setRuns(AUTOMATION_ID, [mkRun("run-x")]);
    vi.mocked(listAutomationRuns).mockReturnValue(Promise.withResolvers<AutomationRun[]>().promise);
    vi.mocked(deleteAutomationRun).mockResolvedValue({ deleted: true });
    vi.mocked(deleteAllAutomationRuns).mockResolvedValue({ deleted: true });

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteRun("run-x");
    });
    expect(deleteAutomationRun).toHaveBeenCalledWith("run-x", WORKSPACE_ID);

    // Let the per-run delete settle so the serialization gate reopens before
    // the next mutation.
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();
    expect(result.current.deleting).toBe(false);

    act(() => {
      result.current.deleteAllRuns();
    });
    expect(deleteAllAutomationRuns).toHaveBeenCalledWith(AUTOMATION_ID, WORKSPACE_ID);
  });
});

describe("useAutomationRuns - double-failure recovery", () => {
  it("restores the specific deleted run if both the delete and the recovery refresh fail", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    // Mount-effect fetch resolves immediately with the initial list so it
    // doesn't interfere with the delete-triggered recovery fetch below.
    vi.mocked(listAutomationRuns)
      .mockResolvedValueOnce([runX, runY])
      .mockRejectedValueOnce(new Error("network down"));
    vi.mocked(deleteAutomationRun).mockRejectedValue(new Error("delete failed"));

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));
    await act(async () => {});
    rerender();

    await act(async () => {
      result.current.deleteRun("run-x");
      // Flush the delete rejection and the subsequent revert-fetch rejection.
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();

    // Both the delete and the revert fetch failed — the store must not be
    // left permanently missing run-x. It should be restored from the
    // pre-delete snapshot rather than silently staying gone.
    expect(result.current.runs.map((r) => r.id).sort()).toEqual(["run-x", "run-y"]);
    expect(result.current.deleting).toBe(false);
  });

  it("restores the full pre-clear snapshot if both delete-all and the recovery refresh fail", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    vi.mocked(listAutomationRuns)
      .mockResolvedValueOnce([runX, runY])
      .mockRejectedValueOnce(new Error("network down"));
    vi.mocked(deleteAllAutomationRuns).mockRejectedValue(new Error("delete-all failed"));

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));
    await act(async () => {});
    rerender();

    await act(async () => {
      result.current.deleteAllRuns();
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();

    // Both delete-all and the revert fetch failed — the store must not be
    // left permanently empty. It should be restored from the pre-clear
    // snapshot rather than silently staying cleared.
    expect(result.current.runs.map((r) => r.id).sort()).toEqual(["run-x", "run-y"]);
    expect(result.current.deleting).toBe(false);
  });
});

describe("useAutomationRuns - status-scoped delete-all", () => {
  it("deletes exactly the given runs through the per-run API and removes them optimistically", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    const runZ = mkRun("run-z");
    setRuns(AUTOMATION_ID, [runX, runY, runZ]);

    vi.mocked(listAutomationRuns).mockReturnValue(Promise.withResolvers<AutomationRun[]>().promise);
    vi.mocked(deleteAutomationRun).mockResolvedValue({ deleted: true });

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteAllRuns(["run-x", "run-z"]);
    });
    rerender();
    expect(result.current.runs.map((r) => r.id)).toEqual(["run-y"]);
    expect(deleteAutomationRun).toHaveBeenCalledTimes(2);
    expect(deleteAutomationRun).toHaveBeenCalledWith("run-x", WORKSPACE_ID);
    expect(deleteAutomationRun).toHaveBeenCalledWith("run-z", WORKSPACE_ID);
    expect(deleteAllAutomationRuns).not.toHaveBeenCalled();
  });

  it("keeps a run created after the deletes completed when reconciling the scoped view", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    const runNew = mkRun("run-new");
    setRuns(AUTOMATION_ID, [runX, runY]);

    const del = deferred<{ deleted: boolean }>();
    vi.mocked(deleteAutomationRun).mockReturnValue(del.promise);
    vi.mocked(listAutomationRuns)
      .mockReturnValueOnce(Promise.withResolvers<AutomationRun[]>().promise) // mount
      .mockResolvedValueOnce([runX, runY]) // in-flight refresh, pre-delete list
      .mockResolvedValue([runY, runNew]); // authoritative post-delete refresh

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteAllRuns(["run-x"]);
    });
    rerender();
    expect(result.current.runs.map((r) => r.id)).toEqual(["run-y"]);

    await act(async () => {
      await result.current.refresh();
    });
    rerender();
    expect(result.current.runs.map((r) => r.id).sort()).toEqual(["run-x", "run-y"]);

    // The success path must end with exactly the authoritative post-delete
    // list: a blanket re-clear would drop run-new, which was created after
    // the deletes completed server-side.
    await act(async () => {
      del.resolve({ deleted: true });
      await del.promise;
    });
    rerender();
    expect(result.current.runs.map((r) => r.id).sort()).toEqual(["run-new", "run-y"]);
  });
});

describe("useAutomationRuns - status-scoped delete-all failure recovery", () => {
  it("shows one aggregated toast and reverts to the server list when any delete fails", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    vi.mocked(listAutomationRuns).mockResolvedValue([runX, runY]);
    vi.mocked(deleteAutomationRun).mockRejectedValue(new Error(BATCH_DELETE_ERROR));

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));
    await act(async () => {});
    rerender();

    await act(async () => {
      result.current.deleteAllRuns(["run-x", "run-y"]);
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();

    // One toast for the whole batch, not one per failed delete.
    expect(toast.error).toHaveBeenCalledTimes(1);
    expect(toast.error).toHaveBeenCalledWith(BATCH_DELETE_ERROR);
    expect(result.current.runs.map((r) => r.id).sort()).toEqual(["run-x", "run-y"]);
    // The serialization slot reopens once the recovery settles.
    expect(result.current.deleting).toBe(false);
  });

  it("restores the pre-delete snapshot if a delete fails and the recovery refresh also fails", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    vi.mocked(listAutomationRuns)
      .mockResolvedValueOnce([runX, runY])
      .mockRejectedValueOnce(new Error("network down"));
    vi.mocked(deleteAutomationRun).mockRejectedValue(new Error(BATCH_DELETE_ERROR));

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));
    await act(async () => {});
    rerender();

    await act(async () => {
      result.current.deleteAllRuns(["run-x"]);
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();

    expect(result.current.runs.map((r) => r.id).sort()).toEqual(["run-x", "run-y"]);
    expect(toast.error).toHaveBeenCalledTimes(2); // delete failure + could-not-refresh
    // The serialization slot reopens even when the recovery refresh fails.
    expect(result.current.deleting).toBe(false);
  });

  it("treats an empty id list as a no-op", () => {
    setRuns(AUTOMATION_ID, [mkRun("run-x")]);
    vi.mocked(listAutomationRuns).mockReturnValue(Promise.withResolvers<AutomationRun[]>().promise);

    const { result } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteAllRuns([]);
    });
    expect(result.current.runs).toHaveLength(1);
    expect(deleteAutomationRun).not.toHaveBeenCalled();
    expect(deleteAllAutomationRuns).not.toHaveBeenCalled();
  });
});

describe("useAutomationRuns - status-scoped delete-all races", () => {
  it("discards a refresh that started before a scoped delete-all and resolves after it succeeds", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    const stale = deferred<AutomationRun[]>();
    vi.mocked(listAutomationRuns)
      .mockReturnValueOnce(Promise.withResolvers<AutomationRun[]>().promise) // mount
      .mockReturnValueOnce(stale.promise) // refresh started before the delete
      .mockResolvedValue([runY]); // authoritative post-delete refresh
    vi.mocked(deleteAutomationRun).mockResolvedValue({ deleted: true });

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.refresh(); // in flight when the delete starts
      result.current.deleteAllRuns(["run-x"]); // succeeds immediately
    });
    // Let the delete's success re-removal land before the stale list resolves.
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();
    expect(result.current.runs.map((r) => r.id)).toEqual(["run-y"]);

    // The stale pre-delete list resolves after the delete confirmed — the
    // mutation guard must discard it instead of resurrecting run-x.
    await act(async () => {
      stale.resolve([runX, runY]);
      await stale.promise;
    });
    rerender();
    expect(result.current.runs.map((r) => r.id)).toEqual(["run-y"]);
  });

  it("does not recover from a partial failure until every delete in the batch settles", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    const slowDelete = deferred<{ deleted: boolean }>();
    vi.mocked(deleteAutomationRun)
      .mockRejectedValueOnce(new Error(BATCH_DELETE_ERROR))
      .mockReturnValueOnce(slowDelete.promise);
    vi.mocked(listAutomationRuns)
      .mockReturnValueOnce(Promise.withResolvers<AutomationRun[]>().promise) // mount
      .mockResolvedValue([runX, runY]); // recovery refresh

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteAllRuns(["run-x", "run-y"]);
    });
    // run-x rejects first while run-y is still pending — the recovery must
    // not run yet, or it would race the in-flight delete.
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();
    expect(listAutomationRuns).toHaveBeenCalledTimes(1); // mount fetch only
    expect(toast.error).not.toHaveBeenCalled();

    // run-y settles successfully; only now does the batch settle and recover.
    await act(async () => {
      slowDelete.resolve({ deleted: true });
      await slowDelete.promise;
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();
    expect(listAutomationRuns).toHaveBeenCalledTimes(2); // mount + recovery
    expect(toast.error).toHaveBeenCalledTimes(1);
    // The server list (both runs still present) wins over the optimistic
    // removal once every delete has settled.
    expect(result.current.runs.map((r) => r.id).sort()).toEqual(["run-x", "run-y"]);
  });
});

describe("useAutomationRuns - serialized deletes", () => {
  it("ignores a second delete-all fired while one is still in flight", () => {
    setRuns(AUTOMATION_ID, [mkRun("run-x"), mkRun("run-y"), mkRun("run-z")]);
    const slow = deferred<{ deleted: boolean }>();
    vi.mocked(deleteAutomationRun).mockReturnValue(slow.promise);
    vi.mocked(listAutomationRuns).mockReturnValue(Promise.withResolvers<AutomationRun[]>().promise);

    const { result } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteAllRuns(["run-x", "run-y"]);
      // The second, overlapping mutation must be a no-op: the two delete
      // batches are serialized so their recoveries cannot race each other.
      result.current.deleteAllRuns(["run-z"]);
    });
    expect(deleteAutomationRun).toHaveBeenCalledTimes(2); // only the first batch
  });

  it("ignores a per-run delete fired while a delete-all is in flight", () => {
    setRuns(AUTOMATION_ID, [mkRun("run-x"), mkRun("run-y")]);
    const slow = deferred<{ deleted: boolean }>();
    vi.mocked(deleteAutomationRun).mockReturnValue(slow.promise);
    vi.mocked(listAutomationRuns).mockReturnValue(Promise.withResolvers<AutomationRun[]>().promise);

    const { result } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteAllRuns(["run-x"]);
      result.current.deleteRun("run-y");
    });
    expect(deleteAutomationRun).toHaveBeenCalledTimes(1); // only the delete-all
  });

  it("shares the serialization slot across hook instances for the same automation", async () => {
    setRuns(AUTOMATION_ID, [mkRun("run-x"), mkRun("run-y")]);
    const slow = deferred<{ deleted: boolean }>();
    vi.mocked(deleteAutomationRun).mockReturnValue(slow.promise);
    vi.mocked(listAutomationRuns).mockReturnValue(Promise.withResolvers<AutomationRun[]>().promise);

    // Instance A starts a delete and "unmounts" (the editor navigated away).
    const first = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));
    act(() => {
      first.result.current.deleteAllRuns(["run-x"]);
    });
    first.unmount();

    // A remounted instance for the same automation must see the shared
    // in-flight slot and be gated, not start its own overlapping delete.
    const second = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));
    expect(second.result.current.deleting).toBe(true);
    act(() => {
      second.result.current.deleteAllRuns(["run-y"]);
      second.result.current.deleteRun("run-y");
    });
    expect(deleteAutomationRun).toHaveBeenCalledTimes(1); // only A's batch

    // A's delete settles and its endDelete releases the shared slot.
    await act(async () => {
      slow.resolve({ deleted: true });
      await slow.promise;
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    second.rerender();
    expect(second.result.current.deleting).toBe(false);
  });

  it("exposes deleting while a delete runs and clears it once the batch settles", async () => {
    setRuns(AUTOMATION_ID, [mkRun("run-x")]);
    const slow = deferred<{ deleted: boolean }>();
    vi.mocked(deleteAutomationRun).mockReturnValue(slow.promise);
    vi.mocked(listAutomationRuns).mockReturnValue(Promise.withResolvers<AutomationRun[]>().promise);

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteAllRuns(["run-x"]);
    });
    rerender();
    expect(result.current.deleting).toBe(true);

    await act(async () => {
      slow.resolve({ deleted: true });
      await slow.promise;
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();
    expect(result.current.deleting).toBe(false);
  });
});

describe("useAutomationRuns - single-failure revert", () => {
  it("shows a toast and reverts to the server list when deleteRun fails but the recovery refresh succeeds", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    // Mount-effect fetch, then the delete-triggered recovery fetch, both
    // succeed with the server's authoritative (unchanged) list.
    vi.mocked(listAutomationRuns).mockResolvedValue([runX, runY]);
    vi.mocked(deleteAutomationRun).mockRejectedValue(new Error("delete failed"));

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));
    await act(async () => {});
    rerender();

    await act(async () => {
      result.current.deleteRun("run-x");
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();

    expect(toast.error).toHaveBeenCalledWith("delete failed");
    // The recovery refresh succeeded, so the store reflects the server's
    // authoritative list rather than the double-failure local-cache fallback.
    expect(result.current.runs.map((r) => r.id).sort()).toEqual(["run-x", "run-y"]);
    expect(result.current.deleting).toBe(false);
  });

  it("shows a toast and reverts to the server list when deleteAllRuns fails but the recovery refresh succeeds", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    vi.mocked(listAutomationRuns).mockResolvedValue([runX, runY]);
    vi.mocked(deleteAllAutomationRuns).mockRejectedValue(new Error("delete-all failed"));

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));
    await act(async () => {});
    rerender();

    await act(async () => {
      result.current.deleteAllRuns();
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();

    expect(toast.error).toHaveBeenCalledWith("delete-all failed");
    expect(result.current.runs.map((r) => r.id).sort()).toEqual(["run-x", "run-y"]);
    expect(result.current.deleting).toBe(false);
  });
});
