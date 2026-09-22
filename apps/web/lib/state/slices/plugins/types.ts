import type { PluginRecord } from "@/lib/types/plugins";
import type { GitHubStarConnectionStatus } from "@/lib/types/github";

export type PluginsState = {
  items: PluginRecord[];
  loading: boolean;
  loaded: boolean;
  error: string | null;
};

/**
 * A repository star state. `null` is a provider result the backend could not
 * resolve: unknown, never "unstarred".
 */
export type PluginStarValue = boolean | null;

export type PluginStarConnectionEntry = {
  /** The connection-status tuple, or null when the read failed or never landed. */
  status: GitHubStarConnectionStatus | null;
  loaded: boolean;
  loading: boolean;
  /** Bumped per read; a response is applied only while its epoch is current. */
  epoch: number;
};

/**
 * In-memory cache for Settings > Plugins star controls. Nothing here is
 * persisted: GitHub owns star state, and every entry is scoped to the
 * workspace, authenticated user, credential generation, and connection
 * capability that produced it.
 */
export type PluginStarsState = {
  /** Connection status per workspace; every surface shares one read. */
  connections: Record<string, PluginStarConnectionEntry>;
  /** Confirmed star states keyed by the scoped key from `pluginStarKey`. */
  values: Record<string, PluginStarValue>;
  /** Per-key request epoch, bumped before each request that must fence its response. */
  epochs: Record<string, number>;
  /** Keys with an in-flight status read; keeps sibling rows from re-requesting. */
  pending: Record<string, true>;
  /** Keys with an in-flight mutation; every mounted row for the key disables. */
  busy: Record<string, true>;
};

export type PluginsSliceState = {
  plugins: PluginsState;
  pluginStars: PluginStarsState;
};

export type PluginsSliceActions = {
  setPlugins: (plugins: PluginRecord[]) => void;
  setPluginsLoading: (loading: boolean) => void;
  setPluginsError: (error: string | null) => void;
  upsertPlugin: (plugin: PluginRecord) => void;
  removePlugin: (id: string) => void;

  /** Opens a connection-status read for `workspaceId` and returns its epoch. */
  beginPluginStarConnectionRequest: (workspaceId: string) => number;
  /**
   * Commits a connection-status read whose epoch is still current. A null
   * status (failed read) keeps the previous tuple so an optional control never
   * regresses on a transient failure; an accepted tuple whose scope changed
   * drops the prior scope's visible states before a new hydration.
   */
  applyPluginStarConnection: (
    workspaceId: string,
    epoch: number,
    status: GitHubStarConnectionStatus | null,
  ) => boolean;
  /** Drops one workspace's connection tuple and every visible star state. */
  resetPluginStarConnection: (workspaceId: string) => void;
  /**
   * Opens a request for `key`, bumping its epoch. Returns null when the request
   * must not be sent: a status read already in flight, or a confirmed
   * non-unknown state that only a refresh would replace.
   */
  beginPluginStarRequest: (key: string, kind: "status" | "mutation") => number | null;
  /** Commits a response only while `epoch` is still the key's epoch. */
  applyPluginStarResult: (key: string, epoch: number, value: PluginStarValue) => boolean;
  /** Releases an in-flight marker without touching the visible state. */
  failPluginStarRequest: (key: string, epoch: number) => void;
};

export type PluginsSlice = PluginsSliceState & PluginsSliceActions;
