import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { listTaskSessionMessages } from "@/lib/api/domains/session-api";
import type { Message } from "@/lib/types/http";

export type LoadMessageWindowResult =
  | { kind: "merged"; merged: true; current: true; targetFound: true }
  | { kind: "deleted-target"; merged: false; current: true; targetFound: false }
  | { kind: "stale"; merged: false; current: false; targetFound: false };

type SessionStore = StoreApi<AppState>;
function compareMessageIDs(left: Message, right: Message) {
  if (left.id < right.id) return -1;
  if (left.id > right.id) return 1;
  return 0;
}
function mergeWindowRows(existing: Message[], window: Message[]): Message[] {
  const byID = new Map(existing.map((message) => [message.id, message]));
  for (const message of window) {
    const current = byID.get(message.id);
    const currentUpdated = current?.updated_at ? Date.parse(current.updated_at) : Number.NaN;
    const incomingUpdated = message.updated_at ? Date.parse(message.updated_at) : Number.NaN;
    const currentIsNewer =
      current &&
      Number.isFinite(currentUpdated) &&
      (!Number.isFinite(incomingUpdated) || currentUpdated > incomingUpdated);
    if (!currentIsNewer) byID.set(message.id, message);
  }
  return [...byID.values()].sort((left, right) => {
    const timeDelta = Date.parse(left.created_at) - Date.parse(right.created_at);
    return timeDelta !== 0 ? timeDelta : compareMessageIDs(left, right);
  });
}

/** Fetches and merges the window containing a scroll target without changing transcript pagination. */
export async function loadMessageWindowAround(
  sessionId: string,
  targetMessageId: string,
  guard: () => boolean,
  store: SessionStore,
): Promise<LoadMessageWindowResult> {
  const response = await listTaskSessionMessages(sessionId, {
    around: targetMessageId,
    limit: 100,
    sort: "desc",
  });
  if (!guard()) return { kind: "stale", merged: false, current: false, targetFound: false };
  const window = response.messages ?? [];
  if (!window.some((message) => message.id === targetMessageId)) {
    return { kind: "deleted-target", merged: false, current: true, targetFound: false };
  }
  const existing = store.getState().messages.bySession[sessionId] ?? [];
  store.getState().mergeMessages(sessionId, mergeWindowRows(existing, window));
  return { kind: "merged", merged: true, current: true, targetFound: true };
}
