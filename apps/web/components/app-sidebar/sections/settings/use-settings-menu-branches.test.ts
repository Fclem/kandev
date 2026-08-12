import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { SettingsMenuNode } from "./settings-menu-branches";

const WORKSPACE_ID = "ws-1";

const state = {
  workspaces: {
    activeId: WORKSPACE_ID,
    items: [{ id: WORKSPACE_ID, name: "Main Workspace" }],
  },
  settingsAgents: { items: [] as unknown[] },
  executors: { items: [] as unknown[] },
  agentDiscovery: { items: [] as unknown[], loaded: false },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

import { useSettingsMenuBranches } from "./use-settings-menu-branches";

const WORKSPACES_HREF = "/settings/workspaces";

/** The integration slugs the Workspaces branch currently lists. */
function listedIntegrations(): Array<string | undefined> {
  const { result } = renderHook(() => useSettingsMenuBranches("accordion"));
  const workspace = result.current[WORKSPACES_HREF]?.children?.[0];
  const integrations = (workspace?.children ?? ([] as SettingsMenuNode[])).find((node) =>
    node.href?.endsWith("/integrations"),
  );
  return (integrations?.children ?? []).map((node) => node.integrationSlug);
}

describe("useSettingsMenuBranches integration listing", () => {
  it("lists every integration in a tree mode — the branch has no enabled filter", () => {
    expect(listedIntegrations()).toEqual([
      "azure-devops",
      "github",
      "gitlab",
      "jira",
      "linear",
      "sentry",
    ]);
  });

  it("leaves the flat menu without branches at all", () => {
    const { result } = renderHook(() => useSettingsMenuBranches("flat"));

    expect(result.current).toEqual({});
  });
});
