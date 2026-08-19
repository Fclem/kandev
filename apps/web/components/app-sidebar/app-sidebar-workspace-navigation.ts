import {
  ACTIVE_WORKSPACE_COOKIE,
  LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE,
  scopedCookieName,
} from "@/lib/routing/route-bootstrap";
import { isOfficeWorkspace, type ModeWorkspace } from "@/lib/state/slices/workspace/selectors";

export { workspaceHomeHref } from "@/lib/navigation/workspace-home";

const ACTIVE_WORKSPACE_COOKIE_MAX_AGE = 31536000;

/**
 * Records a workspace as the active one — one write per workspace change.
 * Callers must not write these cookies themselves: split writes are how the
 * active-workspace record and the legacy office cookie drift out of step.
 */
export function rememberWorkspaceSelection(workspace: ModeWorkspace | undefined): void {
  if (!workspace) return;
  rememberWorkspaceSelectionById(workspace.id, isOfficeWorkspace(workspace) ? "office" : "kanban");
}

/**
 * The same write for a caller that knows the kind but does not hold a workspace
 * record — the setup wizard, whose create response returns an id and nothing
 * else. Passing a fabricated record with an invented `office_workflow_id` would
 * be a lie the type system happily accepts.
 */
export function rememberWorkspaceSelectionById(id: string, kind: "office" | "kanban"): void {
  if (!id || typeof document === "undefined") return;
  writeWorkspaceCookie(ACTIVE_WORKSPACE_COOKIE, id);
  // The office boot path (`src/office-routes.tsx`) reads the office-family
  // cookie (scoped name first, legacy unprefixed name as read-only fallback)
  // to pick an office workspace when the unified one names a kanban board, so
  // it is kept in step here. Only the port-scoped names are ever written.
  if (kind === "office") {
    writeWorkspaceCookie(LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE, id);
  }
}

function writeWorkspaceCookie(name: string, value: string): void {
  if (typeof document === "undefined") return;
  const scoped = scopedCookieName(name);
  document.cookie = `${scoped}=${encodeURIComponent(value)}; path=/; max-age=${ACTIVE_WORKSPACE_COOKIE_MAX_AGE}; samesite=strict`;
  // A pre-upgrade jar can still carry the legacy unprefixed cookie, which the
  // scoped-first readers (frontend and boot-state) consult as read-only
  // fallback. Once any instance records its own scoped selection, the shared
  // legacy value must be expired: it would otherwise keep every instance
  // without its own scoped cookie pinned to the last pre-upgrade selection —
  // the cross-instance bleed this isolation fix removes. No-op when the port
  // is empty (scoped == legacy name; the write above already recorded it).
  if (scoped !== name) {
    document.cookie = `${name}=; path=/; max-age=0; samesite=strict`;
  }
}
