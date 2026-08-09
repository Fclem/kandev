import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";

// Mobile companion to no-silent-model-fallback.spec.ts: the task-create
// picker's fallback explanation must be discoverable on touch — visible
// secondary text, not a hover-only tooltip (icon title).
const GONE_MODEL = "claude-gone";

useRegularMode();

test.describe("No silent model fallback on mobile", () => {
  test("task-create picker shows the fallback note as visible text on touch", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(90_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    if (!agent) throw new Error("no agents available");
    const fallbackProfile = await apiClient.createAgentProfile(agent.id, "Gone Fallback Mobile", {
      model: GONE_MODEL,
      fallback_model: "mock-fast",
    });

    try {
      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await kanban.createTaskButton.first().click();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();

      await testPage.getByTestId("task-title-input").fill("Gone model picker mobile test");
      await testPage.getByTestId("task-description-input").fill("verify visible fallback note");

      const selector = dialog.getByTestId("agent-profile-selector");
      await selector.click();

      const fallbackOption = testPage.getByRole("option", { name: /Gone Fallback Mobile/ });
      await expect(fallbackOption).toBeVisible({ timeout: 15_000 });
      await expect(fallbackOption).not.toHaveAttribute("aria-disabled", "true");

      // The fallback explanation must be readable without hover: the
      // picker renders it as visible secondary text inside the option.
      await expect(fallbackOption).toContainText(/mock-fast/);
      await expect(fallbackOption).toContainText(/no longer available/);
    } finally {
      await apiClient.deleteAgentProfile(fallbackProfile.id, true);
    }
  });
});
