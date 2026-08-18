import { beforeEach, describe, expect, it, vi } from "vitest";
import fs from "node:fs";
import path from "node:path";

// The default cookie port derives from the API base URL. Mock it to a fixed
// port so scoped-name assertions are deterministic and independent of the
// jsdom default origin (http://localhost:3000).
vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://localhost:8443" }),
}));

const GENERAL_COOKIE = "kandev-active-workspace";
const OFFICE_COOKIE = "office-active-workspace";

import {
  mapWorkspaceItem,
  readActiveWorkspaceCookie,
  readCookie,
  readScopedCookie,
  readScopedCookieStoreValue,
  resolveOfficeWorkspaceId,
  resolveSettingsActiveWorkspaceId,
  scopedCookieName,
} from "./route-bootstrap";
import type { ListWorkspacesResponse } from "@/lib/types/http";

type WorkspaceItem = ListWorkspacesResponse["workspaces"][number];

beforeEach(() => {
  document.cookie = `${GENERAL_COOKIE}=; path=/; max-age=0`;
  document.cookie = `${OFFICE_COOKIE}=; path=/; max-age=0`;
  document.cookie = `${GENERAL_COOKIE}_8443=; path=/; max-age=0`;
  document.cookie = `${OFFICE_COOKIE}_8443=; path=/; max-age=0`;
});

describe("mapWorkspaceItem", () => {
  it("normalizes optional workspace fields for store hydration", () => {
    expect(
      mapWorkspaceItem({
        id: "ws-1",
        name: "Workspace",
        owner_id: "owner-1",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-02T00:00:00Z",
      } as WorkspaceItem),
    ).toEqual({
      id: "ws-1",
      name: "Workspace",
      description: null,
      owner_id: "owner-1",
      default_executor_id: null,
      default_environment_id: null,
      default_agent_profile_id: null,
      default_config_agent_profile_id: null,
      office_workflow_id: null,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-02T00:00:00Z",
    });
  });
});

describe("readCookie", () => {
  it("reads encoded cookie values by encoded cookie name", () => {
    document.cookie = `${encodeURIComponent("office-active-workspace")}=${encodeURIComponent("ws 1/2")}`;

    expect(readCookie("office-active-workspace")).toBe("ws 1/2");
    expect(readCookie("missing")).toBeNull();
  });

  it("prefers the general active workspace cookie over the legacy office cookie", () => {
    document.cookie = "office-active-workspace=office-1; path=/";
    document.cookie = "kandev-active-workspace=kanban-1; path=/";

    expect(readActiveWorkspaceCookie()).toBe("kanban-1");
  });

  it("does not use the legacy office cookie as the generic active workspace", () => {
    document.cookie = "office-active-workspace=office-1; path=/";

    expect(readActiveWorkspaceCookie()).toBeNull();
  });
});

describe("resolveSettingsActiveWorkspaceId", () => {
  const OFFICE = { id: "office-1", office_workflow_id: "office-workflow" };
  const OFFICE_TWO = { id: "office-2", office_workflow_id: "office-workflow-2" };
  const KANBAN = { id: "kanban-1", office_workflow_id: null };
  const KANBAN_TWO = { id: "kanban-2", office_workflow_id: null };

  it("prefers the active workspace cookie whatever type it names", () => {
    // Previously this resolver filtered office workspaces out. That was
    // invisible while chrome came from the pathname; now that chrome follows
    // the active workspace, filtering here would switch an Office user's
    // workspace as a side effect of opening Settings.
    expect(resolveSettingsActiveWorkspaceId([KANBAN, OFFICE], OFFICE.id, null)).toBe(OFFICE.id);
    expect(resolveSettingsActiveWorkspaceId([OFFICE, KANBAN], KANBAN.id, null)).toBe(KANBAN.id);
  });

  it("falls back to the stored preference when no cookie matches", () => {
    expect(resolveSettingsActiveWorkspaceId([OFFICE, KANBAN], "ws-missing", OFFICE.id)).toBe(
      OFFICE.id,
    );
  });

  it("falls back to the first workspace when nothing is named", () => {
    expect(resolveSettingsActiveWorkspaceId([OFFICE, KANBAN_TWO], null, null)).toBe(OFFICE.id);
    expect(resolveSettingsActiveWorkspaceId([KANBAN, OFFICE_TWO], null, null)).toBe(KANBAN.id);
  });

  it("returns null when no workspaces exist", () => {
    expect(resolveSettingsActiveWorkspaceId([], "k-1", "k-2")).toBeNull();
  });
});

describe("scopedCookieName", () => {
  it("appends the port suffix when a port is given", () => {
    expect(scopedCookieName(GENERAL_COOKIE, "8443")).toBe("kandev-active-workspace_8443");
    expect(scopedCookieName(OFFICE_COOKIE, "8443")).toBe("office-active-workspace_8443");
  });

  it("keeps the plain name when the port is empty", () => {
    expect(scopedCookieName(GENERAL_COOKIE, "")).toBe("kandev-active-workspace");
  });

  it("derives the default port from the API base URL, not window.location.port", () => {
    // Split-origin dev regression: the SPA runs on the web port and the API on
    // the backend port (no Vite proxy). The cookie suffix must come from the
    // API origin (here mocked to :8443), never from the browser location
    // (jsdom default :3000), or the backend would read a different name.
    expect(window.location.port).toBe("3000");
    expect(scopedCookieName(GENERAL_COOKIE)).toBe("kandev-active-workspace_8443");
  });
});

describe("readScopedCookie", () => {
  it("reads the port-scoped cookie first", () => {
    document.cookie = `${GENERAL_COOKIE}_8443=scoped-1; path=/`;

    expect(readScopedCookie(GENERAL_COOKIE, "8443")).toBe("scoped-1");
  });

  it("falls back to the legacy unprefixed name when no scoped cookie exists", () => {
    document.cookie = `${GENERAL_COOKIE}=legacy-1; path=/`;

    expect(readScopedCookie(GENERAL_COOKIE, "8443")).toBe("legacy-1");
  });

  it("returns null when neither name exists", () => {
    expect(readScopedCookie(GENERAL_COOKIE, "8443")).toBeNull();
  });

  it("keeps the plain-name read for a default-port (no suffix) instance", () => {
    document.cookie = `${GENERAL_COOKIE}=legacy-1; path=/`;

    expect(readScopedCookie(GENERAL_COOKIE, "")).toBe("legacy-1");
  });

  it("uses the API-origin default port for the active-workspace read", () => {
    document.cookie = `${GENERAL_COOKIE}_8443=scoped-1; path=/`;
    document.cookie = `${GENERAL_COOKIE}=legacy-1; path=/`;

    expect(readActiveWorkspaceCookie()).toBe("scoped-1");
  });
});

describe("readScopedCookieStoreValue", () => {
  const scopedStore = {
    get: (name: string) => {
      if (name === `${GENERAL_COOKIE}_8443`) return { value: "scoped-1" };
      if (name === GENERAL_COOKIE) return { value: "legacy-1" };
      return undefined;
    },
  };
  const legacyStore = {
    get: (name: string) => (name === GENERAL_COOKIE ? { value: "legacy-1" } : undefined),
  };

  it("reads the scoped cookieStore entry first", () => {
    expect(readScopedCookieStoreValue(scopedStore, GENERAL_COOKIE, "8443")).toBe("scoped-1");
  });

  it("falls back to the legacy entry when no scoped one exists", () => {
    expect(readScopedCookieStoreValue(legacyStore, GENERAL_COOKIE, "8443")).toBe("legacy-1");
  });

  it("returns null for a null cookieStore (readCookies failure)", () => {
    expect(readScopedCookieStoreValue(null, GENERAL_COOKIE)).toBeNull();
  });
});

describe("resolveOfficeWorkspaceId", () => {
  const OFFICE = { id: "office-1", office_workflow_id: "wf-1" };
  const OFFICE_TWO = { id: "office-2", office_workflow_id: "wf-2" };
  const KANBAN = { id: "kanban-1", office_workflow_id: null };
  const items = [OFFICE, OFFICE_TWO];

  it("prefers the route override over every cookie candidate", () => {
    expect(
      resolveOfficeWorkspaceId(items, {
        routeWorkspaceId: OFFICE_TWO.id,
        generalWorkspaceId: OFFICE.id,
        officeWorkspaceId: OFFICE.id,
        settingsWorkspaceId: OFFICE.id,
      }),
    ).toBe(OFFICE_TWO.id);
  });

  it("prefers the general cookie over the office cookie when it names an office workspace", () => {
    expect(
      resolveOfficeWorkspaceId(items, {
        routeWorkspaceId: null,
        generalWorkspaceId: OFFICE.id,
        officeWorkspaceId: OFFICE_TWO.id,
        settingsWorkspaceId: null,
      }),
    ).toBe(OFFICE.id);
  });

  it("falls through a kanban general cookie to the office cookie", () => {
    expect(
      resolveOfficeWorkspaceId(items, {
        routeWorkspaceId: null,
        generalWorkspaceId: KANBAN.id,
        officeWorkspaceId: OFFICE_TWO.id,
        settingsWorkspaceId: null,
      }),
    ).toBe(OFFICE_TWO.id);
  });

  it("prefers the office cookie over settings", () => {
    expect(
      resolveOfficeWorkspaceId(items, {
        routeWorkspaceId: null,
        generalWorkspaceId: null,
        officeWorkspaceId: OFFICE.id,
        settingsWorkspaceId: OFFICE_TWO.id,
      }),
    ).toBe(OFFICE.id);
  });

  it("falls back to settings, then the first workspace, then null", () => {
    expect(
      resolveOfficeWorkspaceId(items, {
        routeWorkspaceId: null,
        generalWorkspaceId: "ws-missing",
        officeWorkspaceId: null,
        settingsWorkspaceId: OFFICE_TWO.id,
      }),
    ).toBe(OFFICE_TWO.id);
    expect(
      resolveOfficeWorkspaceId(items, {
        routeWorkspaceId: null,
        generalWorkspaceId: "ws-missing",
        officeWorkspaceId: null,
        settingsWorkspaceId: null,
      }),
    ).toBe(OFFICE.id);
    expect(resolveOfficeWorkspaceId([], { routeWorkspaceId: "office-1" })).toBeNull();
  });
});

describe("structural guard: workspace-cookie lookups stay scoped", () => {
  const here = path.dirname(new URL(import.meta.url).pathname);
  const read = (file: string) => fs.readFileSync(path.resolve(here, "..", "..", file), "utf8");

  const cookieStoreReaders = [
    "app/page.tsx",
    "app/settings/layout.tsx",
    "app/office/layout.tsx",
    "app/office/lib/get-active-workspace.ts",
  ];
  const allConsumers = [...cookieStoreReaders, "src/office-routes.tsx"];

  const workspaceCookieNames = `(ACTIVE_WORKSPACE_COOKIE|LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE|"${GENERAL_COOKIE}"|"${OFFICE_COOKIE}")`;

  it("negative: no consumer performs a direct workspace-cookie lookup", () => {
    for (const file of allConsumers) {
      const source = read(file);
      // Direct cookieStore.get(...) or readCookie(...) with a workspace-cookie
      // name (literal or exported constant) is forbidden — the scoped helpers
      // are the only allowed read path.
      const direct = new RegExp(`(cookieStore\\.get|readCookie)\\s*\\(\\s*${workspaceCookieNames}`);
      expect(direct.test(source), `${file} must not read workspace cookies directly`).toBe(false);
    }
  });

  it("positive: each reader references its mandated scoped helper", () => {
    for (const file of cookieStoreReaders) {
      expect(
        read(file).includes("readScopedCookieStoreValue"),
        `${file} must read via readScopedCookieStoreValue`,
      ).toBe(true);
    }
    expect(read("src/office-routes.tsx").includes("readScopedCookie")).toBe(true);
  });

  it("passes the correct cookie names per reader", () => {
    // Whitespace- and trailing-comma-agnostic match: formatting must not
    // break the contract check.
    const compact = (file: string) => read(file).replace(/\s+/g, "").replace(/,/g, "");
    // Kanban readers pass only the general constant.
    for (const file of ["app/page.tsx", "app/settings/layout.tsx"]) {
      const source = compact(file);
      expect(
        source.includes("readScopedCookieStoreValue(cookieStoreACTIVE_WORKSPACE_COOKIE)"),
        `${file} must read the general cookie via the constant`,
      ).toBe(true);
      expect(source.includes("LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE")).toBe(false);
    }
    // Office readers pass both constants, general first.
    for (const file of ["app/office/layout.tsx", "app/office/lib/get-active-workspace.ts"]) {
      const source = compact(file);
      const general = source.indexOf(
        "readScopedCookieStoreValue(cookieStoreACTIVE_WORKSPACE_COOKIE)",
      );
      const office = source.indexOf(
        "readScopedCookieStoreValue(cookieStoreLEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE)",
      );
      expect(general, `${file} must reference the general constant`).toBeGreaterThan(-1);
      expect(office, `${file} must reference the office constant`).toBeGreaterThan(-1);
      expect(general, `${file} must read the general family before the office family`).toBeLessThan(
        office,
      );
    }
    // The sync office effect reads its office candidate via readScopedCookie.
    expect(
      compact("src/office-routes.tsx").includes(
        "readScopedCookie(LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE)",
      ),
    ).toBe(true);
  });
});
