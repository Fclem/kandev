"use client";
import type { Dispatch, SetStateAction } from "react";

import { useEffect, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { ApiError } from "@/lib/api/client";
import {
  getSessionUsageTurn,
  listSessionUsageTurns,
} from "@/lib/api/domains/conversation-usage-api";
import type { UsageTurn } from "@/lib/types/conversation-usage";

type UsageStore = ReturnType<typeof useAppStoreApi>;

const USAGE_TURN_PAGE_SIZE = 100;
const MAX_PAGES_PER_DEMAND = 3;

type UsageReadState = {
  key: string;
  status: "loading" | "available" | "not_found" | "unavailable";
  turn?: UsageTurn;
};

const pageRequests = new Map<string, Promise<void>>();
const turnRequests = new Map<string, Promise<UsageTurn>>();

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
): Promise<void> {
  const requestKey = `${taskId}\u0000${sessionId}\u0000${cursor ?? "<newest>"}\u0000${mode}`;
  const existing = pageRequests.get(requestKey);
  if (existing) return existing;
  const request = listSessionUsageTurns(taskId, sessionId, {
    limit: USAGE_TURN_PAGE_SIZE,
    ...(cursor ? { cursor } : {}),
  })
    .then((page) => {
      store.getState().setSessionUsageTurnPage(sessionId, page, mode);
    })
    .finally(() => pageRequests.delete(requestKey));
  pageRequests.set(requestKey, request);
  return request;
}

function requestSingleTurn(taskId: string, sessionId: string, turnId: string): Promise<UsageTurn> {
  const requestKey = usageKey(taskId, sessionId, turnId);
  const existing = turnRequests.get(requestKey);
  if (existing) return existing;
  const request = getSessionUsageTurn(taskId, sessionId, turnId).finally(() =>
    turnRequests.delete(requestKey),
  );
  turnRequests.set(requestKey, request);
  return request;
}

async function indexUntilTurn(
  store: UsageStore,
  taskId: string,
  sessionId: string,
  turnId: string,
): Promise<void> {
  for (let pageNumber = 0; pageNumber < MAX_PAGES_PER_DEMAND; pageNumber += 1) {
    const index = store.getState().usageTurns.bySessionId[sessionId];
    if (index?.byTurnId[turnId] || index?.exhausted) return;
    await requestTurnPage(store, taskId, sessionId, index?.nextCursor, "append");
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
  taskId,
  sessionId,
  turnId,
  requestKey,
  isActive,
  setReadState,
}: Omit<ReadUsageTurnOptions, "store" | "fallbackToSingleTurn"> & {
  requestKey: string;
}): Promise<void> {
  try {
    const turn = await requestSingleTurn(taskId, sessionId, turnId);
    if (isActive()) setReadState({ key: requestKey, status: "available", turn });
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
      setReadState(null);
      return;
    }
    if (fallbackToSingleTurn && !index?.exhausted) {
      await readSingleTurnFallback({
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

function isMissingIndexedTurn(
  indexedTurn: UsageTurn | undefined,
  indexExhausted: boolean,
): boolean {
  return !indexedTurn && indexExhausted;
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
  const [readState, setReadState] = useState<UsageReadState | null>(null);
  const lastInvalidation = useRef(invalidation);
  const key = usageReadKey(taskId, sessionId, turnId);
  const indexedTurn = turnId ? index?.byTurnId[turnId] : undefined;

  useEffect(() => {
    if (!taskId || !sessionId || !turnId) return;
    let active = true;
    const requestKey = usageKey(taskId, sessionId, turnId);
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
    };
  }, [taskId, sessionId, turnId, fallbackToSingleTurn, store]);

  useEffect(() => {
    if (!taskId || !sessionId || lastInvalidation.current === invalidation) return;
    lastInvalidation.current = invalidation;
    void requestTurnPage(store, taskId, sessionId, undefined, "refresh").catch(() => {
      if (turnId && !store.getState().usageTurns.bySessionId[sessionId]?.byTurnId[turnId]) {
        setReadState({ key: usageKey(taskId, sessionId, turnId), status: "unavailable" });
      }
    });
  }, [invalidation, sessionId, store, taskId, turnId]);

  const currentReadState = readState?.key === key ? readState : null;
  return {
    usageTurn: indexedTurn ?? currentReadState?.turn ?? null,
    isLoading: currentReadState?.status === "loading",
    isUnavailable: currentReadState?.status === "unavailable",
    isNotFound:
      currentReadState?.status === "not_found" ||
      isMissingIndexedTurn(indexedTurn, index?.exhausted === true),
  };
}
