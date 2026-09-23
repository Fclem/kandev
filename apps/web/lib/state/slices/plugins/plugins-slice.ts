import type { SliceCreator } from "@/lib/state/slice-creator";
import type { Draft } from "immer";
import type { PluginsSlice, PluginsSliceState, PluginStarsState } from "./types";
import { pluginStarWorkspacePrefix } from "@/lib/plugins/plugin-star-connection";

export const defaultPluginStarsState: PluginStarsState = {
  connections: {},
  values: {},
  epochs: {},
  pending: {},
  busy: {},
};

export const defaultPluginsState: PluginsSliceState = {
  plugins: { items: [], loading: false, loaded: false, error: null },
  pluginStars: defaultPluginStarsState,
};

type PluginsSliceCreator = SliceCreator<PluginsSlice>;

type ImmerSet = Parameters<PluginsSliceCreator>[0];
type SliceGet = Parameters<PluginsSliceCreator>[1];

/** Drops every star entry of one workspace; keys of other scopes stay cached. */
function clearWorkspacePluginStars(stars: Draft<PluginStarsState>, workspaceId: string): void {
  const prefix = pluginStarWorkspacePrefix(workspaceId);
  const records: Array<Record<string, unknown>> = [
    stars.values,
    stars.epochs,
    stars.pending,
    stars.busy,
  ];
  for (const record of records) {
    for (const key of Object.keys(record)) {
      if (key.startsWith(prefix)) delete record[key];
    }
  }
}

function createPluginStarActions(
  set: ImmerSet,
  get: SliceGet,
): Pick<
  PluginsSlice,
  | "beginPluginStarConnectionRequest"
  | "applyPluginStarConnection"
  | "resetPluginStarConnection"
  | "beginPluginStarRequest"
  | "applyPluginStarResult"
  | "failPluginStarRequest"
> {
  return {
    beginPluginStarConnectionRequest: (workspaceId, userId) => {
      const entry = get().pluginStars.connections[workspaceId];
      const epoch = (entry?.epoch ?? 0) + 1;
      // A different authenticated user owns a different connection: drop the
      // previous tuple and its star states before the new read so the surface
      // never shows, or acts on, another account's state.
      const identityChanged = (entry?.userId ?? null) !== userId;
      set((draft) => {
        if (identityChanged) clearWorkspacePluginStars(draft.pluginStars, workspaceId);
        draft.pluginStars.connections[workspaceId] = {
          status: identityChanged ? null : (entry?.status ?? null),
          userId,
          loaded: identityChanged ? false : (entry?.loaded ?? false),
          loading: true,
          epoch,
        };
      });
      return epoch;
    },

    applyPluginStarConnection: (workspaceId, epoch, status, userId) => {
      const entry = get().pluginStars.connections[workspaceId];
      // A response whose scope was invalidated (or superseded) is discarded, as
      // is one that answers for a different authenticated user.
      if (!entry || entry.epoch !== epoch || (entry.userId ?? null) !== userId) return false;
      if (status && status.user_id !== userId) return false;
      set((draft) => {
        const stars = draft.pluginStars;
        const previous = entry.status;
        if (!status) {
          // A failed read keeps the last tuple and the states it scoped, so an
          // optional control never regresses on a transient failure.
          stars.connections[workspaceId] = {
            status: previous,
            userId,
            loaded: true,
            loading: false,
            epoch,
          };
          return;
        }
        const scopeChanged =
          previous !== null &&
          (previous.user_id !== status.user_id ||
            previous.credential_generation !== status.credential_generation ||
            previous.active !== status.active ||
            previous.capable !== status.capable ||
            previous.reconnect_required !== status.reconnect_required);
        if (scopeChanged) clearWorkspacePluginStars(stars, workspaceId);
        stars.connections[workspaceId] = { status, userId, loaded: true, loading: false, epoch };
      });
      return true;
    },

    resetPluginStarConnection: (workspaceId) =>
      set((draft) => {
        delete draft.pluginStars.connections[workspaceId];
        clearWorkspacePluginStars(draft.pluginStars, workspaceId);
      }),

    beginPluginStarRequest: (key, kind) => {
      const stars = get().pluginStars;
      if (kind === "status") {
        const value = stars.values[key];
        if (stars.pending[key] || (value !== undefined && value !== null)) return null;
      }
      const epoch = (stars.epochs[key] ?? 0) + 1;
      set((draft) => {
        draft.pluginStars.epochs[key] = epoch;
        if (kind === "mutation") {
          draft.pluginStars.busy[key] = true;
          delete draft.pluginStars.pending[key];
          return;
        }
        draft.pluginStars.pending[key] = true;
      });
      return epoch;
    },

    applyPluginStarResult: (key, epoch, value) => {
      if (get().pluginStars.epochs[key] !== epoch) return false;
      set((draft) => {
        draft.pluginStars.values[key] = value;
        delete draft.pluginStars.pending[key];
        delete draft.pluginStars.busy[key];
      });
      return true;
    },

    failPluginStarRequest: (key, epoch) => {
      if (get().pluginStars.epochs[key] !== epoch) return;
      set((draft) => {
        delete draft.pluginStars.pending[key];
        delete draft.pluginStars.busy[key];
      });
    },
  };
}

export const createPluginsSlice: SliceCreator<PluginsSlice> = (set: ImmerSet, get: SliceGet) => ({
  ...defaultPluginsState,
  ...createPluginStarActions(set, get),
  setPlugins: (plugins) =>
    set((draft) => {
      draft.plugins.items = plugins;
      draft.plugins.loaded = true;
      draft.plugins.error = null;
    }),
  setPluginsLoading: (loading) =>
    set((draft) => {
      draft.plugins.loading = loading;
    }),
  setPluginsError: (error) =>
    set((draft) => {
      draft.plugins.error = error;
    }),
  upsertPlugin: (plugin) =>
    set((draft) => {
      const idx = draft.plugins.items.findIndex((p) => p.id === plugin.id);
      if (idx >= 0) {
        draft.plugins.items[idx] = plugin;
      } else {
        draft.plugins.items.push(plugin);
      }
    }),
  removePlugin: (id) =>
    set((draft) => {
      draft.plugins.items = draft.plugins.items.filter((p) => p.id !== id);
    }),
});
