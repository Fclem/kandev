import type { StateCreator } from "zustand";
import type { PluginsSlice } from "./types";

type SliceCreator = StateCreator<PluginsSlice, [["zustand/immer", never]], [], PluginsSlice>;

export function createPluginStarCountActions(
  set: Parameters<SliceCreator>[0],
  get: Parameters<SliceCreator>[1],
): Pick<PluginsSlice, "seedPluginStarCount" | "applyPluginStarMutationResult"> {
  return {
    seedPluginStarCount: (repository, count) => {
      if (count === null || get().pluginStars.repositoryCounts[repository] !== undefined) return;
      set((draft) => {
        draft.pluginStars.repositoryCounts[repository] = count;
      });
    },

    applyPluginStarMutationResult: (key, epoch, repository, value) => {
      const stars = get().pluginStars;
      if (stars.epochs[key] !== epoch || !stars.busy[key]) return false;
      set((draft) => {
        const current = draft.pluginStars;
        const previous = current.values[key];
        const count = current.repositoryCounts[repository];
        if (typeof previous === "boolean" && previous !== value && count !== undefined) {
          current.repositoryCounts[repository] = Math.max(0, count + (value ? 1 : -1));
        }
        current.values[key] = value;
        // Consuming the epoch prevents a repeated completion from applying twice.
        delete current.epochs[key];
        delete current.pending[key];
        delete current.busy[key];
      });
      return true;
    },
  };
}
