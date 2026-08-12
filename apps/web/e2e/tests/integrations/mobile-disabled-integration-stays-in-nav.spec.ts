import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

// Mobile half of the post-removal contract from
// docs/specs/ui/remove-hide-disabled-nav-options.md: left-panel nav visibility is
// configuration-only, so a disabled-but-configured integration stays visible in the
// mobile menu's integrations group. The global sidebar is desktop-only, so on a
// phone the links live in the kanban hamburger sheet. The desktop sidebar and
// Settings-tree halves live in disabled-integration-stays-in-nav.spec.ts.
test.describe("disabled integration stays in the mobile menu", () => {
  test("a disabled-but-configured integration remains in the hamburger sheet", async ({
    testPage,
    apiClient,
  }) => {
    // Make GitHub configured/healthy so it is eligible to appear in the nav.
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");

    // Disable GitHub through its integration toggle.
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

    const kanban = new MobileKanbanPage(testPage);
    await kanban.goto();
    await kanban.mobileMenuButton.click();

    // Scope to the open menu sheet so the display:none desktop sidebar that
    // shares the same "GitHub" accessible name never matches.
    const sheet = testPage.getByRole("dialog");
    await expect(sheet.getByText("Integrations", { exact: true })).toBeVisible();
    await expect(sheet.getByRole("link", { name: "GitHub" })).toBeVisible({ timeout: 10_000 });
  });
});
