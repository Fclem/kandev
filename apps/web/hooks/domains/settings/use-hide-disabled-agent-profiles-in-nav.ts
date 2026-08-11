"use client";

import { useCallback, useMemo, useSyncExternalStore } from "react";

const STORAGE_KEY = "kandev:agents:hideDisabledInNav:v1";
const SYNC_EVENT = "kandev:agents:hide-disabled-in-nav-changed";

/** Reads the persisted "hide disabled agent profiles from nav" flag, defaulting to `false`. */
function readHideDisabled(): boolean {
  if (typeof window === "undefined") return false;
  try {
    return window.localStorage.getItem(STORAGE_KEY) === "true";
  } catch {
    // Quota / private mode — fall through; the setting just defaults to off.
    return false;
  }
}

/**
 * The agents settings page's "Hide disabled agent profiles from left panel
 * navigation" setting: a single, install-wide, `localStorage`-backed boolean
 * (not per-profile), defaulting to `false`. Shares the
 * `useSyncExternalStore` + `storage` event + custom-event broadcast shape
 * with `hooks/domains/integrations/use-hide-disabled-integrations-in-nav.ts`,
 * so the Settings left panel's Agents tree re-renders immediately in the
 * same tab and other tabs update via the browser `storage` event.
 */
export function useHideDisabledAgentProfilesInNav() {
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

  const getSnapshot = useCallback(() => readHideDisabled(), []);
  const getServerSnapshot = useCallback(() => false, []);

  const hideDisabled = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  const setHideDisabled = useCallback((next: boolean) => {
    try {
      window.localStorage.setItem(STORAGE_KEY, String(next));
    } catch (error) {
      throw new Error(`Failed to persist ${STORAGE_KEY}`, { cause: error });
    }
    window.dispatchEvent(new Event(SYNC_EVENT));
  }, []);

  return { hideDisabled, setHideDisabled };
}
