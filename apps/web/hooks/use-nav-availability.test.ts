import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { getGitHubIntegrationStatus } from "./use-nav-availability";
import type { GitHubStatus } from "@/lib/types/github";
import type { AvailabilityKey } from "@/lib/navigation/types";

const mocks = vi.hoisted(() => {
  const workspaceId = "workspace-1";
  return {
    workspaceId,
    state: {
      workspaces: {
        activeId: workspaceId as string | null,
        items: [{ id: workspaceId }],
      },
    },
    azureDevOpsAvailable: vi.fn(() => false),
    githubStatus: vi.fn(() => ({ status: null as GitHubStatus | null, loading: false })),
    gitlabAvailable: vi.fn(() => false),
    jiraAuthed: vi.fn(() => false),
    linearAuthed: vi.fn(() => false),
  };
});

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));
vi.mock("@/hooks/domains/azure-devops/use-azure-devops-availability", () => ({
  useAzureDevOpsAvailable: mocks.azureDevOpsAvailable,
}));
vi.mock("@/hooks/domains/github/use-github-status", () => ({
  useGitHubStatus: mocks.githubStatus,
}));
vi.mock("@/hooks/domains/gitlab/use-task-mr", () => ({
  useGitLabAvailable: mocks.gitlabAvailable,
}));
vi.mock("@/hooks/domains/jira/use-jira-availability", () => ({
  useJiraAuthed: mocks.jiraAuthed,
}));
vi.mock("@/hooks/domains/linear/use-linear-availability", () => ({
  useLinearAuthed: mocks.linearAuthed,
}));

import { useNavAvailability } from "./use-nav-availability";

function status(overrides: Partial<GitHubStatus>): GitHubStatus {
  return {
    authenticated: false,
    username: "",
    auth_method: "none",
    token_configured: false,
    required_scopes: [],
    ...overrides,
  };
}

describe("getGitHubIntegrationStatus", () => {
  it("shows checking while GitHub status is loading and not configured", () => {
    expect(getGitHubIntegrationStatus(null, true)).toEqual({ ready: false, label: "Checking" });
  });

  it("treats a configured token as ready even before live auth is green", () => {
    expect(getGitHubIntegrationStatus(status({ token_configured: true }), false)).toEqual({
      ready: true,
      label: "Configured",
    });
  });

  it("uses the Connected label for authenticated status", () => {
    expect(getGitHubIntegrationStatus(status({ authenticated: true }), false)).toEqual({
      ready: true,
      label: "Connected",
    });
  });

  it("shows setup only when no auth or token is configured", () => {
    expect(getGitHubIntegrationStatus(status({}), false)).toEqual({
      ready: false,
      label: "Setup",
    });
  });
});

const NAV_GATED_KEYS: AvailabilityKey[] = ["azure-devops", "github", "gitlab", "jira", "linear"];

function setConfigured(key: AvailabilityKey, configured: boolean) {
  switch (key) {
    case "azure-devops":
      mocks.azureDevOpsAvailable.mockReturnValue(configured);
      return;
    case "github":
      mocks.githubStatus.mockReturnValue({
        status: configured ? status({ authenticated: true }) : status({}),
        loading: false,
      });
      return;
    case "gitlab":
      mocks.gitlabAvailable.mockReturnValue(configured);
      return;
    case "jira":
      mocks.jiraAuthed.mockReturnValue(configured);
      return;
    case "linear":
      mocks.linearAuthed.mockReturnValue(configured);
      return;
  }
}

describe("useNavAvailability", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.state.workspaces.activeId = mocks.workspaceId;
    mocks.state.workspaces.items = [{ id: mocks.workspaceId }];
    mocks.githubStatus.mockReturnValue({ status: null, loading: false });
  });

  it("scopes workspace integrations to the active workspace", () => {
    renderHook(() => useNavAvailability());

    expect(mocks.jiraAuthed).toHaveBeenCalledWith(mocks.workspaceId);
    expect(mocks.linearAuthed).toHaveBeenCalledWith(mocks.workspaceId);
    expect(mocks.azureDevOpsAvailable).toHaveBeenCalledWith(mocks.workspaceId);
  });

  it("falls back to default workspace resolution for a stale active id", () => {
    mocks.state.workspaces.items = [{ id: "workspace-2" }];

    renderHook(() => useNavAvailability());

    expect(mocks.jiraAuthed).toHaveBeenCalledWith(null);
    expect(mocks.linearAuthed).toHaveBeenCalledWith(null);
    expect(mocks.azureDevOpsAvailable).toHaveBeenCalledWith(null);
  });

  describe.each(NAV_GATED_KEYS)("gates %s nav visibility on configuration only", (key) => {
    it("lists a configured integration regardless of its enabled toggle", () => {
      setConfigured(key, true);

      const { result } = renderHook(() => useNavAvailability());

      expect(result.current[key]).toBe(true);
    });

    it("hides an unconfigured integration even when it is enabled", () => {
      setConfigured(key, false);

      const { result } = renderHook(() => useNavAvailability());

      expect(result.current[key]).toBe(false);
    });
  });
});
