"use client";

/* eslint-disable max-lines -- this component composes the complete transcript surface. */

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  memo,
  type MutableRefObject,
  type RefObject,
} from "react";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import {
  type ChatInputContainerHandle,
  type ChatSubmitPayload,
  type ChatSubmitResult,
} from "@/components/task/chat/chat-input-container";
import { MessageList } from "@/components/task/chat/message-list";
import {
  type MessageListHandle,
  type LastPromptEdge,
  getLastUserMessageId,
  getFirstUserMessageId,
  resolveLastPromptControls,
} from "@/components/task/chat/message-list-shared";
import { AnchoredLastPromptBar } from "@/components/task/chat/anchored-last-prompt-bar";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useIsTaskArchived } from "./task-archived-context";
import { useChatPanelState } from "./chat/use-chat-panel-state";
import { ChatInputArea, useSubmitHandler, useChatPanelHandlers } from "./chat/chat-input-area";
import { ClarificationPanelSection } from "./chat/clarification-panel-section";
import { useComposerAgentStartHint } from "./chat/use-composer-agent-start-hint";
import { PanelSearchBar } from "@/components/search/panel-search-bar";
import { SessionSearchHits } from "@/components/task/chat/session-search-hits";
import { usePanelSearch } from "@/hooks/use-panel-search";
import { useSessionSearch } from "@/hooks/domains/session/use-session-search";
import { useLazyLoadMessages } from "@/hooks/use-lazy-load-messages";
import { findUnreadDividerItemId, lastRenderedMessageId } from "@/lib/session-unread-divider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useSessionReadTracking } from "./chat/use-session-read-tracking";
import { useDrainOlderMessages } from "@/components/task/chat/use-drain-older-messages";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getSessionWorkspacePath } from "@/lib/session-workspace-path";
import { routePanelMouseDown } from "./chat/route-panel-mouse-down";
import { useTranslation } from "react-i18next";

import { loadMessageWindowAround } from "@/hooks/domains/session/load-message-window";
import { TaskChatLaunchError } from "./simple/components/task-chat-launch-error";
import { useTaskLaunchErrorContext } from "./task-launch-error-context";
import { useTaskStatusSummary } from "@/hooks/domains/task/use-task-status-summary";
import { TaskMarkdownFileLinkProvider } from "@/components/shared/task-markdown-file-link-provider";

/** Returns a `clarificationKey` that increments each time a pending
 * clarification is resolved, letting the composer reset its input state for
 * the next clarification round. */
function useClarificationKey(agentMessageCount: number) {
  const lastCountRef = useRef(agentMessageCount);
  const [clarificationKey, setClarificationKey] = useState(0);
  useEffect(() => {
    lastCountRef.current = agentMessageCount;
  }, [agentMessageCount]);
  const handleClarificationResolved = useCallback(() => setClarificationKey((k) => k + 1), []);
  return { clarificationKey, handleClarificationResolved };
}

/** Identity for a prompt-history target owned by a non-Dockview host. */
export type PendingMessageScrollTarget = {
  sessionId: string;
  messageId: string;
  token: number;
  hostPanelId: string;
};

/** Scrolls a non-Dockview host target after the message row becomes rendered. */
type PendingMessageScrollOptions = {
  messageListRef: RefObject<MessageListHandle | null>;
  sessionId: string | null;
  messageId: string | null | undefined;
  target?: PendingMessageScrollTarget | null;
  onConsumed: ((messageId: string) => void) | undefined;
  /** Stable store-derived revision. The hook only uses its identity. */
  readinessKey: unknown;
  isInitialMessagesLoading: boolean;
  isVisible?: boolean;
};

type PendingScrollLifecycle = {
  sessionId: string | null;
  messageId: string | null | undefined;
  target?: PendingMessageScrollTarget | null;
  isVisible: boolean;
};

type PendingScrollRefs = {
  requestKeys: MutableRefObject<Set<string>>;
  completedAround: MutableRefObject<Set<string>>;
  targetIdentity: MutableRefObject<string | null>;
  failedTargetKey: MutableRefObject<string | null>;
  scrollSucceeded: MutableRefObject<boolean>;
  reassertionTimer: MutableRefObject<number | null>;
  reassertionAttempted: MutableRefObject<Set<string>>;
  mounted: MutableRefObject<boolean>;
  lifecycle: MutableRefObject<PendingScrollLifecycle>;
};

function isPendingTargetCurrent(
  refs: PendingScrollRefs,
  targetKey: string,
  sessionId: string,
  messageId: string,
) {
  return (
    refs.mounted.current &&
    refs.lifecycle.current.isVisible &&
    refs.targetIdentity.current === targetKey &&
    refs.lifecycle.current.sessionId === sessionId &&
    refs.lifecycle.current.messageId === messageId
  );
}

function schedulePendingReassertion(options: {
  refs: PendingScrollRefs;
  targetKey: string;
  messageId: string;
  messageListRef: RefObject<MessageListHandle | null>;
  isCurrentTarget: () => boolean;
  onConsumed: ((messageId: string) => void) | undefined;
  setHasError: (value: boolean) => void;
}) {
  const { refs, targetKey, messageId, messageListRef, isCurrentTarget, onConsumed, setHasError } =
    options;
  if (
    refs.reassertionTimer.current !== null ||
    refs.reassertionAttempted.current.has(targetKey) ||
    !isCurrentTarget()
  ) {
    return;
  }
  refs.reassertionTimer.current = window.setTimeout(() => {
    refs.reassertionTimer.current = null;
    if (!isCurrentTarget()) return;
    refs.reassertionAttempted.current.add(targetKey);
    if (
      messageListRef.current?.scrollToMessage(messageId, {
        align: "start",
        behavior: "auto",
      })
    ) {
      refs.completedAround.current.delete(targetKey);
      onConsumed?.(messageId);
    } else {
      refs.failedTargetKey.current = targetKey;
      setHasError(true);
    }
  }, 250);
}

function requestPendingTargetWindow(options: {
  refs: PendingScrollRefs;
  targetKey: string;
  sessionId: string;
  messageId: string;
  target?: PendingMessageScrollTarget | null;
  targetToken: number;
  store: ReturnType<typeof useAppStoreApi>;
  isCurrentTarget: () => boolean;
  setIsLoading: (value: boolean) => void;
  setHasError: (value: boolean) => void;
  onConsumed: ((messageId: string) => void) | undefined;
  scheduleReassertion: () => void;
}) {
  const {
    refs,
    targetKey,
    sessionId,
    messageId,
    target,
    targetToken,
    store,
    isCurrentTarget,
    setIsLoading,
    setHasError,
    onConsumed,
    scheduleReassertion,
  } = options;
  refs.requestKeys.current.add(targetKey);
  setIsLoading(true);
  void loadMessageWindowAround(
    sessionId,
    messageId,
    () => isCurrentTarget() && (!target || refs.lifecycle.current.target?.token === targetToken),
    store,
  )
    .then((result) => {
      if (!isCurrentTarget()) return;
      if (result.kind === "deleted-target") {
        onConsumed?.(messageId);
      } else if (result.kind === "merged") {
        refs.completedAround.current.add(targetKey);
        if (refs.scrollSucceeded.current) scheduleReassertion();
      }
    })
    .catch(() => {
      if (isCurrentTarget()) {
        refs.completedAround.current.delete(targetKey);
        refs.failedTargetKey.current = targetKey;
        setHasError(true);
      }
    })
    .finally(() => {
      refs.requestKeys.current.delete(targetKey);
      if (refs.mounted.current) setIsLoading(refs.requestKeys.current.size > 0);
    });
}

function attemptPendingScroll(options: {
  refs: PendingScrollRefs;
  targetKey: string;
  sessionId: string;
  messageId: string;
  target?: PendingMessageScrollTarget | null;
  targetToken: number;
  messageListRef: RefObject<MessageListHandle | null>;
  store: ReturnType<typeof useAppStoreApi>;
  isInitialMessagesLoading: boolean;
  isCurrentTarget: () => boolean;
  onConsumed: ((messageId: string) => void) | undefined;
  setIsLoading: (value: boolean) => void;
  setHasError: (value: boolean) => void;
}) {
  const {
    refs,
    targetKey,
    sessionId,
    messageId,
    target,
    targetToken,
    messageListRef,
    store,
    isInitialMessagesLoading,
    isCurrentTarget,
    onConsumed,
    setIsLoading,
    setHasError,
  } = options;
  if (!isCurrentTarget()) return;
  if (
    refs.completedAround.current.has(targetKey) &&
    (refs.reassertionTimer.current !== null || refs.reassertionAttempted.current.has(targetKey))
  ) {
    return;
  }
  const scheduleReassertion = () =>
    schedulePendingReassertion({
      refs,
      targetKey,
      messageId,
      messageListRef,
      isCurrentTarget,
      onConsumed,
      setHasError,
    });
  if (
    messageListRef.current?.scrollToMessage(messageId, {
      align: "start",
      behavior: "auto",
    })
  ) {
    refs.scrollSucceeded.current = true;
    if (refs.completedAround.current.has(targetKey)) {
      scheduleReassertion();
    } else if (!refs.requestKeys.current.has(targetKey)) {
      onConsumed?.(messageId);
    }
    return;
  }
  if (isInitialMessagesLoading) return;
  const loaded = store
    .getState()
    .messages.bySession[sessionId]?.some((message) => message.id === messageId);
  if (loaded) return;
  if (refs.requestKeys.current.has(targetKey) || refs.completedAround.current.has(targetKey)) {
    return;
  }
  requestPendingTargetWindow({
    refs,
    targetKey,
    sessionId,
    messageId,
    target,
    targetToken,
    store,
    isCurrentTarget,
    setIsLoading,
    setHasError,
    onConsumed,
    scheduleReassertion,
  });
}

// eslint-disable-next-line max-lines-per-function -- coordinates the target lifecycle, request guard, and retry state.
export function usePendingMessageScroll({
  messageListRef,
  sessionId,
  messageId,
  target,
  onConsumed,
  readinessKey,
  isInitialMessagesLoading,
  isVisible = true,
}: PendingMessageScrollOptions) {
  const store = useAppStoreApi();
  const [isLoading, setIsLoading] = useState(false);
  const [hasError, setHasError] = useState(false);
  const [retryVersion, setRetryVersion] = useState(0);
  const requestKeysRef = useRef(new Set<string>());
  const completedAroundRef = useRef(new Set<string>());
  const targetIdentityRef = useRef<string | null>(null);
  const failedTargetKeyRef = useRef<string | null>(null);
  const scrollSucceededRef = useRef(false);
  const reassertionTimerRef = useRef<number | null>(null);
  const reassertionAttemptedRef = useRef(new Set<string>());
  const mountedRef = useRef(true);
  const lifecycleRef = useRef<PendingScrollLifecycle>({ sessionId, messageId, target, isVisible });
  const retry = useCallback(() => {
    const targetKey = targetIdentityRef.current;
    failedTargetKeyRef.current = null;
    if (targetKey) {
      completedAroundRef.current.delete(targetKey);
      reassertionAttemptedRef.current.delete(targetKey);
    }
    setHasError(false);
    setRetryVersion((version) => version + 1);
  }, []);

  const cancelReassertion = useCallback(() => {
    if (reassertionTimerRef.current !== null) {
      window.clearTimeout(reassertionTimerRef.current);
      reassertionTimerRef.current = null;
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      cancelReassertion();
    };
  }, [cancelReassertion]);

  const effectiveSessionId = target?.sessionId ?? sessionId;
  const effectiveMessageId = target?.messageId ?? messageId;
  const targetToken = target?.token ?? 0;
  const targetHostPanelId = target?.hostPanelId ?? "pending";
  const effectiveTargetKey =
    effectiveSessionId && effectiveMessageId
      ? `${effectiveSessionId}\u0000${effectiveMessageId}\u0000${targetToken}\u0000${targetHostPanelId}`
      : null;

  lifecycleRef.current = {
    sessionId: effectiveSessionId,
    messageId: effectiveMessageId,
    target,
    isVisible,
  };
  const targetBelongsToHost = !target || target.sessionId === sessionId;
  useEffect(() => {
    if (!targetBelongsToHost) {
      targetIdentityRef.current = null;
      failedTargetKeyRef.current = null;
      completedAroundRef.current.clear();
      cancelReassertion();
      setIsLoading(false);
      setHasError(false);
      onConsumed?.(target?.messageId ?? "");
      return;
    }
    if (!effectiveSessionId || !effectiveMessageId || !effectiveTargetKey) {
      targetIdentityRef.current = null;
      failedTargetKeyRef.current = null;
      completedAroundRef.current.clear();
      cancelReassertion();
      setIsLoading(false);
      setHasError(false);
      return;
    }
    if (targetIdentityRef.current !== effectiveTargetKey) {
      scrollSucceededRef.current = false;
      targetIdentityRef.current = effectiveTargetKey;
      failedTargetKeyRef.current = null;
      setHasError(false);
      completedAroundRef.current.clear();
      cancelReassertion();
    }
    if (!isVisible) {
      cancelReassertion();
      completedAroundRef.current.delete(effectiveTargetKey);
    }
    if (failedTargetKeyRef.current === effectiveTargetKey) return;
    if (!isVisible) return;
    const isCurrentTarget = () =>
      isPendingTargetCurrent(
        {
          requestKeys: requestKeysRef,
          completedAround: completedAroundRef,
          targetIdentity: targetIdentityRef,
          failedTargetKey: failedTargetKeyRef,
          scrollSucceeded: scrollSucceededRef,
          reassertionTimer: reassertionTimerRef,
          reassertionAttempted: reassertionAttemptedRef,
          mounted: mountedRef,
          lifecycle: lifecycleRef,
        },
        effectiveTargetKey,
        effectiveSessionId,
        effectiveMessageId,
      );
    const refs: PendingScrollRefs = {
      requestKeys: requestKeysRef,
      completedAround: completedAroundRef,
      targetIdentity: targetIdentityRef,
      failedTargetKey: failedTargetKeyRef,
      scrollSucceeded: scrollSucceededRef,
      reassertionTimer: reassertionTimerRef,
      reassertionAttempted: reassertionAttemptedRef,
      mounted: mountedRef,
      lifecycle: lifecycleRef,
    };
    const frame = requestAnimationFrame(() =>
      attemptPendingScroll({
        refs,
        targetKey: effectiveTargetKey,
        sessionId: effectiveSessionId,
        messageId: effectiveMessageId,
        target,
        targetToken,
        messageListRef,
        store,
        isInitialMessagesLoading,
        isCurrentTarget,
        onConsumed,
        setIsLoading,
        setHasError,
      }),
    );
    return () => cancelAnimationFrame(frame);
  }, [
    cancelReassertion,
    effectiveMessageId,
    effectiveSessionId,
    effectiveTargetKey,
    isInitialMessagesLoading,
    isVisible,
    messageId,
    messageListRef,
    onConsumed,
    readinessKey,
    retryVersion,
    sessionId,
    store,
    targetHostPanelId,
    targetBelongsToHost,
    targetToken,
  ]);
  return { isLoading, hasError, retry };
}

/** Computes the render-item key the unread "New" divider should appear
 * immediately before: tracks the latest rendered message id for session read
 * tracking, then maps the resulting divider anchor onto the grouped items. */
function useUnreadDividerBeforeItemKey(
  sessionId: string | null,
  isVisible: boolean,
  groupedItems: Parameters<typeof lastRenderedMessageId>[0],
  isInitialMessagesLoading: boolean,
) {
  const latestMessageId = useMemo(() => lastRenderedMessageId(groupedItems), [groupedItems]);
  const dividerAnchor = useSessionReadTracking(
    sessionId,
    isVisible,
    latestMessageId,
    isInitialMessagesLoading,
  );
  return useMemo(
    () => findUnreadDividerItemId(groupedItems, dividerAnchor),
    [groupedItems, dividerAnchor],
  );
}

/** Floating session-search overlay over the transcript: the search bar plus
 * its hits list, with next/prev cycling through hits. Renders nothing while
 * the search is closed. */
function SessionSearchOverlay({
  search,
  agentLabel,
  agentName,
}: {
  search: ReturnType<typeof useSessionSearch>;
  agentLabel: string | null;
  agentName: string | null;
}) {
  const currentIdx = search.activeHitId
    ? search.hits.findIndex((h) => h.id === search.activeHitId)
    : -1;
  const total = search.hits.length;
  const handleNext = useCallback(() => {
    if (!total) return;
    const next = search.hits[(Math.max(currentIdx, -1) + 1) % total];
    if (next) search.setActiveHit(next.id);
  }, [search, currentIdx, total]);
  const handlePrev = useCallback(() => {
    if (!total) return;
    const prevIdx = (Math.max(currentIdx, 0) - 1 + total) % total;
    const prev = search.hits[prevIdx];
    if (prev) search.setActiveHit(prev.id);
  }, [search, currentIdx, total]);
  if (!search.isOpen) return null;
  return (
    <div className="absolute top-2 right-2 z-20 flex flex-col items-end gap-1">
      <PanelSearchBar
        className="static"
        value={search.query}
        onChange={search.setQuery}
        onNext={handleNext}
        onPrev={handlePrev}
        onClose={search.close}
        matchInfo={{ current: currentIdx >= 0 ? currentIdx + 1 : 0, total }}
        isLoading={search.isSearching}
        // Session search already debounces in useDebouncedSearch; skip the
        // bar's debounce so we don't stack 150ms + 180ms per keystroke.
        debounceMs={0}
      />
      <SessionSearchHits
        hits={search.hits}
        query={search.query}
        activeHitId={search.activeHitId}
        onSelect={search.setActiveHit}
        isSearching={search.isSearching}
        agentLabel={agentLabel}
        agentName={agentName}
      />
    </div>
  );
}

/** Returns the AgentProfileOption for the session's profile, or null. Uses
 * primitive profile id to avoid getSnapshot-cache errors from returning
 * fresh objects on every selector call. */
function useSessionAgentProfile(sessionId: string | null | undefined) {
  const profileId = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId]?.agent_profile_id ?? null) : null,
  );
  return useAppStore((state) =>
    profileId
      ? (state.agentProfiles.items.find((p: { id: string }) => p.id === profileId) ?? null)
      : null,
  );
}

/** Resolves the agent profile name + registry slug for the given session.
 * Label is "Profile Name" from the "Agent • Profile Name" store label; slug
 * feeds <AgentLogo> which fetches the logo by agent type. */
function useSessionAgentIdentity(sessionId: string | null | undefined): {
  label: string | null;
  name: string | null;
} {
  const profile = useSessionAgentProfile(sessionId);
  // User-supplied session name wins over the derived profile label,
  // matching the session tab title precedence (resolveSessionTabTitle).
  const sessionName = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId]?.name ?? null) : null,
  );
  return useMemo(() => {
    if (!profile) return { label: sessionName, name: null };
    const parts = profile.label.split(" \u2022 ");
    const label = sessionName || parts[1] || parts[0] || profile.label;
    return { label, name: profile.agent_name ?? null };
  }, [profile, sessionName]);
}

type TaskChatPanelProps = {
  onSend?: (payload: ChatSubmitPayload) => ChatSubmitResult;
  sessionId?: string | null;
  taskId?: string | null;
  /**
   * Task this panel belongs to, independent of whether it has a session yet.
   * Only the status row uses it, so a task with no session still shows its
   * dependency and autopilot chips. `taskId` above stays session-gated because
   * it also drives plan mode, the composer, and read tracking.
   */
  statusTaskId?: string | null;
  onOpenFile?: (path: string, repo?: string) => void;
  showRequestChangesTooltip?: boolean;
  onRequestChangesTooltipDismiss?: () => void;
  /** Callback to open a file at a specific line (for comment clicks) */
  onOpenFileAtLine?: (filePath: string) => void;
  /** Hide the sessions dropdown (session tabs in dockview replace it) */
  hideSessionsDropdown?: boolean;
  /**
   * Embedded multi-panel hosts do not own the global workbench or shortcuts.
   * They keep the conversation and composer, but suppress those side effects.
   */
  embedded?: boolean;
  /**
   * Whether this panel is the one actually on screen — gates the
   * Slack-style unread-divider read tracking (see
   * chat/use-session-read-tracking.ts). Dockview-hosted callers must pass
   * real tab-activation state (see hooks/use-panel-active.ts); other hosts
   * (mobile, quick chat, kanban preview) default to true since mounting
   * already implies visibility for them.
   */
  isVisible?: boolean;
  panelId?: string | null;
  /** Legacy message-only target for non-Dockview hosts. */
  pendingScrollToMessageId?: string | null;
  /** Identity-bearing target for mobile/non-Dockview prompt navigation. */
  pendingScrollTarget?: PendingMessageScrollTarget | null;
  /** Called after a non-Dockview scroll target reaches a rendered row. */
  onPendingScrollConsumed?: (messageId: string) => void;
};

type ScrollTargetConsumptionParams = {
  resolvedSessionId: string | null;
  isVisible: boolean;
  panelId: string | null;
  messageListRef: RefObject<MessageListHandle | null>;
  /** Whether the target row is present in the current message snapshot. */
  targetRendered?: boolean;
  isInitialMessagesLoading: boolean;
  /** Rendered-transcript revision: retry a retained target when a row mounts
   * after the initial load or when older pages are prepended. */
  renderedMessageCount: number;
};

let dockviewOwnerGeneration = 0;
const activeDockviewOwners = new Map<string, { generation: number; token: number | null }>();

function getDockviewOwnerKey(sessionId: string | null, panelId: string | null) {
  return sessionId && panelId ? `${sessionId}\u0000${panelId}` : null;
}

function shouldDeferTargetWindowLoad(
  messages: readonly { id: string }[],
  targetMessageId: string,
  isInitialMessagesLoading: boolean,
  targetRendered: boolean,
) {
  const loaded = messages.some((message) => message.id === targetMessageId);
  if (isInitialMessagesLoading) return true;
  return loaded && targetRendered;
}

type DockviewScrollTarget = {
  sessionId: string;
  messageId: string;
  token: number;
  hostPanelId: string;
};

type DockviewScrollLifecycle = {
  reassertionKey: MutableRefObject<string | null>;
  reassertionTargetKey: MutableRefObject<string | null>;
  reassertionAttempted: MutableRefObject<Set<string>>;
  requestKeys: MutableRefObject<Set<string>>;
  completedAround: MutableRefObject<Set<string>>;
  failedTargetKey: MutableRefObject<string | null>;
  scrollSucceeded: MutableRefObject<boolean>;
  reassertionTimer: MutableRefObject<number | null>;
  mounted: MutableRefObject<boolean>;
  lifecycle: MutableRefObject<{
    resolvedSessionId: string | null;
    isVisible: boolean;
    panelId: string | null;
  }>;
};

function isDockviewTargetCurrent(refs: DockviewScrollLifecycle, target: DockviewScrollTarget) {
  const latest = useDockviewStore.getState().scrollTarget;
  return (
    refs.mounted.current &&
    refs.lifecycle.current.isVisible &&
    refs.lifecycle.current.resolvedSessionId === target.sessionId &&
    refs.lifecycle.current.panelId === target.hostPanelId &&
    latest?.token === target.token &&
    latest.sessionId === target.sessionId &&
    latest.hostPanelId === target.hostPanelId
  );
}

function scheduleDockviewReassertion(options: {
  refs: DockviewScrollLifecycle;
  target: DockviewScrollTarget;
  targetKey: string;
  messageListRef: RefObject<MessageListHandle | null>;
  clearScrollTarget: (token: number) => void;
  setJumpError: (value: boolean) => void;
}) {
  const { refs, target, targetKey, messageListRef, clearScrollTarget, setJumpError } = options;
  if (
    refs.reassertionTimer.current !== null ||
    refs.reassertionAttempted.current.has(targetKey) ||
    !isDockviewTargetCurrent(refs, target)
  ) {
    return;
  }
  refs.reassertionKey.current = targetKey;
  refs.reassertionTimer.current = window.setTimeout(() => {
    refs.reassertionTimer.current = null;
    refs.reassertionKey.current = null;
    if (!isDockviewTargetCurrent(refs, target)) return;
    refs.reassertionAttempted.current.add(targetKey);
    if (
      messageListRef.current?.scrollToMessage(target.messageId, {
        align: "start",
        behavior: "auto",
      })
    ) {
      refs.completedAround.current.delete(targetKey);
      clearScrollTarget(target.token);
    } else {
      refs.failedTargetKey.current = targetKey;
      setJumpError(true);
    }
  }, 250);
}

function requestDockviewTargetWindow(options: {
  refs: DockviewScrollLifecycle;
  target: DockviewScrollTarget;
  targetKey: string;
  appStore: ReturnType<typeof useAppStoreApi>;
  isCurrentTarget: () => boolean;
  setJumpLoading: (value: boolean) => void;
  setJumpError: (value: boolean) => void;
  clearScrollTarget: (token: number) => void;
  scheduleReassertion: () => void;
}) {
  const {
    refs,
    target,
    targetKey,
    appStore,
    isCurrentTarget,
    setJumpLoading,
    setJumpError,
    clearScrollTarget,
    scheduleReassertion,
  } = options;
  refs.requestKeys.current.add(targetKey);
  setJumpLoading(true);
  void loadMessageWindowAround(
    target.sessionId,
    target.messageId,
    () => isCurrentTarget(),
    appStore,
  )
    .then((result) => {
      if (!isCurrentTarget()) return;
      if (result.kind === "merged") {
        refs.completedAround.current.add(targetKey);
        if (refs.scrollSucceeded.current) scheduleReassertion();
      } else if (result.kind === "deleted-target") {
        clearScrollTarget(target.token);
      }
    })
    .catch(() => {
      if (isCurrentTarget()) {
        refs.failedTargetKey.current = targetKey;
        setJumpError(true);
      }
    })
    .finally(() => {
      refs.requestKeys.current.delete(targetKey);
      if (refs.mounted.current) setJumpLoading(refs.requestKeys.current.size > 0);
    });
}

function attemptDockviewScroll(options: {
  refs: DockviewScrollLifecycle;
  target: DockviewScrollTarget;
  targetKey: string;
  appStore: ReturnType<typeof useAppStoreApi>;
  messageListRef: RefObject<MessageListHandle | null>;
  isInitialMessagesLoading: boolean;
  targetRendered: boolean;
  isCurrentTarget: () => boolean;
  setJumpLoading: (value: boolean) => void;
  setJumpError: (value: boolean) => void;
  clearScrollTarget: (token: number) => void;
}) {
  const {
    refs,
    target,
    targetKey,
    appStore,
    messageListRef,
    isInitialMessagesLoading,
    targetRendered,
    isCurrentTarget,
    setJumpLoading,
    setJumpError,
    clearScrollTarget,
  } = options;
  if (
    refs.completedAround.current.has(targetKey) &&
    (refs.reassertionTimer.current !== null || refs.reassertionAttempted.current.has(targetKey))
  ) {
    return;
  }
  if (!isCurrentTarget()) return;
  const scheduleReassertion = () =>
    scheduleDockviewReassertion({
      refs,
      target,
      targetKey,
      messageListRef,
      clearScrollTarget,
      setJumpError,
    });
  const didScroll = messageListRef.current?.scrollToMessage(target.messageId, {
    align: "start",
  });
  if (didScroll) {
    refs.scrollSucceeded.current = true;
    if (refs.completedAround.current.has(targetKey)) {
      scheduleReassertion();
    } else if (!refs.requestKeys.current.has(targetKey)) {
      clearScrollTarget(target.token);
    }
    return;
  }
  const sessionMessages = appStore.getState().messages.bySession[target.sessionId] ?? [];
  if (
    shouldDeferTargetWindowLoad(
      sessionMessages,
      target.messageId,
      isInitialMessagesLoading,
      targetRendered,
    )
  ) {
    return;
  }
  if (refs.requestKeys.current.has(targetKey) || refs.completedAround.current.has(targetKey)) {
    return;
  }
  requestDockviewTargetWindow({
    refs,
    target,
    targetKey,
    appStore,
    isCurrentTarget,
    setJumpLoading,
    setJumpError,
    clearScrollTarget,
    scheduleReassertion,
  });
}

/**
 * Consumes Dockview prompt-history targets. Around-window targets stay owned
 * through their first rendered placement and one delayed reassertion.
 */
// eslint-disable-next-line max-lines-per-function -- Dockview target ownership and delayed reassertion share one lifecycle.
export function useScrollTargetConsumption({
  resolvedSessionId,
  isVisible,
  panelId,
  messageListRef,
  isInitialMessagesLoading,
  targetRendered = false,
  renderedMessageCount,
}: ScrollTargetConsumptionParams) {
  const scrollTarget = useDockviewStore((state) => state.scrollTarget);
  const clearScrollTarget = useDockviewStore((state) => state.clearScrollTarget);
  const appStore = useAppStoreApi();
  const clearScrollTargetForOwner = useDockviewStore((state) => state.clearScrollTargetForOwner);
  const [jumpLoading, setJumpLoading] = useState(false);
  const [jumpError, setJumpError] = useState(false);
  const [retryVersion, setRetryVersion] = useState(0);
  const reassertionKeyRef = useRef<string | null>(null);
  const reassertionTargetKeyRef = useRef<string | null>(null);
  const reassertionAttemptedRef = useRef(new Set<string>());
  const requestKeysRef = useRef(new Set<string>());
  const completedAroundRef = useRef(new Set<string>());
  const failedTargetKeyRef = useRef<string | null>(null);
  const scrollSucceededRef = useRef(false);
  const reassertionTimerRef = useRef<number | null>(null);
  const ownerCleanupTimerRef = useRef<number | null>(null);
  const ownerTargetTokenRef = useRef<number | null>(null);
  const previousSessionId = useRef<string | null>(null);
  const previousPanelId = useRef<string | null>(null);
  const mountedRef = useRef(true);
  const lifecycleRef = useRef({ resolvedSessionId, isVisible, panelId });

  const retry = useCallback(() => {
    failedTargetKeyRef.current = null;
    const targetKey = reassertionTargetKeyRef.current;
    if (targetKey) {
      completedAroundRef.current.delete(targetKey);
      reassertionAttemptedRef.current.delete(targetKey);
    }
    setJumpError(false);
    setRetryVersion((version) => version + 1);
  }, []);

  const cancelOwnerCleanup = useCallback(() => {
    if (ownerCleanupTimerRef.current !== null) {
      window.clearTimeout(ownerCleanupTimerRef.current);
      ownerCleanupTimerRef.current = null;
    }
  }, []);

  const scheduleOwnerCleanup = useCallback(
    (targetToken: number, ownerKey: string) => {
      cancelOwnerCleanup();
      ownerCleanupTimerRef.current = window.setTimeout(() => {
        ownerCleanupTimerRef.current = null;
        const activeOwner = activeDockviewOwners.get(ownerKey);
        if (activeOwner?.token === targetToken) return;
        clearScrollTarget(targetToken);
      }, 0);
    },
    [cancelOwnerCleanup, clearScrollTarget],
  );

  const cancelReassertion = useCallback(() => {
    if (reassertionTimerRef.current !== null) {
      window.clearTimeout(reassertionTimerRef.current);
      reassertionTimerRef.current = null;
    }
    reassertionKeyRef.current = null;
  }, []);
  lifecycleRef.current = { resolvedSessionId, isVisible, panelId };
  const ownerKey = getDockviewOwnerKey(resolvedSessionId, panelId);
  ownerTargetTokenRef.current =
    scrollTarget?.sessionId === resolvedSessionId && scrollTarget.hostPanelId === panelId
      ? scrollTarget.token
      : null;

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      cancelReassertion();
    };
  }, [cancelReassertion]);

  useEffect(() => {
    // Only a DOCKVIEW host (defined panelId) may invalidate or clear a
    // retained target. Non-Dockview hosts own their target lifecycle.
    if (!panelId || !ownerKey) return;
    const ownerGeneration = ++dockviewOwnerGeneration;
    activeDockviewOwners.set(ownerKey, {
      generation: ownerGeneration,
      token: ownerTargetTokenRef.current,
    });
    cancelOwnerCleanup();
    const previousSession = previousSessionId.current;
    const previousPanel = previousPanelId.current;
    previousSessionId.current = resolvedSessionId;
    previousPanelId.current = panelId;
    if (
      previousSession &&
      previousPanel &&
      (previousSession !== resolvedSessionId || previousPanel !== panelId)
    ) {
      clearScrollTargetForOwner(previousSession, previousPanel);
      cancelReassertion();
    }
    return () => {
      if (activeDockviewOwners.get(ownerKey)?.generation === ownerGeneration) {
        activeDockviewOwners.delete(ownerKey);
      }
      cancelReassertion();
      const targetToken = ownerTargetTokenRef.current;
      if (targetToken !== null) scheduleOwnerCleanup(targetToken, ownerKey);
    };
  }, [
    cancelOwnerCleanup,
    cancelReassertion,
    clearScrollTargetForOwner,
    panelId,
    ownerKey,
    resolvedSessionId,
    scheduleOwnerCleanup,
  ]);

  useEffect(() => {
    if (isVisible) return;
    cancelReassertion();
    if (
      !scrollTarget ||
      !panelId ||
      scrollTarget.sessionId !== resolvedSessionId ||
      scrollTarget.hostPanelId !== panelId
    ) {
      return;
    }
    const targetKey = `${scrollTarget.sessionId}\u0000${scrollTarget.hostPanelId}\u0000${scrollTarget.token}`;
    const navigationStarted =
      reassertionTargetKeyRef.current === targetKey &&
      (scrollSucceededRef.current ||
        requestKeysRef.current.has(targetKey) ||
        completedAroundRef.current.has(targetKey));
    if (navigationStarted) clearScrollTarget(scrollTarget.token);
  }, [cancelReassertion, clearScrollTarget, isVisible, panelId, resolvedSessionId, scrollTarget]);

  useEffect(() => {
    if (
      !scrollTarget ||
      !panelId ||
      scrollTarget.sessionId !== resolvedSessionId ||
      scrollTarget.hostPanelId !== panelId
    ) {
      failedTargetKeyRef.current = null;
      setJumpError(false);
      if (!isVisible) {
        cancelReassertion();
        completedAroundRef.current.clear();
      }
      return;
    }
    const targetKey = `${scrollTarget.sessionId}\u0000${scrollTarget.hostPanelId}\u0000${scrollTarget.token}`;
    if (reassertionTargetKeyRef.current !== targetKey) {
      reassertionTargetKeyRef.current = targetKey;
      reassertionAttemptedRef.current.delete(targetKey);
      failedTargetKeyRef.current = null;
      setJumpError(false);
      scrollSucceededRef.current = false;
    }
    if (reassertionKeyRef.current && reassertionKeyRef.current !== targetKey) {
      cancelReassertion();
      reassertionKeyRef.current = null;
    }
    if (failedTargetKeyRef.current === targetKey) return;
    const refs: DockviewScrollLifecycle = {
      reassertionKey: reassertionKeyRef,
      reassertionTargetKey: reassertionTargetKeyRef,
      reassertionAttempted: reassertionAttemptedRef,
      requestKeys: requestKeysRef,
      completedAround: completedAroundRef,
      failedTargetKey: failedTargetKeyRef,
      scrollSucceeded: scrollSucceededRef,
      reassertionTimer: reassertionTimerRef,
      mounted: mountedRef,
      lifecycle: lifecycleRef,
    };
    const isCurrentTarget = () => isDockviewTargetCurrent(refs, scrollTarget);
    let frame = requestAnimationFrame(() => {
      frame = requestAnimationFrame(() =>
        attemptDockviewScroll({
          refs,
          target: scrollTarget,
          targetKey,
          appStore,
          messageListRef,
          isInitialMessagesLoading,
          targetRendered,
          isCurrentTarget,
          setJumpLoading,
          setJumpError,
          clearScrollTarget,
        }),
      );
    });
    return () => cancelAnimationFrame(frame);
  }, [
    appStore,
    cancelReassertion,
    clearScrollTarget,
    isInitialMessagesLoading,
    targetRendered,
    isVisible,
    messageListRef,
    panelId,
    renderedMessageCount,
    resolvedSessionId,
    retryVersion,
    scrollTarget,
  ]);
  return { isLoading: jumpLoading, hasError: jumpError, retry };
}

// eslint-disable-next-line complexity, max-lines-per-function -- composes many sub-panels; each concern already factored into its own hook
export const TaskChatPanel = memo(function TaskChatPanel({
  onSend,
  sessionId = null,
  taskId: taskIdHint = null,
  statusTaskId = null,
  onOpenFile,
  showRequestChangesTooltip = false,
  onRequestChangesTooltipDismiss,
  onOpenFileAtLine,
  hideSessionsDropdown,
  embedded = false,
  isVisible = true,
  panelId = null,
  pendingScrollToMessageId = null,
  pendingScrollTarget,
  onPendingScrollConsumed,
}: TaskChatPanelProps) {
  const isArchived = useIsTaskArchived();
  const chatInputRef = useRef<ChatInputContainerHandle>(null);
  const launchErrorContext = useTaskLaunchErrorContext();
  const launchStatusSummary = useTaskStatusSummary(
    launchErrorContext?.taskId,
    launchErrorContext?.statusSummary,
  );
  const { t } = useTranslation();
  useSettingsData(true);
  const panelState = useChatPanelState({
    sessionId,
    taskId: taskIdHint,
    disableWorkbenchEffects: embedded,
    onOpenFile,
    onOpenFileAtLine,
  });
  const { isSending, handleSubmit } = useSubmitHandler(panelState, onSend);
  const {
    resolvedSessionId,
    session,
    taskId,
    isWorking,
    messagesLoading,
    historyRefreshPending,
    isInitialMessagesLoading,
    groupedItems,
    allMessages,
    footerActionMessages,
    permissionsByToolCallId,
    childrenByParentToolCallId,
    agentMessageCount,
    pendingClarification,
    pendingClarificationGroup,
  } = panelState;
  const showAgentStartHint = useComposerAgentStartHint(
    resolvedSessionId,
    session?.state,
    allMessages,
    footerActionMessages,
  );
  const { handleCancelTurn } = useChatPanelHandlers(resolvedSessionId, chatInputRef, {
    enableFocusShortcut: !embedded,
  });
  const { clarificationKey, handleClarificationResolved } = useClarificationKey(agentMessageCount);

  const panelRef = useRef<HTMLDivElement>(null);
  const messageListRef = useRef<MessageListHandle>(null);
  const dividerBeforeItemKey = useUnreadDividerBeforeItemKey(
    resolvedSessionId,
    isVisible,
    groupedItems,
    isInitialMessagesLoading,
  );
  // Kanban previews intentionally pass `isVisible=false` so they do not
  // advance the read cursor, but their transcript is rendered in a visible
  // non-Dockview host. Keep read visibility separate from scroll geometry.
  const transcriptIsVisible = panelId === null || isVisible;
  const dockviewScrollTarget = useDockviewStore((state) => state.scrollTarget);
  const {
    isLoading: isDockviewJumpLoading,
    hasError: hasDockviewJumpError,
    retry: retryDockviewJump,
  } = useScrollTargetConsumption({
    resolvedSessionId,
    isVisible,
    panelId,
    messageListRef,
    isInitialMessagesLoading,
    targetRendered: Boolean(
      dockviewScrollTarget?.messageId &&
      allMessages.some((message) => message.id === dockviewScrollTarget.messageId),
    ),
    renderedMessageCount: allMessages.length,
  });
  const {
    isLoading: isPendingJumpLoading,
    hasError: hasPendingJumpError,
    retry: retryPendingJump,
  } = usePendingMessageScroll({
    messageListRef,
    sessionId: resolvedSessionId,
    messageId: pendingScrollToMessageId,
    target: pendingScrollTarget,
    onConsumed: onPendingScrollConsumed,
    readinessKey: allMessages,
    isInitialMessagesLoading,
    isVisible,
  });
  const isJumpLoading = isDockviewJumpLoading || isPendingJumpLoading;
  const lastPromptMessageId = useMemo(() => getLastUserMessageId(allMessages), [allMessages]);
  const lastPromptMessage = useMemo(
    () =>
      lastPromptMessageId
        ? (allMessages.find((message) => message.id === lastPromptMessageId) ?? null)
        : null,
    [allMessages, lastPromptMessageId],
  );
  const [lastPromptEdge, setLastPromptEdge] = useState<LastPromptEdge>("visible");
  const showAnchoredPromptBar = useAppStore((state) => state.userSettings.showAnchoredPromptBar);
  const showScrollToLastPrompt = useAppStore((state) => state.userSettings.showScrollToLastPrompt);
  const showScrollToStart = useAppStore((state) => state.userSettings.showScrollToStart);
  const { isMobile } = useResponsiveBreakpoint();
  // The anchored bar is a desktop-only, opt-in affordance; mobile always
  // falls back to the scroll button.
  const showAnchoredBar = !isMobile && showAnchoredPromptBar;
  const [anchoredBarHeight, setAnchoredBarHeight] = useState(0);
  const { anchoredBarVisible, scrollButtonEligible, scrollDirection } =
    resolveLastPromptControls(lastPromptEdge);
  const showScrollButton =
    showScrollToLastPrompt && Boolean(lastPromptMessageId) && scrollButtonEligible;
  const scrollToLastPrompt = useCallback(() => {
    if (lastPromptMessageId) {
      messageListRef.current?.scrollToMessage(lastPromptMessageId, { align: "start" });
    }
  }, [lastPromptMessageId]);
  const firstMessageId = useMemo(() => getFirstUserMessageId(allMessages), [allMessages]);
  const [isFirstMessageHidden, setIsFirstMessageHidden] = useState(false);
  const showScrollToStartButton =
    showScrollToStart && Boolean(firstMessageId) && isFirstMessageHidden;
  const { loadMoreRaw, hasMore } = useLazyLoadMessages(resolvedSessionId);
  // A paginated session's `firstMessageId` only reflects the oldest message in
  // the currently loaded page while `hasMore` is true — jumping there directly
  // lands on a partial-page boundary, not the transcript's real start. Drain
  // older pages first so `firstMessageId` (derived from `allMessages` above,
  // which grows as pages prepend) has settled on the true first prompt by the
  // time the scroll fires.
  const [pendingScrollToStart, setPendingScrollToStart] = useState(false);
  useDrainOlderMessages(resolvedSessionId, pendingScrollToStart && hasMore);
  useEffect(() => {
    if (!pendingScrollToStart || hasMore) return;
    setPendingScrollToStart(false);
    if (firstMessageId) {
      messageListRef.current?.scrollToMessage(firstMessageId, { align: "start" });
    }
  }, [pendingScrollToStart, hasMore, firstMessageId]);
  const scrollToStart = useCallback(() => {
    if (hasMore) {
      setPendingScrollToStart(true);
      return;
    }
    if (firstMessageId) {
      messageListRef.current?.scrollToMessage(firstMessageId, { align: "start" });
    }
  }, [hasMore, firstMessageId]);
  // Search can target backend rows before the visible transcript boundary.
  const search = useSessionSearch(resolvedSessionId, loadMoreRaw);
  const { label: agentLabel, name: agentName } = useSessionAgentIdentity(resolvedSessionId);
  usePanelSearch({
    containerRef: panelRef,
    isOpen: search.isOpen,
    onOpen: search.open,
    onClose: search.close,
  });

  // The message list has no focus-capturing child (unlike TipTap/xterm in the
  // plan/terminal panels), so clicking a message leaves focus on <body>. Make
  // the panel root itself focusable and route non-interactive clicks to it so
  // Ctrl+F can detect focus within the session panel.
  const handlePanelMouseDown = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => routePanelMouseDown(e, panelRef),
    [],
  );

  return (
    <PanelRoot
      ref={panelRef}
      data-testid="session-chat"
      data-panel-kind="session"
      data-session-id={resolvedSessionId ?? undefined}
      tabIndex={-1}
      onMouseDown={handlePanelMouseDown}
      className="outline-none"
    >
      <PanelBody padding={false} className="relative">
        {launchErrorContext && (
          <TaskChatLaunchError
            taskId={launchErrorContext.taskId}
            workspaceId={launchErrorContext.workspaceId}
            statusSummary={launchStatusSummary}
            runErrors={[]}
            repositories={launchErrorContext.repositories}
          />
        )}
        <TaskMarkdownFileLinkProvider
          taskId={taskId}
          sessionId={resolvedSessionId}
          worktreePath={getSessionWorkspacePath(session)}
          onOpenFile={onOpenFile}
        >
          <MessageList
            ref={messageListRef}
            items={groupedItems}
            messages={allMessages}
            footerActionMessages={footerActionMessages}
            permissionsByToolCallId={permissionsByToolCallId}
            childrenByParentToolCallId={childrenByParentToolCallId}
            taskId={taskId ?? undefined}
            sessionId={resolvedSessionId}
            messagesLoading={messagesLoading}
            historyRefreshPending={historyRefreshPending}
            isWorking={isWorking}
            sessionState={session?.state}
            worktreePath={getSessionWorkspacePath(session)}
            onOpenFile={onOpenFile}
            dividerBeforeItemKey={dividerBeforeItemKey}
            lastPromptMessageId={lastPromptMessageId}
            onLastPromptEdgeChange={setLastPromptEdge}
            firstMessageId={firstMessageId}
            onFirstMessageHiddenChange={setIsFirstMessageHidden}
            anchoredBarHeight={showAnchoredBar && lastPromptMessage ? anchoredBarHeight : 0}
            isVisible={transcriptIsVisible}
            stickyPromptBar={
              showAnchoredBar && lastPromptMessage ? (
                <AnchoredLastPromptBar
                  promptText={lastPromptMessage.content}
                  isVisible={anchoredBarVisible}
                  onScrollUp={scrollToLastPrompt}
                  showScrollToLastPrompt={showScrollToLastPrompt}
                  onHeightChange={setAnchoredBarHeight}
                />
              ) : undefined
            }
          />
        </TaskMarkdownFileLinkProvider>
        {isJumpLoading && (
          <div
            data-testid="transcript-jump-loading"
            role="status"
            aria-live="polite"
            className="absolute right-3 top-3 rounded-md bg-background px-2 py-1 text-xs text-muted-foreground shadow"
          >
            {t("task:loading")}
          </div>
        )}
        {(hasPendingJumpError || hasDockviewJumpError) && (
          <button
            type="button"
            data-testid="transcript-jump-retry"
            className="absolute right-3 top-3 min-h-11 rounded-md border border-border bg-background px-2 py-1 text-xs text-muted-foreground shadow"
            onClick={() => {
              retryPendingJump();
              retryDockviewJump();
            }}
          >
            {t("task:retry")}
          </button>
        )}
        <SessionSearchOverlay search={search} agentLabel={agentLabel} agentName={agentName} />
      </PanelBody>
      {!isArchived && (
        <ClarificationPanelSection
          pending={Boolean(pendingClarification)}
          messages={pendingClarificationGroup}
          onResolved={handleClarificationResolved}
          shortcutScopeRef={panelRef}
          maxHeightVh={50}
        />
      )}
      <ChatFooter
        isArchived={isArchived}
        chatInputRef={chatInputRef}
        clarificationKey={clarificationKey}
        onClarificationResolved={handleClarificationResolved}
        handleSubmit={handleSubmit}
        handleCancelTurn={handleCancelTurn}
        showRequestChangesTooltip={showRequestChangesTooltip}
        onRequestChangesTooltipDismiss={onRequestChangesTooltipDismiss}
        panelState={panelState}
        isSending={isSending}
        hideSessionsDropdown={hideSessionsDropdown}
        hidePlanMode={embedded}
        showScrollToLastPrompt={showScrollButton}
        onScrollToLastPrompt={scrollToLastPrompt}
        lastPromptScrollDirection={scrollDirection}
        showScrollToStart={showScrollToStartButton}
        onScrollToStart={scrollToStart}
        statusTaskId={statusTaskId ?? taskIdHint}
        showAgentStartHint={showAgentStartHint}
      />
    </PanelRoot>
  );
});

type ChatFooterProps = {
  isArchived: boolean;
  chatInputRef: RefObject<
    import("@/components/task/chat/chat-input-container").ChatInputContainerHandle | null
  >;
  clarificationKey: number;
  onClarificationResolved: () => void;
  handleSubmit: ReturnType<typeof useSubmitHandler>["handleSubmit"];
  handleCancelTurn: () => Promise<void>;
  showRequestChangesTooltip: boolean;
  onRequestChangesTooltipDismiss?: () => void;
  panelState: ReturnType<typeof useChatPanelState>;
  isSending: boolean;
  hideSessionsDropdown?: boolean;
  hidePlanMode?: boolean;
  showScrollToLastPrompt: boolean;
  onScrollToLastPrompt: () => void;
  lastPromptScrollDirection: "up" | "down";
  showScrollToStart: boolean;
  onScrollToStart: () => void;
  statusTaskId: string | null;
  /** Recovered-idle sessions render the composer hint (see ChatInputArea). */
  showAgentStartHint: boolean;
};

/**
 * Composer footer: renders the chat input area (or the read-only archived
 * banner) and forwards the recovered-idle agent-start-hint visibility from
 * the panel down to the input.
 */
function ChatFooter({
  isArchived,
  chatInputRef,
  clarificationKey,
  onClarificationResolved,
  handleSubmit,
  handleCancelTurn,
  showRequestChangesTooltip,
  onRequestChangesTooltipDismiss,
  panelState,
  isSending,
  hideSessionsDropdown,
  hidePlanMode,
  showScrollToLastPrompt,
  onScrollToLastPrompt,
  lastPromptScrollDirection,
  showScrollToStart,
  onScrollToStart,
  statusTaskId,
  showAgentStartHint,
}: ChatFooterProps) {
  const { t } = useTranslation();
  if (isArchived) {
    return (
      <div className="bg-muted/50 flex-shrink-0 px-4 py-3 text-center text-sm text-muted-foreground border-t">
        {t("task:thisTaskIsArchivedAndRead")}
      </div>
    );
  }
  return (
    <ChatInputArea
      chatInputRef={chatInputRef}
      clarificationKey={clarificationKey}
      onClarificationResolved={onClarificationResolved}
      handleSubmit={handleSubmit}
      handleCancelTurn={handleCancelTurn}
      showRequestChangesTooltip={showRequestChangesTooltip}
      onRequestChangesTooltipDismiss={onRequestChangesTooltipDismiss}
      panelState={panelState}
      isSending={isSending}
      hideSessionsDropdown={hideSessionsDropdown}
      hidePlanMode={hidePlanMode}
      showScrollToLastPrompt={showScrollToLastPrompt}
      onScrollToLastPrompt={onScrollToLastPrompt}
      lastPromptScrollDirection={lastPromptScrollDirection}
      showScrollToStart={showScrollToStart}
      onScrollToStart={onScrollToStart}
      statusTaskId={statusTaskId}
      showAgentStartHint={showAgentStartHint}
    />
  );
}
