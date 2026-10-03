import { watchWs } from "../../helpers/causal-waits";
import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

const IMPROVE_TEMPLATE = "improve-kandev";
const ISSUE_TEMPLATE = "report-kandev-issue";

test.describe("Improve Kandev workflow visibility", () => {
  test("shows task-bearing Improve templates and excludes unrelated hidden workflows", async ({
    apiClient,
    testPage,
    seedData,
  }) => {
    const workspace = await apiClient.createWorkspace("Improve Kanban visibility fixture");
    try {
      const improve = await apiClient.e2eCreateHiddenWorkflow(
        workspace.id,
        "Improve",
        IMPROVE_TEMPLATE,
      );
      expect(improve.workflow_template_id).toBe(IMPROVE_TEMPLATE);
      const issue = await apiClient.e2eCreateHiddenWorkflow(
        workspace.id,
        "Open issue",
        ISSUE_TEMPLATE,
      );
      expect(issue.workflow_template_id).toBe(ISSUE_TEMPLATE);
      const unrelated = await apiClient.e2eCreateHiddenWorkflow(workspace.id, "Internal");
      const [improveStep, issueStep, unrelatedStep] = await Promise.all([
        apiClient.createWorkflowStep(improve.id, "Improve", 0, { is_start_step: true }),
        apiClient.createWorkflowStep(issue.id, "Open issue", 0, { is_start_step: true }),
        apiClient.createWorkflowStep(unrelated.id, "Internal", 0, { is_start_step: true }),
      ]);
      const improveTask = await apiClient.seedTask(workspace.id, "Improve task card", {
        workflow_id: improve.id,
        workflow_step_id: improveStep.id,
      });
      const issueTask = await apiClient.seedTask(workspace.id, "Issue task card", {
        workflow_id: issue.id,
        workflow_step_id: issueStep.id,
      });
      await apiClient.seedTask(workspace.id, "Unrelated hidden task", {
        workflow_id: unrelated.id,
        workflow_step_id: unrelatedStep.id,
      });
      await apiClient.saveUserSettings({
        workspace_id: workspace.id,
        workflow_filter_id: "",
        repository_ids: [],
      });

      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await expect(kanban.taskCard(improveTask.task_id)).toBeVisible();
      await expect(kanban.taskCard(issueTask.task_id)).toBeVisible();
      await expect(kanban.taskCardByTitle("Unrelated hidden task")).toHaveCount(0);
    } finally {
      await apiClient.saveUserSettings({
        workspace_id: seedData.workspaceId,
        workflow_filter_id: seedData.workflowId,
        repository_ids: [],
      });
      await apiClient.deleteWorkspace(workspace.id, workspace.name).catch(() => {});
    }
  });

  test("recognizes bootstrap-created templates on the open board without a reload", async ({
    apiClient,
    testPage,
    seedData,
  }) => {
    const workspace = await apiClient.createWorkspace("Improve Kanban live fixture");
    try {
      await apiClient.createRepository(workspace.id, seedData.repositoryPath, "main", {
        name: "Kandev",
        provider: "github",
        provider_owner: "kdlbs",
        provider_name: "kandev",
        remote_url: "https://github.com/kdlbs/kandev",
      });
      await apiClient.mockGitHubSetWorkspaceConnection(workspace.id, {
        source: "pat",
        status: "active",
        login: "kandev-maint",
      });
      await apiClient.saveUserSettings({
        workspace_id: workspace.id,
        workflow_filter_id: "",
        repository_ids: [],
      });

      const watcher = watchWs(testPage);
      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      const improveCreated = watcher.waitForEvent("workflow.created", {
        where: (payload) => payload.workflow_template_id === IMPROVE_TEMPLATE,
      });
      const issueCreated = watcher.waitForEvent("workflow.created", {
        where: (payload) => payload.workflow_template_id === ISSUE_TEMPLATE,
      });
      const bootstrap = await testPage.evaluate(async (workspaceId) => {
        const response = await fetch("/api/v1/system/improve-kandev/bootstrap", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ workspace_id: workspaceId, create_workspace: false }),
        });
        return { status: response.status, body: await response.json() };
      }, workspace.id);
      expect(bootstrap.status).toBe(200);
      expect(bootstrap.body.workspace_id).toBe(workspace.id);
      expect(bootstrap.body.workflow_id).toBeTruthy();
      expect(bootstrap.body.issue_workflow_id).toBeTruthy();
      const [improveEvent, issueEvent] = await Promise.all([improveCreated, issueCreated]);
      expect(improveEvent.payload.workflow_template_id).toBe(IMPROVE_TEMPLATE);
      expect(issueEvent.payload.workflow_template_id).toBe(ISSUE_TEMPLATE);

      const workflowId = bootstrap.body.workflow_id as string;
      const steps = await apiClient.listWorkflowSteps(workflowId);
      const startStep = steps.steps.find((step) => step.is_start_step) ?? steps.steps[0];
      expect(startStep).toBeDefined();
      const task = await apiClient.createTask(workspace.id, "Live Improve task card", {
        workflow_id: workflowId,
        workflow_step_id: startStep.id,
      });
      await expect(kanban.taskCard(task.id)).toBeVisible();
    } finally {
      await apiClient.mockGitHubDeleteWorkspaceConnection(workspace.id).catch(() => {});
      await apiClient.saveUserSettings({
        workspace_id: seedData.workspaceId,
        workflow_filter_id: seedData.workflowId,
        repository_ids: [],
      });
      await apiClient.deleteWorkspace(workspace.id, workspace.name).catch(() => {});
    }
  });
});
