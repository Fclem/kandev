import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { markPRPanelOffered, wasPRPanelOffered } from "@/lib/local-storage";
import { focusOrAddPanel } from "@/lib/state/dockview-layout-builders";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useNormalizedTaskReviews } from "./review-panel-provider";
import { resolvePRPanelTargetGroup } from "./dockview-review-panel-target-group";

const BUILT_IN_REVIEW_PROVIDERS = new Set(["github", "gitlab"]);

/** Auto-open one registered provider review when no built-in review owns the slot. */
export function useAutoRegisteredReviewPanel() {
  const taskId = useAppStore((s) => s.tasks.activeTaskId);
  const sessionId = useAppStore((s) => s.tasks.activeSessionId);
  const reviews = useNormalizedTaskReviews(taskId);
  const pluginReview = reviews.find((review) => !BUILT_IN_REVIEW_PROVIDERS.has(review.providerId));
  const hasBuiltInReview = reviews.some((review) =>
    BUILT_IN_REVIEW_PROVIDERS.has(review.providerId),
  );
  const hasApi = useDockviewStore((s) => !!s.api);

  useEffect(() => {
    if (!taskId || !sessionId || !hasApi) return;
    const api = useDockviewStore.getState().api;
    if (!api) return;
    const panel = api.getPanel("review-detail");
    if (!pluginReview || hasBuiltInReview) {
      panel?.api.close();
      return;
    }
    if (panel) {
      if (
        panel.params?.providerId !== pluginReview.providerId ||
        panel.params?.reviewKey !== pluginReview.reviewKey
      ) {
        panel.api.updateParameters({
          providerId: pluginReview.providerId,
          reviewKey: pluginReview.reviewKey,
        });
      }
      return;
    }
    if (wasPRPanelOffered(sessionId)) return;
    focusOrAddPanel(api, {
      id: "review-detail",
      component: "review-detail",
      title: pluginReview.title,
      position: {
        referenceGroup: resolvePRPanelTargetGroup(
          api,
          sessionId,
          useDockviewStore.getState().centerGroupId,
        ),
      },
      inactive: true,
      params: { providerId: pluginReview.providerId, reviewKey: pluginReview.reviewKey },
    });
    markPRPanelOffered(sessionId);
  }, [hasApi, hasBuiltInReview, pluginReview, sessionId, taskId]);
}
