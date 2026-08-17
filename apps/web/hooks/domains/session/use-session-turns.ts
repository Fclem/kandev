import { useEffect, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { listSessionTurns } from "@/lib/api/domains/session-api";
import type { Turn } from "@/lib/types/http";

const EMPTY_TURNS: Turn[] = [];
let fetchSequence = 0;
const lastAppliedSequence = new Map<string, number>();
/** Bounded retries with backoff for transient turn-fetch failures. */
const MAX_TURN_FETCH_ATTEMPTS = 3;

export function useSessionTurns(sessionId: string | null): Turn[] {
  const turns = useAppStore((state) => (sessionId ? state.turns.bySession[sessionId] : undefined));
  const hydrated = useAppStore((state) =>
    sessionId ? Boolean(state.turns.hydratedBySession[sessionId]) : true,
  );
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const store = useAppStoreApi();
  const activeGeneration = useRef({ sessionId: activeSessionId, value: 0 });
  const [retryTick, setRetryTick] = useState(0);
  const failedAttemptsRef = useRef(0);

  useEffect(() => {
    // A new session starts a fresh retry budget.
    failedAttemptsRef.current = 0;
  }, [sessionId]);

  if (activeGeneration.current.sessionId !== activeSessionId) {
    activeGeneration.current = {
      sessionId: activeSessionId,
      value: activeGeneration.current.value + 1,
    };
  }

  useEffect(() => {
    if (!sessionId || hydrated) return;
    const generation = activeGeneration.current.value;
    const sequence = ++fetchSequence;
    let disposed = false;
    const controller = new AbortController();
    let retryTimer: number | undefined;

    void listSessionTurns(sessionId, { init: { signal: controller.signal } })
      .then(({ turns: fetchedTurns }) => {
        if (disposed || activeGeneration.current.value !== generation) return;
        const appliedSequence = lastAppliedSequence.get(sessionId) ?? 0;
        if (sequence < appliedSequence) return;
        failedAttemptsRef.current = 0;
        lastAppliedSequence.set(sessionId, sequence);
        store.getState().replaceSessionTurns(sessionId, fetchedTurns);
      })
      .catch((error: unknown) => {
        if (disposed || controller.signal.aborted) return;
        const attempts = failedAttemptsRef.current + 1;
        failedAttemptsRef.current = attempts;
        console.error("[useSessionTurns] failed to fetch turns for", sessionId, error);
        if (attempts < MAX_TURN_FETCH_ATTEMPTS) {
          retryTimer = window.setTimeout(
            () => setRetryTick((tick) => tick + 1),
            2000 * 2 ** attempts,
          );
        }
      });

    return () => {
      disposed = true;
      controller.abort();
      if (retryTimer !== undefined) window.clearTimeout(retryTimer);
      // Clear the stale sequence guard when a session is abandoned before
      // hydration, so a re-created same-ID session cannot be poisoned. Fully
      // hydrated sessions keep the entry — session IDs are UUIDs, so reuse is
      // not a production concern.
      if (!store.getState().turns.hydratedBySession[sessionId]) {
        lastAppliedSequence.delete(sessionId);
      }
    };
  }, [sessionId, hydrated, store, activeSessionId, retryTick]);

  return hydrated ? (turns ?? EMPTY_TURNS) : EMPTY_TURNS;
}
