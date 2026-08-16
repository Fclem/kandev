import { listSessionTurns } from "@/lib/api/domains/session-api";
import { useAppStoreApi } from "@/components/state-provider";
import { createDebugLogger } from "@/lib/debug/log";

const debug = createDebugLogger("messages:fetch:turns");

// Single-flight promises for in-progress turn hydration per session. The
// loaded marker lives in the store (`turns.loadedBySession`) because it must
// be seeded by SSR hydration too; this module map only deduplicates requests
// racing while the first is still in flight.
const inFlightTurnsLoad = new Map<string, Promise<void>>();

/** Test seam: drop in-flight entries so tests don't leak dedup state. */
export function clearInFlightTurnsLoadForTest(): void {
  inFlightTurnsLoad.clear();
}

/**
 * Ensures the store holds this session's FULL persisted turn history.
 *
 * The boot/SSR state hydrates turns only for the page-load active session;
 * switching to another session fetched its messages but never its turns, so
 * every message of that session resolved to `turn = null` and the debug
 * dialog showed `turn_metadata: null` (and turn-derived UI like agent status
 * stayed empty) even though the turns exist server-side with metadata.
 *
 * Completion is recorded in `turns.loadedBySession[sessionId]`, NOT by array
 * presence: WS `session.turn.*` events seed individual live turns without the
 * history, so a non-empty `bySession` list must never suppress the full
 * hydration. The marker is set even when the REST list is empty, so sessions
 * without turns are fetched exactly once; it is cleared only by session
 * removal (the store slice deletes it) or not set at all on failure, so a
 * failed fetch is retried on the next message fetch. Enrichment only — never
 * delays or fails message loading.
 */
export async function ensureSessionTurnsLoaded(
  sessionId: string,
  store: ReturnType<typeof useAppStoreApi>,
): Promise<void> {
  const state = store.getState();
  if (state.turns.loadedBySession[sessionId]) return;
  const inFlight = inFlightTurnsLoad.get(sessionId);
  if (inFlight) return inFlight;
  // The session may have been removed since the message fetch started; a
  // pending hydration must not resurrect `turns.bySession[sessionId]` via
  // addTurn after removeTaskSession deleted it. Deliberately NOT tied to the
  // message fetch's liveness flag: dockview keeps sibling session panels
  // mounted (hidden) and their fetch liveness flips during layout settle,
  // which would abort legitimate hydration. The REST response is the DB's
  // authoritative turn list, so merging it is always safe.
  if (!state.taskSessions.items[sessionId]) return;

  const promise = (async () => {
    try {
      const { turns } = await listSessionTurns(sessionId, { cache: "no-store" });
      // Re-check before merging: the session may have been removed while the
      // request was in flight.
      const state = store.getState();
      if (!state.taskSessions.items[sessionId]) return;
      // Only merge turns the store has not seen yet. Turns already present
      // reached the store via WS `session.turn.*` events, which are delivered
      // live and are therefore at least as fresh as this REST snapshot; a
      // merge via addTurn's Object.assign would otherwise overwrite newer
      // live fields (metadata, updated_at) with the older snapshot.
      const existingIds = new Set((state.turns.bySession[sessionId] ?? []).map((turn) => turn.id));
      for (const turn of turns) {
        if (existingIds.has(turn.id)) continue;
        store.getState().addTurn(turn);
      }
      store.getState().markTurnsLoaded(sessionId);
    } catch (err) {
      debug("turn fetch failed", { sessionId, err });
    } finally {
      inFlightTurnsLoad.delete(sessionId);
    }
  })();
  inFlightTurnsLoad.set(sessionId, promise);
  await promise;
}
