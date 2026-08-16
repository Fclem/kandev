import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createDefaultUserSettings } from "@/lib/ssr/user-settings";
import { compareUserSettingsRevisions } from "@/lib/settings/user-settings-revision";
import type { UserSettingsState } from "@/lib/state/slices/settings/types";
import { SettingsTargetProvider } from "@/components/settings/settings-target-provider";
import { emitSettingsTargetRequest } from "@/lib/settings-discovery/target";
import { formatDateTime } from "@/lib/i18n/formats";
import { ApiError } from "@/lib/api/client";
import type { AuthSession } from "@/lib/api/domains/auth-api";
import { SecuritySettings } from "./security-settings";

const authApiMocks = vi.hoisted(() => ({
  listSessions: vi.fn(),
  revokeSession: vi.fn(),
  changePassword: vi.fn(),
}));
const settingsApiMocks = vi.hoisted(() => ({ updateUserSettings: vi.fn() }));
const toastMocks = vi.hoisted(() => ({ error: vi.fn() }));
const storeMocks = vi.hoisted(() => ({
  state: {} as Record<string, unknown>,
  setUserSettings: vi.fn(),
}));

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
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector(storeMocks.state),
  useAppStoreApi: () => ({ getState: () => storeMocks.state }),
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

function setBaseline(overrides: Partial<UserSettingsState> = {}) {
  storeMocks.state = {
    userSettings: makeUserSettings({ revision: 1, ...overrides }),
    setUserSettings: storeMocks.setUserSettings,
  };
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

function renderLoaded() {
  render(
    <TooltipProvider>
      <SecuritySettings />
    </TooltipProvider>,
  );
  return screen.findByTestId(SELECT_TEST_ID);
}

async function chooseDisplay(name: "Absolute time" | "Relative time") {
  fireEvent.click(screen.getByTestId(SELECT_TEST_ID));
  fireEvent.click(await screen.findByRole("option", { name }));
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
  storeMocks.setUserSettings.mockReset().mockImplementation((settings: UserSettingsState) => {
    const current = storeMocks.state.userSettings as UserSettingsState;
    if (compareUserSettingsRevisions(settings.revision, current.revision) === -1) return;
    storeMocks.state.userSettings = settings;
  });
  setBaseline({ lastSeenDisplay: "absolute" });
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
    setBaseline({ lastSeenDisplay: "relative" });
    await renderLoaded();

    const trigger = screen.getByTestId(RELATIVE_TEST_ID);
    expect(trigger.textContent).not.toBe("");
  });

  it("exposes the absolute timestamp as accessible name and native title", async () => {
    setBaseline({ lastSeenDisplay: "relative" });
    await renderLoaded();

    const trigger = screen.getByTestId(RELATIVE_TEST_ID);
    const absolute = formatDateTime(new Date(LAST_SEEN_AT));
    expect(trigger.getAttribute("aria-label")).toBe(absolute);
    expect(trigger.getAttribute("title")).toBe(absolute);
  });

  it("opens the absolute-time tooltip on keyboard focus of the relative trigger", async () => {
    setBaseline({ lastSeenDisplay: "relative" });
    await renderLoaded();

    const trigger = screen.getByTestId(RELATIVE_TEST_ID);
    fireEvent.focus(trigger);
    await waitFor(() => expect(screen.getByRole("tooltip")).toBeTruthy());
    expect(screen.getByRole("tooltip").textContent).toBe(formatDateTime(new Date(LAST_SEEN_AT)));
  });

  it("renders an empty cell for an unparseable last_seen_at", async () => {
    authApiMocks.listSessions.mockResolvedValue({
      sessions: [{ ...SESSION, last_seen_at: "not-a-date" }],
    });
    setBaseline({ lastSeenDisplay: "relative" });
    await renderLoaded();

    expect(screen.getByTestId(EMPTY_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId(RELATIVE_TEST_ID)).toBeNull();
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
    await renderLoaded();

    await chooseDisplay(RELATIVE_OPTION);
    await waitFor(() => expect(screen.getByTestId(RELATIVE_TEST_ID)).toBeTruthy());

    // An unrelated full snapshot re-asserts the baseline value while the write
    // is still in flight; the optimistic override must survive.
    act(() => {
      (storeMocks.state.setUserSettings as (s: UserSettingsState) => void)(
        makeUserSettings({ lastSeenDisplay: "absolute", revision: 3 }),
      );
    });
    expect(screen.getByTestId(RELATIVE_TEST_ID)).toBeTruthy();

    await act(async () => {
      resolvePatch(patchResponse({ last_seen_display: "relative", revision: 4 }));
    });
    await waitFor(() => expect(storeMocks.setUserSettings).toHaveBeenCalled());
  });
});

describe("Last seen display queued write ordering", () => {
  it("keeps B's optimistic value visible while queued write A settles", async () => {
    const { promise: pendingA, resolve: resolveA } = deferredPatch();
    settingsApiMocks.updateUserSettings
      .mockReturnValueOnce(pendingA)
      .mockResolvedValueOnce(patchResponse({ last_seen_display: "absolute", revision: 3 }));
    await renderLoaded();

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
      expect((storeMocks.state.userSettings as UserSettingsState).lastSeenDisplay).toBe("absolute"),
    );
  });

  it("does not toast or revert on a stale failed write for a newer selection", async () => {
    const { promise: pendingA, reject: rejectA } = deferredPatch();
    settingsApiMocks.updateUserSettings
      .mockReturnValueOnce(pendingA)
      .mockResolvedValueOnce(patchResponse({ last_seen_display: "absolute", revision: 3 }));
    await renderLoaded();

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
      expect((storeMocks.state.userSettings as UserSettingsState).lastSeenDisplay).toBe("absolute"),
    );
  });

  it("settles two consecutive failures on the server-confirmed baseline", async () => {
    settingsApiMocks.updateUserSettings.mockRejectedValue(new ApiError("boom", 400, null));
    await renderLoaded();

    await chooseDisplay(RELATIVE_OPTION); // A
    await chooseDisplay(ABSOLUTE_OPTION); // B (queued behind A)

    // Both fail: the last operation clears the override and the cell renders
    // the server-confirmed baseline (absolute), not the pre-B optimistic value.
    await waitFor(() => expect(toastMocks.error).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(screen.queryByTestId(RELATIVE_TEST_ID)).toBeNull());
    expect(screen.getByText(formatDateTime(new Date(LAST_SEEN_AT)))).toBeTruthy();
  });
});

describe("Last seen display write and lifecycle races", () => {
  it("discards a deferred PATCH response older than a newer WS snapshot", async () => {
    const { promise: pending, resolve: resolvePatch } = deferredPatch();
    settingsApiMocks.updateUserSettings.mockReturnValue(pending);
    await renderLoaded();

    await chooseDisplay(RELATIVE_OPTION); // write in flight (resolves at rev 2)

    // A newer WS snapshot (foreign change at rev 5) lands first.
    act(() => {
      (storeMocks.state.setUserSettings as (s: UserSettingsState) => void)(
        makeUserSettings({ lastSeenDisplay: "absolute", revision: 5 }),
      );
    });

    // The deferred PATCH response (rev 2) is discarded by the revision guard.
    await act(async () => {
      resolvePatch(patchResponse({ last_seen_display: "relative", revision: 2 }));
    });
    await waitFor(() => expect(storeMocks.setUserSettings).toHaveBeenCalled());
    const settled = storeMocks.state.userSettings as UserSettingsState;
    expect(settled.revision).toBe(5);
    expect(settled.lastSeenDisplay).toBe("absolute");
  });

  it("converges the store when the component unmounts mid-write and remounts", async () => {
    const { promise: pending, resolve: resolvePatch } = deferredPatch();
    settingsApiMocks.updateUserSettings.mockReturnValue(pending);

    const first = render(
      <TooltipProvider>
        <SecuritySettings />
      </TooltipProvider>,
    );
    await screen.findByTestId(SELECT_TEST_ID);

    await chooseDisplay(RELATIVE_OPTION);
    await waitFor(() => expect(screen.getByTestId(RELATIVE_TEST_ID)).toBeTruthy());

    // Unmount mid-write; the write still settles and the store converges.
    first.unmount();
    await act(async () => {
      resolvePatch(patchResponse({ last_seen_display: "relative", revision: 2 }));
    });
    await waitFor(() => expect(storeMocks.setUserSettings).toHaveBeenCalled());
    expect((storeMocks.state.userSettings as UserSettingsState).lastSeenDisplay).toBe("relative");

    // Remount renders the confirmed value.
    const second = render(
      <TooltipProvider>
        <SecuritySettings />
      </TooltipProvider>,
    );
    await screen.findByTestId(RELATIVE_TEST_ID);
    second.unmount();
  });

  it("advances the relative label as time passes while the page stays open", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-15T15:45:00Z"));
    setBaseline({ lastSeenDisplay: "relative" });
    render(
      <TooltipProvider>
        <SecuritySettings />
      </TooltipProvider>,
    );
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    const before = screen.getByTestId(RELATIVE_TEST_ID).textContent;
    act(() => {
      vi.advanceTimersByTime(60_000);
    });
    const after = screen.getByTestId(RELATIVE_TEST_ID).textContent;
    expect(after).not.toBe(before);
  });
});

describe("Last seen display discovery", () => {
  it("registers the sessions and last-seen-display targets", async () => {
    const reveal = vi.fn();
    authApiMocks.listSessions.mockResolvedValue({ sessions: [SESSION] });
    render(
      <SettingsTargetProvider revealTarget={reveal}>
        <TooltipProvider>
          <SecuritySettings />
        </TooltipProvider>
      </SettingsTargetProvider>,
    );
    await screen.findByTestId(SELECT_TEST_ID);

    emitSettingsTargetRequest("setting-account-sessions");
    await waitFor(() => expect(reveal).toHaveBeenCalledTimes(1));

    emitSettingsTargetRequest("setting-account-last-seen-display");
    await waitFor(() => expect(reveal).toHaveBeenCalledTimes(2));
  });
});
