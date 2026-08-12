"use client";

import { useEffect, useCallback, useRef } from "react";
import { t } from "@/lib/i18n";
import { toast } from "@/lib/toast/sonner";
import {
  listAutomationRuns,
  deleteAutomationRun,
  deleteAllAutomationRuns,
} from "@/lib/api/domains/automation-api";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { AutomationRun } from "@/lib/types/automation";

const EMPTY_RUNS: AutomationRun[] = [];

/**
 * Monotonic mutation counter shared by the list fetches of one hook
 * instance. A delete-all bumps it the moment it starts; any list response
 * that captured an older value is stale by definition and must be discarded,
 * otherwise it would resurrect rows the delete removed. This closes the race
 * where a refresh started before the delete resolves after it succeeded.
 */
type ListEpoch = { current: number };

function fetchRuns(
  automationId: string,
  epoch: ListEpoch,
  setRunsLoading: (automationId: string, loading: boolean) => void,
  setRuns: (automationId: string, runs: AutomationRun[]) => void,
  onError?: () => void,
): void {
  const captured = epoch.current;
  setRunsLoading(automationId, true);
  listAutomationRuns(automationId)
    .then((result) => {
      // Discard responses that went stale while in flight: a delete-all that
      // started after this fetch captured the epoch owns the store now.
      if (epoch.current === captured) setRuns(automationId, result ?? []);
    })
    .catch(() => {
      if (epoch.current === captured) onError?.();
    })
    .finally(() => {
      setRunsLoading(automationId, false);
    });
}

/**
 * Shared revert for both delete-all paths: on failure, refresh from the
 * server (authoritative — it drops whatever actually succeeded); if that also
 * fails, fall back to the pre-delete snapshot so the store is never left
 * permanently missing rows the delete never removed. Both writes are
 * epoch-guarded so a newer mutation cannot be clobbered by this recovery.
 */
function revertAfterFailedDelete(
  automationId: string,
  fallbackRuns: AutomationRun[],
  epoch: ListEpoch,
  setRuns: (automationId: string, runs: AutomationRun[]) => void,
): void {
  const captured = epoch.current;
  listAutomationRuns(automationId)
    .then((result) => {
      if (epoch.current === captured) setRuns(automationId, result ?? []);
    })
    .catch(() => {
      if (epoch.current === captured) setRuns(automationId, fallbackRuns);
      toast.error(t("automations:couldNotRefreshRuns"));
    });
}

type DeleteAllStore = {
  epoch: ListEpoch;
  clearRuns: (automationId: string) => void;
  removeRun: (automationId: string, runId: string) => void;
  setRuns: (automationId: string, runs: AutomationRun[]) => void;
  getRuns: () => AutomationRun[];
};

function executeDeleteAll(
  automationId: string,
  workspaceId: string,
  runIds: string[] | undefined,
  store: DeleteAllStore,
): void {
  // An empty scope is a no-op: the UI never offers delete-all for an empty
  // view, and falling back to the all-runs delete here would be a dangerous
  // surprise for a caller that computed zero ids.
  if (runIds && runIds.length === 0) return;
  // Every list fetch already in flight is now stale: nothing it returns may
  // overwrite the post-delete state.
  store.epoch.current += 1;
  const previousRuns = store.getRuns();
  if (!runIds) {
    // Unscoped delete-all: remove every run for the automation in one call.
    // Snapshot the full list so we can restore it if both the delete-all and
    // the recovery refresh below fail.
    store.clearRuns(automationId); // optimistic
    deleteAllAutomationRuns(automationId, workspaceId)
      .then(() => {
        // See deleteRun: guard against an in-flight refresh() resurrecting
        // rows between the optimistic clear and this success callback.
        store.clearRuns(automationId);
      })
      .catch((err: unknown) => {
        const msg = err instanceof Error ? err.message : t("automations:failedToDeleteRuns");
        toast.error(msg);
        revertAfterFailedDelete(automationId, previousRuns, store.epoch, store.setRuns);
      });
    return;
  }
  // Status-scoped delete-all: remove exactly the given runs through the
  // per-run API. Archived/Cancelled are read-time-derived statuses that only
  // exist in the loaded payload, so the visible set is the only exact scope.
  const ids = [...new Set(runIds)];
  ids.forEach((id) => store.removeRun(automationId, id)); // optimistic
  Promise.allSettled(ids.map((id) => deleteAutomationRun(id, workspaceId))).then((results) => {
    if (results.every((result) => result.status === "fulfilled")) {
      // In-flight refresh guard, same as deleteRun and the unscoped path: a
      // refresh that started after the delete began may still return
      // pre-delete rows while the deletes are in flight server-side.
      ids.forEach((id) => store.removeRun(automationId, id));
      return;
    }
    // One toast and one recovery refresh for the whole batch, and only after
    // every delete has settled: recovering on the first rejection would race
    // the still-pending deletes and could resurrect rows that later succeed.
    const first = results.find((r): r is PromiseRejectedResult => r.status === "rejected");
    const msg =
      first && first.reason instanceof Error
        ? first.reason.message
        : t("automations:failedToDeleteRuns");
    toast.error(msg);
    revertAfterFailedDelete(automationId, previousRuns, store.epoch, store.setRuns);
  });
}

export function useAutomationRuns(automationId: string | null, workspaceId: string) {
  const runs = useAppStore((state) =>
    automationId ? (state.automationRuns.byAutomationId[automationId] ?? EMPTY_RUNS) : EMPTY_RUNS,
  );
  const loading = useAppStore((state) =>
    automationId ? (state.automationRuns.loading[automationId] ?? false) : false,
  );
  const setRuns = useAppStore((state) => state.setAutomationRuns);
  const setRunsLoading = useAppStore((state) => state.setAutomationRunsLoading);
  const removeRun = useAppStore((state) => state.removeAutomationRun);
  const clearRuns = useAppStore((state) => state.clearAutomationRuns);
  const restoreRun = useAppStore((state) => state.restoreAutomationRun);
  const storeApi = useAppStoreApi();
  const epoch = useRef(0);

  useEffect(() => {
    if (!automationId || loading) return;
    fetchRuns(automationId, epoch, setRunsLoading, setRuns, () => setRuns(automationId, []));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [automationId]);

  const refresh = useCallback(() => {
    if (!automationId) return;
    fetchRuns(automationId, epoch, setRunsLoading, setRuns);
  }, [automationId, setRuns, setRunsLoading]);

  const deleteRun = useCallback(
    (runId: string) => {
      if (!automationId) return;
      // Snapshot the run being removed so we can restore it precisely (not
      // the whole list, which could clobber unrelated concurrent changes)
      // if both the delete and the recovery refresh below fail.
      const deletedRun = storeApi
        .getState()
        .automationRuns.byAutomationId[automationId]?.find((r) => r.id === runId);
      removeRun(automationId, runId); // optimistic
      deleteAutomationRun(runId, workspaceId)
        .then(() => {
          // Re-apply the removal: an in-flight refresh() / initial load can
          // resolve between the optimistic removeRun above and this success
          // callback and overwrite the store with the pre-delete list,
          // resurrecting the row. Removing it again here is a no-op unless
          // that happened.
          removeRun(automationId, runId);
        })
        .catch((err: unknown) => {
          // An Error message here is an API diagnostic and stays English; the
          // fallback for a non-Error rejection is copy.
          const msg = err instanceof Error ? err.message : t("automations:failedToDeleteRun");
          toast.error(msg);
          // revert on failure
          listAutomationRuns(automationId)
            .then((result) => setRuns(automationId, result ?? []))
            .catch(() => {
              // The recovery refresh also failed — the store would
              // otherwise stay permanently missing this row even though
              // the delete never succeeded server-side. Fall back to
              // re-inserting just the run we know we removed.
              if (deletedRun) {
                restoreRun(automationId, deletedRun);
              }
              toast.error(t("automations:couldNotRefreshRuns"));
            });
        });
    },
    [automationId, removeRun, restoreRun, setRuns, storeApi, workspaceId],
  );

  const deleteAllRuns = useCallback(
    (runIds?: string[]) => {
      if (!automationId) return;
      executeDeleteAll(automationId, workspaceId, runIds, {
        epoch,
        clearRuns,
        removeRun,
        setRuns,
        getRuns: () => storeApi.getState().automationRuns.byAutomationId[automationId] ?? [],
      });
    },
    [automationId, clearRuns, removeRun, setRuns, storeApi, workspaceId],
  );

  return { runs, loading, refresh, deleteRun, deleteAllRuns };
}
