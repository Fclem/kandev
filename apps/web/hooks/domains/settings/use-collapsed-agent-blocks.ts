"use client";

import { useCallback, useMemo, useSyncExternalStore } from "react";

const STORAGE_KEY = "kandev:agents:collapsedBlocks:v1";
const SYNC_EVENT = "kandev:agents:collapsed-blocks-changed";

type CollapsedBlocks = Record<string, boolean>;

/**
 * Reads the stored collapse record as a raw string. The string (not a parsed
 * object) is the `useSyncExternalStore` snapshot: a fresh object every call
 * would re-render the hook forever, while the primitive is stable across reads
 * of unchanged storage. Absent entry, invalid JSON, and read failures all
 * degrade to the default (all expanded), matching the shared
 * `useLocalStorageBoolean` contract.
 */
function readStoredBlocksRaw(): string {
  if (typeof window === "undefined") return "";
  try {
    return window.localStorage.getItem(STORAGE_KEY) ?? "";
  } catch {
    return "";
  }
}

function parseBlocks(raw: string): CollapsedBlocks {
  if (raw === "") return {};
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) return {};
    return parsed as CollapsedBlocks;
  } catch {
    return {};
  }
}

function isCollapsed(raw: string, agentName: string): boolean {
  return parseBlocks(raw)[agentName] === true;
}

/**
 * Per-agent collapsed/expanded preference for the Settings > Agents installed
 * agent cards, persisted as one JSON record in `localStorage` keyed by agent
 * name (absent entry = expanded). Uses the same
 * `useSyncExternalStore` + `storage` event + custom-event broadcast shape as
 * the shared `useLocalStorageBoolean` primitive, so every tab re-renders when
 * the preference changes anywhere. Read failures degrade to expanded; a failed
 * write throws instead of reporting a successful save.
 */
export function useCollapsedAgentBlocks() {
  const subscribe = useMemo(
    () => (notify: () => void) => {
      const onCustomEvent = () => notify();
      window.addEventListener(SYNC_EVENT, onCustomEvent);
      window.addEventListener("storage", onCustomEvent);
      return () => {
        window.removeEventListener(SYNC_EVENT, onCustomEvent);
        window.removeEventListener("storage", onCustomEvent);
      };
    },
    [],
  );

  const getSnapshot = useCallback(() => readStoredBlocksRaw(), []);
  const getServerSnapshot = useCallback(() => "", []);

  const raw = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  const collapsed = useCallback((agentName: string) => isCollapsed(raw, agentName), [raw]);

  const setCollapsed = useCallback((agentName: string, next: boolean) => {
    const updated = { ...parseBlocks(readStoredBlocksRaw()), [agentName]: next };
    try {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(updated));
    } catch (error) {
      throw new Error(`Failed to persist ${STORAGE_KEY}`, { cause: error });
    }
    window.dispatchEvent(new Event(SYNC_EVENT));
  }, []);

  return { collapsed, setCollapsed };
}
