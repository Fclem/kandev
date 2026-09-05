"use client";

/* eslint-disable max-lines -- this component composes the complete transcript surface. */

import { useCallback, useEffect, useMemo, useRef, useState, memo, type RefObject } from "react";
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
  readinessKey: string;
  isInitialMessagesLoading: boolean;
  isVisible?: boolean;
};

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
  const scrollSucceededRef = useRef(false);
  const reassertionTimerRef = useRef<number | null>(null);
  const reassertionAttemptedRef = useRef(new Set<string>());
  const mountedRef = useRef(true);
  const lifecycleRef = useRef({ sessionId, messageId, target, isVisible });
  const retry = useCallback(() => setRetryVersion((version) => version + 1), []);

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
  // eslint-disable-next-line max-lines-per-function -- target loading and delayed reassertion share one lifecycle.
  useEffect(() => {
    if (!targetBelongsToHost) {
      targetIdentityRef.current = null;
      completedAroundRef.current.clear();
      cancelReassertion();
      setIsLoading(false);
      setHasError(false);
      onConsumed?.(target?.messageId ?? "");
      return;
    }
    if (!effectiveSessionId || !effectiveMessageId || !effectiveTargetKey) {
      targetIdentityRef.current = null;
      completedAroundRef.current.clear();
      cancelReassertion();
      setIsLoading(false);
      setHasError(false);
      return;
    }
    if (targetIdentityRef.current !== effectiveTargetKey) {
      scrollSucceededRef.current = false;
      targetIdentityRef.current = effectiveTargetKey;
      completedAroundRef.current.clear();
      cancelReassertion();
    }
    if (!isVisible) {
      cancelReassertion();
      completedAroundRef.current.delete(effectiveTargetKey);
    }
    setHasError(false);
    let frame = 0;
    const isCurrentTarget = () =>
      mountedRef.current &&
      lifecycleRef.current.isVisible &&
      targetIdentityRef.current === effectiveTargetKey &&
      lifecycleRef.current.sessionId === effectiveSessionId &&
      lifecycleRef.current.messageId === effectiveMessageId;
    if (!isVisible) return;
    const scheduleReassertion = () => {
      if (
        reassertionTimerRef.current !== null ||
        reassertionAttemptedRef.current.has(effectiveTargetKey) ||
        !isCurrentTarget()
      )
        return;
      reassertionTimerRef.current = window.setTimeout(() => {
        reassertionTimerRef.current = null;
        if (!isCurrentTarget()) return;
        reassertionAttemptedRef.current.add(effectiveTargetKey);
        if (
          messageListRef.current?.scrollToMessage(effectiveMessageId, {
            align: "start",
            behavior: "auto",
          })
        ) {
          completedAroundRef.current.delete(effectiveTargetKey);
          onConsumed?.(effectiveMessageId);
        }
      }, 250);
    };
    const attemptScroll = () => {
      if (!isCurrentTarget()) return;
      if (
        completedAroundRef.current.has(effectiveTargetKey) &&
        (reassertionTimerRef.current !== null ||
          reassertionAttemptedRef.current.has(effectiveTargetKey))
      ) {
        return;
      }
      if (
        messageListRef.current?.scrollToMessage(effectiveMessageId, {
          align: "start",
          behavior: "auto",
        })
      ) {
        scrollSucceededRef.current = true;
        if (completedAroundRef.current.has(effectiveTargetKey)) {
          scheduleReassertion();
        } else if (!requestKeysRef.current.has(effectiveTargetKey)) {
          onConsumed?.(effectiveMessageId);
        }
        return;
      }
      if (isInitialMessagesLoading) return;
      const loaded = store
        .getState()
        .messages.bySession[
          effectiveSessionId
        ]?.some((message) => message.id === effectiveMessageId);
      if (loaded) return;
      if (
        requestKeysRef.current.has(effectiveTargetKey) ||
        completedAroundRef.current.has(effectiveTargetKey)
      ) {
        return;
      }
      requestKeysRef.current.add(effectiveTargetKey);
      setIsLoading(true);
      void loadMessageWindowAround(
        effectiveSessionId,
        effectiveMessageId,
        () =>
          isCurrentTarget() &&
          targetIdentityRef.current === effectiveTargetKey &&
          (!target || lifecycleRef.current.target?.token === targetToken),
        store,
      )
        .then((result) => {
          if (!isCurrentTarget()) return;
          if (result.kind === "deleted-target") {
            onConsumed?.(effectiveMessageId);
          } else if (result.kind === "merged") {
            completedAroundRef.current.add(effectiveTargetKey);
            if (scrollSucceededRef.current) scheduleReassertion();
          }
        })
        .catch(() => {
          if (isCurrentTarget()) {
            completedAroundRef.current.delete(effectiveTargetKey);
            setHasError(true);
          }
        })
        .finally(() => {
          requestKeysRef.current.delete(effectiveTargetKey);
          if (mountedRef.current) setIsLoading(requestKeysRef.current.size > 0);
        });
      return;
    };
    frame = requestAnimationFrame(attemptScroll);
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
function shouldDeferTargetWindowLoad(
  messages: readonly { id: string }[],
  targetMessageId: string,
  isInitialMessagesLoading: boolean,
  targetRendered: boolean,
) {
  const loaded = messages.some((message) => message.id === targetMessageId);
  return (
    (isInitialMessagesLoading && (messages.length === 0 || loaded)) || (loaded && targetRendered)
  );
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
  const reassertionKeyRef = useRef<string | null>(null);
  const reassertionTargetKeyRef = useRef<string | null>(null);
  const reassertionAttemptedRef = useRef(new Set<string>());
  const requestKeysRef = useRef(new Set<string>());
  const completedAroundRef = useRef(new Set<string>());
  const scrollSucceededRef = useRef(false);
  const reassertionTimerRef = useRef<number | null>(null);
  const ownerCleanupTimerRef = useRef<number | null>(null);
  const ownerTargetTokenRef = useRef<number | null>(null);
  const previousSessionId = useRef<string | null>(null);
  const previousPanelId = useRef<string | null>(null);
  const mountedRef = useRef(true);
  const lifecycleRef = useRef({ resolvedSessionId, isVisible, panelId });

  const cancelOwnerCleanup = useCallback(() => {
    if (ownerCleanupTimerRef.current !== null) {
      window.clearTimeout(ownerCleanupTimerRef.current);
      ownerCleanupTimerRef.current = null;
    }
  }, []);

  const scheduleOwnerCleanup = useCallback(
    (targetToken: number) => {
      cancelOwnerCleanup();
      ownerCleanupTimerRef.current = window.setTimeout(() => {
        ownerCleanupTimerRef.current = null;
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
    if (!panelId) return;
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
      cancelReassertion();
      const targetToken = ownerTargetTokenRef.current;
      if (targetToken !== null) scheduleOwnerCleanup(targetToken);
    };
  }, [
    cancelOwnerCleanup,
    cancelReassertion,
    clearScrollTargetForOwner,
    panelId,
    resolvedSessionId,
    scheduleOwnerCleanup,
  ]);

  useEffect(() => {
    if (!isVisible) cancelReassertion();
  }, [cancelReassertion, isVisible]);

  // eslint-disable-next-line max-lines-per-function -- Target ownership and delayed reassertion share one lifecycle.
  useEffect(() => {
    if (
      !scrollTarget ||
      !panelId ||
      scrollTarget.sessionId !== resolvedSessionId ||
      scrollTarget.hostPanelId !== panelId
    ) {
      if (!isVisible) {
        cancelReassertion();
        completedAroundRef.current.clear();
      }
      return;
    }
    const targetToken = scrollTarget.token;
    const targetKey = `${scrollTarget.sessionId}\u0000${scrollTarget.hostPanelId}\u0000${targetToken}`;
    if (reassertionTargetKeyRef.current !== targetKey) {
      reassertionTargetKeyRef.current = targetKey;
      reassertionAttemptedRef.current.delete(targetKey);
      scrollSucceededRef.current = false;
    }
    if (reassertionKeyRef.current && reassertionKeyRef.current !== targetKey) {
      cancelReassertion();
      reassertionKeyRef.current = null;
    }
    const isCurrentTarget = () => {
      const latest = useDockviewStore.getState().scrollTarget;
      return (
        mountedRef.current &&
        lifecycleRef.current.isVisible &&
        lifecycleRef.current.resolvedSessionId === scrollTarget.sessionId &&
        lifecycleRef.current.panelId === scrollTarget.hostPanelId &&
        latest?.token === targetToken &&
        latest.sessionId === scrollTarget.sessionId &&
        latest.hostPanelId === scrollTarget.hostPanelId
      );
    };
    const scheduleReassertion = () => {
      if (
        reassertionTimerRef.current !== null ||
        reassertionAttemptedRef.current.has(targetKey) ||
        !isCurrentTarget()
      )
        return;
      reassertionKeyRef.current = targetKey;
      reassertionTimerRef.current = window.setTimeout(() => {
        reassertionTimerRef.current = null;
        reassertionKeyRef.current = null;
        if (!isCurrentTarget()) return;
        reassertionAttemptedRef.current.add(targetKey);
        if (
          messageListRef.current?.scrollToMessage(scrollTarget.messageId, {
            align: "start",
            behavior: "auto",
          })
        ) {
          completedAroundRef.current.delete(targetKey);
          clearScrollTarget(targetToken);
        }
      }, 250);
    };
    let frame = requestAnimationFrame(() => {
      frame = requestAnimationFrame(() => {
        if (
          completedAroundRef.current.has(targetKey) &&
          (reassertionTimerRef.current !== null || reassertionAttemptedRef.current.has(targetKey))
        ) {
          return;
        }
        if (!isCurrentTarget()) return;
        const didScroll = messageListRef.current?.scrollToMessage(scrollTarget.messageId, {
          align: "start",
        });
        if (didScroll) {
          scrollSucceededRef.current = true;
          if (completedAroundRef.current.has(targetKey)) {
            scheduleReassertion();
          } else if (!requestKeysRef.current.has(targetKey)) {
            clearScrollTarget(targetToken);
          }
          return;
        }
        const sessionMessages =
          appStore.getState().messages.bySession[scrollTarget.sessionId] ?? [];
        if (
          shouldDeferTargetWindowLoad(
            sessionMessages,
            scrollTarget.messageId,
            isInitialMessagesLoading,
            targetRendered,
          )
        ) {
          return;
        }
        if (requestKeysRef.current.has(targetKey) || completedAroundRef.current.has(targetKey)) {
          return;
        }
        requestKeysRef.current.add(targetKey);
        setJumpLoading(true);
        void loadMessageWindowAround(
          scrollTarget.sessionId,
          scrollTarget.messageId,
          () => isCurrentTarget(),
          appStore,
        )
          .then((result) => {
            if (!isCurrentTarget()) return;
            if (result.kind === "merged") {
              completedAroundRef.current.add(targetKey);
              if (scrollSucceededRef.current) scheduleReassertion();
            } else if (result.kind === "deleted-target") {
              clearScrollTarget(targetToken);
            }
          })
          .catch(() => {
            if (isCurrentTarget()) clearScrollTarget(targetToken);
          })
          .finally(() => {
            requestKeysRef.current.delete(targetKey);
            if (mountedRef.current) setJumpLoading(requestKeysRef.current.size > 0);
          });
      });
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
    scrollTarget,
  ]);
  return jumpLoading;
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
  const isDockviewJumpLoading = useScrollTargetConsumption({
    resolvedSessionId,
    isVisible,
    panelId,
    messageListRef,
    isInitialMessagesLoading,
    targetRendered: Boolean(
      useDockviewStore.getState().scrollTarget?.messageId &&
      allMessages.some(
        (message) => message.id === useDockviewStore.getState().scrollTarget?.messageId,
      ),
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
    readinessKey: `${allMessages.length}:${isInitialMessagesLoading}:${allMessages
      .map((message) => message.id)
      .join("\u0000")}`,
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
        {hasPendingJumpError && (
          <button
            type="button"
            data-testid="transcript-jump-retry"
            className="absolute right-3 top-3 min-h-11 rounded-md border border-border bg-background px-2 py-1 text-xs text-muted-foreground shadow"
            onClick={retryPendingJump}
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
