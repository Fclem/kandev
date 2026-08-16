import type { Draft } from "immer";
import { getQuickChatSetupSessionId } from "./quick-chat-session";
import {
  closeQuickChatSession,
  pruneStaleSettledLedger,
  reconcileQuickChatSessions,
  reconcileQuickTerminalTabs,
  removeQuickChatSession,
  removeQuickChatSessionsForTask,
  upsertQuickChatSession,
} from "./quick-chat-sync";
import type { QuickChatSession, UISlice } from "./types";

type ImmerSet = (recipe: (draft: Draft<UISlice>) => void) => void;

function findWorkspaceConfigSession(
  sessions: UISlice["quickChat"]["sessions"],
  workspaceId: string,
) {
  return sessions.find(
    (session) => session.workspaceId === workspaceId && session.kind === "config",
  );
}

function openQuickChat(set: ImmerSet) {
  return (
    sessionId: string,
    workspaceId: string,
    agentProfileId?: string,
    kind: "chat" | "config" = "chat",
    taskId?: string,
  ) =>
    set((draft) => {
      draft.quickChat.unseenIdleByWorkspace = {};
      if (!sessionId) {
        const existing =
          kind === "config"
            ? findWorkspaceConfigSession(draft.quickChat.sessions, workspaceId)
            : undefined;
        const setupSessionId = getQuickChatSetupSessionId(workspaceId, kind);
        if (
          !existing &&
          !draft.quickChat.sessions.some((session) => session.sessionId === setupSessionId)
        ) {
          draft.quickChat.sessions.push({ sessionId: setupSessionId, workspaceId, kind });
        }
        draft.quickChat.isOpen = true;
        draft.quickChat.activeSessionId = existing?.sessionId ?? setupSessionId;
        draft.quickChat.activeKind = "conversation";
        return;
      }
      const existing = draft.quickChat.sessions.find((session) => session.sessionId === sessionId);
      if (existing) {
        if (existing.workspaceId !== workspaceId) return;
        if (agentProfileId) existing.agentProfileId = agentProfileId;
        if (taskId) existing.taskId = taskId;
      } else {
        draft.quickChat.sessions.push({ sessionId, workspaceId, agentProfileId, kind, taskId });
      }
      draft.quickChat.sessionOwnership[sessionId] = { workspaceId, taskId };
      draft.quickChat.syncRevisionByWorkspace[workspaceId] =
        (draft.quickChat.syncRevisionByWorkspace[workspaceId] ?? 0) + 1;
      draft.quickChat.isOpen = true;
      draft.quickChat.activeSessionId = sessionId;
      draft.quickChat.activeKind = "conversation";
    });
}

function addQuickChatSession(set: ImmerSet) {
  return (
    sessionId: string,
    workspaceId: string,
    agentProfileId?: string,
    kind: "chat" | "config" = "chat",
    taskId?: string,
  ) =>
    set((draft) => {
      const existing = draft.quickChat.sessions.find((session) => session.sessionId === sessionId);
      const activeWorkspaceId = draft.quickChat.sessions.find(
        (session) => session.sessionId === draft.quickChat.activeSessionId,
      )?.workspaceId;
      if (existing) {
        if (existing.workspaceId !== workspaceId) return;
        if (agentProfileId) existing.agentProfileId = agentProfileId;
        if (taskId) existing.taskId = taskId;
      } else {
        draft.quickChat.sessions.push({ sessionId, workspaceId, agentProfileId, kind, taskId });
      }
      draft.quickChat.sessionOwnership[sessionId] = { workspaceId, taskId };
      draft.quickChat.syncRevisionByWorkspace[workspaceId] =
        (draft.quickChat.syncRevisionByWorkspace[workspaceId] ?? 0) + 1;
      if (!draft.quickChat.isOpen || !activeWorkspaceId || activeWorkspaceId === workspaceId) {
        draft.quickChat.activeSessionId = sessionId;
        draft.quickChat.activeKind = "conversation";
      }
    });
}

export function buildQuickChatActions(set: ImmerSet) {
  return {
    addQuickChatSession: addQuickChatSession(set),
    openQuickChat: openQuickChat(set),
    closeQuickChat: () =>
      set((draft) => {
        draft.quickChat.isOpen = false;
      }),
    closeQuickChatSession: (sessionId: string) =>
      set((draft) => {
        draft.quickChat = closeQuickChatSession(draft.quickChat, sessionId);
      }),
    setActiveQuickChatSession: (sessionId: string, workspaceId: string) =>
      set((draft) => {
        const session = draft.quickChat.sessions.find((item) => item.sessionId === sessionId);
        if (!session || session.workspaceId !== workspaceId) return;
        const markers = draft.quickChat.unseenIdleByWorkspace[workspaceId];
        if (markers) delete markers[sessionId];
        draft.quickChat.activeSessionId = sessionId;
        draft.quickChat.activeKind = "conversation";
      }),
    renameQuickChatSession: (sessionId: string, name: string) =>
      set((draft) => {
        const session = draft.quickChat.sessions.find((item) => item.sessionId === sessionId);
        if (!session) return;
        session.name = name;
        draft.quickChat.syncRevisionByWorkspace[session.workspaceId] =
          (draft.quickChat.syncRevisionByWorkspace[session.workspaceId] ?? 0) + 1;
      }),
    syncQuickChatSessions: (workspaceId: string, sessions: QuickChatSession[]) =>
      set((draft) => {
        draft.quickChat = reconcileQuickChatSessions(draft.quickChat, workspaceId, sessions);
      }),
    syncQuickTerminalTabs: (workspaceId: string, tabs: UISlice["quickChat"]["terminalTabs"]) =>
      set((draft) => {
        draft.quickChat = reconcileQuickTerminalTabs(draft.quickChat, workspaceId, tabs);
      }),
    upsertQuickChatSessionFromEvent: (session: QuickChatSession) =>
      set((draft) => {
        draft.quickChat = upsertQuickChatSession(draft.quickChat, session);
      }),
    removeQuickChatSessionsForTask: (taskId: string) =>
      set((draft) => {
        draft.quickChat = removeQuickChatSessionsForTask(draft.quickChat, taskId);
      }),
    removeQuickChatSession: (sessionId: string) =>
      set((draft) => {
        draft.quickChat = removeQuickChatSession(draft.quickChat, sessionId);
      }),
    markQuickChatUnseenIdle: (sessionId: string, workspaceId: string) =>
      set((draft) => {
        draft.quickChat.unseenIdleByWorkspace[workspaceId] = {
          ...draft.quickChat.unseenIdleByWorkspace[workspaceId],
          [sessionId]: true,
        };
      }),
    clearQuickChatUnseenIdle: (sessionId?: string, workspaceId?: string) =>
      set((draft) => {
        if (!sessionId || !workspaceId) {
          draft.quickChat.unseenIdleByWorkspace = {};
          return;
        }
        const markers = draft.quickChat.unseenIdleByWorkspace[workspaceId];
        if (!markers) return;
        delete markers[sessionId];
        if (Object.keys(markers).length === 0)
          delete draft.quickChat.unseenIdleByWorkspace[workspaceId];
      }),
    recordQuickChatSettled: (sessionId: string, updatedAt: string) => {
      let recorded = false;
      set((draft) => {
        if (!updatedAt || draft.quickChat.lastSettledAtBySession[sessionId] >= updatedAt) return;
        draft.quickChat.lastSettledAtBySession = pruneStaleSettledLedger({
          ...draft.quickChat.lastSettledAtBySession,
          [sessionId]: updatedAt,
        });
        recorded = true;
      });
      return recorded;
    },
    setQuickChatInitialPrompt: (sessionId: string, prompt?: string) =>
      set((draft) => {
        const session = draft.quickChat.sessions.find((item) => item.sessionId === sessionId);
        if (session) session.initialPrompt = prompt;
      }),
  };
}
