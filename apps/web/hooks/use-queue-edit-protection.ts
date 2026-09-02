import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "@/lib/toast/sonner";
import {
  beginQueuedMessageEdit,
  endQueuedMessageEdit,
  renewQueuedMessageEdit,
  type QueueEditLease,
} from "@/lib/api/domains/queue-api";
import type { QueuedMessage } from "@/lib/state/slices/session/types";

type QueueEditProtectionArgs = {
  sessionId: string | null;
  entries: QueuedMessage[];
};

type ActiveEdit = {
  sessionId: string;
  entryId: string;
  lease: QueueEditLease;
};

const EDIT_RENEW_INTERVAL_MS = 20_000;

/** Acquires a target-bound server lease before activating a queue editor. */
export function useQueueEditProtection({ sessionId, entries }: QueueEditProtectionArgs) {
  const { t } = useTranslation();
  const [editingEntryId, setEditingEntryId] = useState<string | null>(null);
  const [editLease, setEditLease] = useState<QueueEditLease | null>(null);
  const activeEditRef = useRef<ActiveEdit | null>(null);
  const sessionIdRef = useRef(sessionId);
  sessionIdRef.current = sessionId;

  const beginEdit = useCallback(
    async (entryId: string): Promise<boolean> => {
      if (!sessionId || editingEntryId || activeEditRef.current) return false;
      try {
        const lease = await beginQueuedMessageEdit(sessionId, entryId);
        if (sessionIdRef.current !== sessionId) {
          await endQueuedMessageEdit(lease).catch(() => undefined);
          return false;
        }
        activeEditRef.current = { sessionId, entryId, lease };
        setEditLease(lease);
        setEditingEntryId(entryId);
        return true;
      } catch (err) {
        console.error("Failed to acquire queued message edit lease:", err);
        toast.error(t("chat:failedToSetQueueAutoRun"));
        return false;
      }
    },
    [editingEntryId, sessionId, t],
  );

  const completeEdit = useCallback(async (entryId: string): Promise<void> => {
    const activeEdit = activeEditRef.current;
    if (!activeEdit || activeEdit.entryId !== entryId) return;
    activeEditRef.current = null;
    setEditingEntryId(null);
    setEditLease(null);
    await endQueuedMessageEdit(activeEdit.lease).catch((err) => {
      console.error("Failed to release queued message edit lease:", err);
    });
  }, []);

  useEffect(() => {
    const activeEdit = activeEditRef.current;
    if (!activeEdit || activeEdit.sessionId !== sessionId) return;
    if (!entries.some((entry) => entry.id === activeEdit.entryId)) {
      void completeEdit(activeEdit.entryId);
    }
  }, [completeEdit, entries, sessionId]);

  useEffect(() => {
    const activeEdit = activeEditRef.current;
    if (!activeEdit) return;
    const renew = async () => {
      try {
        const lease = await renewQueuedMessageEdit(activeEdit.lease);
        if (activeEditRef.current?.lease.lease_id !== lease.lease_id) return;
        activeEditRef.current.lease = lease;
        setEditLease(lease);
      } catch (err) {
        console.error("Queued message edit lease renewal failed:", err);
        await completeEdit(activeEdit.entryId);
        toast.error(t("chat:failedToSetQueueAutoRun"));
      }
    };
    const timer = window.setInterval(() => void renew(), EDIT_RENEW_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [completeEdit, editingEntryId, t]);

  useEffect(() => {
    const activeEdit = activeEditRef.current;
    if (!activeEdit || activeEdit.sessionId === sessionId) return;
    activeEditRef.current = null;
    setEditingEntryId(null);
    setEditLease(null);
    void endQueuedMessageEdit(activeEdit.lease).catch((err) => {
      console.error("Failed to release queued message edit lease:", err);
    });
  }, [sessionId]);

  useEffect(
    () => () => {
      const activeEdit = activeEditRef.current;
      activeEditRef.current = null;
      if (activeEdit) {
        void endQueuedMessageEdit(activeEdit.lease).catch((err) => {
          console.error("Failed to release queued message edit lease:", err);
        });
      }
    },
    [],
  );

  return { editingEntryId, editLease, beginEdit, completeEdit };
}
type QueuedGhostEditStartArgs = {
  canEdit: boolean;
  editing: boolean;
  saving: boolean;
  onEditStart?: () => void | Promise<boolean | void>;
  onStart: () => void;
};

export function useQueuedGhostStartEdit({
  canEdit,
  editing,
  saving,
  onEditStart,
  onStart,
}: QueuedGhostEditStartArgs) {
  const editStartingRef = useRef(false);
  return useCallback(async () => {
    if (!canEdit || editing || saving || editStartingRef.current) return;
    editStartingRef.current = true;
    try {
      if ((await onEditStart?.()) === false) return;
      onStart();
    } finally {
      editStartingRef.current = false;
    }
  }, [canEdit, editing, onEditStart, onStart, saving]);
}
