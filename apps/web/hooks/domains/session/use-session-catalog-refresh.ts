"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { isSessionStoppable } from "./session-action-state";
import { useSessionRestart } from "./use-session-restart";
import { sessionCatalogSourceOwnerSchema } from "@/lib/types/session-catalog";
import type { TaskSession } from "@/lib/types/http";
import type { SessionCatalogRefreshEntry } from "@/lib/types/session-catalog";

export type SessionCatalogRefreshTarget = {
  taskId: string;
  sessionId: string;
  profileId: string;
  noticeRevision: number;
};

export function useSessionCatalogRefresh(sessionId: string | null) {
  const { owner, session, entry, profileId, sourceOwner } = useCatalogRefreshSnapshot(sessionId);
  const [target, setTarget] = useState<SessionCatalogRefreshTarget | null>(null);
  const anchorRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (session) owner.getState().hydrateSessionCatalogRefresh(session);
  }, [owner, session]);
  useEffect(() => {
    if (
      target &&
      (target.sessionId !== sessionId ||
        target.taskId !== session?.task_id ||
        target.profileId !== profileId ||
        target.noticeRevision !== entry?.record?.revision)
    )
      setTarget(null);
  }, [target, sessionId, session, profileId, entry?.record?.revision]);
  const { restart, isRestarting } = useSessionRestart(
    target?.sessionId ?? sessionId,
    target?.taskId ?? session?.task_id ?? null,
  );
  const { canRefresh, isChecking } = catalogRefreshAvailability(session, entry, isRestarting);

  const requestRefresh = useCallback(() => {
    if (!canRefresh || !session || !entry?.record) return;
    setTarget({
      taskId: session.task_id,
      sessionId: session.id,
      profileId: entry.record.profile_id,
      noticeRevision: entry.record.revision,
    });
  }, [canRefresh, session, entry?.record]);
  const dismiss = useCallback(() => setTarget(null), []);
  const confirm = useCallback(async () => {
    if (!target) return;
    const state = owner.getState();
    const current = state.taskSessions.items[target.sessionId];
    const record = state.sessionCatalogRefresh.bySessionId[target.sessionId]?.record;
    setTarget(null);
    if (
      current?.task_id !== target.taskId ||
      (current.execution_profile_id || current.agent_profile_id) !== target.profileId ||
      record?.revision !== target.noticeRevision ||
      record.disposition === "cleared"
    )
      return;
    await restart();
  }, [owner, restart, target]);

  return {
    session,
    entry,
    sourceOwner,
    anchorRef,
    target,
    requestRefresh,
    dismiss,
    confirm,
    canRefresh,
    isChecking,
    isRestarting,
  };
}

function useCatalogRefreshSnapshot(sessionId: string | null) {
  const owner = useAppStoreApi();
  const session = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId] : undefined,
  );
  const entry = useAppStore((state) =>
    sessionId ? state.sessionCatalogRefresh.bySessionId[sessionId] : undefined,
  );
  const liveExecutionId = useAppStore((state) =>
    sessionId ? state.sessionAgentctl?.itemsBySessionId?.[sessionId]?.agentExecutionId : undefined,
  );
  const currentExecutionId = liveExecutionId ?? session?.agent_execution_id;
  const profileId = session?.execution_profile_id || session?.agent_profile_id;
  const sourceOwner = useMemo(() => {
    const source = sessionCatalogSourceOwnerSchema.safeParse(session?.metadata?.acp_model_state);
    const metadataOwner =
      source.success && source.data.execution_id === currentExecutionId ? source.data : undefined;
    const provider = entry?.provider;
    const providerOwner =
      provider &&
      provider.profile_id === profileId &&
      provider.owner.execution_id === currentExecutionId
        ? provider.owner
        : undefined;
    // Postcommit provider evidence can advance before the full HTTP session snapshot.
    if (!metadataOwner) return providerOwner;
    return providerOwner && providerOwner.source_generation > metadataOwner.source_generation
      ? providerOwner
      : metadataOwner;
  }, [session?.metadata, currentExecutionId, profileId, entry?.provider]);
  return { owner, session, entry, profileId, sourceOwner };
}

function catalogRefreshAvailability(
  session: TaskSession | undefined,
  entry: SessionCatalogRefreshEntry | undefined,
  isRestarting: boolean,
) {
  const isChecking = entry?.checking?.status === "checking";
  const record = entry?.record;
  const canRefresh =
    !!session &&
    !!record &&
    record.disposition !== "cleared" &&
    (session.execution_profile_id || session.agent_profile_id) === record.profile_id &&
    isSessionStoppable(session.state) &&
    !isRestarting &&
    !isChecking;
  return { canRefresh, isChecking };
}
