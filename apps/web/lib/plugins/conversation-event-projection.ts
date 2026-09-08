import type { RawSessionEvent } from "@/lib/ws/client";
import type { PluginConversationMessage, PluginConversationTurn } from "./types";

export function eventField(event: RawSessionEvent, key: string): unknown {
  if (!event.payload || typeof event.payload !== "object" || !(key in event.payload)) {
    return undefined;
  }
  return Reflect.get(event.payload, key);
}

export function eventString(event: RawSessionEvent, key: string): string | null {
  const value = eventField(event, key);
  return typeof value === "string" && value !== "" ? value : null;
}

export function compareConversationMessages(
  left: Pick<PluginConversationMessage, "createdAt" | "id">,
  right: Pick<PluginConversationMessage, "createdAt" | "id">,
  sort: "asc" | "desc",
): number {
  const created = left.createdAt.localeCompare(right.createdAt);
  const ordered = created === 0 ? left.id.localeCompare(right.id) : created;
  return sort === "asc" ? ordered : -ordered;
}

export function eventMatchesTask(event: RawSessionEvent, taskId: string | null): boolean {
  return taskId === null || event.task_id === taskId;
}

export function messageFromEvent(event: RawSessionEvent): PluginConversationMessage | null {
  const id = eventString(event, "message_id");
  const authorType = eventString(event, "author_type");
  const contentValue = eventField(event, "content");
  const content = typeof contentValue === "string" ? contentValue : null;
  const createdAt = eventString(event, "created_at");
  const explicitUpdatedAt = eventString(event, "updated_at");
  const updatedAt = explicitUpdatedAt ?? (event.event_type === "message.added" ? createdAt : null);
  if (
    !id ||
    (authorType !== "user" && authorType !== "agent") ||
    content === null ||
    !createdAt ||
    !updatedAt
  ) {
    return null;
  }
  const promptIndex = eventField(event, "prompt_index");
  return {
    id,
    taskId: event.task_id,
    sessionId: event.session_id,
    turnId: eventString(event, "turn_id") ?? undefined,
    authorType,
    type: eventString(event, "message_type") ?? "message",
    content,
    createdAt,
    updatedAt,
    promptIndex: typeof promptIndex === "number" ? promptIndex : undefined,
    senderTaskId: eventString(event, "sender_task_id") ?? undefined,
  };
}

export function turnFromEvent(event: RawSessionEvent): PluginConversationTurn | null {
  const id = eventString(event, "turn_id") ?? eventString(event, "id");
  const startedAt = eventString(event, "started_at");
  const explicitUpdatedAt = eventString(event, "updated_at");
  const updatedAt =
    explicitUpdatedAt ?? (event.event_type === "session.turn.started" ? startedAt : null);
  if (!id || !startedAt || !updatedAt) return null;
  return {
    id,
    taskId: event.task_id,
    sessionId: event.session_id,
    startedAt,
    completedAt: eventString(event, "completed_at") ?? undefined,
    updatedAt,
  };
}
