import { useEffect, useMemo, useSyncExternalStore } from "react";
import { useAppStore } from "@/components/state-provider";
import { mrTaskKey } from "@/components/gitlab/mr-detail-panel";
import { prTaskKey } from "@/components/github/pr-utils";
import { usePluginRegistry, type PluginReviewProviderRegistration } from "@/lib/plugins/registry";
import type { ReviewItemSummary } from "@/lib/plugins/types";
import type { TaskMR } from "@/lib/types/gitlab";
import type { TaskPR } from "@/lib/types/github";

/** Provider IDs are registry-owned strings; GitHub and GitLab are built-in adapters. */
export type ReviewProvider = string;

const EMPTY_GITHUB_REVIEWS: TaskPR[] = [];
const EMPTY_GITLAB_REVIEWS: TaskMR[] = [];

function githubReviewItem(pr: TaskPR): ReviewItemSummary {
  return {
    providerId: "github",
    reviewKey: prTaskKey(pr),
    title: pr.pr_title || `Pull Request #${pr.pr_number}`,
    url: pr.pr_url,
    repositoryId: pr.repository_id || `${pr.owner}/${pr.repo}`,
    state: pr.state,
    ...(pr.checks_state ? { statusBadge: { label: pr.checks_state } } : {}),
  };
}

function gitLabReviewItem(mr: TaskMR): ReviewItemSummary {
  return {
    providerId: "gitlab",
    reviewKey: mrTaskKey(mr),
    title: mr.mr_title || `Merge Request !${mr.mr_iid}`,
    url: mr.mr_url,
    repositoryId: mr.repository_id || mr.project_path,
    state: mr.state,
    ...(mr.pipeline_state ? { statusBadge: { label: mr.pipeline_state } } : {}),
  };
}

function useProviderUpdates(
  taskId: string | null,
  providers: PluginReviewProviderRegistration[],
): number {
  const source = useMemo(() => {
    let version = 0;
    return {
      getSnapshot: () => version,
      subscribe: (listener: () => void) => {
        if (!taskId) return () => {};
        return combineUnsubscribers(
          providers.map((provider) =>
            provider.subscribe(taskId, () => {
              version += 1;
              listener();
            }),
          ),
        );
      },
    };
  }, [taskId, providers]);
  // Version snapshots stay stable even when a provider allocates an array on
  // every getSnapshot call, which is permitted by the plugin boundary.
  return useSyncExternalStore(source.subscribe, source.getSnapshot, source.getSnapshot);
}

function combineUnsubscribers(unsubscribers: Array<() => void>): () => void {
  return () => unsubscribers.forEach((unsubscribe) => unsubscribe());
}

/**
 * Single normalized selector for built-in and registered reviews. Plugin
 * providers refresh through their external-store subscription; their panels
 * remain revocable because registry changes re-render this hook.
 */
export function useNormalizedTaskReviews(taskId: string | null): readonly ReviewItemSummary[] {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const prs = useAppStore((state) =>
    taskId ? (state.taskPRs.byTaskId[taskId] ?? EMPTY_GITHUB_REVIEWS) : EMPTY_GITHUB_REVIEWS,
  );
  const mrs = useAppStore((state) =>
    taskId && workspaceId
      ? (state.taskMRs.byWorkspaceId[workspaceId]?.[taskId] ?? EMPTY_GITLAB_REVIEWS)
      : EMPTY_GITLAB_REVIEWS,
  );
  const registry = usePluginRegistry();
  const registryVersion = registry.getVersion();
  const providers = useMemo(() => registry.getReviewProviders(), [registry, registryVersion]);
  const providerVersion = useProviderUpdates(taskId, providers);

  useEffect(() => {
    if (!taskId) return;
    const controller = new AbortController();
    providers.forEach((provider) => {
      void provider.refresh(taskId, controller.signal).catch(() => {
        // Provider renders its own empty/error state.
      });
    });
    return () => controller.abort();
  }, [providers, taskId]);

  return useMemo(
    () => [
      ...prs.map(githubReviewItem),
      ...mrs.map(gitLabReviewItem),
      ...providers.flatMap((provider) => provider.getSnapshot(taskId ?? "")),
    ],
    [mrs, prs, providers, providerVersion, taskId],
  );
}

export function resolveReviewPanelProvider(
  params: {
    providerId?: unknown;
    provider?: unknown;
    reviewKey?: unknown;
    prKey?: unknown;
    mrKey?: unknown;
  },
  hasGitHubPR: boolean,
  hasGitLabMR: boolean,
): ReviewProvider | null {
  if (typeof params.providerId === "string" && params.providerId) return params.providerId;
  if (params.provider === "gitlab" || typeof params.mrKey === "string") return "gitlab";
  if (params.provider === "github" || typeof params.prKey === "string") return "github";
  if (hasGitHubPR) return "github";
  if (hasGitLabMR) return "gitlab";
  return null;
}

/** Normalize new provider-neutral params while retaining every saved-layout alias. */
export function resolveReviewKey(params: {
  reviewKey?: unknown;
  prKey?: unknown;
  mrKey?: unknown;
}): string | undefined {
  if (typeof params.reviewKey === "string") return params.reviewKey;
  if (typeof params.prKey === "string") return params.prKey;
  if (typeof params.mrKey === "string") return params.mrKey;
  return undefined;
}
