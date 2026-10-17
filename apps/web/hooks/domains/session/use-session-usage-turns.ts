"use client";
import type { Dispatch, SetStateAction } from "react";

import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { ApiError } from "@/lib/api/client";
import {
  getSessionUsageTurn,
  listSessionUsageTurns,
} from "@/lib/api/domains/conversation-usage-api";
import type { UsageTurn, UsageTurnsPage } from "@/lib/types/conversation-usage";

type UsageStore = ReturnType<typeof useAppStoreApi>;

const USAGE_TURN_PAGE_SIZE = 100;
const MAX_PAGES_PER_DEMAND = 3;

type UsageReadState = {
  key: string;
  status: "loading" | "not_found" | "unavailable";
};

const pageRequests = new Map<string, Promise<UsageTurnsPage>>();
const turnRequests = new Map<string, Promise<void>>();

function usageKey(taskId: string, sessionId: string, turnId: string) {
  return `${taskId}\u0000${sessionId}\u0000${turnId}`;
}

function usageReadKey(
  taskId: string | null,
  sessionId: string | null,
  turnId: string | null,
): string {
  if (!taskId || !sessionId || !turnId) return "";
  return usageKey(taskId, sessionId, turnId);
}

function requestTurnPage(
  store: UsageStore,
  taskId: string,
  sessionId: string,
  cursor: string | undefined,
  mode: "append" | "refresh",
): Promise<UsageTurnsPage> {
  const requestKey = `${taskId}\u0000${sessionId}\u0000${cursor ?? "<newest>"}\u0000${mode}`;
  const existing = pageRequests.get(requestKey);
  if (existing) return existing;
  // Keep the newest-page read in flight until it covers every observed invalidation.
  const request = (async () => {
    let refreshedInvalidation: number;
    let page: UsageTurnsPage;
    do {
      refreshedInvalidation = store.getState().usageInvalidation.bySessionId[sessionId] ?? 0;
      page = await listSessionUsageTurns(taskId, sessionId, {
        limit: USAGE_TURN_PAGE_SIZE,
        ...(cursor ? { cursor } : {}),
      });
      store.getState().setSessionUsageTurnPage(sessionId, page, mode, refreshedInvalidation);
    } while (
      !cursor &&
      (store.getState().usageInvalidation.bySessionId[sessionId] ?? 0) !== refreshedInvalidation
    );
    return page;
  })().finally(() => pageRequests.delete(requestKey));
  pageRequests.set(requestKey, request);
  return request;
}

function requestSingleTurn(
  store: UsageStore,
  taskId: string,
  sessionId: string,
  turnId: string,
): Promise<void> {
  const requestKey = usageKey(taskId, sessionId, turnId);
  const existing = turnRequests.get(requestKey);
  if (existing) return existing;
  const request = (async () => {
    let refreshedInvalidation: number;
    do {
      refreshedInvalidation = store.getState().usageInvalidation.bySessionId[sessionId] ?? 0;
      try {
        const turn = await getSessionUsageTurn(taskId, sessionId, turnId);
        store.getState().setSessionUsageTurn(sessionId, turn, refreshedInvalidation);
      } catch (error) {
        const state = store.getState();
        const index = state.usageTurns.bySessionId[sessionId];
        if (
          !(error instanceof ApiError && error.status === 404) ||
          (state.usageInvalidation.bySessionId[sessionId] ?? 0) === refreshedInvalidation ||
          (index?.exhausted && !index.byTurnId[turnId])
        ) {
          throw error;
        }
      }
    } while (
      (store.getState().usageInvalidation.bySessionId[sessionId] ?? 0) !== refreshedInvalidation
    );
  })().finally(() => turnRequests.delete(requestKey));
  turnRequests.set(requestKey, request);
  return request;
}

function indexedTurnIsStale(store: UsageStore, sessionId: string, turnId: string): boolean {
  const state = store.getState();
  const index = state.usageTurns.bySessionId[sessionId];
  return (
    !!index?.byTurnId[turnId] &&
    (index.refreshedInvalidationByTurnId?.[turnId] ?? 0) <
      (state.usageInvalidation.bySessionId[sessionId] ?? 0)
  );
}

/**
 * A demanded turn missing from the index is most likely newer than the indexed pages, so the
 * newest page is refreshed once before the cursor walks to older pages. `exhausted` only
 * means older history is fully loaded; it never short-circuits this refresh.
 */
async function indexUntilTurn(
  store: UsageStore,
  taskId: string,
  sessionId: string,
  turnId: string,
): Promise<void> {
  const readIndex = () => store.getState().usageTurns.bySessionId[sessionId];
  if (readIndex()?.byTurnId[turnId]) {
    if (!indexedTurnIsStale(store, sessionId, turnId)) return;
    const invalidation = store.getState().usageInvalidation.bySessionId[sessionId] ?? 0;
    if (readIndex()?.refreshedInvalidation !== invalidation) {
      await requestTurnPage(store, taskId, sessionId, undefined, "refresh");
    }
    if (indexedTurnIsStale(store, sessionId, turnId)) {
      await requestSingleTurn(store, taskId, sessionId, turnId);
    }
    return;
  }
  await requestTurnPage(store, taskId, sessionId, undefined, "refresh");
  for (let pageNumber = 1; pageNumber < MAX_PAGES_PER_DEMAND; pageNumber += 1) {
    const index = readIndex();
    if (index?.byTurnId[turnId] || index?.exhausted || !index?.nextCursor) return;
    await requestTurnPage(store, taskId, sessionId, index.nextCursor, "append");
  }
}

type ReadUsageTurnOptions = {
  store: UsageStore;
  taskId: string;
  sessionId: string;
  turnId: string;
  fallbackToSingleTurn: boolean;
  isActive: () => boolean;
  setReadState: Dispatch<SetStateAction<UsageReadState | null>>;
};

async function readSingleTurnFallback({
  store,
  taskId,
  sessionId,
  turnId,
  requestKey,
  isActive,
  setReadState,
}: Omit<ReadUsageTurnOptions, "fallbackToSingleTurn"> & {
  requestKey: string;
}): Promise<void> {
  try {
    await requestSingleTurn(store, taskId, sessionId, turnId);
    if (isActive()) setReadState(null);
  } catch (error) {
    if (!isActive()) return;
    const notFound = error instanceof ApiError && error.status === 404;
    setReadState({ key: requestKey, status: notFound ? "not_found" : "unavailable" });
  }
}

async function readUsageTurn({
  store,
  taskId,
  sessionId,
  turnId,
  fallbackToSingleTurn,
  isActive,
  setReadState,
}: ReadUsageTurnOptions): Promise<void> {
  const requestKey = usageKey(taskId, sessionId, turnId);
  try {
    await indexUntilTurn(store, taskId, sessionId, turnId);
    if (!isActive()) return;
    const index = store.getState().usageTurns.bySessionId[sessionId];
    if (index?.byTurnId[turnId]) {
      if (indexedTurnIsStale(store, sessionId, turnId)) {
        await requestSingleTurn(store, taskId, sessionId, turnId);
      }
      if (isActive()) setReadState(null);
      return;
    }
    if (fallbackToSingleTurn && !index?.exhausted) {
      await readSingleTurnFallback({
        store,
        taskId,
        sessionId,
        turnId,
        requestKey,
        isActive,
        setReadState,
      });
      return;
    }
    setReadState(index?.exhausted ? { key: requestKey, status: "not_found" } : null);
  } catch {
    if (isActive()) setReadState({ key: requestKey, status: "unavailable" });
  }
}

function projectUsageTurnRead(
  store: UsageStore,
  sessionId: string | null,
  turnId: string | null,
  indexedTurn: UsageTurn | undefined,
  readState: UsageReadState | null,
) {
  const stale = !!(sessionId && turnId && indexedTurnIsStale(store, sessionId, turnId));
  const usageTurn = stale ? null : (indexedTurn ?? null);
  return {
    usageTurn,
    isLoading: readState?.status === "loading" || (stale && !readState),
    isUnavailable: !usageTurn && readState?.status === "unavailable",
    isNotFound: !usageTurn && readState?.status === "not_found",
  };
}

export function useSessionUsageTurn(
  taskId: string | null,
  sessionId: string | null,
  turnId: string | null,
  { fallbackToSingleTurn = false }: { fallbackToSingleTurn?: boolean } = {},
) {
  const store = useAppStoreApi();
  const index = useAppStore((state) =>
    sessionId ? state.usageTurns.bySessionId[sessionId] : undefined,
  );
  const invalidation = useAppStore((state) =>
    sessionId ? (state.usageInvalidation.bySessionId[sessionId] ?? 0) : 0,
  );
  const connectionStatus = useAppStore((state) => state.connection.status);
  const [readState, setReadState] = useState<UsageReadState | null>(null);
  const wasConnected = useRef(connectionStatus === "connected");
  const key = usageReadKey(taskId, sessionId, turnId);
  const indexedTurn = turnId ? index?.byTurnId[turnId] : undefined;
  const activeKey = useRef(key);

  const refreshNewestPage = useCallback(
    (forceHistorical = false) => {
      if (!taskId || !sessionId) return;
      if (turnId && indexedTurnIsStale(store, sessionId, turnId)) {
        setReadState({ key: usageKey(taskId, sessionId, turnId), status: "loading" });
      }
      void requestTurnPage(store, taskId, sessionId, undefined, "refresh")
        .then(async (page) => {
          if (activeKey.current !== usageReadKey(taskId, sessionId, turnId)) return;
          if (!turnId) return;
          const index = store.getState().usageTurns.bySessionId[sessionId];
          if (!index?.byTurnId[turnId]) {
            if (fallbackToSingleTurn && !index?.exhausted) {
              const requestKey = usageKey(taskId, sessionId, turnId);
              await readSingleTurnFallback({
                store,
                taskId,
                sessionId,
                turnId,
                requestKey,
                isActive: () => activeKey.current === requestKey,
                setReadState,
              });
            }
            return;
          }
          if (
            !page.turns.some((turn) => turn.turn_id === turnId) &&
            (forceHistorical || indexedTurnIsStale(store, sessionId, turnId))
          ) {
            await requestSingleTurn(store, taskId, sessionId, turnId);
          }
          if (activeKey.current === usageReadKey(taskId, sessionId, turnId)) {
            setReadState(null);
          }
        })
        .catch(() => {
          if (activeKey.current !== usageReadKey(taskId, sessionId, turnId)) return;
          if (turnId) {
            setReadState({ key: usageKey(taskId, sessionId, turnId), status: "unavailable" });
          }
        });
    },
    [fallbackToSingleTurn, sessionId, store, taskId, turnId],
  );

  useEffect(() => {
    if (!taskId || !sessionId || !turnId) return;
    let active = true;
    const requestKey = usageKey(taskId, sessionId, turnId);
    activeKey.current = requestKey;
    setReadState({ key: requestKey, status: "loading" });
    void readUsageTurn({
      store,
      taskId,
      sessionId,
      turnId,
      fallbackToSingleTurn,
      isActive: () => active,
      setReadState,
    });
    return () => {
      active = false;
      activeKey.current = "";
    };
  }, [taskId, sessionId, turnId, fallbackToSingleTurn, store]);

  // The index remembers the invalidation counter its newest page was refreshed at, so a hook
  // mounting after turns completed while the view was unmounted refreshes too. A session with
  // no index yet is loaded by the demand read above.
  useEffect(() => {
    if (!sessionId) return;
    const entry = store.getState().usageTurns.bySessionId[sessionId];
    if (entry && entry.refreshedInvalidation !== invalidation) refreshNewestPage();
  }, [invalidation, refreshNewestPage, sessionId, store]);

  // Invalidations published while the socket was down never arrive; refresh on reconnect.
  useEffect(() => {
    const connected = connectionStatus === "connected";
    const reconnected = connected && !wasConnected.current;
    wasConnected.current = connected;
    if (reconnected) refreshNewestPage(true);
  }, [connectionStatus, refreshNewestPage]);

  const currentReadState = readState?.key === key ? readState : null;
  return projectUsageTurnRead(store, sessionId, turnId, indexedTurn, currentReadState);
}
