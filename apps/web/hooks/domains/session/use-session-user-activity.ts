import { useEffect, useRef, useSyncExternalStore } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  getBackendReloadSnapshot,
  subscribeToBackendReload,
} from "@/lib/platform/backend-reload-coordinator";
import { getWebSocketClient } from "@/lib/ws/connection";

const BEAT_INTERVAL_MS = 15_000;
const IDLE_TIMEOUT_MS = 60_000;

export function useSessionUserActivity(sessionId: string | null, isFocused: boolean): void {
  const featureEnabled = useAppStore((state) => state.features.sessionUserActivity);
  const reloadRequired = useSyncExternalStore(
    subscribeToBackendReload,
    () => getBackendReloadSnapshot().reloadRequired,
    () => false,
  );
  const lastSessionIdRef = useRef(sessionId);
  const lastInteractionAtRef = useRef<number | null>(null);
  const store = useAppStoreApi();

  const sessionChanged = lastSessionIdRef.current !== sessionId;
  if (sessionChanged) lastSessionIdRef.current = sessionId;
  if (sessionChanged || !isFocused) lastInteractionAtRef.current = null;

  useEffect(() => {
    if (!sessionId || !isFocused || !featureEnabled || reloadRequired) return;

    let engaged = false;
    let windowFocused = document.hasFocus();
    const sendBeat = () => {
      if (!store.getState().features.sessionUserActivity) {
        engaged = false;
        return;
      }
      const client = getWebSocketClient();
      if (!client) return;
      void client
        .sendSessionUserActivityBeat(sessionId)
        .then((ack) => {
          if (ack?.enabled === false) {
            const current = store.getState();
            if (current.features.sessionUserActivity) {
              current.setFeatures({ ...current.features, sessionUserActivity: false });
            }
            engaged = false;
          }
        })
        .catch(() => undefined);
    };
    const checkEngagement = () => {
      if (!store.getState().features.sessionUserActivity) {
        engaged = false;
        return;
      }
      const lastInteractionAt = lastInteractionAtRef.current;
      const isEngaged =
        isFocused &&
        windowFocused &&
        document.visibilityState === "visible" &&
        lastInteractionAt !== null &&
        Date.now() - lastInteractionAt < IDLE_TIMEOUT_MS;
      if (isEngaged && !engaged) sendBeat();
      engaged = isEngaged;
    };
    const recordInteraction = () => {
      lastInteractionAtRef.current = Date.now();
      checkEngagement();
    };
    const onVisibilityChange = () => checkEngagement();
    const onWindowFocus = () => {
      windowFocused = document.hasFocus();
      checkEngagement();
    };
    const onWindowBlur = () => {
      windowFocused = document.hasFocus();
      checkEngagement();
    };

    document.addEventListener("pointerdown", recordInteraction, { passive: true });
    document.addEventListener("keydown", recordInteraction, { passive: true });
    document.addEventListener("wheel", recordInteraction, { passive: true });
    document.addEventListener("touchstart", recordInteraction, { passive: true });
    document.addEventListener("visibilitychange", onVisibilityChange);
    window.addEventListener("focus", onWindowFocus);
    window.addEventListener("blur", onWindowBlur);
    const interval = window.setInterval(() => {
      checkEngagement();
      if (engaged) sendBeat();
    }, BEAT_INTERVAL_MS);
    checkEngagement();

    return () => {
      window.clearInterval(interval);
      document.removeEventListener("pointerdown", recordInteraction);
      document.removeEventListener("keydown", recordInteraction);
      document.removeEventListener("wheel", recordInteraction);
      document.removeEventListener("touchstart", recordInteraction);
      document.removeEventListener("visibilitychange", onVisibilityChange);
      window.removeEventListener("focus", onWindowFocus);
      window.removeEventListener("blur", onWindowBlur);
    };
  }, [featureEnabled, isFocused, reloadRequired, sessionId, store]);
}
