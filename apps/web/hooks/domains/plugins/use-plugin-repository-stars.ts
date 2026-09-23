"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useOptionalAppStore, useOptionalAppStoreApi } from "@/components/state-provider";
import {
  fetchGitHubStarConnection,
  fetchGitHubStarStates,
  startGitHubStarConnection,
  starGitHubRepository,
  unstarGitHubRepository,
} from "@/lib/api/domains/github-api";
import { parseGitHubRepositoryUrl } from "@/lib/plugins/github-repository-url";
import {
  STAR_CONNECTION_OUTCOME_PARAM,
  buildPluginStarReturnPath,
  pluginStarKey,
  resolvePluginStarControlMode,
  type PluginStarControlMode,
} from "@/lib/plugins/plugin-star-connection";
import type { AppState } from "@/lib/state/store";
import type { PluginStarConnectionEntry, PluginStarsState } from "@/lib/state/slices/plugins/types";
import type { GitHubStarConnectionStatus } from "@/lib/types/github";

/** Stable fallback so a store-less render keeps a referentially stable snapshot. */
const EMPTY_PLUGIN_STARS: PluginStarsState = {
  connections: {},
  values: {},
  epochs: {},
  pending: {},
  busy: {},
};

const selectPluginStars = (state: AppState): PluginStarsState =>
  state.pluginStars ?? EMPTY_PLUGIN_STARS;

// Mirrors lib/ws/use-websocket.tsx: authentication-disabled installs act as the
// single default user, so their star cache never churns on identity changes.
const selectAuthenticatedUserId = (state: AppState): string | null =>
  state.auth.user?.id ?? (state.auth.mode === "disabled" ? "default-user" : null);

/**
 * Whether the cached connection tuple needs a fresh read: absent, never
 * loaded, or read for a different authenticated account. An entry loaded for
 * another user is stale even though it is loaded.
 */
function needsConnectionLoad(
  entry: PluginStarConnectionEntry | undefined,
  authUserId: string | null,
): boolean {
  if (!entry) return true;
  if (entry.userId !== authUserId) return true;
  return !entry.loading && !entry.loaded;
}

export type PluginRepositoryStar = {
  /** What the surface renders; `omit` means an unsupported deployment. */
  mode: PluginStarControlMode;
  /** Confirmed star state, or null while unknown: never rendered as unstarred. */
  starred: boolean | null;
  /** True while a star mutation is in flight for this repository. */
  busy: boolean;
  /** True while the start flow is redirecting the browser to GitHub. */
  starting: boolean;
  /** A capable connection that still needs re-consent keeps its star controls. */
  reconnectRequired: boolean;
  /**
   * Runs the dedicated start flow (first consent or replacement) and navigates
   * the browser to the returned authorization URL. `reconnect` is the same
   * flow; only the CTA label differs.
   */
  connect: () => void;
  reconnect: () => void;
  toggle: () => void;
};

type PluginStarViewInput = {
  stars: PluginStarsState;
  workspaceId: string | null | undefined;
  canonicalRepository: string | null;
  status: GitHubStarConnectionStatus | null;
};

/**
 * Resolves what one surface renders. Hydration is gated on the capable tuple
 * rather than on the CTA: an unknown or failed connection must never become an
 * asserted star state, and a non-canonical repository gets no control at all.
 */
function resolvePluginStarView({
  stars,
  workspaceId,
  canonicalRepository,
  status,
}: PluginStarViewInput): {
  mode: PluginStarControlMode;
  scopeKey: string | null;
  starred: boolean | null;
  busy: boolean;
} {
  const mode = canonicalRepository ? resolvePluginStarControlMode(status) : "omit";
  if (!workspaceId || !canonicalRepository || !status || mode !== "stars") {
    return { mode, scopeKey: null, starred: null, busy: false };
  }
  const scopeKey = pluginStarKey(workspaceId, status, canonicalRepository);
  return {
    mode,
    scopeKey,
    starred: stars.values[scopeKey] ?? null,
    busy: Boolean(stars.busy[scopeKey]),
  };
}

/**
 * Star state for one plugin repository on one surface.
 *
 * The connection-status read always happens first; repository states are only
 * requested once the tuple is capable, and only for the exact scope key it
 * produced. Status and mutation responses are fenced by that key's request
 * epoch, so a late response from a previous workspace, user, credential
 * generation, or capability scope can never populate the current one.
 */
export function usePluginRepositoryStar({
  workspaceId,
  repositoryUrl,
}: {
  workspaceId?: string | null;
  repositoryUrl?: string | null;
}): PluginRepositoryStar {
  const store = useOptionalAppStoreApi();
  const stars = useOptionalAppStore(selectPluginStars, EMPTY_PLUGIN_STARS);
  // The connection tuple belongs to one authenticated user: reading the
  // identity here is what invalidates the cache on an account switch.
  const authUserId = useOptionalAppStore(selectAuthenticatedUserId, null);
  const [starting, setStarting] = useState(false);
  const canonicalRepository = useMemo(
    () => parseGitHubRepositoryUrl(repositoryUrl),
    [repositoryUrl],
  );

  const entry = workspaceId ? stars.connections[workspaceId] : undefined;
  const status = entry?.status ?? null;
  const view = resolvePluginStarView({ stars, workspaceId, canonicalRepository, status });
  const { mode, scopeKey, starred, busy } = view;

  const loadConnection = useCallback(() => {
    if (!store || !workspaceId) return;
    const epoch = store.getState().beginPluginStarConnectionRequest(workspaceId, authUserId);
    fetchGitHubStarConnection(workspaceId, { cache: "no-store" })
      .then((response) =>
        store.getState().applyPluginStarConnection(workspaceId, epoch, response, authUserId),
      )
      // A failed read leaves the previous tuple and its states untouched.
      .catch(() =>
        store.getState().applyPluginStarConnection(workspaceId, epoch, null, authUserId),
      );
  }, [authUserId, store, workspaceId]);

  useEffect(() => {
    if (!store || !workspaceId) return;
    const current = store.getState().pluginStars?.connections[workspaceId];
    if (needsConnectionLoad(current, authUserId)) loadConnection();
  }, [
    authUserId,
    loadConnection,
    store,
    workspaceId,
    entry?.loaded,
    entry?.loading,
    entry?.userId,
  ]);

  const hydrate = useCallback(() => {
    if (!store || !scopeKey || !workspaceId || !repositoryUrl) return;
    const epoch = store.getState().beginPluginStarRequest(scopeKey, "status");
    if (epoch === null) return;
    fetchGitHubStarStates({ workspace_id: workspaceId, repository_urls: [repositoryUrl] })
      .then((response) => {
        const states = response.states ?? {};
        // A missing key is an unknown result, exactly like an explicit null.
        const value = states[canonicalRepository ?? ""] ?? states[repositoryUrl] ?? null;
        store.getState().applyPluginStarResult(scopeKey, epoch, value);
      })
      .catch(() => store.getState().failPluginStarRequest(scopeKey, epoch));
  }, [canonicalRepository, repositoryUrl, scopeKey, store, workspaceId]);

  useEffect(() => {
    hydrate();
  }, [hydrate]);

  const toggle = useCallback(() => {
    if (!store || !scopeKey || !workspaceId || !repositoryUrl || starred === null) return;
    const epoch = store.getState().beginPluginStarRequest(scopeKey, "mutation");
    if (epoch === null) return;
    const request = starred
      ? unstarGitHubRepository({ workspace_id: workspaceId, repository_url: repositoryUrl })
      : starGitHubRepository({ workspace_id: workspaceId, repository_url: repositoryUrl });
    request
      .then((response) => store.getState().applyPluginStarResult(scopeKey, epoch, response.starred))
      // A failed mutation restores the actionable state: nothing is optimistic.
      .catch(() => store.getState().failPluginStarRequest(scopeKey, epoch));
  }, [repositoryUrl, scopeKey, starred, store, workspaceId]);

  const connect = useCallback(() => {
    if (!store || !workspaceId || starting || mode === "omit") return;
    const returnPath = buildPluginStarReturnPath(
      typeof window === "undefined" ? "" : window.location.pathname,
      typeof window === "undefined" ? "" : window.location.search,
    );
    setStarting(true);
    startGitHubStarConnection({ workspace_id: workspaceId, return_path: returnPath })
      .then((response) => {
        if (typeof window === "undefined" || !response.authorization_url) {
          setStarting(false);
          return;
        }
        window.location.assign(response.authorization_url);
      })
      .catch(() => setStarting(false));
  }, [mode, starting, store, workspaceId]);

  return {
    mode,
    starred,
    busy,
    starting,
    reconnectRequired: Boolean(status?.reconnect_required),
    connect,
    // First consent and replacement are the same dedicated start flow.
    reconnect: connect,
    toggle,
  };
}

/**
 * Consumes the navigation-only OAuth outcome on a Settings > Plugins landing
 * URL: it removes the parameter and invalidates the cached connection tuples so
 * every mounted star control re-reads the connection before hydrating again.
 * The value itself is never trusted and never selects copy.
 */
export function usePluginStarConnectionOutcome(): void {
  const store = useOptionalAppStoreApi();

  useEffect(() => {
    if (!store || typeof window === "undefined") return;
    const params = new URLSearchParams(window.location.search);
    if (!params.has(STAR_CONNECTION_OUTCOME_PARAM)) return;

    params.delete(STAR_CONNECTION_OUTCOME_PARAM);
    const query = params.toString();
    window.history.replaceState(
      window.history.state,
      "",
      `${window.location.pathname}${query ? `?${query}` : ""}${window.location.hash}`,
    );
    const pluginStars = store.getState().pluginStars;
    for (const workspaceId of Object.keys(pluginStars?.connections ?? {})) {
      store.getState().resetPluginStarConnection(workspaceId);
    }
  }, [store]);
}
