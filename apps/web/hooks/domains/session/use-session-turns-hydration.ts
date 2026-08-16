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
      // Reconcile each REST row against the store's existing row (if any).
      // Two directions matter:
      // - A WS-delivered row can be NEWER than this REST snapshot (completion
      //   delivered while the request was in flight); merging it would
      //   clobber newer live fields via addTurn's Object.assign.
      // - A WS-delivered row can be OLDER (the matching completion event was
      //   missed during a disconnect window), leaving an incomplete start
      //   snapshot; the REST full history is the fix and must win.
      // Completion state takes precedence over timestamps: WS rows carry
      // RFC3339Nano fractions while the REST DTO truncates to whole seconds,
      // so a completion within the same second can look equal or older — but
      // a completed REST row is always more advanced than an incomplete WS
      // row for the same turn, and an existing completion must never be
      // rolled back. Only when the completion states agree do we fall back
      // to `updated_at`; a missing/invalid timestamp is treated as stale
      // (never newest) so malformed rows cannot clobber live data.
      const existingById = new Map(
        (state.turns.bySession[sessionId] ?? []).map((turn) => [turn.id, turn]),
      );
      for (const turn of turns) {
        const existing = existingById.get(turn.id);
        if (!existing) {
          store.getState().addTurn(turn);
          continue;
        }
        const restCompleted = !!turn.completed_at;
        const existingCompleted = !!existing.completed_at;
        if (restCompleted && !existingCompleted) {
          store.getState().addTurn(turn);
          continue;
        }
        if (existingCompleted && !restCompleted) {
          continue;
        }
        const existingUpdated = existing.updated_at ? Date.parse(existing.updated_at) : -Infinity;
        const restUpdated = turn.updated_at ? Date.parse(turn.updated_at) : -Infinity;
        if (restUpdated > existingUpdated) {
          store.getState().addTurn(turn);
        }
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
