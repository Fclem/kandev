import { useEffect, useMemo, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { listTaskSessionMessages } from "@/lib/api/domains/session-api";
import type { Message } from "@/lib/types/http";
import { getWebSocketClient } from "@/lib/ws/connection";

const EMPTY_PROMPTS: Message[] = [];

/** Loads the prompt-only window without initializing the transcript cache. */
export function useSessionPrompts(sessionId: string | null) {
  const prompts = useAppStore((state) =>
    sessionId ? (state.messagePrompts.bySession[sessionId] ?? EMPTY_PROMPTS) : EMPTY_PROMPTS,
  );
  const meta = useAppStore((state) =>
    sessionId
      ? (state.messagePrompts.metaBySession[sessionId] ?? {
          isLoading: false,
          isLoadingMore: false,
          hasMore: false,
          oldestCursor: null,
        })
      : { isLoading: false, isLoadingMore: false, hasMore: false, oldestCursor: null },
  );
  const store = useAppStoreApi();
  const connectionStatus = useAppStore((state) => state.connection.status);
  const readinessRef = useRef<Promise<unknown> | null>(null);
  const [fetchFailed, setFetchFailed] = useState(false);

  useEffect(() => {
    if (!sessionId || connectionStatus !== "connected") return;
    const client = getWebSocketClient();
    if (!client) return;
    const subscription = client.subscribeSessionWithReady(sessionId);
    readinessRef.current = subscription.ready;
    return () => {
      readinessRef.current = null;
      subscription.unsubscribe();
    };
  }, [connectionStatus, sessionId]);

  useEffect(() => {
    if (!sessionId || connectionStatus !== "connected") return;
    let current = true;
    setFetchFailed(false);
    store.getState().setPromptMessagesLoading(sessionId, true);
    void (readinessRef.current ?? Promise.resolve())
      .then(() =>
        listTaskSessionMessages(sessionId, { author_type: "user", limit: 20, sort: "desc" }),
      )
      .then((response) => {
        if (!current) return;
        store
          .getState()
          .replacePromptMessages(sessionId, [...(response.messages ?? [])].reverse(), {
            hasMore: response.has_more ?? false,
            oldestCursor: response.cursor ?? null,
          });
      })
      .catch(() => {
        if (current) setFetchFailed(true);
      })
      .finally(() => {
        if (current) store.getState().setPromptMessagesLoading(sessionId, false);
      });
    return () => {
      current = false;
    };
  }, [connectionStatus, sessionId, store]);

  return useMemo(
    () => ({
      prompts,
      isLoading: meta.isLoading || fetchFailed,
      hasMore: meta.hasMore,
      oldestCursor: meta.oldestCursor,
      isLoadingMore: meta.isLoadingMore,
    }),
    [fetchFailed, meta.hasMore, meta.isLoading, meta.isLoadingMore, meta.oldestCursor, prompts],
  );
}
