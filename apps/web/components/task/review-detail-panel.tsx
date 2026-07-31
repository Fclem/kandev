"use client";

import { useEffect, useMemo, useSyncExternalStore } from "react";
import { PRDetailPanelComponent } from "@/components/github/pr-detail-panel";
import { MRDetailPanelComponent } from "@/components/gitlab/mr-detail-panel";
import { useAppStore } from "@/components/state-provider";
import { useTaskMRs } from "@/hooks/domains/gitlab/use-task-mr";
import { usePluginRegistry } from "@/lib/plugins/registry";
import type { PluginReviewProviderRegistration } from "@/lib/plugins/registry";
import { resolveReviewKey, resolveReviewPanelProvider } from "./review-panel-provider";

function useReviewProviderVersion(
  provider: PluginReviewProviderRegistration | undefined,
  taskId: string | null,
): number {
  const source = useMemo(() => {
    let version = 0;
    return {
      getSnapshot: () => version,
      subscribe: (listener: () => void) => {
        if (!provider || !taskId) return () => {};
        return provider.subscribe(taskId, () => {
          version += 1;
          listener();
        });
      },
    };
  }, [provider, taskId]);
  return useSyncExternalStore(source.subscribe, source.getSnapshot, source.getSnapshot);
}

function RegisteredReviewPanel({
  panelId,
  provider,
  taskId,
  workspaceId,
  sessionId,
  reviewKey,
  presentation,
}: {
  panelId: string;
  provider: PluginReviewProviderRegistration;
  taskId: string;
  workspaceId: string;
  sessionId: string | null;
  reviewKey: string;
  presentation: "desktop" | "mobile";
}) {
  const version = useReviewProviderVersion(provider, taskId);
  const items = provider.getSnapshot(taskId);
  const selected = items.find((item) => item.reviewKey === reviewKey);

  useEffect(() => {
    const controller = new AbortController();
    void provider.refresh(taskId, controller.signal).catch(() => {
      // Provider's selector/empty state owns user-facing refresh errors.
    });
    return () => controller.abort();
  }, [provider, taskId]);

  if (!selected) {
    const EmptyState = provider.EmptyState;
    return EmptyState ? <EmptyState /> : <ReviewUnavailable />;
  }
  const ReviewPanel = provider.ReviewPanel;
  void version;
  return (
    <ReviewPanel
      panelId={panelId}
      presentation={presentation}
      workspaceId={workspaceId}
      taskId={taskId}
      sessionId={sessionId ?? undefined}
      reviewKey={reviewKey}
    />
  );
}

function ReviewUnavailable() {
  return (
    <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
      Review unavailable.
    </div>
  );
}

export function ReviewDetailPanelComponent({
  panelId,
  params,
  presentation = "desktop",
}: {
  panelId: string;
  params?: Record<string, unknown>;
  presentation?: "desktop" | "mobile";
}) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const sessionId = useAppStore((state) => state.tasks.activeSessionId);
  const registry = usePluginRegistry();
  const registryVersion = registry.getVersion();
  const hasGitHubPR = useAppStore((state) => {
    const prs = activeTaskId ? state.taskPRs.byTaskId[activeTaskId] : undefined;
    return Array.isArray(prs) && prs.length > 0;
  });
  const hasGitLabMR = useTaskMRs(activeTaskId).length > 0;
  const panelParams = params ?? {};
  const provider = resolveReviewPanelProvider(panelParams, hasGitHubPR, hasGitLabMR);
  const reviewKey = resolveReviewKey(panelParams);
  const registeredProvider = useMemo(
    () => (provider ? registry.getReviewProvider(provider) : undefined),
    [provider, registry, registryVersion],
  );

  if (registeredProvider && activeTaskId && workspaceId && reviewKey) {
    return (
      <RegisteredReviewPanel
        panelId={panelId}
        provider={registeredProvider}
        taskId={activeTaskId}
        workspaceId={workspaceId}
        sessionId={sessionId}
        reviewKey={reviewKey}
        presentation={presentation}
      />
    );
  }

  if (provider && provider !== "github" && provider !== "gitlab") return <ReviewUnavailable />;

  if (provider === "gitlab") {
    return <MRDetailPanelComponent panelId={panelId} params={{ mrKey: reviewKey }} />;
  }
  return <PRDetailPanelComponent panelId={panelId} params={{ prKey: reviewKey }} />;
}
