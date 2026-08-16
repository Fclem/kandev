import { listSessionTurns } from "@/lib/api/domains/session-api";
import { useAppStoreApi } from "@/components/state-provider";
import { createDebugLogger } from "@/lib/debug/log";
import type { Turn } from "@/lib/types/http";
import { parseTurnTimestamp, shouldApplyTurnUpdate } from "@/lib/state/slices/session/turn-actions";

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
 * Returns the id of the latest incomplete turn in the list, or null when
 * every turn is completed. Compares via the strict timestamp parser so
 * fractional (RFC3339Nano) and whole-second (RFC3339) started_at values
 * order correctly.
 */
function latestIncompleteTurnId(turns: Turn[]): string | null {
  let latestId: string | null = null;
  let latestStarted: bigint | null = null;
  for (const turn of turns) {
    if (turn.completed_at) continue;
    const started = parseTurnTimestamp(turn.started_at);
    if (started === null) continue;
    if (latestStarted === null || started > latestStarted) {
      latestId = turn.id;
      latestStarted = started;
    }
  }
  return latestId;
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
      // Reconcile each REST row against the store's existing row (if any)
      // via the store's shared predicate (see shouldApplyTurnUpdate in
      // turn-actions.ts — completion state takes precedence over timestamps,
      // and malformed timestamps are stale). The same predicate guards the
      // WS write path in addTurn, so every post-hydration update is
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
      // Reconcile the active-turn marker: the WS session.turn.started event
      // may have been missed (the same REST/WS gap this hydration closes),
      // leaving activeBySession null while the session runs — agent status
      // and files-panel source gating read it. Mirror the boot-state rule:
      // latest incomplete turn, or null when none remains incomplete (the WS
      // completion event may have been missed instead).
      const activeTurnId = latestIncompleteTurnId(
        store.getState().turns.bySession[sessionId] ?? [],
      );
      store.getState().setActiveTurn(sessionId, activeTurnId);
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
