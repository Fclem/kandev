import { useEffect, useMemo } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { fetchTaskSession } from "@/lib/api";
import { isStaleSessionStateEvent } from "@/lib/ws/handlers/agent-session";
import type { TaskSession } from "@/lib/types/http";

type UseSessionResult = {
  session: TaskSession | null;
  isActive: boolean;
  isFailed: boolean;
  errorMessage: string | undefined;
};

/** Session states that still render the chat as "working"/busy. */
const BUSY_SESSION_STATES = new Set(["STARTING", "RUNNING", "CREATED"]);
/** Poll cadence for the post-subscribe reconcile while the state reads busy. */
const RECONCILE_POLL_INTERVAL_MS = 750;
/** Hard cap on the reconcile poll so it can never run forever or hammer the API. */
const RECONCILE_POLL_MAX_MS = 30_000;

export function useSession(sessionId: string | null): UseSessionResult {
  const store = useAppStoreApi();
  const session = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId] ?? null) : null,
  );
  const connectionStatus = useAppStore((state) => state.connection.status);
  const agentctlReady = useAppStore((state) =>
    sessionId ? state.sessionAgentctl.itemsBySessionId[sessionId]?.status === "ready" : false,
  );

  const isActive = useMemo(() => {
    if (!session?.state) return false;
    if (session.state === "RUNNING" || session.state === "WAITING_FOR_INPUT") return true;
    // Workspace infrastructure (agentctl) is ready even though the agent CLI hasn't started
    if (session.state === "CREATED" && agentctlReady) return true;
    return false;
  }, [session?.state, agentctlReady]);

  const isFailed = useMemo(() => {
    return session?.state === "FAILED";
  }, [session?.state]);

  useEffect(() => {
    if (connectionStatus !== "connected") return;
    if (!session?.id) return;
    const client = getWebSocketClient();
    if (!client) return;
    const sessionId = session.id;
    const unsubscribe = client.subscribeSession(sessionId);

    // Reconcile the session state after subscribing to close the WS-subscribe
    // race: a fast agent's RUNNING -> WAITING_FOR_INPUT transition can fan out
    // before this subscription registers server-side, so the client would sit
    // on a stale RUNNING state (isAgentBusy stuck true) forever. setTaskSession
    // merges the authoritative state (incoming state wins), mirroring the
    // message refetch in useSessionMessages. Because the backend's own state
    // write can land a beat after the agent finishes, poll the authoritative
    // state on a bounded schedule until it settles out of a busy state (or the
    // hard cap elapses), so a missed terminal event is always recovered. Errors
    // are ignored — live events still apply.
    let cancelled = false;
    let pollTimer: ReturnType<typeof setTimeout> | undefined;
    const startedAt = Date.now();

    const scheduleNext = () => {
      if (cancelled) return;
      const current = store.getState().taskSessions.items[sessionId];
      const stillBusy = !current || BUSY_SESSION_STATES.has(current.state);
      if (!stillBusy) return;
      if (Date.now() - startedAt >= RECONCILE_POLL_MAX_MS) return;
      pollTimer = setTimeout(reconcile, RECONCILE_POLL_INTERVAL_MS);
    };

    function reconcile() {
      fetchTaskSession(sessionId)
        .then((res) => {
          if (cancelled || !res.session) return;
          // The HTTP snapshot can lag the WS stream (e.g. read STARTING/CREATED
          // after RUNNING/WAITING_FOR_INPUT already landed), so applying it
          // unconditionally would clobber a fresher live state back to busy and
          // re-stick the UI. Drop the fetch when it's older than what's stored,
          // matching the state_changed WS handler's stale-snapshot guard.
          const current = store.getState().taskSessions.items[sessionId];
          if (isStaleSessionStateEvent(current, res.session.updated_at)) return;
          store.getState().setTaskSession(res.session);
        })
        .catch(() => {})
        .finally(scheduleNext);
    }

    reconcile();

    return () => {
      cancelled = true;
      if (pollTimer) clearTimeout(pollTimer);
      unsubscribe();
    };
  }, [session?.id, connectionStatus, store]);

  return { session, isActive, isFailed, errorMessage: session?.error_message };
}
