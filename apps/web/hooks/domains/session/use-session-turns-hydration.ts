import { listSessionTurns } from "@/lib/api/domains/session-api";
import { useAppStoreApi } from "@/components/state-provider";
import { createDebugLogger } from "@/lib/debug/log";
import { shouldApplyTurnUpdate } from "@/lib/state/slices/session/turn-actions";

const debug = createDebugLogger("messages:fetch:turns");

// Single-flight promises for in-progress turn hydration per session. The
// loaded marker lives in the store (`turns.loadedBySession`) because it must
// be seeded by SSR hydration too; this module map only deduplicates requests
// racing while the first is still in flight.
const inFlightTurnsLoad = new Map<string, Promise<void>>();

// Transient failures (network blip, backend restart) must not leave a
// session's turn metadata unresolved indefinitely: message loading proceeds
// either way, so without a retry the regression stays visible until an
// unrelated lifecycle event (visibility change, reconnect, session switch)
// triggers another fetch. Retry a bounded number of times with backoff; the
// in-flight entry covers the retry window, and after exhaustion the marker is
// still unset so the next natural fetch retries.
const TURN_HYDRATION_MAX_ATTEMPTS = 3;
const TURN_HYDRATION_RETRY_BASE_MS = 250;
const TURN_HYDRATION_RETRY_MAX_MS = 1_000;

/** Test seam: drop in-flight entries so tests don't leak dedup state. */
export function clearInFlightTurnsLoadForTest(): void {
  inFlightTurnsLoad.clear();
}

/**
 * One hydration attempt: fetch the session's full persisted turn history and
 * merge it into the store. Returns true on success (marker set) and false on
 * a transient failure the caller may retry.
 */
async function fetchAndReconcileSessionTurns(
  store: ReturnType<typeof useAppStoreApi>,
  sessionId: string,
  hydrationEpoch: number,
): Promise<boolean> {
  try {
    const { turns } = await listSessionTurns(sessionId, { cache: "no-store" });
    // Re-check before merging: the session may have been removed while the
    // request was in flight.
    const state = store.getState();
    if (!state.taskSessions.items[sessionId]) return true;
    // Reconcile each REST row against the store's existing row (if any) via
    // the store's shared predicate (see shouldApplyTurnUpdate in
    // turn-actions.ts — completion state takes precedence over timestamps,
    // and malformed timestamps are stale). The same predicate guards the WS
    // write path in addTurn, so every post-hydration update is
    // freshness-protected against stale rows from either transport.
    const existingById = new Map(
      (state.turns.bySession[sessionId] ?? []).map((turn) => [turn.id, turn]),
    );
    for (const turn of turns) {
      const existing = existingById.get(turn.id);
      if (!existing || shouldApplyTurnUpdate(existing, turn)) {
        store.getState().addTurn(turn);
      }
    }
    // Reconcile the active-turn marker in the store: the WS
    // session.turn.started event may have been missed (the same REST/WS gap
    // this hydration closes), leaving activeBySession null while the session
    // runs — agent status and files-panel source gating read it. The
    // store-owned action applies the settled-session rule (no marker for
    // orphaned incomplete turns on IDLE/etc. sessions) and rejects this
    // hydration if an authoritative clear bumped the epoch meanwhile.
    store.getState().reconcileActiveTurnAfterHydration(sessionId, hydrationEpoch);
    store.getState().markTurnsLoaded(sessionId);
    return true;
  } catch (err) {
    debug("turn fetch failed", { sessionId, err });
    return false;
  }
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
 * removal (the store slice deletes it) or not set at all on failure.
 * Transient failures are retried with bounded backoff inside this call
 * (see fetchAndReconcileSessionTurns); after exhaustion the marker stays
 * unset so the next natural trigger retries. Enrichment only — never delays
 * or fails message loading.
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
  // Capture the active-marker generation: if an authoritative clear (source
  // adoption) bumps it while the request is in flight, the store-owned
  // reconciliation rejects this hydration's marker write.
  const hydrationEpoch = state.turns.reconcileEpochBySession[sessionId] ?? 0;

  const promise = (async () => {
    try {
      for (let attempt = 1; ; attempt++) {
        if (await fetchAndReconcileSessionTurns(store, sessionId, hydrationEpoch)) return;
        debug("turn fetch attempt failed", { sessionId, attempt });
        if (attempt >= TURN_HYDRATION_MAX_ATTEMPTS) return;
        const delay = Math.min(
          TURN_HYDRATION_RETRY_BASE_MS * 2 ** (attempt - 1),
          TURN_HYDRATION_RETRY_MAX_MS,
        );
        await new Promise((resolve) => window.setTimeout(resolve, delay));
      }
    } finally {
      inFlightTurnsLoad.delete(sessionId);
    }
  })();
  inFlightTurnsLoad.set(sessionId, promise);
  await promise;
}
