import { useSyncExternalStore } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  sendSessionUserActivityBeat: vi.fn(),
  featureEnabled: true,
  deferFeatureNotifications: false,
  featureListeners: new Set<() => void>(),
  reloadRequired: false,
  reloadListeners: new Set<() => void>(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { features: { sessionUserActivity: boolean } }) => unknown) =>
    useSyncExternalStore(
      (listener) => {
        mocks.featureListeners.add(listener);
        return () => mocks.featureListeners.delete(listener);
      },
      () => selector({ features: { sessionUserActivity: mocks.featureEnabled } }),
    ),
  useAppStoreApi: () => ({
    getState: () => ({
      features: { sessionUserActivity: mocks.featureEnabled },
      setFeatures: (features: { sessionUserActivity: boolean }) => {
        mocks.featureEnabled = features.sessionUserActivity;
        if (!mocks.deferFeatureNotifications)
          mocks.featureListeners.forEach((listener) => listener());
      },
    }),
  }),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({
    sendSessionUserActivityBeat: mocks.sendSessionUserActivityBeat,
  }),
}));

vi.mock("@/lib/platform/backend-reload-coordinator", () => ({
  getBackendReloadSnapshot: () => ({ reloadRequired: mocks.reloadRequired }),
  subscribeToBackendReload: (listener: () => void) => {
    mocks.reloadListeners.add(listener);
    return () => mocks.reloadListeners.delete(listener);
  },
}));

import { useSessionUserActivity } from "./use-session-user-activity";

function setReloadRequired(reloadRequired: boolean) {
  mocks.reloadRequired = reloadRequired;
  mocks.reloadListeners.forEach((listener) => listener());
}

function interact(type: "pointerdown" | "keydown" | "wheel" | "touchstart") {
  act(() => document.dispatchEvent(new Event(type)));
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(1_000);
  mocks.sendSessionUserActivityBeat.mockReset().mockResolvedValue({ enabled: true });
  mocks.featureEnabled = true;
  mocks.deferFeatureNotifications = false;
  mocks.featureListeners.clear();
  mocks.reloadRequired = false;
  mocks.reloadListeners.clear();
  vi.spyOn(document, "hasFocus").mockReturnValue(true);
  Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("useSessionUserActivity", () => {
  // @covers AC-TASKS-USER-ACTIVITY-001.1, AC-TASKS-USER-ACTIVITY-001.5
  it("reports only for a visible, focused, recently interacted session with its ID only", async () => {
    renderHook(() => useSessionUserActivity("session-a", true));

    interact("pointerdown");
    await act(async () => Promise.resolve());
    expect(mocks.sendSessionUserActivityBeat).toHaveBeenLastCalledWith("session-a");
    expect(mocks.sendSessionUserActivityBeat).toHaveBeenCalledTimes(1);

    await act(async () => vi.advanceTimersByTimeAsync(15_000));
    expect(mocks.sendSessionUserActivityBeat).toHaveBeenCalledTimes(2);

    await act(async () => vi.advanceTimersByTimeAsync(45_000));
    expect(mocks.sendSessionUserActivityBeat).toHaveBeenCalledTimes(4);
    await act(async () => vi.advanceTimersByTimeAsync(15_000));
    expect(mocks.sendSessionUserActivityBeat).toHaveBeenCalledTimes(4);
  });

  // @covers AC-TASKS-USER-ACTIVITY-001.1, AC-TASKS-USER-ACTIVITY-001.2
  it("starts immediately after a touch interaction and stops while blurred or hidden", async () => {
    renderHook(() => useSessionUserActivity("session-a", true));
    interact("touchstart");
    await act(async () => Promise.resolve());
    expect(mocks.sendSessionUserActivityBeat).toHaveBeenCalledTimes(1);

    vi.mocked(document.hasFocus).mockReturnValue(false);
    act(() => window.dispatchEvent(new Event("blur")));
    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    expect(mocks.sendSessionUserActivityBeat).toHaveBeenCalledTimes(1);

    vi.mocked(document.hasFocus).mockReturnValue(true);
    act(() => window.dispatchEvent(new Event("focus")));
    await act(async () => Promise.resolve());
    expect(mocks.sendSessionUserActivityBeat).toHaveBeenCalledTimes(2);
    Object.defineProperty(document, "visibilityState", { configurable: true, value: "hidden" });
    act(() => document.dispatchEvent(new Event("visibilitychange")));
    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    expect(mocks.sendSessionUserActivityBeat).toHaveBeenCalledTimes(2);
  });

  // @covers AC-TASKS-USER-ACTIVITY-001.2
  it("does not carry activity across session switches and cleans up on unmount", async () => {
    const { rerender, unmount } = renderHook(
      ({ sessionId, isFocused }: { sessionId: string; isFocused: boolean }) =>
        useSessionUserActivity(sessionId, isFocused),
      { initialProps: { sessionId: "session-a", isFocused: false } },
    );
    interact("keydown");
    expect(mocks.sendSessionUserActivityBeat).not.toHaveBeenCalled();
    rerender({ sessionId: "session-a", isFocused: true });
    await act(async () => vi.advanceTimersByTimeAsync(15_000));
    expect(mocks.sendSessionUserActivityBeat).not.toHaveBeenCalled();
    interact("keydown");
    await act(async () => Promise.resolve());
    expect(mocks.sendSessionUserActivityBeat).toHaveBeenCalledWith("session-a");

    rerender({ sessionId: "session-b", isFocused: true });
    await act(async () => vi.advanceTimersByTimeAsync(15_000));
    expect(mocks.sendSessionUserActivityBeat).toHaveBeenCalledTimes(1);
    interact("wheel");
    await act(async () => Promise.resolve());
    expect(mocks.sendSessionUserActivityBeat).toHaveBeenLastCalledWith("session-b");
    unmount();
    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    expect(mocks.sendSessionUserActivityBeat).toHaveBeenCalledTimes(2);
  });
});

describe("useSessionUserActivity acknowledgements", () => {
  // @covers AC-TASKS-USER-ACTIVITY-003.1
  it("applies a disabled acknowledgement across session switches and other reporters", async () => {
    const oldAcknowledgement = Promise.withResolvers<{ enabled: boolean }>();
    mocks.sendSessionUserActivityBeat.mockImplementationOnce(() => oldAcknowledgement.promise);
    const { rerender } = renderHook(
      ({ sessionId }: { sessionId: string }) => useSessionUserActivity(sessionId, true),
      { initialProps: { sessionId: "session-a" } },
    );
    const otherReporter = renderHook(
      ({ isFocused }: { isFocused: boolean }) => useSessionUserActivity("session-c", isFocused),
      { initialProps: { isFocused: false } },
    );
    interact("pointerdown");
    expect(mocks.sendSessionUserActivityBeat).toHaveBeenCalledTimes(1);

    rerender({ sessionId: "session-b" });
    interact("pointerdown");
    expect(mocks.sendSessionUserActivityBeat).toHaveBeenLastCalledWith("session-b");
    otherReporter.rerender({ isFocused: true });
    mocks.deferFeatureNotifications = true;
    await act(async () => {
      oldAcknowledgement.resolve({ enabled: false });
      await Promise.resolve();
      document.dispatchEvent(new Event("keydown"));
      await vi.advanceTimersByTimeAsync(15_000);
    });
    expect(mocks.featureEnabled).toBe(false);
    expect(mocks.sendSessionUserActivityBeat).toHaveBeenCalledTimes(2);
    expect(mocks.sendSessionUserActivityBeat).toHaveBeenLastCalledWith("session-b");
    mocks.deferFeatureNotifications = false;
    act(() => mocks.featureListeners.forEach((listener) => listener()));
  });

  // @covers AC-TASKS-USER-ACTIVITY-003.1
  it("stops for a disabled flag, a disabled acknowledgement, and reload-required state", async () => {
    mocks.featureEnabled = false;
    const disabled = renderHook(() => useSessionUserActivity("session-a", true));
    interact("pointerdown");
    await act(async () => vi.advanceTimersByTimeAsync(15_000));
    expect(mocks.sendSessionUserActivityBeat).not.toHaveBeenCalled();
    disabled.unmount();

    mocks.featureEnabled = true;
    mocks.sendSessionUserActivityBeat.mockResolvedValue({ enabled: false });
    renderHook(() => useSessionUserActivity("session-a", true));
    interact("pointerdown");
    await act(async () => Promise.resolve());
    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    expect(mocks.sendSessionUserActivityBeat).toHaveBeenCalledTimes(1);
  });

  // @covers AC-TASKS-USER-ACTIVITY-003.1
  it("stops when the backend requires a reload", async () => {
    const { rerender } = renderHook(() => useSessionUserActivity("session-a", true));
    interact("pointerdown");
    await act(async () => Promise.resolve());
    expect(mocks.sendSessionUserActivityBeat).toHaveBeenCalledTimes(1);

    act(() => setReloadRequired(true));
    rerender();
    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    expect(mocks.sendSessionUserActivityBeat).toHaveBeenCalledTimes(1);
  });
});
