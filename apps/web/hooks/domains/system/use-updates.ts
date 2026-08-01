"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { checkUpdates, fetchUpdates, saveUpdatesChannel } from "@/lib/api/domains/system-api";
import type { UpdatesChannel } from "@/lib/types/system";

export function useUpdates() {
  const updates = useAppStore((s) => s.system.updates);
  const setSystemUpdates = useAppStore((s) => s.setSystemUpdates);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isChecking, setIsChecking] = useState(false);
  const latestRequest = useRef(0);
  const latestReload = useRef(0);
  const latestCheck = useRef(0);

  const reload = useCallback(async () => {
    const request = ++latestRequest.current;
    const reloadRequest = ++latestReload.current;
    setIsLoading(true);
    setError(null);
    try {
      const res = await fetchUpdates({ cache: "no-store" });
      if (request === latestRequest.current) setSystemUpdates(res);
    } catch (e) {
      if (request === latestRequest.current) {
        setError(e instanceof Error ? e.message : String(e));
      }
    } finally {
      if (reloadRequest === latestReload.current) setIsLoading(false);
    }
  }, [setSystemUpdates]);

  /**
   * Triggers a server-side re-poll of the selected update channel. The
   * backend rate-limits this per-process to one call per 30s and replies
   * with the fresh row (or 429 — surfaced via the returned promise).
   */
  const check = useCallback(async () => {
    const request = ++latestRequest.current;
    const checkRequest = ++latestCheck.current;
    setIsChecking(true);
    setError(null);
    try {
      const res = await checkUpdates();
      if (request === latestRequest.current) setSystemUpdates(res);
      return res;
    } catch (e) {
      if (request !== latestRequest.current) return undefined;
      setError(e instanceof Error ? e.message : String(e));
      throw e;
    } finally {
      if (checkRequest === latestCheck.current) setIsChecking(false);
    }
  }, [setSystemUpdates]);

  const saveChannel = useCallback(
    async (channel: UpdatesChannel) => {
      const request = ++latestRequest.current;
      setError(null);
      try {
        const res = await saveUpdatesChannel(channel);
        if (request === latestRequest.current) setSystemUpdates(res);
        return res;
      } catch (e) {
        if (request === latestRequest.current) {
          setError(e instanceof Error ? e.message : String(e));
        }
        throw e;
      }
    },
    [setSystemUpdates],
  );

  useEffect(() => {
    if (updates) return;
    void reload();
  }, [updates, reload]);

  return { updates, isLoading, isChecking, error, reload, check, saveChannel };
}
