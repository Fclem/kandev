import { listWorkspaces, fetchUserSettings } from "@/lib/api";
import { readCookies } from "@/lib/server/cookies";
import {
  ACTIVE_WORKSPACE_COOKIE,
  LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE,
  readScopedCookieStoreValue,
  resolveOfficeWorkspaceId,
} from "@/lib/routing/route-bootstrap";

/**
 * Server-side helper to resolve the active workspace ID.
 * Re-fetches workspaces and user settings (Next.js will deduplicate
 * these calls within the same request when the layout also fetches them).
 *
 * Priority order (canonical resolver):
 * 1. urlWorkspaceId when it matches a valid office workspace.
 * 2. General active-workspace cookie (scoped, then legacy) when it matches a
 *    valid office workspace.
 * 3. office-active-workspace cookie (scoped, then legacy) when it matches a
 *    valid office workspace.
 * 4. userSettings.workspace_id when it matches a valid office workspace.
 * 5. First available office workspace as fallback.
 *
 * Does NOT write to user settings - the caller must not pollute the shared
 * workspace_id that kanban uses.
 */
export async function getActiveWorkspaceId(urlWorkspaceId?: string): Promise<string | null> {
  const [workspacesRes, settingsRes, cookieStore] = await Promise.all([
    listWorkspaces({ cache: "no-store" }).catch(() => ({ workspaces: [] })),
    fetchUserSettings({ cache: "no-store" }).catch(() => null),
    readCookies().catch(() => null),
  ]);

  // Only consider office workspaces (those with office_workflow_id set).
  const workspaces = workspacesRes.workspaces.filter((w) => w.office_workflow_id);

  return resolveOfficeWorkspaceId(workspaces, {
    routeWorkspaceId: urlWorkspaceId,
    generalWorkspaceId: readScopedCookieStoreValue(cookieStore, ACTIVE_WORKSPACE_COOKIE),
    officeWorkspaceId: readScopedCookieStoreValue(
      cookieStore,
      LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE,
    ),
    settingsWorkspaceId: settingsRes?.settings?.workspace_id ?? null,
  });
}
