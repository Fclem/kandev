import type { Draft } from "immer";
import type { AppState, HydrationState } from "../store";
import { migrateSidebarViewDraft, migrateView } from "../slices/ui/ui-slice";
import {
  applyStoredQuickChatNames,
  reconcileQuickTerminalTabs,
} from "@/lib/state/slices/ui/quick-chat-sync";
import { compareUserSettingsRevisions } from "@/lib/settings/user-settings-revision";
import { isSettledSessionState, parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";
import { deepMerge, mergeSessionMap, mergeLoadingState } from "./merge-strategies";

/**
 * Hydration options for controlling merge behavior
 */
export type HydrationOptions = {
  /** Active session ID to avoid overwriting live data */
  activeSessionId?: string | null;
  /** Whether to skip hydrating session runtime state (shell, processes, git) */
  skipSessionRuntime?: boolean;
  /** Force merge this session even if it's active (for navigation refresh) */
  forceMergeSessionId?: string | null;
};

/** Deep-merge a field with optional loading state preservation. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function mergeWithLoading(draft: any, source: any | undefined): void {
  if (!source) return;
  deepMerge(draft, source);
  mergeLoadingState(draft, source);
}

/** Merge kanban tasks by ID, keeping the version with the newer updatedAt timestamp. */
function mergeKanbanTasks(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  draft: Draft<any>,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  source: any[] | undefined,
): void {
  if (!source || source.length === 0) return;
  const draftTasks = draft.tasks as Array<{ id: string; updatedAt?: string }>;
  const existingById = new Map(draftTasks.map((t) => [t.id, t]));

  for (const incoming of source) {
    const existing = existingById.get(incoming.id);
    if (!existing) {
      draftTasks.push(incoming);
    } else {
      const existingTime = existing.updatedAt ? new Date(existing.updatedAt).getTime() : 0;
      const incomingTime = incoming.updatedAt ? new Date(incoming.updatedAt).getTime() : 0;
      const idx = draftTasks.findIndex((t) => t.id === incoming.id);
      if (incomingTime >= existingTime) {
        if (idx >= 0) draftTasks[idx] = incoming;
      } else if (idx >= 0) {
        backfillServerDerivedFields(draftTasks[idx], incoming);
      }
    }
  }
}

/**
 * Fields the server derives per read and lightweight WS events omit.
 *
 * A task.updated that arrives before hydration inserts a copy with a newer
 * `updatedAt` and no dependency projection, so the timestamp rule above would
 * keep that copy and the boot payload's edges would be lost for the rest of the
 * session — which is exactly how the dependency chip silently vanished on any
 * task whose agent had produced activity. Backfilling only fills gaps, so a real
 * WS-side value (including an explicit "no edges") still wins.
 */
const SERVER_DERIVED_TASK_FIELDS = [
  "blocked",
  "blockedReason",
  "dependsOn",
  "blocks",
  "startWhenUnblocked",
] as const;

function backfillServerDerivedFields(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  target: any,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  source: any,
): void {
  for (const field of SERVER_DERIVED_TASK_FIELDS) {
    if (target[field] === undefined && source[field] !== undefined) {
      target[field] = source[field];
    }
  }
}

/** Hydrate kanban and workspace slices. */
function hydrateKanbanAndWorkspace(draft: Draft<AppState>, state: HydrationState): void {
  if (state.kanban) {
    // Merge tasks by ID with timestamp comparison to avoid overwriting fresher WS data
    const { tasks, ...kanbanRest } = state.kanban;
    if (Object.keys(kanbanRest).length > 0) deepMerge(draft.kanban, kanbanRest);
    mergeKanbanTasks(draft.kanban, tasks);
  }
  if (state.kanbanMulti) deepMerge(draft.kanbanMulti, state.kanbanMulti);
  if (state.workflows) deepMerge(draft.workflows, state.workflows);
  if (state.tasks) deepMerge(draft.tasks, state.tasks);
  if (state.workspaces) deepMerge(draft.workspaces, state.workspaces);
  if (state.repositories) deepMerge(draft.repositories, state.repositories);
  if (state.repositoryBranches) deepMerge(draft.repositoryBranches, state.repositoryBranches);
}

/** Hydrate settings slices, preserving loading states. */
function hydrateSettings(draft: Draft<AppState>, state: HydrationState): void {
  if (state.executors) deepMerge(draft.executors, state.executors);
  if (state.settingsAgents) deepMerge(draft.settingsAgents, state.settingsAgents);
  if (state.agentDiscovery) deepMerge(draft.agentDiscovery, state.agentDiscovery);
  mergeWithLoading(draft.availableAgents, state.availableAgents);
  if (state.agentProfiles) deepMerge(draft.agentProfiles, state.agentProfiles);
  mergeWithLoading(draft.editors, state.editors);
  mergeWithLoading(draft.prompts, state.prompts);
  mergeWithLoading(draft.notificationProviders, state.notificationProviders);
  if (state.settingsData) deepMerge(draft.settingsData, state.settingsData);
  if (state.sleepInhibition) deepMerge(draft.sleepInhibition, state.sleepInhibition);
  if (state.userSettings && shouldHydrateUserSettings(draft.userSettings, state.userSettings)) {
    deepMerge(draft.userSettings, state.userSettings);
    bridgeSidebarViewsFromUserSettings(draft, state.userSettings);
  }
}

function shouldHydrateUserSettings(
  current: Draft<AppState["userSettings"]>,
  incoming: Partial<AppState["userSettings"]>,
): boolean {
  const order = compareUserSettingsRevisions(incoming.revision, current.revision);
  return order === null ? !current.loaded : order >= 0;
}

function bridgeSidebarViewsFromUserSettings(
  draft: Draft<AppState>,
  userSettings: Partial<AppState["userSettings"]>,
): void {
  const serverViews = userSettings.sidebarViews;
  const normalized = serverViews?.map(migrateView) ?? [];
  if (normalized.length > 0) {
    draft.sidebarViews.views = normalized;
  }
  if (
    userSettings.sidebarActiveViewId &&
    draft.sidebarViews.views.some((v) => v.id === userSettings.sidebarActiveViewId)
  ) {
    draft.sidebarViews.activeViewId = userSettings.sidebarActiveViewId;
  } else if (
    draft.sidebarViews.views.length > 0 &&
    !draft.sidebarViews.views.some((v) => v.id === draft.sidebarViews.activeViewId)
  ) {
    draft.sidebarViews.activeViewId = draft.sidebarViews.views[0].id;
  }
  if (userSettings.sidebarDraft !== undefined) {
    draft.sidebarViews.draft = userSettings.sidebarDraft
      ? migrateSidebarViewDraft(userSettings.sidebarDraft)
      : null;
  }
  if (userSettings.sidebarTaskPrefs) {
    if (draft.sidebarTaskPrefs.syncPending) return;
    const nextPrefs = { ...userSettings.sidebarTaskPrefs };
    if (draft.sidebarTaskPrefs.syncError) nextPrefs.syncError = draft.sidebarTaskPrefs.syncError;
    draft.sidebarTaskPrefs = nextPrefs;
  }
}

/**
 * Seeds settled boundaries for settled sessions in the hydrated payload
 * (monotonically: a valid, strictly-newer candidate wins; existing boundaries
 * and their precision are preserved otherwise). This is the production SSR
 * path — StateHydrator hydrates the root store through hydrateState, so
 * settled-session boundaries must be established here, not only in
 * mergeInitialState, or an old/unknown delayed WS start would pass the guard
 * before any later session-list refresh.
 */
function seedSettledSessionBoundaries(draft: Draft<AppState>, state: HydrationState): void {
  for (const session of Object.values(state.taskSessions?.items ?? {})) {
    if (!session || !isSettledSessionState(session.state)) continue;
    const candidate = parseTurnTimestamp(session.updated_at);
    if (candidate === null) continue;
    const current = parseTurnTimestamp(draft.turns.settledBoundaryBySession[session.id]);
    if (current === null || candidate > current) {
      draft.turns.settledBoundaryBySession[session.id] = session.updated_at;
    }
  }
}

/**
 * A server snapshot may still name a turn that an authoritative boundary
 * (source adoption / settled-session clear) retired client-side. Never let a
 * force-merge resurrect its marker. Only sweeps markers this merge actually
 * INSTALLED (mirroring mergeSessionMap's write predicate): the protected
 * active session's entries are skipped by the merge, so its live marker must
 * not be cleared, and pre-existing non-force-merged markers are the client's
 * live state, not the snapshot's.
 */
function clearHydratedRetiredActiveMarkers(
  draft: Draft<AppState>,
  state: HydrationState,
  activeSessionId: string | null,
  forceMergeSessionId: string | null,
  preMergeActiveSessions: Set<string>,
): void {
  for (const sessionId in state.turns!.activeBySession) {
    const shouldForceMerge = forceMergeSessionId && sessionId === forceMergeSessionId;
    const skippedAsActive = !shouldForceMerge && sessionId === activeSessionId;
    if (skippedAsActive) continue;
    if (!shouldForceMerge && preMergeActiveSessions.has(sessionId)) continue;
    const hydrated = draft.turns.activeBySession[sessionId];
    if (!hydrated) continue;
    const boundary = parseTurnTimestamp(draft.turns.settledBoundaryBySession[sessionId]);
    if (boundary === null) continue;
    const hydratedTurn = state.turns!.bySession?.[sessionId]?.find((turn) => turn.id === hydrated);
    const started = parseTurnTimestamp(hydratedTurn?.started_at);
    if (started !== null && started <= boundary) {
      draft.turns.activeBySession[sessionId] = null;
    }
  }
}

/**
 * Marks the sessions whose turn lists this merge actually installed as fully
 * loaded. The hydrated lists are complete server-side snapshots; the marker
 * keeps turn-derived UI from re-fetching or mistaking WS-seeded live turns
 * for the full history. Membership must be captured BEFORE the merge (the
 * merge writes the key, which would otherwise make every session look
 * pre-existing), and the merge's skip-as-active predicate must be mirrored
 * (active sessions keep their live state and marker).
 */
function markHydratedTurnsLoaded(
  draft: Draft<AppState>,
  state: HydrationState,
  activeSessionId: string | null,
  forceMergeSessionId: string | null,
  preMergeSessions: Set<string>,
): void {
  for (const sessionId in state.turns!.bySession) {
    const shouldForceMerge = forceMergeSessionId && sessionId === forceMergeSessionId;
    const skippedAsActive = !shouldForceMerge && sessionId === activeSessionId;
    if (skippedAsActive) continue;
    if (shouldForceMerge || !preMergeSessions.has(sessionId)) {
      draft.turns.loadedBySession[sessionId] = true;
    }
  }
}

/** Hydrate session slices, protecting active sessions. */
function hydrateSession(
  draft: Draft<AppState>,
  state: HydrationState,
  activeSessionId: string | null,
  forceMergeSessionId: string | null,
): void {
  if (state.messages) {
    if (state.messages.bySession)
      mergeSessionMap(
        draft.messages.bySession,
        state.messages.bySession,
        activeSessionId,
        forceMergeSessionId,
      );
    if (state.messages.metaBySession)
      mergeSessionMap(
        draft.messages.metaBySession,
        state.messages.metaBySession,
        activeSessionId,
        forceMergeSessionId,
      );
  }
  // Seed settled boundaries BEFORE the marker merge/clear: the stale-marker
  // sweep reads the boundary, so a settled SSR session must have it installed
  // first or a pre-boundary active marker survives (see
  // clearHydratedRetiredActiveMarkers).
  if (state.taskSessions) {
    deepMerge(draft.taskSessions, state.taskSessions);
    seedSettledSessionBoundaries(draft, state);
  }
  if (state.turns) {
    if (state.turns.bySession) {
      const preMergeSessions = new Set(Object.keys(draft.turns.bySession));
      mergeSessionMap(
        draft.turns.bySession,
        state.turns.bySession,
        activeSessionId,
        forceMergeSessionId,
      );
      markHydratedTurnsLoaded(draft, state, activeSessionId, forceMergeSessionId, preMergeSessions);
    }
    if (state.turns.activeBySession) {
      const preMergeActiveSessions = new Set(Object.keys(draft.turns.activeBySession));
      mergeSessionMap(
        draft.turns.activeBySession,
        state.turns.activeBySession,
        activeSessionId,
        forceMergeSessionId,
      );
      clearHydratedRetiredActiveMarkers(
        draft,
        state,
        activeSessionId,
        forceMergeSessionId,
        preMergeActiveSessions,
      );
    }
  }
  if (state.taskSessionsByTask) deepMerge(draft.taskSessionsByTask, state.taskSessionsByTask);
  if (state.sessionAgentctl) {
    mergeSessionMap(
      draft.sessionAgentctl.itemsBySessionId,
      state.sessionAgentctl?.itemsBySessionId,
      activeSessionId,
      forceMergeSessionId,
    );
  }
  if (state.worktrees) deepMerge(draft.worktrees, state.worktrees);
  if (state.sessionWorktreesBySessionId)
    deepMerge(draft.sessionWorktreesBySessionId, state.sessionWorktreesBySessionId);
  if (state.pendingModel) deepMerge(draft.pendingModel, state.pendingModel);
  if (state.activeModel) deepMerge(draft.activeModel, state.activeModel);
}

/** Hydrate session runtime slices (volatile state). */
function hydrateSessionRuntime(
  draft: Draft<AppState>,
  state: HydrationState,
  activeSessionId: string | null,
  forceMergeSessionId: string | null,
): void {
  // Merge a `{ bySessionId }`-shaped slice, protecting the active session.
  const mergeBySession = (key: keyof AppState & keyof HydrationState): void => {
    const source = state[key] as { bySessionId?: Record<string, unknown> } | undefined;
    if (!source) return;
    const target = draft[key] as { bySessionId?: Record<string, unknown> } | undefined;
    if (!target?.bySessionId) return;
    mergeSessionMap(target.bySessionId, source.bySessionId, activeSessionId, forceMergeSessionId);
  };

  if (state.terminal) deepMerge(draft.terminal, state.terminal);
  if (state.shell) {
    mergeSessionMap(
      draft.shell.outputs,
      state.shell?.outputs,
      activeSessionId,
      forceMergeSessionId,
    );
    mergeSessionMap(
      draft.shell.statuses,
      state.shell?.statuses,
      activeSessionId,
      forceMergeSessionId,
    );
  }
  if (state.processes) deepMerge(draft.processes, state.processes);
  if (state.gitStatus) {
    mergeSessionMap(
      draft.gitStatus.byEnvironmentId,
      state.gitStatus?.byEnvironmentId,
      activeSessionId,
      forceMergeSessionId,
    );
  }
  mergeBySession("contextWindow");
  if (state.environmentIdBySessionId) {
    Object.assign(draft.environmentIdBySessionId, state.environmentIdBySessionId);
  }
  mergeBySession("sessionModels");
  mergeBySession("sessionMcpStatus");
  if (state.agents) deepMerge(draft.agents, state.agents);
  mergeBySession("prepareProgress");
}

/** Hydrate UI slices without overwriting active connection state. */
export function hydrateUI(draft: Draft<AppState>, state: HydrationState): void {
  if (state.previewPanel) deepMerge(draft.previewPanel, state.previewPanel);
  if (state.rightPanel) deepMerge(draft.rightPanel, state.rightPanel);
  if (state.diffs) deepMerge(draft.diffs, state.diffs);
  if (state.quickChat) {
    // Merge quick chat sessions, preserving isOpen from client
    if (state.quickChat.sessions) {
      const previousSessions = draft.quickChat.sessions;
      const previousActiveSessionId = draft.quickChat.activeSessionId;
      // Local renames live in localStorage and override the SSR-provided name
      // (which derives from the backend task title). Apply on every hydration
      // so a renamed chat keeps its local name across reloads and tab switches.
      draft.quickChat.sessions = applyStoredQuickChatNames(state.quickChat.sessions);
      restoreQuickChatSelection(draft, previousSessions, previousActiveSessionId);
    }
    if (state.quickChat.terminalTabs) hydrateQuickTerminalState(draft, state.quickChat);
  }
  if (state.connection) {
    const { status: _status, ...rest } = state.connection || {};
    if (Object.keys(rest).length > 0) {
      Object.assign(draft.connection, rest);
    }
  }
}

function hydrateQuickTerminalState(
  draft: Draft<AppState>,
  quickChat: NonNullable<HydrationState["quickChat"]>,
): void {
  const terminalTabs = quickChat.terminalTabs;
  if (!terminalTabs) return;
  const workspaceIds = new Set(terminalTabs.map((tab) => tab.workspaceId));
  for (const tab of draft.quickChat.terminalTabs) workspaceIds.add(tab.workspaceId);
  for (const session of quickChat.sessions ?? []) workspaceIds.add(session.workspaceId);
  for (const workspaceId of workspaceIds) {
    const tabs = terminalTabs.filter((tab) => tab.workspaceId === workspaceId);
    draft.quickChat = reconcileQuickTerminalTabs(draft.quickChat, workspaceId, tabs);
  }

  const activeTerminalTabId = quickChat.activeTerminalTabId;
  if (
    quickChat.activeKind === "terminal" &&
    activeTerminalTabId &&
    draft.quickChat.terminalTabs.some((tab) => tab.tabId === activeTerminalTabId)
  ) {
    draft.quickChat.activeKind = "terminal";
    draft.quickChat.activeTerminalTabId = activeTerminalTabId;
  }
  for (const [workspaceId, tabId] of Object.entries(quickChat.lastTerminalTabIdByWorkspace ?? {})) {
    if (draft.quickChat.terminalTabs.some((tab) => tab.tabId === tabId)) {
      draft.quickChat.lastTerminalTabIdByWorkspace[workspaceId] = tabId;
    }
  }
}

function restoreQuickChatSelection(
  draft: Draft<AppState>,
  previousSessions: AppState["quickChat"]["sessions"],
  previousActiveSessionId: string | null,
): void {
  draft.quickChat.terminalTabs ??= [];
  draft.quickChat.activeKind ??= "conversation";
  draft.quickChat.activeTerminalTabId ??= null;
  draft.quickChat.lastTerminalTabIdByWorkspace ??= {};
  if (draft.quickChat.activeKind === "terminal") {
    if (
      draft.quickChat.activeTerminalTabId &&
      draft.quickChat.terminalTabs.some((tab) => tab.tabId === draft.quickChat.activeTerminalTabId)
    ) {
      return;
    }
    draft.quickChat.activeKind = "conversation";
  }
  if (
    draft.quickChat.activeSessionId &&
    draft.quickChat.sessions.some(
      (session) => session.sessionId === draft.quickChat.activeSessionId,
    )
  ) {
    return;
  }
  const previousWorkspaceId = previousSessions.find(
    (session) => session.sessionId === previousActiveSessionId,
  )?.workspaceId;
  const fallbackSession =
    draft.quickChat.sessions.find((session) => session.workspaceId === previousWorkspaceId) ??
    draft.quickChat.sessions[0];
  if (fallbackSession) {
    draft.quickChat.activeSessionId = fallbackSession.sessionId;
    draft.quickChat.activeKind = "conversation";
    return;
  }
  const fallbackTerminal = draft.quickChat.terminalTabs.find(
    (tab) => tab.workspaceId === previousWorkspaceId,
  );
  if (fallbackTerminal) {
    draft.quickChat.activeKind = "terminal";
    draft.quickChat.activeTerminalTabId = fallbackTerminal.tabId;
    return;
  }
  draft.quickChat.activeSessionId = null;
  if (draft.quickChat.terminalTabs.length === 0) draft.quickChat.isOpen = false;
}

/**
 * Hydrates the app state with SSR data using smart merge strategies.
 *
 * Features:
 * - Deep merge for nested objects
 * - Avoids overwriting active sessions
 * - Preserves loading states to prevent flickering
 * - Partial hydration support
 */
export function hydrateState(
  draft: Draft<AppState>,
  state: HydrationState,
  options: HydrationOptions = {},
): void {
  const {
    activeSessionId = null,
    skipSessionRuntime = false,
    forceMergeSessionId = null,
  } = options;

  hydrateKanbanAndWorkspace(draft, state);
  hydrateSettings(draft, state);
  hydrateSession(draft, state, activeSessionId, forceMergeSessionId);

  if (!skipSessionRuntime) {
    hydrateSessionRuntime(draft, state, activeSessionId, forceMergeSessionId);
  }

  hydrateGitHub(draft, state);
  hydrateUI(draft, state);

  // Office slice — shallow merge SSR-provided data into the store.
  if (state.office) {
    Object.assign(draft.office, state.office);
  }

  // Feature flags - overwrite whole map. SSR is authoritative; the backend
  // is the only source of truth for what's enabled in this deployment.
  if (state.features) {
    Object.assign(draft.features, state.features);
  }

  // System slice - shallow-merge whichever fields the caller supplied.
  // `system` aggregates many independently-fetched fields (info, diskUsage,
  // updates, jobs, metrics, ...); callers only ever provide the
  // subset they fetched, so use the same leaf-level deepMerge as the other
  // multi-field slices above rather than overwriting the whole object.
  if (state.system) deepMerge(draft.system, state.system);
  if (state.agentRuntime !== undefined) draft.agentRuntime = state.agentRuntime;
}

/** Hydrate GitHub slices, preserving loading states. */
function hydrateGitHub(draft: Draft<AppState>, state: HydrationState): void {
  if (state.githubStatus) mergeWithLoading(draft.githubStatus, state.githubStatus);
  if (state.githubAppRegistrations) {
    mergeWithLoading(draft.githubAppRegistrations, state.githubAppRegistrations);
  }
  if (state.taskPRs) deepMerge(draft.taskPRs, state.taskPRs);
  if (state.azureDevOpsTaskPullRequests) {
    deepMerge(draft.azureDevOpsTaskPullRequests, state.azureDevOpsTaskPullRequests);
  }
  if (state.azureDevOpsTaskWorkItems) {
    deepMerge(draft.azureDevOpsTaskWorkItems, state.azureDevOpsTaskWorkItems);
  }
  if (state.prWatches) mergeWithLoading(draft.prWatches, state.prWatches);
  if (state.reviewWatches) mergeWithLoading(draft.reviewWatches, state.reviewWatches);
}
