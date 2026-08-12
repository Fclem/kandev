import { test, expect } from "../../fixtures/test-base";
import { SETTINGS_TAKEOVER_TESTID, setSettingsMenuMode } from "../../helpers/settings-menu";

// Covers the post-removal contract from docs/specs/ui/remove-hide-disabled-nav-options.md:
// left-panel nav visibility is configuration-only, so a disabled-but-configured
// integration stays visible in the desktop sidebar Integrations section and the
// Settings left panel's per-workspace Integrations tree. The "Hide disabled
// integrations from left panel navigation" setting is gone. The mobile-menu half
// of the contract lives in mobile-disabled-integration-stays-in-nav.spec.ts.
test.describe("disabled integration stays in left-panel navigation", () => {
  test("a disabled-but-configured integration remains visible in the sidebar and Settings tree", async ({
    testPage,
    apiClient,
  }) => {
    // Make GitHub configured/healthy so it is eligible to appear in the nav
    // regardless of its enabled state.
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");

    await testPage.goto("/settings/integrations");
    const githubSwitch = testPage.locator("#github-enabled");
    await expect(githubSwitch).toHaveAttribute("aria-checked", "true");
    await githubSwitch.click();
    await expect(githubSwitch).toHaveAttribute("aria-checked", "false");
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();
    await expect(testPage.getByTestId("settings-floating-save")).not.toBeVisible();

    // Desktop sidebar: leave Settings (the takeover replaces the sidebar) and
    // confirm the disabled-but-configured GitHub entry stays listed.
    await testPage.goto("/tasks");
    await expect(testPage.getByRole("link", { name: "GitHub", exact: true })).toBeVisible({
      timeout: 10_000,
    });

    // The Settings left panel's per-workspace Integrations tree is also part
    // of left-panel navigation. The tree is opt-in — `flat`, the default menu
    // mode, renders no branches at all — so choose a tree mode first.
    await setSettingsMenuMode(testPage, "accordion");
    const { workspaces } = await apiClient.listWorkspaces();
    const workspaceId = workspaces[0].id;
    const integrationsPath = `/settings/workspaces/${workspaceId}/integrations`;
    await testPage.goto(integrationsPath);
    const settingsTree = testPage.getByTestId(SETTINGS_TAKEOVER_TESTID);
    // By href, not by name: a workspace another spec created can itself be
    // called "GitHub", and the row's own name gains a badge once the
    // integration reports connected — neither of which changes its route.
    const githubTreeRow = settingsTree.locator(`a[href="${integrationsPath}/github"]`);
    await expect(githubTreeRow).toBeVisible({ timeout: 10_000 });
  });
});
