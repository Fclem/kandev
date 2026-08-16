import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createDefaultUserSettings } from "@/lib/ssr/user-settings";
import { compareUserSettingsRevisions } from "@/lib/settings/user-settings-revision";
import type { UserSettingsState } from "@/lib/state/slices/settings/types";
import { SettingsTargetProvider } from "@/components/settings/settings-target-provider";
import { emitSettingsTargetRequest } from "@/lib/settings-discovery/target";
import { formatDateTime } from "@/lib/i18n/formats";
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

function patchResponse(overrides: Record<string, unknown> = {}) {
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

    await chooseDisplay("Relative time");

    await waitFor(() =>
      expect(settingsApiMocks.updateUserSettings).toHaveBeenCalledWith({
        last_seen_display: "relative",
      }),
    );
  });

  it("shows the error toast on a failed save", async () => {
    settingsApiMocks.updateUserSettings.mockRejectedValue(new Error("boom"));
    await renderLoaded();

    await chooseDisplay("Relative time");

    await waitFor(() => expect(toastMocks.error).toHaveBeenCalled());
  });

  it("keeps the optimistic value while a write is in flight even when the store re-asserts the baseline", async () => {
    let resolvePatch!: (value: ReturnType<typeof patchResponse>) => void;
    const pending = new Promise<ReturnType<typeof patchResponse>>((resolve) => {
      resolvePatch = resolve;
    });
    settingsApiMocks.updateUserSettings.mockReturnValue(pending);
    await renderLoaded();

    await chooseDisplay("Relative time");
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
