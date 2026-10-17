import type { StateCreator } from "zustand";
import type { SessionRuntimeSlice } from "./types";

export type ImmerSet = Parameters<
  StateCreator<SessionRuntimeSlice, [["zustand/immer", never]], [], SessionRuntimeSlice>
>[0];

export function buildSessionUsageTurnActions(set: ImmerSet) {
  return {
    setSessionUsageTurn: (
      sessionId: string,
      turn: Parameters<SessionRuntimeSlice["setSessionUsageTurn"]>[1],
      refreshedInvalidation: number,
    ) =>
      set((draft) => {
        const index = draft.usageTurns.bySessionId[sessionId];
        if (!index) return;
        const generations = (index.refreshedInvalidationByTurnId ??= {});
        if ((generations[turn.turn_id] ?? 0) > refreshedInvalidation) return;
        index.byTurnId[turn.turn_id] = turn;
        generations[turn.turn_id] = refreshedInvalidation;
      }),
    setSessionUsageTurnPage: (
      sessionId: Parameters<SessionRuntimeSlice["setSessionUsageTurnPage"]>[0],
      page: Parameters<SessionRuntimeSlice["setSessionUsageTurnPage"]>[1],
      mode: Parameters<SessionRuntimeSlice["setSessionUsageTurnPage"]>[2] = "append",
      refreshedInvalidation?: Parameters<SessionRuntimeSlice["setSessionUsageTurnPage"]>[3],
    ) =>
      set((draft) => {
        const index =
          draft.usageTurns.bySessionId[sessionId] ??
          (draft.usageTurns.bySessionId[sessionId] = {
            byTurnId: {},
            exhausted: false,
          });
        const hadPagination = index.exhausted || index.nextCursor !== undefined;
        const generations = (index.refreshedInvalidationByTurnId ??= {});
        for (const usageTurn of page.turns) {
          const generation = refreshedInvalidation ?? 0;
          if ((generations[usageTurn.turn_id] ?? 0) > generation) continue;
          index.byTurnId[usageTurn.turn_id] = usageTurn;
          generations[usageTurn.turn_id] = generation;
        }
        if (mode === "refresh" && refreshedInvalidation !== undefined) {
          index.refreshedInvalidation = refreshedInvalidation;
        }
        // A refresh only merges the newest page: previously loaded turns (possibly pushed
        // to a deeper page) are retained, and an existing cursor/exhausted state is kept.
        if (mode === "refresh" && hadPagination) return;
        index.nextCursor = page.next_cursor;
        if (page.turns.length === 0 || !page.next_cursor) index.exhausted = true;
      }),
  };
}
