import type { SliceSet } from "@/lib/state/slice-creator";
import type { TaskSession } from "@/lib/types/http";
import { sessionCatalogMetadataSchema, sameSessionCatalogOwner } from "@/lib/types/session-catalog";
import type {
  SessionCatalogRecord,
  SessionProviderCatalog,
  SessionCatalogRefreshEntry,
  SessionCatalogObservationTarget,
} from "@/lib/types/session-catalog";
import type { SessionRuntimeSlice } from "./types";

export type SessionCatalogRefreshActions = {
  applySessionCatalogRefresh: (
    taskId: string,
    sessionId: string,
    record: SessionCatalogRecord,
  ) => void;
  applySessionProviderCatalog: (
    taskId: string,
    sessionId: string,
    provider: SessionProviderCatalog,
  ) => void;
  observeSessionCatalogRefresh: (target: SessionCatalogObservationTarget) => void;
  hydrateSessionCatalogRefresh: (session: TaskSession) => void;
};

type CatalogTimers = Map<string, number | NodeJS.Timeout>;

function observeCatalog(
  set: SliceSet<SessionRuntimeSlice>,
  get: () => SessionRuntimeSlice,
  timers: CatalogTimers,
  target: SessionCatalogObservationTarget,
) {
  const current = get().sessionCatalogRefresh.bySessionId[target.sessionId];
  if (
    !current ||
    current.taskId !== target.taskId ||
    current.record?.revision !== target.noticeRevision
  )
    return;
  if (
    current.record.disposition === "cleared" ||
    (current.record.result && sameSessionCatalogOwner(current.record.owner, target.owner))
  )
    return;
  if (
    current.checking?.noticeRevision === target.noticeRevision &&
    sameSessionCatalogOwner(current.checking.owner, target.owner)
  )
    return;
  clearTimeout(timers.get(target.sessionId));
  timers.delete(target.sessionId);
  if (!target.owner || target.owner.source_generation === 0) {
    set((draft) => {
      draft.sessionCatalogRefresh.bySessionId[target.sessionId].checking = {
        ...target,
        status: "unavailable",
      };
    });
    return;
  }
  const deadlineAt = Date.now() + 60000;
  timers.set(
    target.sessionId,
    setTimeout(() => {
      timers.delete(target.sessionId);
      set((draft) => {
        const checking = draft.sessionCatalogRefresh.bySessionId[target.sessionId]?.checking;
        if (
          checking?.status !== "checking" ||
          checking.deadlineAt !== deadlineAt ||
          checking.noticeRevision !== target.noticeRevision ||
          !sameSessionCatalogOwner(checking.owner, target.owner)
        )
          return;
        checking.status = "unavailable";
      });
    }, 60000),
  );
  set((draft) => {
    draft.sessionCatalogRefresh.bySessionId[target.sessionId].checking = {
      ...target,
      deadlineAt,
      status: "checking",
    };
  });
}

export function buildSessionCatalogRefreshActions(
  set: SliceSet<SessionRuntimeSlice>,
  get: () => SessionRuntimeSlice,
): SessionCatalogRefreshActions {
  const timers: CatalogTimers = new Map();
  return {
    applySessionCatalogRefresh: (
      taskId: string,
      sessionId: string,
      record: SessionCatalogRecord,
    ) => {
      const previous = get().sessionCatalogRefresh.bySessionId[sessionId];
      if (
        previous &&
        (previous.taskId !== taskId || (previous.record?.revision ?? 0) >= record.revision)
      )
        return;
      if (previous?.checking && record.revision > previous.checking.noticeRevision) {
        clearTimeout(timers.get(sessionId));
        timers.delete(sessionId);
      }
      set((draft) => {
        const entry: SessionCatalogRefreshEntry = draft.sessionCatalogRefresh.bySessionId[
          sessionId
        ] ?? { taskId };
        if (entry.checking && record.revision > entry.checking.noticeRevision)
          delete entry.checking;
        entry.record = record;
        draft.sessionCatalogRefresh.bySessionId[sessionId] = entry;
      });
    },
    applySessionProviderCatalog: (
      taskId: string,
      sessionId: string,
      provider: SessionProviderCatalog,
    ) =>
      set((draft) => {
        const existing = draft.sessionCatalogRefresh.bySessionId[sessionId];
        if (
          existing &&
          (existing.taskId !== taskId || (existing.provider?.revision ?? 0) >= provider.revision)
        )
          return;
        const entry: SessionCatalogRefreshEntry = existing ?? { taskId };
        entry.provider = provider;
        draft.sessionCatalogRefresh.bySessionId[sessionId] = entry;
      }),
    observeSessionCatalogRefresh: (target: SessionCatalogObservationTarget) =>
      observeCatalog(set, get, timers, target),
    hydrateSessionCatalogRefresh: (session: TaskSession) => {
      if (!session.id || !session.task_id) return;
      const metadata = sessionCatalogMetadataSchema.safeParse(session.metadata);
      if (!metadata.success) return;
      const state = get();
      const record = metadata.data.profile_catalog_refresh;
      const provider = metadata.data.acp_model_state?.catalog_discovery;
      if (record) state.applySessionCatalogRefresh(session.task_id, session.id, record);
      if (provider) state.applySessionProviderCatalog(session.task_id, session.id, provider);
    },
  };
}
