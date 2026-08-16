import { useEffect, useRef } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { listSessionTurns } from "@/lib/api/domains/session-api";
import type { Turn } from "@/lib/types/http";

const EMPTY_TURNS: Turn[] = [];
let fetchSequence = 0;
const lastAppliedSequence = new Map<string, number>();

export function useSessionTurns(sessionId: string | null): Turn[] {
  const turns = useAppStore((state) => (sessionId ? state.turns.bySession[sessionId] : undefined));
  const hydrated = useAppStore((state) =>
    sessionId ? Boolean(state.turns.hydratedBySession[sessionId]) : true,
  );
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const store = useAppStoreApi();
  const activeGeneration = useRef({ sessionId: activeSessionId, value: 0 });

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

    void listSessionTurns(sessionId)
      .then(({ turns: fetchedTurns }) => {
        const appliedSequence = lastAppliedSequence.get(sessionId) ?? 0;
        if (
          disposed ||
          activeGeneration.current.value !== generation ||
          sequence < appliedSequence
        ) {
          return;
        }
        lastAppliedSequence.set(sessionId, sequence);
        store.getState().replaceSessionTurns(sessionId, fetchedTurns);
      })
      .catch(() => {});

    return () => {
      disposed = true;
    };
  }, [sessionId, hydrated, store, activeSessionId]);

  return turns ?? EMPTY_TURNS;
}
