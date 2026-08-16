const PAGESHOW_EVENT = "pageshow";
const RESTORED_NAVIGATION_TYPE = "back_forward";

type RestoreReloadTarget = Pick<EventTarget, "addEventListener" | "removeEventListener">;

interface RestoreReloadOptions {
  target?: RestoreReloadTarget;
  reload?: () => void;
  getNavigationType?: () => string | undefined;
}

function readNavigationType(): string | undefined {
  try {
    const entry = performance.getEntriesByType("navigation")[0];
    return entry ? (entry as PerformanceNavigationTiming).type : undefined;
  } catch {
    // Navigation Timing is unavailable (or the entry is not exposed); the
    // persisted flag on pageshow remains the restore signal.
    return undefined;
  }
}

/**
 * Reloads the page when it is restored from a frozen browser snapshot
 * (back/forward cache restore, including Chrome's "Duplicate tab") instead of
 * being loaded fresh. A restored page keeps the frozen JS heap and DOM, so
 * state can be stale until a real load; the reload re-fetches the no-store
 * boot payload and reconnects the WebSocket.
 *
 * A restore is detected on `pageshow` when `event.persisted` is true, or when
 * the current document's navigation type is `back_forward` (covers
 * state-clone restores where `persisted` is false). Fresh loads
 * (`navigate`), manual refreshes (`reload`), and in-app SPA routing never
 * reload; a reload produces a fresh document and cannot loop.
 *
 * Returns an uninstall function.
 */
export function installBfcacheRestoreReload({
  target = window,
  reload = () => window.location.reload(),
  getNavigationType = readNavigationType,
}: RestoreReloadOptions = {}): () => void {
  const handlePageshow = (event: Event) => {
    const persisted = (event as PageTransitionEvent).persisted === true;
    let restoredByNavigationType = false;
    try {
      restoredByNavigationType = getNavigationType() === RESTORED_NAVIGATION_TYPE;
    } catch {
      // A throwing navigation-type reader degrades to persisted-only
      // detection; a restore is still reloaded when persisted is true.
    }
    if (persisted || restoredByNavigationType) {
      reload();
    }
  };

  target.addEventListener(PAGESHOW_EVENT, handlePageshow);
  return () => target.removeEventListener(PAGESHOW_EVENT, handlePageshow);
}
