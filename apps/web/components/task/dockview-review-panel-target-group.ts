import type { DockviewApi } from "dockview-react";
import { CENTER_GROUP, isCenterCandidateGroupId } from "@/lib/state/layout-manager";

/** Resolve the center-group anchor shared by all auto-opened review panels. */
export function resolvePRPanelTargetGroup(
  api: DockviewApi,
  sessionId: string,
  centerGroupId: string,
): string {
  const sessionGroupId = api.getPanel(`session:${sessionId}`)?.group?.id;
  if (sessionGroupId && isCenterCandidateGroupId(sessionGroupId)) return sessionGroupId;
  return isCenterCandidateGroupId(centerGroupId) ? centerGroupId : CENTER_GROUP;
}
