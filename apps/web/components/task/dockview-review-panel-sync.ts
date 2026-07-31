"use client";

import { useEffect } from "react";
import type { DockviewApi } from "dockview-react";
import { prTaskKey } from "@/components/github/pr-utils";
import { mrTaskKey } from "@/components/gitlab/mr-detail-panel";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getPrimaryTaskPR } from "@/hooks/domains/github/use-task-pr";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { ReviewItemSummary } from "@/lib/plugins/types";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";
import { useNormalizedTaskReviews } from "./review-panel-provider";

export type CanonicalReviewParams = {
  providerId: string | undefined;
  provider: "github" | "gitlab" | undefined;
  reviewKey: string | undefined;
  prKey: string | undefined;
  mrKey: string | undefined;
};

export function resolveCanonicalReviewParams(
  prs: TaskPR[] | undefined,
  mrs: TaskMR[] | undefined,
  registeredReviews: readonly ReviewItemSummary[] = [],
): CanonicalReviewParams {
  const pr = getPrimaryTaskPR(prs);
  if (pr) {
    const key = prTaskKey(pr);
    return {
      providerId: "github",
      provider: "github",
      reviewKey: key,
      prKey: key,
      mrKey: undefined,
    };
  }

  const mr = mrs?.[0];
  if (mr) {
    const key = mrTaskKey(mr);
    return {
      providerId: "gitlab",
      provider: "gitlab",
      reviewKey: key,
      prKey: undefined,
      mrKey: key,
    };
  }

  const registered = registeredReviews.find(
    (review) => review.providerId !== "github" && review.providerId !== "gitlab",
  );
  if (registered) {
    return {
      providerId: registered.providerId,
      provider: undefined,
      reviewKey: registered.reviewKey,
      prKey: undefined,
      mrKey: undefined,
    };
  }

  return {
    providerId: undefined,
    provider: undefined,
    reviewKey: undefined,
    prKey: undefined,
    mrKey: undefined,
  };
}

function hasSameReviewParams(
  current: Record<string, unknown> | undefined,
  next: CanonicalReviewParams,
): boolean {
  return (
    current?.providerId === next.providerId &&
    current?.provider === next.provider &&
    current?.reviewKey === next.reviewKey &&
    current?.prKey === next.prKey &&
    current?.mrKey === next.mrKey
  );
}

/**
 * Update only the review identity of a layout-owned PR Details panel.
 *
 * Layout profile and task-layout restoration own panel existence and position.
 * This helper deliberately never calls add, close, move, or activate APIs.
 */
export function syncCanonicalReviewPanel(api: DockviewApi, next: CanonicalReviewParams): boolean {
  const panel = api.getPanel("pr-detail");
  if (!panel || hasSameReviewParams(panel.params, next)) return false;
  panel.api.updateParameters(next);
  return true;
}

function reviewIdentity(params: CanonicalReviewParams): string {
  return `${params.providerId ?? "none"}:${params.reviewKey ?? ""}`;
}

/** Keep an existing canonical PR Details panel in sync with the active task. */
export function useSyncReviewPanel() {
  const appStore = useAppStoreApi();
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const reviews = useNormalizedTaskReviews(taskId);
  const registeredReview = reviews.find(
    (review) => review.providerId !== "github" && review.providerId !== "gitlab",
  );
  const identity = useAppStore((state) => {
    if (!taskId || !workspaceId) return "none";
    return reviewIdentity(
      resolveCanonicalReviewParams(
        state.taskPRs.byTaskId[taskId],
        state.taskMRs.byWorkspaceId[workspaceId]?.[taskId],
        registeredReview ? [registeredReview] : [],
      ),
    );
  });
  const hasApi = useDockviewStore((state) => !!state.api);

  useEffect(() => {
    if (!taskId || !workspaceId || !hasApi) return;

    let innerFrame: number | null = null;
    const outerFrame = requestAnimationFrame(() => {
      innerFrame = requestAnimationFrame(() => {
        const live = appStore.getState();
        if (live.tasks.activeTaskId !== taskId || live.workspaces.activeId !== workspaceId) return;

        const api = useDockviewStore.getState().api;
        if (!api) return;
        syncCanonicalReviewPanel(
          api,
          resolveCanonicalReviewParams(
            live.taskPRs.byTaskId[taskId],
            live.taskMRs.byWorkspaceId[workspaceId]?.[taskId],
            registeredReview ? [registeredReview] : [],
          ),
        );
      });
    });

    return () => {
      cancelAnimationFrame(outerFrame);
      if (innerFrame !== null) cancelAnimationFrame(innerFrame);
    };
  }, [appStore, hasApi, identity, registeredReview, taskId, workspaceId]);
}
