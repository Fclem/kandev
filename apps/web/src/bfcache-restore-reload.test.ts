import { afterEach, describe, expect, it, vi } from "vitest";
import { installBfcacheRestoreReload } from "./bfcache-restore-reload";

afterEach(() => {
  vi.unstubAllGlobals();
});

function createHarness() {
  const target = new EventTarget();
  const reload = vi.fn();
  const cleanup = installBfcacheRestoreReload({ target, reload });

  return {
    target,
    reload,
    cleanup,
    dispatch(persisted: boolean) {
      const event = new Event("pageshow");
      Object.defineProperty(event, "persisted", { value: persisted });
      target.dispatchEvent(event);
      return event;
    },
  };
}

describe("installBfcacheRestoreReload", () => {
  it("reloads when the page is restored from a frozen snapshot (persisted=true)", () => {
    const harness = createHarness();

    harness.dispatch(true);

    expect(harness.reload).toHaveBeenCalledOnce();
  });

  it("does not reload on a cold back/forward traversal (persisted=false)", () => {
    // Regression for the round-1 finding: a history traversal not served
    // from bfcache (e.g. an open WebSocket made the no-store page ineligible)
    // loads fresh and must not be reloaded a second time, even though its
    // navigation entry reports type `back_forward`. Stub that entry so
    // reintroducing the navigation-type fallback would fail this test.
    vi.stubGlobal("performance", {
      // happy-dom's Event constructor calls performance.now(); keep it intact
      // while exposing the back_forward navigation entry.
      now: () => 0,
      getEntriesByType: (type: string) => (type === "navigation" ? [{ type: "back_forward" }] : []),
    });
    const harness = createHarness();

    harness.dispatch(false);

    expect(harness.reload).not.toHaveBeenCalled();
  });

  it("does not reload on a fresh load (persisted=false)", () => {
    const harness = createHarness();

    harness.dispatch(false);

    expect(harness.reload).not.toHaveBeenCalled();
  });

  it("does not reload on a manual refresh (persisted=false)", () => {
    const harness = createHarness();

    harness.dispatch(false);

    expect(harness.reload).not.toHaveBeenCalled();
  });

  it("does not reload when the event carries no persisted flag", () => {
    const harness = createHarness();

    harness.target.dispatchEvent(new Event("pageshow"));

    expect(harness.reload).not.toHaveBeenCalled();
  });

  it("uninstall removes the listener", () => {
    const harness = createHarness();

    harness.cleanup();
    harness.dispatch(true);

    expect(harness.reload).not.toHaveBeenCalled();
  });
});
