"use client";

import { useEffect, useCallback } from "react";
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
 * Shared revert for both delete-all paths: on failure, refresh from the
 * server (authoritative — it drops whatever actually succeeded); if that also
 * fails, fall back to the pre-delete snapshot so the store is never left
 * permanently missing rows the delete never removed.
 */
function revertAfterFailedDelete(
  automationId: string,
  fallbackRuns: AutomationRun[],
  setRuns: (automationId: string, runs: AutomationRun[]) => void,
): void {
  listAutomationRuns(automationId)
    .then((result) => setRuns(automationId, result ?? []))
    .catch(() => {
      setRuns(automationId, fallbackRuns);
      toast.error(t("automations:couldNotRefreshRuns"));
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

  useEffect(() => {
    if (!automationId || loading) return;
    setRunsLoading(automationId, true);
    listAutomationRuns(automationId)
      .then((result) => {
        setRuns(automationId, result ?? []);
      })
      .catch(() => {
        setRuns(automationId, []);
      })
      .finally(() => {
        setRunsLoading(automationId, false);
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [automationId]);

  const refresh = useCallback(() => {
    if (!automationId) return;
    setRunsLoading(automationId, true);
    listAutomationRuns(automationId)
      .then((result) => {
        setRuns(automationId, result ?? []);
      })
      .catch(() => {})
      .finally(() => {
        setRunsLoading(automationId, false);
      });
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
      // An empty scope is a no-op: the UI never offers delete-all for an
      // empty view, and falling back to the all-runs delete here would be
      // a dangerous surprise for a caller that computed zero ids.
      if (runIds && runIds.length === 0) return;
      if (!runIds) {
        // Unscoped delete-all: remove every run for the automation. Snapshot
        // the full list so we can restore it if both the delete-all and the
        // recovery refresh below fail.
        const previousRuns = storeApi.getState().automationRuns.byAutomationId[automationId] ?? [];
        clearRuns(automationId); // optimistic
        deleteAllAutomationRuns(automationId, workspaceId)
          .then(() => {
            // See deleteRun: guard against an in-flight refresh() resurrecting
            // rows between the optimistic clear and this success callback.
            clearRuns(automationId);
          })
          .catch((err: unknown) => {
            const msg = err instanceof Error ? err.message : t("automations:failedToDeleteRuns");
            toast.error(msg);
            // revert on failure
            revertAfterFailedDelete(automationId, previousRuns, setRuns);
          });
        return;
      }
      // Status-scoped delete-all: remove exactly the given runs through the
      // per-run API. Archived/Cancelled are read-time-derived statuses that
      // only exist in the loaded payload, so the visible set is the only
      // exact scope. Snapshot the full list for the double-failure restore.
      const ids = [...new Set(runIds)];
      const previousRuns = storeApi.getState().automationRuns.byAutomationId[automationId] ?? [];
      ids.forEach((id) => removeRun(automationId, id)); // optimistic
      Promise.all(ids.map((id) => deleteAutomationRun(id, workspaceId)))
        .then(() => {
          // In-flight refresh guard, same as deleteRun and the unscoped path.
          ids.forEach((id) => removeRun(automationId, id));
        })
        .catch((err: unknown) => {
          // One toast and one recovery refresh for the whole batch, not one
          // per failed delete; the refresh returns the server's authoritative
          // list, which drops whatever actually succeeded.
          const msg = err instanceof Error ? err.message : t("automations:failedToDeleteRuns");
          toast.error(msg);
          revertAfterFailedDelete(automationId, previousRuns, setRuns);
        });
    },
    [automationId, clearRuns, removeRun, setRuns, storeApi, workspaceId],
  );

  return { runs, loading, refresh, deleteRun, deleteAllRuns };
}
