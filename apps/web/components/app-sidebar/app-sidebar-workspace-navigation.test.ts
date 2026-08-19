import { beforeEach, describe, expect, it, vi } from "vitest";
import { LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE } from "@/lib/routing/route-bootstrap";

// The write path scopes names with the API-origin port; pin it so the
// assertions are deterministic.
vi.mock("@/lib/config", () => ({
  getBackendConfig: vi.fn(() => ({ apiBaseUrl: "http://localhost:8443" })),
}));

import { getBackendConfig } from "@/lib/config";

import {
  rememberWorkspaceSelection,
  rememberWorkspaceSelectionById,
  workspaceHomeHref,
} from "./app-sidebar-workspace-navigation";

const ACTIVE_WORKSPACE_COOKIE = "kandev-active-workspace";
const kanban = { id: "kanban-1", office_workflow_id: "" };
const office = { id: "office-1", office_workflow_id: "wf-office" };
const officeWithReservedChars = {
  id: "office/2;mode",
  office_workflow_id: "wf-office-reserved",
};

describe("app sidebar workspace navigation", () => {
  beforeEach(() => {
    document.cookie = "kandev-active-workspace=; path=/; max-age=0";
    document.cookie = "office-active-workspace=; path=/; max-age=0";
    document.cookie = "kandev-active-workspace_8443=; path=/; max-age=0";
    document.cookie = "office-active-workspace_8443=; path=/; max-age=0";
  });

  it("routes workspace home by active workspace type", () => {
    expect(workspaceHomeHref(kanban)).toBe("/?home=overview&workspaceId=kanban-1");
    expect(workspaceHomeHref(office)).toBe("/office?workspaceId=office-1");
    expect(workspaceHomeHref(undefined)).toBe("/?home=overview");
  });

  it("records the active workspace in one write under the port-scoped names", () => {
    rememberWorkspaceSelection(kanban);
    rememberWorkspaceSelection(office);

    expect(document.cookie).toContain(`${ACTIVE_WORKSPACE_COOKIE}_8443=office-1`);
    expect(document.cookie).toContain(`${LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE}_8443=office-1`);
    // The legacy unprefixed names are never written (read-only fallback only);
    // they may only appear as empty expired entries from the twin-cleanup.
    expect(document.cookie).not.toContain(`${ACTIVE_WORKSPACE_COOKIE}=office-1`);
    expect(document.cookie).not.toContain(`${LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE}=office-1`);
  });

  it("does not write the legacy office cookie for a kanban selection", () => {
    rememberWorkspaceSelection(office);
    rememberWorkspaceSelection(kanban);

    // The office boot paths read the legacy cookie to pick an office workspace
    // when the unified cookie names a kanban board, so a kanban selection must
    // leave it pointing at the office workspace last used.
    expect(document.cookie).toContain(`${ACTIVE_WORKSPACE_COOKIE}_8443=kanban-1`);
    expect(document.cookie).toContain(`${LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE}_8443=office-1`);
  });

  it("writes scoped active and office workspace cookies with an encoded id", () => {
    rememberWorkspaceSelection(officeWithReservedChars);

    expect(document.cookie).toContain(
      `${ACTIVE_WORKSPACE_COOKIE}_8443=${encodeURIComponent(officeWithReservedChars.id)}`,
    );
    expect(document.cookie).toContain(
      `${LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE}_8443=${encodeURIComponent(officeWithReservedChars.id)}`,
    );
  });

  it("records a workspace known only by id and kind", () => {
    // The setup wizard path: the create response returns an id and nothing
    // else, so there is no record to pass.
    rememberWorkspaceSelectionById("office-new", "office");

    expect(document.cookie).toContain(`${ACTIVE_WORKSPACE_COOKIE}_8443=office-new`);
    expect(document.cookie).toContain(`${LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE}_8443=office-new`);
    expect(document.cookie).not.toContain(`${ACTIVE_WORKSPACE_COOKIE}=office-new`);
    expect(document.cookie).not.toContain(`${LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE}=office-new`);
  });

  it("expires a pre-upgrade legacy cookie when the scoped twin is written", () => {
    // Pre-upgrade jar: only the unprefixed names exist.
    document.cookie = `${ACTIVE_WORKSPACE_COOKIE}=pre-upgrade; path=/`;
    document.cookie = `${LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE}=pre-upgrade; path=/`;

    rememberWorkspaceSelection(office);

    // The scoped twin is recorded and the legacy value is gone (empty
    // expired entry at most), so no other instance on the host falls back to
    // the shared pre-upgrade selection.
    expect(document.cookie).toContain(`${ACTIVE_WORKSPACE_COOKIE}_8443=office-1`);
    expect(document.cookie).toContain(`${LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE}_8443=office-1`);
    expect(document.cookie).not.toContain(`${ACTIVE_WORKSPACE_COOKIE}=pre-upgrade`);
    expect(document.cookie).not.toContain(`${LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE}=pre-upgrade`);
  });

  it("keeps the plain name when there is no port (scoped == legacy)", () => {
    vi.mocked(getBackendConfig).mockReturnValue({ apiBaseUrl: "http://localhost" });
    try {
      rememberWorkspaceSelection(kanban);
      // No-port instance: the write itself is the plain name, so the cleanup
      // must not expire what was just recorded.
      expect(document.cookie).toContain(`${ACTIVE_WORKSPACE_COOKIE}=kanban-1`);
    } finally {
      vi.mocked(getBackendConfig).mockReturnValue({ apiBaseUrl: "http://localhost:8443" });
    }
  });
});
