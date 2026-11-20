import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import {
  sessionCatalogDiscoveryPayloadSchema,
  sessionCatalogRefreshPayloadSchema,
} from "@/lib/types/session-catalog";
import type { WsHandlers } from "./types";

function matchesSessionTask(store: StoreApi<AppState>, taskId: string, sessionId: string): boolean {
  const session = store.getState().taskSessions.items[sessionId];
  return !session || session.task_id === taskId;
}

export function registerSessionCatalogRefreshHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.catalog_refresh_updated": (message) => {
      const parsed = sessionCatalogRefreshPayloadSchema.safeParse(message.payload);
      if (!parsed.success) return;
      const { task_id, session_id, record } = parsed.data;
      if (!matchesSessionTask(store, task_id, session_id)) return;
      store.getState().applySessionCatalogRefresh(task_id, session_id, record);
    },
    "session.catalog_discovery_updated": (message) => {
      const parsed = sessionCatalogDiscoveryPayloadSchema.safeParse(message.payload);
      if (!parsed.success) return;
      const { task_id, session_id, provider } = parsed.data;
      if (!matchesSessionTask(store, task_id, session_id)) return;
      store.getState().applySessionProviderCatalog(task_id, session_id, provider);
    },
  };
}
