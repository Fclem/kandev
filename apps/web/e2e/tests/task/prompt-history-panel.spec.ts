import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

test.describe("Prompt history panel", () => {
  test("opens the active session prompts from the workbench menu", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const prompt = "Prompt history seeded user prompt";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Prompt history task",
      seedData.agentProfileId,
      {
        description: prompt,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("Prompt history task did not create a session");
    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return DONE_STATES.includes(sessions[0]?.state ?? "");
      })
      .toBe(true);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForDockviewReady();
    await session.addPanelButton().click();
    await testPage.getByTestId("add-panel-prompt-history-item").click();

    await expect(testPage.getByTestId("prompt-history-panel")).toBeVisible();
    await expect(testPage.getByTestId("prompt-history-row-0")).toContainText(prompt);
  });
});
