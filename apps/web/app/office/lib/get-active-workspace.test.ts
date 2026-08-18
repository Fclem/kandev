import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("@/lib/server/cookies", () => ({
  readCookies: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  listWorkspaces: vi.fn(),
  fetchUserSettings: vi.fn(),
}));

vi.mock("@/lib/api/domains/settings-api", () => ({
  updateUserSettings: vi.fn(),
}));

// The scoped cookie helpers derive the default port from the API base URL;
// pin it so scoped-name fixtures are deterministic.
vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://localhost:8443" }),
}));

import { readCookies } from "@/lib/server/cookies";
import { listWorkspaces, fetchUserSettings } from "@/lib/api";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import { getActiveWorkspaceId } from "./get-active-workspace";

const mockReadCookies = vi.mocked(readCookies);
const mockListWorkspaces = vi.mocked(listWorkspaces);
const mockFetchUserSettings = vi.mocked(fetchUserSettings);
const mockUpdateUserSettings = vi.mocked(updateUserSettings);

const OFFICE_WS_ID = "ws-office-1";
const OFFICE_WS_ID_2 = "ws-office-2";
const KANBAN_WS_ID = "ws-kanban-1";
const OFFICE_WS = { id: OFFICE_WS_ID, name: "Office", office_workflow_id: "wf-1" };
const OFFICE_WS_2 = { id: OFFICE_WS_ID_2, name: "Office 2", office_workflow_id: "wf-2" };
const KANBAN_WS = { id: KANBAN_WS_ID, name: "Kanban" }; // no office_workflow_id

function makeSettings(workspaceId: string) {
  return { settings: { workspace_id: workspaceId } };
}

// CookieStore mock keyed by exact cookie name (scoped or legacy).
function makeCookieStore(entries: Record<string, string>) {
  return {
    get: (name: string) => (name in entries ? { value: entries[name] } : undefined),
  };
}

beforeEach(() => {
  mockReadCookies.mockReset();
  mockListWorkspaces.mockReset();
  mockFetchUserSettings.mockReset();
  mockUpdateUserSettings.mockReset();
  mockUpdateUserSettings.mockResolvedValue({} as never);
  // Default: no cookie set
  mockReadCookies.mockResolvedValue(makeCookieStore({}) as never);
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("getActiveWorkspaceId", () => {
  describe("URL workspaceId arg wins when it matches an office workspace", () => {
    it("returns the URL workspace id and does NOT call updateUserSettings", async () => {
      mockListWorkspaces.mockResolvedValue({ workspaces: [OFFICE_WS] } as never);
      mockFetchUserSettings.mockResolvedValue(makeSettings("ws-other-1") as never);

      const result = await getActiveWorkspaceId(OFFICE_WS_ID);

      expect(result).toBe(OFFICE_WS_ID);
      expect(mockUpdateUserSettings).not.toHaveBeenCalled();
    });

    it("ignores the URL param when it does not match any office workspace", async () => {
      mockListWorkspaces.mockResolvedValue({ workspaces: [OFFICE_WS] } as never);
      mockFetchUserSettings.mockResolvedValue(makeSettings(OFFICE_WS_ID) as never);

      // "ws-unknown" not in office list - falls back to settings match
      const result = await getActiveWorkspaceId("ws-unknown-1");

      expect(result).toBe(OFFICE_WS_ID);
      expect(mockUpdateUserSettings).not.toHaveBeenCalled();
    });
  });

  describe("cookie wins after URL param misses", () => {
    it("returns the cookie workspace and does NOT call updateUserSettings", async () => {
      mockReadCookies.mockResolvedValue(
        makeCookieStore({ "office-active-workspace": OFFICE_WS_ID_2 }) as never,
      );
      mockListWorkspaces.mockResolvedValue({ workspaces: [OFFICE_WS, OFFICE_WS_2] } as never);
      mockFetchUserSettings.mockResolvedValue(makeSettings(KANBAN_WS_ID) as never);

      const result = await getActiveWorkspaceId();

      expect(result).toBe(OFFICE_WS_ID_2);
      expect(mockUpdateUserSettings).not.toHaveBeenCalled();
    });

    it("ignores cookie when it does not match any office workspace", async () => {
      mockReadCookies.mockResolvedValue(
        makeCookieStore({ "office-active-workspace": "ws-stale" }) as never,
      );
      mockListWorkspaces.mockResolvedValue({ workspaces: [OFFICE_WS] } as never);
      mockFetchUserSettings.mockResolvedValue(makeSettings(OFFICE_WS_ID) as never);

      const result = await getActiveWorkspaceId();

      expect(result).toBe(OFFICE_WS_ID);
      expect(mockUpdateUserSettings).not.toHaveBeenCalled();
    });
  });

  describe("regression: kanban workspace in settings must not trigger updateUserSettings", () => {
    it("falls back to first office workspace and does NOT call updateUserSettings", async () => {
      mockListWorkspaces.mockResolvedValue({ workspaces: [KANBAN_WS, OFFICE_WS] } as never);
      mockFetchUserSettings.mockResolvedValue(makeSettings(KANBAN_WS_ID) as never);

      const result = await getActiveWorkspaceId();

      expect(result).toBe(OFFICE_WS_ID);
      expect(mockUpdateUserSettings).not.toHaveBeenCalled();
    });
  });

  describe("settings.workspace_id points to a valid office workspace", () => {
    it("returns the settings workspace id and does NOT call updateUserSettings", async () => {
      mockListWorkspaces.mockResolvedValue({ workspaces: [OFFICE_WS] } as never);
      mockFetchUserSettings.mockResolvedValue(makeSettings(OFFICE_WS_ID) as never);

      const result = await getActiveWorkspaceId();

      expect(result).toBe(OFFICE_WS_ID);
      expect(mockUpdateUserSettings).not.toHaveBeenCalled();
    });
  });

  describe("no office workspaces exist", () => {
    it("returns null and does not call updateUserSettings", async () => {
      mockListWorkspaces.mockResolvedValue({ workspaces: [] } as never);
      mockFetchUserSettings.mockResolvedValue(makeSettings(OFFICE_WS_ID) as never);

      const result = await getActiveWorkspaceId();

      expect(result).toBeNull();
      expect(mockUpdateUserSettings).not.toHaveBeenCalled();
    });
  });
});

describe("getActiveWorkspaceId — port-scoped cookie reads", () => {
  it("prefers the scoped office cookie over the legacy name", async () => {
    mockReadCookies.mockResolvedValue(
      makeCookieStore({
        "office-active-workspace": OFFICE_WS_ID,
        "office-active-workspace_8443": OFFICE_WS_ID_2,
      }) as never,
    );
    mockListWorkspaces.mockResolvedValue({ workspaces: [OFFICE_WS, OFFICE_WS_2] } as never);
    mockFetchUserSettings.mockResolvedValue(makeSettings(KANBAN_WS_ID) as never);

    const result = await getActiveWorkspaceId();

    expect(result).toBe(OFFICE_WS_ID_2);
    expect(mockUpdateUserSettings).not.toHaveBeenCalled();
  });

  it("prefers the scoped general cookie over the office cookie when it names an office workspace", async () => {
    mockReadCookies.mockResolvedValue(
      makeCookieStore({
        "kandev-active-workspace_8443": OFFICE_WS_ID,
        "office-active-workspace_8443": OFFICE_WS_ID_2,
      }) as never,
    );
    mockListWorkspaces.mockResolvedValue({ workspaces: [OFFICE_WS, OFFICE_WS_2] } as never);
    mockFetchUserSettings.mockResolvedValue(makeSettings(KANBAN_WS_ID) as never);

    const result = await getActiveWorkspaceId();

    expect(result).toBe(OFFICE_WS_ID);
    expect(mockUpdateUserSettings).not.toHaveBeenCalled();
  });

  it("falls through a kanban general cookie to the scoped office cookie", async () => {
    mockReadCookies.mockResolvedValue(
      makeCookieStore({
        "kandev-active-workspace_8443": KANBAN_WS_ID,
        "office-active-workspace_8443": OFFICE_WS_ID_2,
      }) as never,
    );
    mockListWorkspaces.mockResolvedValue({ workspaces: [OFFICE_WS, OFFICE_WS_2] } as never);
    mockFetchUserSettings.mockResolvedValue(makeSettings(KANBAN_WS_ID) as never);

    const result = await getActiveWorkspaceId();

    expect(result).toBe(OFFICE_WS_ID_2);
    expect(mockUpdateUserSettings).not.toHaveBeenCalled();
  });

  it("ignores scoped cookies when the office feature list does not contain them", async () => {
    mockReadCookies.mockResolvedValue(
      makeCookieStore({ "office-active-workspace_8443": "ws-stale" }) as never,
    );
    mockListWorkspaces.mockResolvedValue({ workspaces: [OFFICE_WS] } as never);
    mockFetchUserSettings.mockResolvedValue(makeSettings(OFFICE_WS_ID) as never);

    const result = await getActiveWorkspaceId();

    expect(result).toBe(OFFICE_WS_ID);
    expect(mockUpdateUserSettings).not.toHaveBeenCalled();
  });
});
