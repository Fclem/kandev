"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { checkUpdates, fetchUpdates, saveUpdatesChannel } from "@/lib/api/domains/system-api";
import type { UpdatesChannel, UpdatesResponse } from "@/lib/types/system";

type UpdatesRequestCoordinator = {
  readRevision: number;
  saveRevision: number;
  activeSaves: number;
  saveTail: Promise<void>;
  reloadFlight: {
    request: number;
    promise: Promise<UpdatesResponse>;
    published: boolean;
  } | null;
};

// Hooks own their loading/error UI, but every instance writes the same Zustand
// store. Coordinate response authority per store so one mounted reader cannot
// overwrite a newer channel save performed by another instance.
const coordinators = new WeakMap<object, UpdatesRequestCoordinator>();

function coordinatorFor(store: object): UpdatesRequestCoordinator {
  let coordinator = coordinators.get(store);
  if (!coordinator) {
    coordinator = {
      readRevision: 0,
      saveRevision: 0,
      activeSaves: 0,
      saveTail: Promise.resolve(),
      reloadFlight: null,
    };
    coordinators.set(store, coordinator);
  }
  return coordinator;
}

export function useUpdates() {
  const store = useAppStoreApi();
  const coordinator = coordinatorFor(store);
  const updates = useAppStore((s) => s.system.updates);
  const setSystemUpdates = useAppStore((s) => s.setSystemUpdates);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isChecking, setIsChecking] = useState(false);
  const latestReload = useRef(0);
  const latestCheck = useRef(0);

  const reload = useCallback(async () => {
    const reloadRequest = ++latestReload.current;
    setIsLoading(true);
    setError(null);
    let flight = coordinator.reloadFlight;
    if (!flight) {
      flight = {
        request: ++coordinator.readRevision,
        promise: fetchUpdates({ cache: "no-store" }),
        published: false,
      };
      coordinator.reloadFlight = flight;
    }
    try {
      const res = await flight.promise;
      if (
        !flight.published &&
        flight.request === coordinator.readRevision &&
        coordinator.activeSaves === 0
      ) {
        flight.published = true;
        setSystemUpdates(res);
      }
    } catch (e) {
      if (flight.request === coordinator.readRevision && coordinator.activeSaves === 0) {
        setError(e instanceof Error ? e.message : String(e));
      }
    } finally {
      if (coordinator.reloadFlight === flight) coordinator.reloadFlight = null;
      if (reloadRequest === latestReload.current) setIsLoading(false);
    }
  }, [coordinator, setSystemUpdates]);

  /**
   * Triggers a server-side re-poll of the selected update channel. The
   * backend rate-limits this per-process to one call per 30s and replies
   * with the fresh row (or 429 — surfaced via the returned promise).
   */
  const check = useCallback(async () => {
    const request = ++coordinator.readRevision;
    const checkRequest = ++latestCheck.current;
    setIsChecking(true);
    setError(null);
    try {
      const res = await checkUpdates();
      if (request === coordinator.readRevision && coordinator.activeSaves === 0) {
        setSystemUpdates(res);
      }
      return res;
    } catch (e) {
      if (request !== coordinator.readRevision || coordinator.activeSaves > 0) return undefined;
      setError(e instanceof Error ? e.message : String(e));
      throw e;
    } finally {
      if (checkRequest === latestCheck.current) setIsChecking(false);
    }
  }, [coordinator, setSystemUpdates]);

  const saveChannel = useCallback(
    (channel: UpdatesChannel): Promise<UpdatesResponse> => {
      const request = ++coordinator.saveRevision;
      coordinator.activeSaves += 1;
      coordinator.readRevision += 1;
      setError(null);
      const previousSave = coordinator.saveTail;
      const operation = (async () => {
        await previousSave;
        try {
          const res = await saveUpdatesChannel(channel);
          if (request === coordinator.saveRevision) setSystemUpdates(res);
          return res;
        } catch (e) {
          if (request === coordinator.saveRevision) {
            setError(e instanceof Error ? e.message : String(e));
          }
          throw e;
        } finally {
          coordinator.activeSaves -= 1;
          coordinator.readRevision += 1;
        }
      })();
      coordinator.saveTail = operation.then(
        () => undefined,
        () => undefined,
      );
      return operation;
    },
    [coordinator, setSystemUpdates],
  );

  useEffect(() => {
    if (updates) return;
    void reload();
  }, [updates, reload]);

  return { updates, isLoading, isChecking, error, reload, check, saveChannel };
}
