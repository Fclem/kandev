import { test, expect } from "../../fixtures/test-base";

// Covers docs/specs/agents/hide-disabled-profiles-nav.md's nav-visibility
// scenarios: with "Hide disabled agent profiles from left panel navigation"
// off (the default), a disabled profile still shows in the Settings left
// panel's Agents tree; turning the setting on hides it; re-enabling the
// profile reveals it again — all without a reload.
test.describe("hide disabled agent profiles from left panel navigation", () => {
  test("off by default keeps a disabled profile visible; on hides it; re-enabling reveals it", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(120_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const profile = agent.profiles[0];
    const agentLabel = profile.agentDisplayName || agent.name;
    // The tree renders `${agentLabel} • ${profile.name}` and appends the
    // "Disabled" badge suffix while the profile is disabled — an unanchored
    // regex matches both states.
    const profileLink = new RegExp(`${escapeRegExp(agentLabel)} • ${escapeRegExp(profile.name)}`);

    try {
      // Disable the seeded profile via the API. The settings-list toggle
      // itself is covered by agent-profile-disable.spec.ts.
      await apiClient.updateAgentProfile(profile.id, { enabled: false });

      await testPage.goto("/settings/agents");
      const settingsTree = testPage.getByTestId("app-sidebar-settings-mode");
      const disabledLink = settingsTree.getByRole("link", { name: profileLink });
      await expect(disabledLink).toBeVisible({ timeout: 15_000 });

      // The setting is off by default.
      const hideDisabledSwitch = testPage.locator("#hide-disabled-agent-profiles-in-nav");
      await expect(hideDisabledSwitch).toHaveAttribute("aria-checked", "false");

      // Turn "hide disabled" on — the tree entry disappears immediately,
      // while the Agents group header stays.
      await hideDisabledSwitch.click();
      await expect(hideDisabledSwitch).toHaveAttribute("aria-checked", "true");
      await expect(disabledLink).not.toBeVisible();
      await expect(settingsTree.getByRole("link", { name: "Agents", exact: true })).toBeVisible();

      // The settings page itself still lists the disabled profile.
      const rowToggle = testPage.getByTestId(`profile-enabled-toggle-${profile.id}`);
      await expect(rowToggle).toHaveAttribute("data-state", "unchecked", { timeout: 15_000 });

      // Re-enabling from the list reveals the profile again even with the
      // setting still on (no reload).
      await rowToggle.click();
      await expect(rowToggle).toHaveAttribute("data-state", "checked");
      await expect(disabledLink).toBeVisible({ timeout: 15_000 });
    } finally {
      // Always restore so worker-scoped seedData stays valid for later tests.
      await apiClient.updateAgentProfile(profile.id, { enabled: true }).catch(() => {});
    }
  });
});

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
