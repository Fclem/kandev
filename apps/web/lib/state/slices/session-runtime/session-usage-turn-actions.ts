import type { StateCreator } from "zustand";
import type { SessionRuntimeSlice } from "./types";

export type ImmerSet = Parameters<
  StateCreator<SessionRuntimeSlice, [["zustand/immer", never]], [], SessionRuntimeSlice>
>[0];

export function buildSessionUsageTurnActions(set: ImmerSet) {
  return {
    setSessionUsageTurnPage: (
      sessionId: Parameters<SessionRuntimeSlice["setSessionUsageTurnPage"]>[0],
      page: Parameters<SessionRuntimeSlice["setSessionUsageTurnPage"]>[1],
      mode: Parameters<SessionRuntimeSlice["setSessionUsageTurnPage"]>[2] = "append",
    ) =>
      set((draft) => {
        const index =
          draft.usageTurns.bySessionId[sessionId] ??
          (draft.usageTurns.bySessionId[sessionId] = {
            byTurnId: {},
            newestPageTurnIds: [],
            exhausted: false,
          });
        const hadPagination = index.exhausted || index.nextCursor !== undefined;
        if (mode === "refresh") {
          index.newestPageTurnIds = page.turns.map((turn) => turn.turn_id);
        }
        for (const usageTurn of page.turns) index.byTurnId[usageTurn.turn_id] = usageTurn;
        if (mode === "append" && index.newestPageTurnIds.length === 0) {
          index.newestPageTurnIds = page.turns.map((turn) => turn.turn_id);
        }
        // A refresh only merges the newest page: previously loaded turns (possibly pushed
        // to a deeper page) are retained, and an existing cursor/exhausted state is kept.
        if (mode === "refresh" && hadPagination) return;
        index.nextCursor = page.next_cursor;
        if (page.turns.length === 0 || !page.next_cursor) index.exhausted = true;
      }),
  };
}
