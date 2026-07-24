import { useCallback, useLayoutEffect, useRef } from "react";
import { listTaskSessionMessages } from "@/lib/api";
import { useAppStore } from "@/components/state-provider";
import { createDebugLogger } from "@/lib/debug/log";

const debug = createDebugLogger("messages:lazyload");
const inFlightRequestsBySession = new Map<string, Promise<number>>();

function describeSkip(args: { sessionId: string | null; hasMore: boolean }): string {
  if (!args.sessionId) return "no-session";
  if (!args.hasMore) return "no-more";
  return "no-cursor";
}

type LoadMoreResponseLog = {
  sessionId: string;
  requestedBefore: string;
  ordered: Array<{
    id: string;
    created_at: string;
    type?: string;
    author_type?: string;
  }>;
  responseHasMore: boolean;
  newOldestCursor: string | null;
};

function logLoadMoreResponse(args: LoadMoreResponseLog) {
  const { sessionId, requestedBefore, ordered, responseHasMore, newOldestCursor } = args;
  const first = ordered[0];
  debug("loadMore: response", {
    sessionId,
    requestedBefore,
    fetchedCount: ordered.length,
    responseHasMore,
    newOldestId: newOldestCursor,
    newOldestCreatedAt: first?.created_at ?? null,
    newOldestType: first?.type ?? null,
    newOldestAuthor: first?.author_type ?? null,
  });
  if (ordered.length === 0 && responseHasMore) {
    debug("loadMore: WARNING empty batch with has_more=true — pagination may be stuck", {
      sessionId,
      before: requestedBefore,
    });
  }
  if (!responseHasMore && ordered.length > 0) {
    debug("loadMore: reached oldest — check that the first prompt is present", {
      sessionId,
      newOldestId: newOldestCursor,
      newOldestAuthor: first?.author_type,
      newOldestType: first?.type,
    });
  }
}

export function useLazyLoadMessages(sessionId: string | null) {
  // Use refs for values that should not trigger callback recreation
  const hasMore = useAppStore((state) =>
    sessionId ? (state.messages.metaBySession[sessionId]?.hasMore ?? false) : false,
  );
  const oldestCursor = useAppStore((state) =>
    sessionId ? (state.messages.metaBySession[sessionId]?.oldestCursor ?? null) : null,
  );
  const isLoading = useAppStore((state) =>
    sessionId ? (state.messages.metaBySession[sessionId]?.isLoading ?? false) : false,
  );

  // Store current values in refs to avoid recreating loadMore on every state change
  const stateRef = useRef({ sessionId, hasMore, oldestCursor, isLoading });
  useLayoutEffect(() => {
    stateRef.current = { sessionId, hasMore, oldestCursor, isLoading };
  }, [sessionId, hasMore, oldestCursor, isLoading]);

  const prependMessages = useAppStore((state) => state.prependMessages);
  const setMessagesMetadata = useAppStore((state) => state.setMessagesMetadata);

  // Stable loadMore - only depends on sessionId and store actions
  const loadMore = useCallback(() => {
    if (!sessionId) {
      debug("loadMore: skipped", {
        sessionId,
        reason: describeSkip({ sessionId, hasMore: stateRef.current.hasMore }),
        hasMore: stateRef.current.hasMore,
        oldestCursor: stateRef.current.oldestCursor,
      });
      return Promise.resolve(0);
    }

    const inFlightRequest = inFlightRequestsBySession.get(sessionId);
    if (inFlightRequest) return inFlightRequest;

    const { hasMore, oldestCursor } = stateRef.current;

    if (!hasMore || !oldestCursor) {
      debug("loadMore: skipped", {
        sessionId,
        reason: describeSkip({ sessionId, hasMore }),
        hasMore,
        oldestCursor,
      });
      return Promise.resolve(0);
    }

    const request = (async () => {
      debug("loadMore: requesting older page", { sessionId, before: oldestCursor, limit: 20 });

      // Update ref synchronously so concurrent callers share this request.
      stateRef.current.isLoading = true;
      setMessagesMetadata(sessionId, { isLoading: true });
      try {
        const response = await listTaskSessionMessages(sessionId, {
          limit: 20,
          before: oldestCursor,
          sort: "desc",
        });
        const orderedMessages = [...(response.messages ?? [])].reverse();
        // After reversing, orderedMessages[0] is the oldest message in this batch
        const newOldestCursor = orderedMessages[0]?.id ?? (response.has_more ? oldestCursor : null);
        logLoadMoreResponse({
          sessionId,
          requestedBefore: oldestCursor,
          ordered: orderedMessages,
          responseHasMore: response.has_more,
          newOldestCursor,
        });
        // Sync ref immediately so the next intersection callback sees correct state
        // (the useEffect sync may not have run yet between store update and next observer fire)
        if (stateRef.current.sessionId === sessionId) {
          stateRef.current = {
            sessionId,
            hasMore: response.has_more,
            oldestCursor: newOldestCursor,
            isLoading: false,
          };
        }
        prependMessages(sessionId, orderedMessages, {
          hasMore: response.has_more,
          oldestCursor: newOldestCursor,
        });
        return orderedMessages.length;
      } catch (error) {
        console.error("[useLazyLoadMessages] Error loading messages:", error);
        debug("loadMore: error", { sessionId, error });
        if (stateRef.current.sessionId === sessionId) stateRef.current.isLoading = false;
        setMessagesMetadata(sessionId, { isLoading: false });
        return 0;
      }
    })();
    inFlightRequestsBySession.set(sessionId, request);
    void request.finally(() => {
      if (inFlightRequestsBySession.get(sessionId) === request) {
        inFlightRequestsBySession.delete(sessionId);
      }
    });
    return request;
  }, [sessionId, prependMessages, setMessagesMetadata]);

  return { loadMore, hasMore, isLoading, oldestCursor };
}
