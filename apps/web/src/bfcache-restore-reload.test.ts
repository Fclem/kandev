import { describe, expect, it, vi } from "vitest";
import { installBfcacheRestoreReload } from "./bfcache-restore-reload";

function createHarness(navigationType: string | undefined = "navigate") {
  const target = new EventTarget();
  const reload = vi.fn();
  const getNavigationType = vi.fn(() => navigationType);
  const cleanup = installBfcacheRestoreReload({ target, reload, getNavigationType });

  return {
    target,
    reload,
    getNavigationType,
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
    const harness = createHarness("navigate");

    harness.dispatch(true);

    expect(harness.reload).toHaveBeenCalledOnce();
  });

  it("reloads on a back_forward-typed restore even when persisted is false", () => {
    const harness = createHarness("back_forward");

    harness.dispatch(false);

    expect(harness.reload).toHaveBeenCalledOnce();
  });

  it("does not reload on a fresh load (navigate)", () => {
    const harness = createHarness("navigate");

    harness.dispatch(false);

    expect(harness.reload).not.toHaveBeenCalled();
  });

  it("does not reload on a manual refresh (reload)", () => {
    const harness = createHarness("reload");

    harness.dispatch(false);

    expect(harness.reload).not.toHaveBeenCalled();
  });

  it("does not reload when no restore signal is available", () => {
    const harness = createHarness(undefined);

    harness.dispatch(false);

    expect(harness.reload).not.toHaveBeenCalled();
  });

  it("keeps reloading on persisted restores when the Navigation Timing API is unavailable", () => {
    const harness = createHarness("navigate");
    harness.getNavigationType.mockImplementation(() => {
      throw new Error("performance unavailable");
    });

    expect(() => harness.dispatch(true)).not.toThrow();
    expect(harness.reload).toHaveBeenCalledOnce();

    harness.reload.mockClear();
    expect(() => harness.dispatch(false)).not.toThrow();
    expect(harness.reload).not.toHaveBeenCalled();
  });

  it("uninstall removes the listener", () => {
    const harness = createHarness("back_forward");

    harness.cleanup();
    harness.dispatch(true);

    expect(harness.reload).not.toHaveBeenCalled();
  });
});
