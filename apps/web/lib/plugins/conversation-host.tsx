"use client";

import * as React from "react";
import { useMessageFavoritesStore } from "@/lib/state/slices/message-favorites";
import {
  eventMatchesTask,
  eventString,
  messageFromEvent,
  turnFromEvent,
} from "./conversation-event-projection";
import type {
  PluginConversationApi,
  PluginConversationError,
  PluginConversationMessage,
  PluginConversationTurn,
  PluginSessionMessagesQuery,
  PluginSessionMessagesState,
  PluginSessionTurnsState,
} from "./types";
import {
  ConversationScopeContext,
  parseConversationResponse,
  pluginConversationUrl,
  type ConversationScope,
} from "./conversation-scope";
export { PluginConversationScopeProvider } from "./conversation-scope";

type MessagePage = {
  messages: PluginConversationMessage[];
  hasMore: boolean;
  cursor: string | null;
};

type TurnsPage = { turns: PluginConversationTurn[] };

const EMPTY_MESSAGES: PluginSessionMessagesState = {
  messages: [],
  loading: false,
  hydrated: false,
  loadingMore: false,
  error: null,
  hasMore: false,
  removed: false,
  loadMore: async () => 0,
  retry: () => undefined,
};

const EMPTY_TURNS: PluginSessionTurnsState = {
  turns: [],
  loading: false,
  hydrated: false,
  error: null,
  removed: false,
  retry: () => undefined,
};

function conversationError(error: unknown): PluginConversationError {
  if (typeof error === "object" && error !== null && "code" in error) {
    return error as PluginConversationError;
  }
  return {
    code: "upstream_failure",
    // i18n-exempt: plugin-facing transport diagnostic; plugins render their own localized error UI.
    message: error instanceof Error ? error.message : "Conversation request failed",
    retryable: true,
  };
}

function resolveTaskId(
  scope: ConversationScope,
  taskId: string | null | undefined,
): { taskId: string | null; error: PluginConversationError | null } {
  const resolved = taskId === undefined ? scope.taskId : taskId;
  if (resolved === "" || (typeof resolved === "string" && resolved !== scope.taskId)) {
    return {
      taskId: null,
      error: {
        code: "invalid_query",
        // i18n-exempt: plugin-facing programmer diagnostic; plugins render their own localized error UI.
        message: "taskId must match the active task",
        retryable: false,
      },
    };
  }
  return { taskId: resolved, error: null };
}

type MessagesState = Omit<PluginSessionMessagesState, "loadMore" | "retry">;
type MessagesSetter = React.Dispatch<React.SetStateAction<MessagesState>>;

function useOrderedMessageEvents({
  scope,
  sessionId,
  taskId,
  authorTypes,
  authorsKey,
  sort,
  setState,
  cursorRef,
  messagesRef,
}: {
  scope: ConversationScope | null;
  sessionId: string | null;
  taskId: string | null;
  authorTypes?: readonly ("user" | "agent")[];
  authorsKey: string;
  sort: "asc" | "desc";
  setState: MessagesSetter;
  cursorRef: React.MutableRefObject<string | null>;
  messagesRef: React.MutableRefObject<readonly PluginConversationMessage[]>;
}) {
  React.useEffect(() => {
    if (!scope || !sessionId) return;
    return scope.subscribe((event) => {
      if (event.event_type === "session.removed") {
        setState((current) => ({ ...current, removed: true, hasMore: false, loading: false }));
        cursorRef.current = null;
        return true;
      }
      if (!event.event_type.startsWith("message.") || !eventMatchesTask(event, taskId)) return true;
      const messageId = eventString(event, "message_id");
      if (!messageId) return false;
      if (event.event_type === "message.deleted") {
        setState((current) => {
          const messages = current.messages.filter((message) => message.id !== messageId);
          messagesRef.current = messages;
          return { ...current, messages };
        });
        return true;
      }
      const message = messageFromEvent(event);
      if (!message) return false;
      if (authorTypes && !authorTypes.includes(message.authorType)) return true;
      setState((current) => {
        const messages = current.messages.filter((item) => item.id !== message.id);
        messages.push(message);
        messages.sort((left, right) => {
          const created = left.createdAt.localeCompare(right.createdAt);
          const ordered = created === 0 ? left.id.localeCompare(right.id) : created;
          return sort === "asc" ? ordered : -ordered;
        });
        messagesRef.current = messages;
        return { ...current, messages };
      });
      return true;
    });
  }, [authorTypes, authorsKey, cursorRef, messagesRef, scope, sessionId, setState, sort, taskId]);
}

function useInitialMessagePage({
  scope,
  sessionId,
  error,
  revision,
  loadPage,
  setState,
  cursorRef,
  loadMoreRef,
  messagesRef,
  requestRevisionRef,
}: {
  scope: ConversationScope | null;
  sessionId: string | null;
  error: PluginConversationError | null;
  revision: number;
  loadPage: (cursor: string | null, append: boolean) => Promise<number>;
  setState: MessagesSetter;
  cursorRef: React.MutableRefObject<string | null>;
  loadMoreRef: React.MutableRefObject<Promise<number> | null>;
  messagesRef: React.MutableRefObject<readonly PluginConversationMessage[]>;
  requestRevisionRef: React.MutableRefObject<number>;
}) {
  React.useEffect(() => {
    requestRevisionRef.current += 1;
    cursorRef.current = null;
    loadMoreRef.current = null;
    messagesRef.current = [];
    if (!scope || !sessionId) {
      setState({ ...EMPTY_MESSAGES, messages: [] });
      return;
    }
    if (error) {
      setState({ ...EMPTY_MESSAGES, messages: [], error });
      return;
    }
    setState({ ...EMPTY_MESSAGES, messages: [], loading: true });
    void loadPage(null, false).catch((cause: unknown) => {
      if (scope.signal.aborted || scope.isTerminal()) return;
      setState({ ...EMPTY_MESSAGES, messages: [], error: conversationError(cause) });
    });
  }, [
    cursorRef,
    error,
    loadMoreRef,
    loadPage,
    messagesRef,
    requestRevisionRef,
    revision,
    scope,
    sessionId,
    setState,
  ]);
}
function isScopeRequestable(scope: ConversationScope): boolean {
  return !scope.signal.aborted && !scope.isTerminal();
}

function canLoadMore(
  scope: ConversationScope | null,
  sessionId: string | null,
  state: { removed: boolean; hasMore: boolean },
  cursor: string | null,
): boolean {
  return Boolean(
    scope && sessionId && isScopeRequestable(scope) && !state.removed && state.hasMore && cursor,
  );
}
function useMessagePageLoader({
  scope,
  sessionId,
  taskId,
  error,
  authorsKey,
  sort,
  limit,
  setState,
  cursorRef,
  messagesRef,
  requestRevisionRef,
}: {
  scope: ConversationScope | null;
  sessionId: string | null;
  taskId: string | null;
  error: PluginConversationError | null;
  authorsKey: string;
  sort: "asc" | "desc";
  limit: number;
  setState: MessagesSetter;
  cursorRef: React.MutableRefObject<string | null>;
  messagesRef: React.MutableRefObject<readonly PluginConversationMessage[]>;
  requestRevisionRef: React.MutableRefObject<number>;
}) {
  return React.useCallback(
    async (cursor: string | null, append: boolean): Promise<number> => {
      if (!scope || !sessionId || !isScopeRequestable(scope)) return 0;
      if (error) throw error;
      const capturedRevision = requestRevisionRef.current;
      let binding = await scope.ready();
      let pageCursor = cursor;
      if (pageCursor) {
        const renewal = await scope.renewContinuation(pageCursor);
        pageCursor = renewal.cursor;
        binding = renewal.binding;
      }
      const params = new URLSearchParams({ sort, limit: String(limit) });
      if (taskId !== null) params.set("task_id", taskId);
      for (const author of authorsKey ? authorsKey.split(",") : []) {
        params.append("author_type", author);
      }
      if (pageCursor) params.set("cursor", pageCursor);
      const response = await fetch(
        pluginConversationUrl(
          scope.pluginId,
          `/conversation/task-sessions/${encodeURIComponent(sessionId)}/messages?${params}`,
        ),
        {
          credentials: "include",
          headers: {
            "X-Kandev-Plugin-Binding": binding.bindingToken,
            "X-Kandev-Snapshot-Token": binding.snapshotToken,
          },
          signal: scope.signal,
        },
      );
      const page = await parseConversationResponse<MessagePage>(response);
      if (
        requestRevisionRef.current !== capturedRevision ||
        scope.signal.aborted ||
        scope.isTerminal()
      ) {
        return 0;
      }
      const existing = new Set(messagesRef.current.map((message) => message.id));
      const additions = page.messages.filter((message) => !existing.has(message.id));
      const messages = append ? [...messagesRef.current, ...additions] : page.messages;
      messagesRef.current = messages;
      setState((current) => ({
        ...current,
        messages,
        loading: false,
        hydrated: true,
        loadingMore: false,
        error: null,
        hasMore: page.hasMore,
      }));
      if (!append) scope.commitSnapshot("messages");
      cursorRef.current = page.cursor;
      return additions.length;
    },
    [
      authorsKey,
      error,
      limit,
      messagesRef,
      requestRevisionRef,
      scope,
      sessionId,
      setState,
      sort,
      taskId,
    ],
  );
}

function useMessageRebind({
  scope,
  sessionId,
  error,
  loadPage,
  setState,
  cursorRef,
  loadMoreRef,
  requestRevisionRef,
}: {
  scope: ConversationScope | null;
  sessionId: string | null;
  error: PluginConversationError | null;
  loadPage: (cursor: string | null, append: boolean) => Promise<number>;
  setState: MessagesSetter;
  cursorRef: React.MutableRefObject<string | null>;
  loadMoreRef: React.MutableRefObject<Promise<number> | null>;
  requestRevisionRef: React.MutableRefObject<number>;
}) {
  React.useEffect(() => {
    if (!scope || !sessionId || error) return;
    return scope.subscribeRebind(() => {
      requestRevisionRef.current += 1;
      cursorRef.current = null;
      loadMoreRef.current = null;
      setState((current) => ({ ...current, loading: true, loadingMore: false, error: null }));
      void loadPage(null, false).catch((cause: unknown) => {
        if (scope.signal.aborted || scope.isTerminal()) return;
        setState((current) => ({
          ...current,
          loading: false,
          error: conversationError(cause),
        }));
      });
    });
  }, [error, loadMoreRef, loadPage, requestRevisionRef, scope, sessionId, setState, cursorRef]);
}

function useSessionMessages(query: PluginSessionMessagesQuery): PluginSessionMessagesState {
  const scope = React.useContext(ConversationScopeContext);
  const [revision, setRevision] = React.useState(0);
  const [state, setState] = React.useState<Omit<PluginSessionMessagesState, "loadMore" | "retry">>({
    messages: [],
    loading: false,
    hydrated: false,
    loadingMore: false,
    error: null,
    hasMore: false,
    removed: false,
  });
  const cursorRef = React.useRef<string | null>(null);
  const loadMoreRef = React.useRef<Promise<number> | null>(null);
  const messagesRef = React.useRef<readonly PluginConversationMessage[]>([]);
  const requestRevisionRef = React.useRef(0);
  const resolved = scope ? resolveTaskId(scope, query.taskId) : { taskId: null, error: null };
  const authorsKey = [...(query.authorTypes ?? [])].join(",");
  const sort = query.sort ?? "desc";
  const limit = query.pageSize ?? 20;

  const loadPage = useMessagePageLoader({
    scope,
    sessionId: query.sessionId,
    taskId: resolved.taskId,
    error: resolved.error,
    authorsKey,
    sort,
    limit,
    setState,
    cursorRef,
    messagesRef,
    requestRevisionRef,
  });
  useOrderedMessageEvents({
    scope: resolved.error ? null : scope,
    sessionId: query.sessionId,
    taskId: resolved.taskId,
    authorTypes: query.authorTypes,
    authorsKey,
    sort,
    setState,
    cursorRef,
    messagesRef,
  });

  useInitialMessagePage({
    scope,
    sessionId: query.sessionId,
    error: resolved.error,
    revision,
    loadPage,
    setState,
    cursorRef,
    loadMoreRef,
    messagesRef,
    requestRevisionRef,
  });

  useMessageRebind({
    scope,
    sessionId: query.sessionId,
    error: resolved.error,
    loadPage,
    setState,
    cursorRef,
    loadMoreRef,
    requestRevisionRef,
  });
  const loadMore = React.useCallback((): Promise<number> => {
    if (!canLoadMore(scope, query.sessionId, state, cursorRef.current)) {
      return Promise.resolve(0);
    }
    if (loadMoreRef.current) return loadMoreRef.current;
    setState((current) => ({ ...current, loadingMore: true }));
    const request = loadPage(cursorRef.current, true)
      .catch((error: unknown) => {
        setState((current) => ({
          ...current,
          loadingMore: false,
          error: conversationError(error),
        }));
        throw conversationError(error);
      })
      .finally(() => {
        if (loadMoreRef.current === request) loadMoreRef.current = null;
      });
    loadMoreRef.current = request;
    return request;
  }, [loadPage, query.sessionId, scope, state.hasMore, state.removed]);

  const retry = React.useCallback(() => {
    if (
      !state.error?.retryable ||
      state.removed ||
      !scope ||
      scope.isTerminal() ||
      !query.sessionId
    )
      return;
    setRevision((value) => value + 1);
  }, [query.sessionId, scope, state.error, state.removed]);

  return React.useMemo(() => ({ ...state, loadMore, retry }), [loadMore, retry, state]);
}

type TurnsState = Omit<PluginSessionTurnsState, "retry">;
type TurnsSetter = React.Dispatch<React.SetStateAction<TurnsState>>;

function useOrderedTurnEvents({
  scope,
  sessionId,
  taskId,
  error,
  setState,
}: {
  scope: ConversationScope | null;
  sessionId: string | null;
  taskId: string | null;
  error: PluginConversationError | null;
  setState: TurnsSetter;
}) {
  React.useEffect(() => {
    if (!scope || !sessionId || error) return;
    return scope.subscribe((event) => {
      if (event.event_type === "session.removed") {
        setState((current) => ({ ...current, removed: true, loading: false }));
        return true;
      }
      if (!event.event_type.startsWith("session.turn.") || !eventMatchesTask(event, taskId)) {
        return true;
      }
      const turn = turnFromEvent(event);
      if (!turn) return false;
      setState((current) => {
        const turns = current.turns.filter((item) => item.id !== turn.id);
        turns.push(turn);
        turns.sort((left, right) => {
          const started = left.startedAt.localeCompare(right.startedAt);
          return started === 0 ? left.id.localeCompare(right.id) : started;
        });
        return { ...current, turns, hydrated: true };
      });
      return true;
    });
  }, [error, scope, sessionId, setState, taskId]);
}
function useSessionTurns(
  sessionId: string | null,
  taskId?: string | null,
): PluginSessionTurnsState {
  const scope = React.useContext(ConversationScopeContext);
  const [revision, setRevision] = React.useState(0);
  const [state, setState] = React.useState<Omit<PluginSessionTurnsState, "retry">>({
    ...EMPTY_TURNS,
    turns: [],
  });
  const resolved = scope ? resolveTaskId(scope, taskId) : { taskId: null, error: null };

  useOrderedTurnEvents({
    scope,
    sessionId,
    taskId: resolved.taskId,
    error: resolved.error,
    setState,
  });

  React.useEffect(() => {
    if (!scope || !sessionId) {
      setState({ ...EMPTY_TURNS, turns: [] });
      return;
    }
    if (resolved.error) {
      setState({ ...EMPTY_TURNS, turns: [], error: resolved.error });
      return;
    }
    let current = true;
    setState((previous) =>
      revision === 0
        ? { ...EMPTY_TURNS, turns: [], loading: true }
        : { ...previous, loading: true, error: null },
    );
    void scope
      .ready()
      .then(async (binding) => {
        const params = new URLSearchParams();
        if (resolved.taskId !== null) params.set("task_id", resolved.taskId);
        const queryString = params.size ? `?${params}` : "";
        const response = await fetch(
          pluginConversationUrl(
            scope.pluginId,
            `/conversation/task-sessions/${encodeURIComponent(sessionId)}/turns${queryString}`,
          ),
          {
            credentials: "include",
            headers: {
              "X-Kandev-Plugin-Binding": binding.bindingToken,
              "X-Kandev-Snapshot-Token": binding.snapshotToken,
            },
            signal: scope.signal,
          },
        );
        return parseConversationResponse<TurnsPage>(response);
      })
      .then((page) => {
        if (!current || scope.signal.aborted || scope.isTerminal()) return;
        setState({
          turns: page.turns,
          loading: false,
          hydrated: true,
          error: null,
          removed: false,
        });
        scope.commitSnapshot("turns");
      })
      .catch((error: unknown) => {
        if (!current || scope.signal.aborted || scope.isTerminal()) return;
        setState((previous) => ({
          ...(revision === 0 ? { ...EMPTY_TURNS, turns: [] } : previous),
          loading: false,
          error: conversationError(error),
        }));
      });
    return () => {
      current = false;
    };
  }, [resolved.error, resolved.taskId, revision, scope, sessionId]);

  React.useEffect(() => {
    if (!scope || !sessionId || resolved.error) return;
    return scope.subscribeRebind(() => setRevision((value) => value + 1));
  }, [resolved.error, scope, sessionId]);

  const retry = React.useCallback(() => {
    if (!state.error?.retryable || state.removed || !scope || !sessionId) return;
    setRevision((value) => value + 1);
  }, [scope, sessionId, state.error, state.removed]);

  return React.useMemo(() => ({ ...state, retry }), [retry, state]);
}

function useMessageFavorite(sessionId: string | null, messageId: string): boolean {
  const hydrateSession = useMessageFavoritesStore((state) => state.hydrateSession);
  React.useEffect(() => {
    if (sessionId) hydrateSession(sessionId);
  }, [hydrateSession, sessionId]);
  return useMessageFavoritesStore((state) =>
    sessionId ? Boolean(state.bySession[sessionId]?.[messageId]) : false,
  );
}

export const pluginConversationApi: PluginConversationApi = {
  useSessionMessages,
  useSessionTurns,
  useMessageFavorite,
};
