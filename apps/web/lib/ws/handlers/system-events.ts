import type { StoreApi } from "zustand";
import type { SystemErrorNotification } from "@/lib/state/slices/ui/types";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

/** Shown when the frame carries no usable message text. */
export const SYSTEM_ERROR_FALLBACK_MESSAGE = "The backend reported an error.";

function trimmedString(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

/**
 * Normalizes an untrusted `system.error` payload into a displayable
 * notification. The declared wire shape is `{message, code?}`, but the frame is
 * an error channel — treat a missing, non-object, or empty payload as an error
 * we still have to tell the user about rather than dropping it.
 */
export function toSystemErrorNotification(payload: unknown): SystemErrorNotification {
  const record = (payload ?? {}) as Record<string, unknown>;
  const message = trimmedString(record.message);
  const code = trimmedString(record.code);
  return {
    message: message || SYSTEM_ERROR_FALLBACK_MESSAGE,
    ...(code ? { code } : {}),
  };
}

export function registerSystemEventsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "system.error": (message) => {
      // Hand off to the store; SystemErrorToastBridge renders it as an error toast.
      store.getState().setSystemErrorNotification(toSystemErrorNotification(message?.payload));
    },
    "system.job.update": (message) => {
      // The WS payload is the full SystemJob row published by the backend
      // jobs tracker (see internal/system/jobs). Upsert by id so the
      // jobs map mirrors the latest queued/running/succeeded/failed state.
      store.getState().upsertSystemJob(message.payload);
    },
    "system.metrics.updated": (message) => {
      store.getState().setSystemMetricsSnapshot(message.payload);
    },
  };
}
