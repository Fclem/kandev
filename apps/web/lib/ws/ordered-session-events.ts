import type { BackendMessageType } from "@/lib/types/backend";

export interface RawSessionEvent {
  type: "session.event";
  protocol_version: number;
  event_type: string;
  session_id: string;
  task_id: string | null;
  sequence: number;
  event_id: string;
  payload: unknown;
}

export type CoreSessionStream = {
  wireId: string;
  lastSeenSequence: number;
  resumeToken?: string;
  poisonRecovery?: Promise<void>;
};

const ORDERED_EVENT_ACTIONS: Readonly<Record<string, BackendMessageType>> = {
  "message.added": "session.message.added",
  "message.updated": "session.message.updated",
  "message.deleted": "session.message.deleted",
  "session.turn.started": "session.turn.started",
  "session.turn.completed": "session.turn.completed",
};

const IGNORABLE_ORDERED_EVENT_TYPES: Readonly<Record<string, true>> = {
  "session.turn.removed": true,
  "session.workspace_sources.updated": true,
};

export type OrderedCoreDisposition = "project" | "ignore" | "terminal" | "poison";

export function orderedCoreDisposition(event: RawSessionEvent): OrderedCoreDisposition {
  if (
    event.protocol_version !== 1 ||
    !event.payload ||
    typeof event.payload !== "object" ||
    Array.isArray(event.payload) ||
    !("type" in event.payload) ||
    Reflect.get(event.payload, "type") !== event.event_type
  ) {
    return "poison";
  }
  if (orderedCoreAction(event.event_type)) return "project";
  if (event.event_type === "session.removed") return "terminal";
  if (IGNORABLE_ORDERED_EVENT_TYPES[event.event_type]) return "ignore";
  return "poison";
}

function isPositiveSequence(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

export function isRawSessionEvent(value: unknown): value is RawSessionEvent {
  if (!value || typeof value !== "object") return false;
  const event = value as Partial<RawSessionEvent>;
  const hasNullableTaskId = event.task_id === null || typeof event.task_id === "string";
  return (
    event.type === "session.event" &&
    event.protocol_version === 1 &&
    typeof event.event_type === "string" &&
    event.event_type.length > 0 &&
    typeof event.session_id === "string" &&
    event.session_id.length > 0 &&
    hasNullableTaskId &&
    isPositiveSequence(event.sequence) &&
    typeof event.event_id === "string" &&
    event.event_id.length > 0 &&
    "payload" in event
  );
}

export function orderedCoreAction(eventType: string): BackendMessageType | undefined {
  return ORDERED_EVENT_ACTIONS[eventType];
}

export function projectCoreSessionPayload(event: RawSessionEvent): Record<string, unknown> {
  const payload: Record<string, unknown> =
    event.payload !== null && typeof event.payload === "object" && !Array.isArray(event.payload)
      ? { ...event.payload }
      : {};
  if (
    (event.event_type === "message.added" || event.event_type === "message.updated") &&
    typeof payload.message_type === "string"
  ) {
    payload.type = payload.message_type;
  }
  return payload;
}
