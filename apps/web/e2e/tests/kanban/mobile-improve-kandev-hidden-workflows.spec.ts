import { expect, test } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

const IMPROVE_WORKFLOWS = [
  { name: "Improve", templateId: "improve-kandev" },
  { name: "Open issue", templateId: "report-kandev-issue" },
];

test.describe("Mobile Improve Kandev workflow visibility", () => {
  test("keeps both empty Improve workflows selectable", async ({
    apiClient,
    testPage,
    seedData,
  }) => {
    const workspace = await apiClient.createWorkspace("Mobile Improve visibility fixture");
    try {
      const workflows = await Promise.all(
        IMPROVE_WORKFLOWS.map(({ name, templateId }) =>
          apiClient.e2eCreateHiddenWorkflow(workspace.id, name, templateId),
        ),
      );
      await apiClient.saveUserSettings({
        workspace_id: workspace.id,
        workflow_filter_id: "",
        repository_ids: [],
      });

      const mobile = new MobileKanbanPage(testPage);
      await mobile.goto();
      const drawer = testPage.getByTestId("mobile-board-navigator-drawer");
      for (const workflow of workflows) {
        if (!(await drawer.isVisible())) await mobile.boardNavigator.click();
        await expect(drawer).toBeVisible();
        const item = drawer
          .locator('section[aria-labelledby="mobile-workflow-heading"]')
          .getByRole("button", { name: workflow.name });
        await expect(item).toBeVisible();
        await item.click();
        await expect(item).toHaveAttribute("data-active", "true");
        await expect(mobile.board.locator('[data-testid^="task-card-"]')).toHaveCount(0);
      }
    } finally {
      await apiClient.saveUserSettings({
        workspace_id: seedData.workspaceId,
        workflow_filter_id: seedData.workflowId,
        repository_ids: [],
      });
      await apiClient.deleteWorkspace(workspace.id, workspace.name).catch(() => {});
    }
  });
  test("shows each workflow's task card when selected", async ({
    apiClient,
    testPage,
    seedData,
  }) => {
    const workspace = await apiClient.createWorkspace("Mobile Improve task fixture");
    try {
      const workflows = await Promise.all(
        IMPROVE_WORKFLOWS.map(({ name, templateId }) =>
          apiClient.e2eCreateHiddenWorkflow(workspace.id, name, templateId),
        ),
      );
      const steps = await Promise.all(
        workflows.map((workflow) =>
          apiClient.createWorkflowStep(workflow.id, workflow.name, 0, { is_start_step: true }),
        ),
      );
      const tasks = await Promise.all(
        workflows.map((workflow, index) =>
          apiClient.createTask(workspace.id, `${workflow.name} task card`, {
            workflow_id: workflow.id,
            workflow_step_id: steps[index].id,
          }),
        ),
      );
      await apiClient.saveUserSettings({
        workspace_id: workspace.id,
        workflow_filter_id: "",
        repository_ids: [],
      });

      const mobile = new MobileKanbanPage(testPage);
      await mobile.goto();
      const drawer = testPage.getByTestId("mobile-board-navigator-drawer");
      for (let index = 0; index < workflows.length; index += 1) {
        if (!(await drawer.isVisible())) await mobile.boardNavigator.click();
        await expect(drawer).toBeVisible();
        const item = drawer
          .locator('section[aria-labelledby="mobile-workflow-heading"]')
          .getByRole("button", { name: workflows[index].name });
        await expect(item).toBeVisible();
        await item.click();
        await expect(item).toHaveAttribute("data-active", "true");
        await expect(mobile.taskCard(tasks[index].id)).toBeVisible();
      }
    } finally {
      await apiClient.saveUserSettings({
        workspace_id: seedData.workspaceId,
        workflow_filter_id: seedData.workflowId,
        repository_ids: [],
      });
      await apiClient.deleteWorkspace(workspace.id, workspace.name).catch(() => {});
    }
  });
});
