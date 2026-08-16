const PAGESHOW_EVENT = "pageshow";

type RestoreReloadTarget = Pick<EventTarget, "addEventListener" | "removeEventListener">;

interface RestoreReloadOptions {
  target?: RestoreReloadTarget;
  reload?: () => void;
}

/**
 * Reloads the page when it is restored from a frozen browser snapshot
 * (back/forward cache restore, including Chrome's "Duplicate tab") instead of
 * being loaded fresh. A restored page keeps the frozen JS heap and DOM, so
 * state can be stale until a real load; the reload re-fetches the no-store
 * boot payload and reconnects the WebSocket.
 *
 * A restore is detected on `pageshow` when `event.persisted` is true. That is
 * the only reliable frozen-restore signal: the navigation type
 * `back_forward` also covers cold history traversals and session-restored
 * tabs, which load fresh and must NOT be reloaded a second time. Fresh loads
 * (`navigate`), manual refreshes (`reload`), and in-app SPA routing never
 * reload; a reload produces a fresh document and cannot loop.
 *
 * Returns an uninstall function.
 */
export function installBfcacheRestoreReload({
  target = window,
  reload = () => window.location.reload(),
}: RestoreReloadOptions = {}): () => void {
  const handlePageshow = (event: Event) => {
    if ((event as PageTransitionEvent).persisted === true) {
      reload();
    }
  };

  target.addEventListener(PAGESHOW_EVENT, handlePageshow);
  return () => target.removeEventListener(PAGESHOW_EVENT, handlePageshow);
}
