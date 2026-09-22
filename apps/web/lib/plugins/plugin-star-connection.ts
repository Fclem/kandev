import type { GitHubStarConnectionStatus } from "@/lib/types/github";

/**
 * Pure model for the dedicated personal GitHub star connection.
 *
 * Everything here derives from the connection-status tuple the backend returns:
 * the UI never infers capability, and a repository star state is only asserted
 * from a confirmed status or mutation response.
 */

/** How a plugin repository surface renders its star affordance. */
export type PluginStarControlMode = "omit" | "connect" | "reconnect" | "stars";

/** Settings > Plugins, the only surface a dedicated star flow may return to. */
export const PLUGINS_SETTINGS_PATH = "/settings/plugins";

/** Navigation-only OAuth outcome parameter; its value is never trusted. */
export const STAR_CONNECTION_OUTCOME_PARAM = "star_connection_outcome";

/** Field separator for composite star keys; never valid inside an id. */
const KEY_SEPARATOR = "\u0000";

/**
 * The control the connection-status tuple maps to.
 *
 * `omit` is the unsupported-deployment case: no control, no CTA, and no start
 * request. Capable-but-reconnect-required keeps actionable star controls and
 * adds the reconnect CTA separately. Every tuple the truth table does not list
 * falls back to the non-assertive `connect` CTA.
 */
export function resolvePluginStarControlMode(
  status: GitHubStarConnectionStatus | null | undefined,
): PluginStarControlMode {
  if (!status || !status.available) return "omit";
  if (status.active && status.capable) return "stars";
  if (!status.capable && status.reconnect_required) return "reconnect";
  return "connect";
}

/**
 * Cache key for one repository inside one connection scope. The full
 * workspace/user/generation/capability tuple is part of the key, so a response
 * that lands after a workspace, user, generation, or capability change can only
 * ever address its own stale key.
 */
export function pluginStarKey(
  workspaceId: string,
  status: GitHubStarConnectionStatus,
  canonicalRepository: string,
): string {
  return [
    workspaceId,
    status.user_id,
    String(status.credential_generation),
    status.active ? "1" : "0",
    status.capable ? "1" : "0",
    status.reconnect_required ? "1" : "0",
    canonicalRepository,
  ].join(KEY_SEPARATOR);
}

/** Key prefix that selects every star entry of one workspace. */
export function pluginStarWorkspacePrefix(workspaceId: string): string {
  return `${workspaceId}${KEY_SEPARATOR}`;
}

/**
 * The same-origin return path a dedicated start request may redirect back to.
 * Anything outside Settings > Plugins collapses to the plugins page itself, and
 * a previous outcome parameter is dropped, so the payload can never carry an
 * external origin, scheme, fragment, or replayed outcome.
 */
export function buildPluginStarReturnPath(pathname: string, search: string): string {
  const searchParams = new URLSearchParams(search);
  searchParams.delete(STAR_CONNECTION_OUTCOME_PARAM);
  const query = searchParams.toString();
  const suffix = pathname.slice(PLUGINS_SETTINGS_PATH.length);
  const insidePluginsSettings =
    pathname.startsWith(PLUGINS_SETTINGS_PATH) &&
    (suffix === "" || suffix.startsWith("/")) &&
    !pathname.includes("//") &&
    !pathname.includes("..");
  const path = insidePluginsSettings ? pathname : PLUGINS_SETTINGS_PATH;
  return query ? `${path}?${query}` : path;
}
