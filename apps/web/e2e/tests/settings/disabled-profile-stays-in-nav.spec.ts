import { test, expect } from "../../fixtures/test-base";

// Covers the post-removal contract from docs/specs/ui/remove-hide-disabled-nav-options.md:
// a disabled agent profile always appears in the Settings left panel's Agents tree with
// its Disabled badge — the tree no longer filters on the profile's enabled state. The
// "Hide disabled agent profiles from left panel navigation" setting is gone.
test.describe("disabled agent profile stays in the Settings Agents tree", () => {
  test("a disabled profile remains listed with its Disabled badge", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(120_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const profile = agent.profiles[0];
    // The tree's profile leaf is labelled with the profile name and appends the
    // "Disabled" badge while the profile is disabled — an unanchored regex
    // matches both states.
    const profileLink = new RegExp(escapeRegExp(profile.name));

    try {
      // Disable the seeded profile via the API. The profile editor toggle
      // itself is covered by agent-profile-disable.spec.ts.
      await apiClient.updateAgentProfile(profile.id, { enabled: false });

      // The per-profile Agents tree only renders in a tree menu mode. Accordion
      // opens the branch whose page owns the current route, so /settings/agents
      // shows the profile leaves. Seeded before every navigation (the page
      // boots the mode from localStorage on the first render).
      await testPage.addInitScript(() => {
        // getLocalStorage JSON-parses the value, so the mode must be stored
        // the way setLocalStorage would write it.
        window.localStorage.setItem("kandev.settings.menuMode", JSON.stringify("accordion"));
      });

      await testPage.goto("/settings/agents");
      const settingsTree = testPage.getByTestId("app-sidebar-settings-mode");
      // Accordion opens the route-owned Agents row; the agent node under it
      // (Mock) has no page of its own, so expand it to expose the profile
      // leaves.
      await settingsTree.getByRole("button", { name: "Expand Mock" }).click();
      const disabledLink = settingsTree.getByRole("link", { name: profileLink });
      await expect(disabledLink).toBeVisible({ timeout: 15_000 });
      // The profile is listed, not hidden — the tree has to say why it looks
      // inert, so the Disabled badge is part of the row.
      await expect(disabledLink).toContainText("Disabled");
    } finally {
      // Always restore so worker-scoped seedData stays valid for later tests.
      await apiClient.updateAgentProfile(profile.id, { enabled: true }).catch(() => {});
    }
  });
});

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
