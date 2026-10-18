import { expect, test } from "../../fixtures/test-base";
import { uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import {
  installEffortFixture,
  checkEffortRoute,
  seedEffortFeedback,
} from "./effort-signals-helpers";
import { SessionPage } from "../../pages/session-page";
import { KanbanPage } from "../../pages/kanban-page";

test("AC-PLUGINS-EFFORT-SIGNALS-005.3: effort route panel and card badge fit Pixel 5", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(90000);
  try {
    await installEffortFixture(testPage, apiClient, seedData.workspaceId);
    const task = await apiClient.createTask(seedData.workspaceId, "Mobile effort fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "IDLE",
      agentProfileId: seedData.agentProfileId,
      repositoryId: seedData.repositoryId,
    });
    await apiClient.seedSessionMessage(sessionId, {
      type: "message",
      content: "Fixture human effort prompt",
      authorType: "user",
      newTurn: true,
    });
    await seedEffortFeedback(apiClient, seedData.workspaceId, seedData.repositoryId, task.id);
    await checkEffortRoute(testPage, task.id, true);
    expect(
      (await testPage.getByTestId("effort-summary").boundingBox())?.height,
    ).toBeGreaterThanOrEqual(44);
    await testPage.getByTestId("effort-estimate").tap();
    await testPage.goto(`/t/${task.id}`);
    await new SessionPage(testPage).waitForLoad();
    await testPage.getByRole("button", { name: "Panels", exact: true }).tap();
    await testPage.getByTestId("mobile-plugin-panel-option-kandev-plugin-e2e-effort").tap();
    await expect(testPage.getByTestId("effort-view")).toBeVisible();
    await testPage.getByTestId("effort-summary").tap();
    await expect(testPage.getByTestId("effort-result")).toBeVisible();
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await testPage.screenshot({ path: test.info().outputPath("effort-mobile.png") });
    await new KanbanPage(testPage).goto(seedData.workflowId);
    await expect(
      testPage.locator(`[data-testid="effort-badge"][data-task-id="${task.id}"]`),
    ).toHaveText("1");
  } finally {
    await uninstallFixturePlugin(apiClient);
  }
});
