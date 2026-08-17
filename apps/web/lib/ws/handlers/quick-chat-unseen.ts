import type { StoreApi } from "zustand";
import type { TaskSessionState } from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";

const ACTIVE_STATES: ReadonlySet<TaskSessionState> = new Set(["STARTING", "RUNNING"]);
const SETTLED_STATES: ReadonlySet<TaskSessionState> = new Set([
  "IDLE",
  "WAITING_FOR_INPUT",
  "COMPLETED",
  "FAILED",
  "CANCELLED",
]);

export function maybeMarkQuickChatUnseenIdle(
  store: StoreApi<AppState>,
  sessionId: string,
  transition: {
    previousState: TaskSessionState | undefined;
    fallbackPreviousState: TaskSessionState | undefined;
    newState: TaskSessionState | undefined;
    updatedAt: string | undefined;
  },
): void {
  const { previousState, fallbackPreviousState, newState, updatedAt } = transition;
  const priorState = previousState ?? fallbackPreviousState;
  if (
    !priorState ||
    !newState ||
    !ACTIVE_STATES.has(priorState) ||
    !SETTLED_STATES.has(newState) ||
    !updatedAt
  )
    return;
  if (!store.getState().recordQuickChatSettled(sessionId, updatedAt)) return;
  const quickChat = store.getState().quickChat;
  const session = quickChat.sessions.find((item) => item.sessionId === sessionId);
  if (session && (!quickChat.isOpen || quickChat.activeSessionId !== sessionId))
    store.getState().markQuickChatUnseenIdle(sessionId, session.workspaceId);
}
