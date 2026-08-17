import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createDefaultUserSettings } from "@/lib/ssr/user-settings";
import type { UserSettingsState } from "@/lib/state/slices/settings/types";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { SettingsTargetProvider } from "@/components/settings/settings-target-provider";
import { emitSettingsTargetRequest } from "@/lib/settings-discovery/target";
import { formatDateTime } from "@/lib/i18n/formats";
import { ApiError } from "@/lib/api/client";
import type { AuthSession } from "@/lib/api/domains/auth-api";
import { registerUsersHandlers } from "@/lib/ws/handlers/users";
import type { BackendMessageMap } from "@/lib/types/backend";
import { SecuritySettings } from "./security-settings";

const authApiMocks = vi.hoisted(() => ({
  listSessions: vi.fn(),
  revokeSession: vi.fn(),
  changePassword: vi.fn(),
}));
const settingsApiMocks = vi.hoisted(() => ({ updateUserSettings: vi.fn() }));
const toastMocks = vi.hoisted(() => ({ error: vi.fn() }));

vi.mock("@/lib/api/domains/auth-api", () => ({
  listSessions: (...args: unknown[]) => authApiMocks.listSessions(...args),
  revokeSession: (...args: unknown[]) => authApiMocks.revokeSession(...args),
  changePassword: (...args: unknown[]) => authApiMocks.changePassword(...args),
}));
vi.mock("@/lib/api/domains/settings-api", () => ({
  updateUserSettings: (...args: unknown[]) => settingsApiMocks.updateUserSettings(...args),
}));
vi.mock("@/lib/toast/sonner", () => ({
  toast: { error: (...args: unknown[]) => toastMocks.error(...args) },
}));

const LAST_SEEN_AT = "2026-08-15T15:42:00Z";
const RELATIVE_TEST_ID = "last-seen-relative";
const SELECT_TEST_ID = "last-seen-display-select";
const EMPTY_TEST_ID = "last-seen-empty";
const RELATIVE_OPTION = "Relative time";
const ABSOLUTE_OPTION = "Absolute time";

const SESSION: AuthSession = {
  id: "sess-1",
  created_at: "2026-01-01T00:00:00Z",
  last_seen_at: LAST_SEEN_AT,
  user_agent: "Chrome on macOS",
  ip: "203.0.113.7",
  current: true,
};

function makeUserSettings(overrides: Partial<UserSettingsState> = {}): UserSettingsState {
  return { ...createDefaultUserSettings(), ...overrides };
}

type StoreHolder = { current: ReturnType<typeof useAppStoreApi> | null };

function StoreCapture({ holder }: { holder: StoreHolder }) {
  holder.current = useAppStoreApi();
  return null;
}

function renderSecurity(overrides: Partial<UserSettingsState> = {}, holder?: StoreHolder) {
  return render(
    <StateProvider initialState={{ userSettings: makeUserSettings({ revision: 1, ...overrides }) }}>
      <TooltipProvider>
        {holder ? <StoreCapture holder={holder} /> : null}
        <SecuritySettings />
      </TooltipProvider>
    </StateProvider>,
  );
}

async function renderLoaded(overrides: Partial<UserSettingsState> = {}, holder?: StoreHolder) {
  renderSecurity(overrides, holder);
  await screen.findByTestId(SELECT_TEST_ID);
}

// RemountHarness keeps one StateProvider (and thus one store) mounted while
// toggling only SecuritySettings, so a remounted component reads the same
// converged store the pre-unmount component mutated.
function RemountHarness({ holder, mounted }: { holder: StoreHolder; mounted: boolean }) {
  return (
    <StateProvider initialState={{ userSettings: makeUserSettings({ revision: 1 }) }}>
      <TooltipProvider>
        <StoreCapture holder={holder} />
        {mounted ? <SecuritySettings /> : null}
      </TooltipProvider>
    </StateProvider>
  );
}

// renderRelativeWithFakeTime mounts the card in relative mode under fake
// timers at a fixed clock, optionally overriding the session's last_seen_at.
async function renderRelativeWithFakeTime(lastSeenAt?: string) {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-08-15T15:45:00Z"));
  if (lastSeenAt) {
    authApiMocks.listSessions.mockResolvedValue({
      sessions: [{ ...SESSION, last_seen_at: lastSeenAt }],
    });
  }
  renderSecurity({ lastSeenDisplay: "relative" });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });
}

async function chooseDisplay(name: "Absolute time" | "Relative time") {
  fireEvent.click(screen.getByTestId(SELECT_TEST_ID));
  fireEvent.click(await screen.findByRole("option", { name }));
}

type PatchResponse = {
  settings: {
    user_id: string;
    workspace_id: string;
    repository_ids: string[];
    updated_at: string;
    revision: number;
    [key: string]: unknown;
  };
};

function patchResponse(overrides: Record<string, unknown> = {}): PatchResponse {
  return {
    settings: {
      user_id: "default-user",
      workspace_id: "workspace-1",
      repository_ids: [],
      updated_at: "2026-08-15T15:43:00Z",
      last_seen_display: "relative",
      revision: 2,
      ...overrides,
    },
  };
}

function settingsUpdatedMessage(
  payload: Partial<BackendMessageMap["user.settings.updated"]["payload"]>,
): BackendMessageMap["user.settings.updated"] {
  return {
    type: "notification",
    action: "user.settings.updated",
    payload: {
      user_id: "default-user",
      workspace_id: "workspace-1",
      repository_ids: [],
      ...payload,
    },
  };
}

function dispatchUserSettingsSnapshot(
  holder: StoreHolder,
  payload: Partial<BackendMessageMap["user.settings.updated"]["payload"]>,
) {
  registerUsersHandlers(holder.current!)["user.settings.updated"]?.(
    settingsUpdatedMessage(payload),
  );
}

function deferredPatch(): {
  promise: Promise<PatchResponse>;
  resolve: (value: PatchResponse) => void;
  reject: (error: ApiError) => void;
} {
  let resolve!: (value: PatchResponse) => void;
  let reject!: (error: ApiError) => void;
  const promise = new Promise<PatchResponse>((complete, fail) => {
    resolve = complete;
    reject = fail;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  authApiMocks.listSessions.mockReset().mockResolvedValue({ sessions: [SESSION] });
  authApiMocks.revokeSession.mockReset().mockResolvedValue(undefined);
  authApiMocks.changePassword.mockReset().mockResolvedValue(undefined);
  settingsApiMocks.updateUserSettings.mockReset().mockResolvedValue(patchResponse());
  toastMocks.error.mockReset();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("Last seen display rendering", () => {
  it("renders absolute time by default", async () => {
    await renderLoaded();

    expect(screen.queryByTestId(RELATIVE_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(EMPTY_TEST_ID)).toBeNull();
    expect(screen.getByText(formatDateTime(new Date(LAST_SEEN_AT)))).toBeTruthy();
  });

  it("renders a relative label when the store baseline is relative", async () => {
    await renderLoaded({ lastSeenDisplay: "relative" });

    const trigger = screen.getByTestId(RELATIVE_TEST_ID);
    expect(trigger.textContent).not.toBe("");
  });

  it("exposes the absolute timestamp as accessible name and native title", async () => {
    await renderLoaded({ lastSeenDisplay: "relative" });

    const trigger = screen.getByTestId(RELATIVE_TEST_ID);
    const absolute = formatDateTime(new Date(LAST_SEEN_AT));
    // The trigger is a semantic, focusable time element, not a bare span.
    expect(trigger.tagName).toBe("TIME");
    expect(trigger.getAttribute("dateTime")).toBe(new Date(LAST_SEEN_AT).toISOString());
    expect(trigger.getAttribute("aria-label")).toBe(absolute);
    expect(trigger.getAttribute("title")).toBe(absolute);
  });

  it("opens the absolute-time tooltip on keyboard focus of the relative trigger", async () => {
    await renderLoaded({ lastSeenDisplay: "relative" });

    const trigger = screen.getByTestId(RELATIVE_TEST_ID);
    fireEvent.focus(trigger);
    await waitFor(() => expect(screen.getByRole("tooltip")).toBeTruthy());
    expect(screen.getByRole("tooltip").textContent).toBe(formatDateTime(new Date(LAST_SEEN_AT)));
  });

  it("renders an empty cell for an unparseable last_seen_at", async () => {
    authApiMocks.listSessions.mockResolvedValue({
      sessions: [{ ...SESSION, last_seen_at: "not-a-date" }],
    });
    await renderLoaded({ lastSeenDisplay: "relative" });

    expect(screen.getByTestId(EMPTY_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId(RELATIVE_TEST_ID)).toBeNull();
  });

  it("keeps the Last seen select accessible when no sessions are returned", async () => {
    authApiMocks.listSessions.mockResolvedValue({ sessions: [] });
    await renderLoaded({ lastSeenDisplay: "absolute" });

    // The display option lives on the card independent of table rows.
    expect(screen.getByTestId(SELECT_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId("account-sessions-table")).toBeNull();
  });

  it("keeps the Last seen select reachable when loading sessions fails", async () => {
    authApiMocks.listSessions.mockRejectedValue(new Error("boom"));
    await renderLoaded({ lastSeenDisplay: "absolute" });
    await screen.findByTestId("account-sessions-error");

    // A transient sessions API failure must not hide the persisted preference.
    expect(screen.getByTestId(SELECT_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId("account-sessions-table")).toBeNull();
  });
});

describe("Last seen display persistence", () => {
  it("persists a relative selection through the queued sync", async () => {
    await renderLoaded();

    await chooseDisplay(RELATIVE_OPTION);

    await waitFor(() =>
      expect(settingsApiMocks.updateUserSettings).toHaveBeenCalledWith({
        last_seen_display: "relative",
      }),
    );
  });

  it("surfaces a revoke failure in the sessions card", async () => {
    authApiMocks.revokeSession.mockRejectedValue(new ApiError("boom", 400, null));
    authApiMocks.listSessions.mockResolvedValue({
      sessions: [{ ...SESSION, current: false, id: "sess-2" }],
    });
    await renderLoaded();

    fireEvent.click(screen.getByTestId("account-sessions-revoke"));
    await waitFor(() =>
      expect(screen.getByTestId("account-sessions-error").textContent).toContain("boom"),
    );
  });

  it("shows the error toast and falls back to the baseline on a failed save", async () => {
    settingsApiMocks.updateUserSettings.mockRejectedValue(new Error("boom"));
    await renderLoaded();

    await chooseDisplay(RELATIVE_OPTION);
    // The optimistic value shows while the write is in flight.
    expect(screen.getByTestId(RELATIVE_TEST_ID)).toBeTruthy();

    await waitFor(() => expect(toastMocks.error).toHaveBeenCalled());
    // After the failure settles, the cell falls back to the confirmed baseline.
    await waitFor(() => expect(screen.queryByTestId(RELATIVE_TEST_ID)).toBeNull());
    expect(screen.getByText(formatDateTime(new Date(LAST_SEEN_AT)))).toBeTruthy();
  });

  it("keeps the optimistic value while a write is in flight even when the store re-asserts the baseline", async () => {
    const { promise: pending, resolve: resolvePatch } = deferredPatch();
    settingsApiMocks.updateUserSettings.mockReturnValue(pending);
    const holder: StoreHolder = { current: null };
    await renderLoaded({}, holder);

    await chooseDisplay(RELATIVE_OPTION);
    await waitFor(() => expect(screen.getByTestId(RELATIVE_TEST_ID)).toBeTruthy());

    // An unrelated full snapshot re-asserts the baseline value while the write
    // is still in flight; the optimistic override must survive and still render.
    act(() => {
      dispatchUserSettingsSnapshot(holder, { last_seen_display: "absolute", revision: 3 });
    });
    expect(screen.getByTestId(RELATIVE_TEST_ID)).toBeTruthy();

    // The write settles: the confirmed relative value renders.
    await act(async () => {
      resolvePatch(patchResponse({ last_seen_display: "relative", revision: 4 }));
    });
    await waitFor(() => expect(screen.getByTestId(RELATIVE_TEST_ID)).toBeTruthy());
    expect(holder.current!.getState().userSettings.lastSeenDisplay).toBe("relative");
  });
});

describe("Last seen display queued write ordering", () => {
  it("keeps B's optimistic value visible while queued write A settles", async () => {
    const { promise: pendingA, resolve: resolveA } = deferredPatch();
    settingsApiMocks.updateUserSettings
      .mockReturnValueOnce(pendingA)
      .mockResolvedValueOnce(patchResponse({ last_seen_display: "absolute", revision: 3 }));
    const holder: StoreHolder = { current: null };
    await renderLoaded({}, holder);

    await chooseDisplay(RELATIVE_OPTION); // A
    expect(screen.getByTestId(RELATIVE_TEST_ID)).toBeTruthy();

    await chooseDisplay(ABSOLUTE_OPTION); // B (queued behind A)
    expect(screen.queryByTestId(RELATIVE_TEST_ID)).toBeNull();

    // A settles first: the latest-operation guard keeps B's override visible,
    // so the cell does NOT flip to A's confirmed relative value.
    await act(async () => {
      resolveA(patchResponse({ last_seen_display: "relative", revision: 2 }));
    });
    expect(screen.queryByTestId(RELATIVE_TEST_ID)).toBeNull();

    // B settles: the confirmed absolute value renders.
    await waitFor(() =>
      expect(holder.current!.getState().userSettings.lastSeenDisplay).toBe("absolute"),
    );
  });

  it("does not toast or revert on a stale failed write for a newer selection", async () => {
    const { promise: pendingA, reject: rejectA } = deferredPatch();
    settingsApiMocks.updateUserSettings
      .mockReturnValueOnce(pendingA)
      .mockResolvedValueOnce(patchResponse({ last_seen_display: "absolute", revision: 3 }));
    const holder: StoreHolder = { current: null };
    await renderLoaded({}, holder);

    await chooseDisplay(RELATIVE_OPTION); // A
    await chooseDisplay(ABSOLUTE_OPTION); // B (queued behind A)

    await act(async () => {
      rejectA(new ApiError("boom", 400, null));
    });
    // A's failure is stale (B is the latest operation): no error toast, and
    // B's optimistic absolute stays visible.
    expect(toastMocks.error).not.toHaveBeenCalled();
    expect(screen.queryByTestId(RELATIVE_TEST_ID)).toBeNull();

    await waitFor(() =>
      expect(holder.current!.getState().userSettings.lastSeenDisplay).toBe("absolute"),
    );
  });

  it("settles two consecutive failures on the server-confirmed baseline", async () => {
    const { promise: pendingA, reject: rejectA } = deferredPatch();
    const { promise: pendingB, reject: rejectB } = deferredPatch();
    settingsApiMocks.updateUserSettings.mockReturnValueOnce(pendingA).mockReturnValueOnce(pendingB);
    const holder: StoreHolder = { current: null };
    // Baseline absolute; A moves to relative, then B back to absolute. A
    // foreign snapshot changes the server baseline to relative while both
    // writes are pending, so B's optimistic absolute differs from the store
    // and a wrong unconditional rollback after A's failure is observable.
    await renderLoaded({}, holder);

    await chooseDisplay(RELATIVE_OPTION); // A: optimistic relative, write pending
    expect(screen.getByTestId(RELATIVE_TEST_ID)).toBeTruthy();
    await chooseDisplay(ABSOLUTE_OPTION); // B: optimistic absolute, write queued
    expect(screen.queryByTestId(RELATIVE_TEST_ID)).toBeNull();

    // Foreign snapshot changes the server baseline while both writes pend.
    act(() => {
      dispatchUserSettingsSnapshot(holder, { last_seen_display: "relative", revision: 3 });
    });

    // Reject A (stale, B is the latest operation): no toast, and B's
    // optimistic absolute stays rendered despite the foreign baseline.
    await act(async () => {
      rejectA(new ApiError("boom A", 400, null));
    });
    expect(toastMocks.error).not.toHaveBeenCalled();
    expect(screen.queryByTestId(RELATIVE_TEST_ID)).toBeNull();
    expect(screen.getByText(formatDateTime(new Date(LAST_SEEN_AT)))).toBeTruthy();

    // Reject B (the latest operation): the override clears and the cell falls
    // back to the server-confirmed baseline (the foreign relative snapshot).
    await act(async () => {
      rejectB(new ApiError("boom B", 400, null));
    });
    await waitFor(() => expect(toastMocks.error).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(screen.getByTestId(RELATIVE_TEST_ID)).toBeTruthy());
  });
});

describe("Last seen display write and lifecycle races", () => {
  it("discards a deferred PATCH response older than a newer WS snapshot", async () => {
    const { promise: pending, resolve: resolvePatch } = deferredPatch();
    settingsApiMocks.updateUserSettings.mockReturnValue(pending);
    const holder: StoreHolder = { current: null };
    await renderLoaded({}, holder);

    await chooseDisplay(RELATIVE_OPTION); // write in flight (resolves at rev 2)

    // A newer WS snapshot (foreign change at rev 5) lands first through the
    // real user.settings.updated handler.
    act(() => {
      dispatchUserSettingsSnapshot(holder, { last_seen_display: "absolute", revision: 5 });
    });

    // The deferred PATCH response (rev 2) is discarded by the revision guard;
    // the cell renders the newer foreign value.
    await act(async () => {
      resolvePatch(patchResponse({ last_seen_display: "relative", revision: 2 }));
    });
    await waitFor(() => expect(screen.queryByTestId(RELATIVE_TEST_ID)).toBeNull());
    expect(screen.getByText(formatDateTime(new Date(LAST_SEEN_AT)))).toBeTruthy();
    expect(holder.current!.getState().userSettings.revision).toBe(5);
    expect(holder.current!.getState().userSettings.lastSeenDisplay).toBe("absolute");
  });

  it("converges the store when the component unmounts mid-write and remounts", async () => {
    const { promise: pending, resolve: resolvePatch } = deferredPatch();
    settingsApiMocks.updateUserSettings.mockReturnValue(pending);
    const holder: StoreHolder = { current: null };

    const view = render(<RemountHarness holder={holder} mounted />);
    await screen.findByTestId(SELECT_TEST_ID);

    await chooseDisplay(RELATIVE_OPTION);
    await waitFor(() => expect(screen.getByTestId(RELATIVE_TEST_ID)).toBeTruthy());

    // Unmount only the component; the provider and store stay mounted. A newer
    // WS snapshot lands after unmount through the real handler, then the PATCH
    // resolves (its older revision is discarded), so the store converges.
    view.rerender(<RemountHarness holder={holder} mounted={false} />);
    act(() => {
      dispatchUserSettingsSnapshot(holder, { last_seen_display: "absolute", revision: 5 });
    });
    await act(async () => {
      resolvePatch(patchResponse({ last_seen_display: "relative", revision: 2 }));
    });
    await waitFor(() =>
      expect(holder.current!.getState().userSettings.lastSeenDisplay).toBe("absolute"),
    );
    expect(holder.current!.getState().userSettings.revision).toBe(5);

    // Re-mount the component under the SAME store: it must render the converged
    // store value, not a seeded baseline.
    view.rerender(<RemountHarness holder={holder} mounted />);
    await screen.findByTestId(SELECT_TEST_ID);
    expect(screen.queryByTestId(RELATIVE_TEST_ID)).toBeNull();
    expect(screen.getByText(formatDateTime(new Date(LAST_SEEN_AT)))).toBeTruthy();
    view.unmount();
  });

  it("handles a rejected write after unmount and remounts without a stale override", async () => {
    const { promise: pending, reject: rejectPatch } = deferredPatch();
    settingsApiMocks.updateUserSettings.mockReturnValue(pending);
    const holder: StoreHolder = { current: null };

    const view = render(<RemountHarness holder={holder} mounted />);
    await screen.findByTestId(SELECT_TEST_ID);

    await chooseDisplay(RELATIVE_OPTION);
    await waitFor(() => expect(screen.getByTestId(RELATIVE_TEST_ID)).toBeTruthy());

    // Unmount only the component; the provider and store stay mounted. A newer
    // WS snapshot lands after unmount through the real handler, then the PATCH
    // is rejected: the toast fires, the store keeps the WS value.
    view.rerender(<RemountHarness holder={holder} mounted={false} />);
    act(() => {
      dispatchUserSettingsSnapshot(holder, { last_seen_display: "relative", revision: 3 });
    });
    await act(async () => {
      rejectPatch(new ApiError("boom", 400, null));
    });
    await waitFor(() => expect(toastMocks.error).toHaveBeenCalled());
    await waitFor(() =>
      expect(holder.current!.getState().userSettings.lastSeenDisplay).toBe("relative"),
    );

    // Re-mount the component under the SAME store: it renders the converged WS
    // value, with no stale optimistic override.
    view.rerender(<RemountHarness holder={holder} mounted />);
    await screen.findByTestId(SELECT_TEST_ID);
    expect(screen.getByTestId(RELATIVE_TEST_ID)).toBeTruthy();
    view.unmount();
  });

  it("ticks every second while the timestamp is under a minute old", async () => {
    // last_seen 55s before now: "55 seconds ago", well past the just-now
    // threshold so a one-second advance changes the label.
    await renderRelativeWithFakeTime("2026-08-15T15:44:05Z");

    const before = screen.getByTestId(RELATIVE_TEST_ID).textContent;
    act(() => {
      vi.advanceTimersByTime(1_000);
    });
    expect(screen.getByTestId(RELATIVE_TEST_ID).textContent).not.toBe(before);
  });

  it("ticks every minute once the timestamp is a minute or more old", async () => {
    await renderRelativeWithFakeTime();

    const before = screen.getByTestId(RELATIVE_TEST_ID).textContent;
    act(() => {
      vi.advanceTimersByTime(60_000);
    });
    expect(screen.getByTestId(RELATIVE_TEST_ID).textContent).not.toBe(before);
  });
});

describe("Last seen display discovery", () => {
  it("registers the sessions and last-seen-display targets", async () => {
    const reveal = vi.fn();
    authApiMocks.listSessions.mockResolvedValue({ sessions: [SESSION] });
    render(
      <StateProvider initialState={{ userSettings: makeUserSettings({ revision: 1 }) }}>
        <SettingsTargetProvider revealTarget={reveal}>
          <TooltipProvider>
            <SecuritySettings />
          </TooltipProvider>
        </SettingsTargetProvider>
      </StateProvider>,
    );
    await screen.findByTestId(SELECT_TEST_ID);

    emitSettingsTargetRequest("setting-account-sessions");
    await waitFor(() => expect(reveal).toHaveBeenCalledTimes(1));

    emitSettingsTargetRequest("setting-account-last-seen-display");
    await waitFor(() => expect(reveal).toHaveBeenCalledTimes(2));
  });
});
