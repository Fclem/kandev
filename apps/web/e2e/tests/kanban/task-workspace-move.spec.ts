import type { Page, Request } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { WorkspaceMovePage } from "../../pages/workspace-move-page";
import { runningTurnScript, seedWorkspaceMove, transferBody } from "../../helpers/workspace-move";
import { OfficeApiClient } from "../../helpers/office-api-client";

const TRANSFER_ROUTE = "**/api/v1/tasks/*/transfer";

function transferKeys(requests: Request[]) {
  return requests.map(
    (request) => (request.postDataJSON() as { idempotency_key: string }).idempotency_key,
  );
}

function recordTransfers(page: Page) {
  const requests: Request[] = [];
  page.on("request", (request) => {
    if (request.method() === "POST" && request.url().endsWith("/transfer")) requests.push(request);
  });
  return requests;
}

test.describe("Task workspace move", () => {
  // @covers AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.1, 001.3, 004.1, 004.2, 004.4
  test("moves a running task to another workspace and keeps its sessions", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    const fixture = await seedWorkspaceMove(apiClient, seedData, "Desktop move");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Desktop workspace move keeps a long readable task title",
      seedData.agentProfileId,
      {
        workflow_id: fixture.sourceWorkflow.id,
        workflow_step_id: fixture.sourceStep.id,
        repository_ids: [seedData.repositoryId],
        description: runningTurnScript("desktop"),
      },
    );
    await apiClient.wsRequest("task.plan.create", {
      task_id: task.id,
      title: "Move plan",
      content: "Plan survives the workspace move",
    });
    await apiClient.seedTaskSession(task.id, {
      state: "IDLE",
      agentProfileId: seedData.agentProfileId,
      repositoryId: seedData.repositoryId,
    });
    const sessionsBefore = (await apiClient.listTaskSessions(task.id)).sessions.map((s) => s.id);
    // The move form must read the task after the running turn writes its first marker.
    await testPage.goto(`/t/${task.id}`);
    await expect(testPage.getByText(/desktop before move/).last()).toBeVisible({ timeout: 30_000 });
    const kanban = new KanbanPage(testPage);
    await kanban.goto(fixture.sourceWorkflow.id);
    await expect(kanban.taskCard(task.id)).toBeVisible();
    const listingUrl = testPage.url();

    await kanban.openTaskContextMenu(task.id);
    await kanban.contextMoveTo().focus();
    await testPage.keyboard.press("ArrowRight");
    await expect(kanban.contextChangeWorkspace()).toBeVisible();
    await testPage.screenshot({ path: testInfo.outputPath("ui-01-desktop-move-menu.png") });
    await testPage.keyboard.press("Escape");
    await testPage.keyboard.press("Escape");

    await kanban.openWorkspaceMoveForm(task.id);
    const form = new WorkspaceMovePage(testPage);
    await expect(form.desktopDialog).toBeVisible();
    await expect(form.form.getByTestId("workspace-move-current")).toContainText("Work");
    await expect(form.submitButton).toBeDisabled();
    await form.choose(
      fixture.destination.id,
      fixture.destinationWorkflow.id,
      fixture.destinationStep.id,
    );
    await testPage.screenshot({ path: testInfo.outputPath("ui-02-desktop-ready.png") });
    await form.submit();
    await expect(form.outcome).toHaveAttribute("data-status", "committed");
    await expect(kanban.taskCard(task.id)).toHaveCount(0);
    expect(testPage.url()).toBe(listingUrl);
    await testPage.screenshot({ path: testInfo.outputPath("ui-04-desktop-success.png") });

    await form.press(testPage.getByTestId("workspace-move-open-task"));
    await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}`));
    // The running turn finishes after the move, in the same session transcript.
    await expect(testPage.getByText(/desktop after move/).last()).toBeVisible({ timeout: 30_000 });
    const moved = await apiClient.getTask(task.id);
    expect(moved.workspace_id).toBe(fixture.destination.id);
    expect(moved.workflow_step_id).toBe(fixture.destinationStep.id);
    expect((await apiClient.listTaskSessions(task.id)).sessions.map((s) => s.id).sort()).toEqual(
      [...sessionsBefore].sort(),
    );
    expect((await apiClient.getTaskPlan(task.id)).content).toContain(
      "Plan survives the workspace move",
    );

    await testPage.reload();
    // The persisted transcript keeps the turn's output from before and after the move.
    await expect(testPage.getByText(/desktop before move/).last()).toBeVisible();
    await expect(testPage.getByText(/desktop after move/).last()).toBeVisible();
    await testPage.goto(
      `/?workspaceId=${fixture.destination.id}&workflowId=${fixture.destinationWorkflow.id}`,
    );
    await expect(kanban.taskCardInColumn(moved.title, fixture.destinationStep.id)).toBeVisible();
  });

  // @covers AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.2, 004.3, 005.2
  test("explains an incompatible step and keeps the task where it is", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    const fixture = await seedWorkspaceMove(apiClient, seedData, "Incompatible move");
    const task = await apiClient.createTask(seedData.workspaceId, "Incompatible move task", {
      workflow_id: fixture.sourceWorkflow.id,
      workflow_step_id: fixture.sourceStep.id,
    });
    const requests = recordTransfers(testPage);
    const kanban = new KanbanPage(testPage);
    await kanban.goto(fixture.sourceWorkflow.id);
    await kanban.openWorkspaceMoveForm(task.id);
    const form = new WorkspaceMovePage(testPage);
    await form.choose(
      fixture.destination.id,
      fixture.destinationWorkflow.id,
      fixture.incompatibleStep.id,
    );
    await form.submit();

    await expect(testPage.getByTestId("workspace-move-rejection")).toHaveAttribute(
      "data-code",
      "step_incompatible",
    );
    await expect(form.outcome).toHaveAttribute("role", "alert");
    await testPage.screenshot({ path: testInfo.outputPath("ui-04-desktop-rejected.png") });
    expect((await apiClient.getTask(task.id)).workspace_id).toBe(seedData.workspaceId);
    await expect(kanban.taskCard(task.id)).toBeVisible();

    await form.chooseStep(fixture.destinationStep.id);
    await form.submit();
    await expect(form.outcome).toHaveAttribute("data-status", "committed");
    const keys = transferKeys(requests);
    expect(keys).toHaveLength(2);
    expect(keys[1]).not.toBe(keys[0]);
  });

  // @covers AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.1, 005.3
  test("resolves a lost committed response only through the same request", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    const fixture = await seedWorkspaceMove(apiClient, seedData, "Uncertain move");
    const task = await apiClient.createTask(seedData.workspaceId, "Uncertain move task", {
      workflow_id: fixture.sourceWorkflow.id,
      workflow_step_id: fixture.sourceStep.id,
    });
    const requests = recordTransfers(testPage);
    let attempt = 0;
    await testPage.route(TRANSFER_ROUTE, async (route) => {
      attempt += 1;
      if (attempt === 1) {
        await route.fetch(); // the backend commits; the browser never sees the response
        await route.abort("connectionreset");
        return;
      }
      if (attempt === 2) {
        await route.fulfill({ status: 403, json: { error: "insufficient permissions" } });
        return;
      }
      await route.continue();
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto(fixture.sourceWorkflow.id);
    await kanban.openWorkspaceMoveForm(task.id);
    const form = new WorkspaceMovePage(testPage);
    await form.choose(
      fixture.destination.id,
      fixture.destinationWorkflow.id,
      fixture.destinationStep.id,
    );
    await form.submit();
    await expect(form.outcome).toHaveAttribute("data-status", "uncertain");
    await expect(form.submitButton).toBeDisabled();
    await testPage.screenshot({ path: testInfo.outputPath("ui-04-desktop-uncertain.png") });

    await form.press(testPage.getByTestId("workspace-move-check-result"));
    await expect(form.outcome).toHaveAttribute("data-status", "check-unavailable");
    await expect(form.submitButton).toBeDisabled();
    await testPage.screenshot({ path: testInfo.outputPath("ui-04-desktop-cannot-check.png") });

    await form.press(testPage.getByTestId("workspace-move-check-result"));
    await expect(form.outcome).toHaveAttribute("data-status", "committed");
    expect(new Set(transferKeys(requests)).size).toBe(1);
    expect(requests).toHaveLength(3);
    expect((await apiClient.getTask(task.id)).workspace_id).toBe(fixture.destination.id);
  });

  // @covers AC-TASKS-CROSS-WORKSPACE-TRANSFER-005.3
  test("settles a lost domain rejection before allowing a reviewed move", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const fixture = await seedWorkspaceMove(apiClient, seedData, "Lost rejection");
    const task = await apiClient.createTask(seedData.workspaceId, "Lost rejection task", {
      workflow_id: fixture.sourceWorkflow.id,
      workflow_step_id: fixture.sourceStep.id,
    });
    const requests = recordTransfers(testPage);
    let dropped = false;
    await testPage.route(TRANSFER_ROUTE, async (route) => {
      if (dropped) return route.continue();
      dropped = true;
      const response = await route.fetch(); // real 409 step_incompatible
      expect(response.status()).toBe(409);
      await route.abort("connectionreset");
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto(fixture.sourceWorkflow.id);
    await kanban.openWorkspaceMoveForm(task.id);
    const form = new WorkspaceMovePage(testPage);
    await form.choose(
      fixture.destination.id,
      fixture.destinationWorkflow.id,
      fixture.incompatibleStep.id,
    );
    await form.submit();
    await expect(form.outcome).toHaveAttribute("data-status", "uncertain");

    await form.press(testPage.getByTestId("workspace-move-check-result"));
    await expect(form.outcome).toHaveAttribute("data-status", "operation-rejected");
    await expect(form.submitButton).toBeDisabled();
    expect((await apiClient.getTask(task.id)).workspace_id).toBe(seedData.workspaceId);

    await form.press(testPage.getByTestId("workspace-move-refresh-task"));
    await form.chooseStep(fixture.destinationStep.id);
    await form.submit();
    await expect(form.outcome).toHaveAttribute("data-status", "committed");
    const keys = transferKeys(requests);
    expect(keys[0]).toBe(keys[1]);
    expect(keys[2]).not.toBe(keys[0]);
  });

  // @covers AC-TASKS-CROSS-WORKSPACE-TRANSFER-004.4, 005.4
  test("keeps an open task and its session while it leaves and returns", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const fixture = await seedWorkspaceMove(apiClient, seedData, "Open task move");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Open task continuity",
      seedData.agentProfileId,
      {
        workflow_id: fixture.sourceWorkflow.id,
        workflow_step_id: fixture.sourceStep.id,
        repository_ids: [seedData.repositoryId],
        description: 'e2e:message("continuity transcript")',
      },
    );
    await testPage.goto(`/t/${task.id}`);
    const transcript = testPage.getByText("continuity transcript", { exact: true }).last();
    await expect(transcript).toBeVisible({ timeout: 30_000 });
    const routeBefore = testPage.url();

    await apiClient.transferTask(
      task.id,
      await transferBody(
        apiClient,
        task.id,
        {
          workspaceId: fixture.destination.id,
          workflowId: fixture.destinationWorkflow.id,
          stepId: fixture.destinationStep.id,
        },
        `away-${task.id}`,
      ),
    );
    await expect
      .poll(() => testPage.evaluate(() => window.location.pathname))
      .toBe(new URL(routeBefore).pathname);
    await expect(transcript).toBeVisible();

    await apiClient.transferTask(
      task.id,
      await transferBody(
        apiClient,
        task.id,
        {
          workspaceId: seedData.workspaceId,
          workflowId: fixture.sourceWorkflow.id,
          stepId: fixture.sourceStep.id,
        },
        `back-${task.id}`,
      ),
    );
    await expect(transcript).toBeVisible();
    const kanban = new KanbanPage(testPage);
    await kanban.goto(fixture.sourceWorkflow.id);
    await expect(kanban.taskCard(task.id)).toBeVisible();
  });

  // @covers AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.2, 001.4, 004.3, 005.2
  test("names a workspace-owned label that blocks the move and clears after repair", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }, testInfo) => {
    const fixture = await seedWorkspaceMove(apiClient, seedData, "Label blocked move");
    const task = await apiClient.createTask(seedData.workspaceId, "Label blocked task", {
      workflow_id: fixture.sourceWorkflow.id,
      workflow_step_id: fixture.sourceStep.id,
    });
    const officeApi = new OfficeApiClient(backend.baseUrl, backend.ensureReady);
    await officeApi.addLabel(seedData.workspaceId, task.id, "Source label");
    const requests = recordTransfers(testPage);
    const kanban = new KanbanPage(testPage);
    await kanban.goto(fixture.sourceWorkflow.id);
    await kanban.openWorkspaceMoveForm(task.id);
    const form = new WorkspaceMovePage(testPage);
    await form.choose(
      fixture.destination.id,
      fixture.destinationWorkflow.id,
      fixture.destinationStep.id,
    );
    await form.submit();

    await expect(testPage.getByTestId("workspace-move-rejection")).toHaveAttribute(
      "data-code",
      "workspace_relation_unmapped",
    );
    await expect(form.outcome).toContainText("blocks this move");
    await testPage.screenshot({ path: testInfo.outputPath("ui-04-desktop-label-blocked.png") });
    expect((await apiClient.getTask(task.id)).workspace_id).toBe(seedData.workspaceId);
    await expect(kanban.taskCard(task.id)).toBeVisible();

    await officeApi.removeLabel(seedData.workspaceId, task.id, "Source label");
    await form.press(testPage.getByTestId("workspace-move-cancel"));
    await expect(form.desktopDialog).toHaveCount(0);
    await kanban.openWorkspaceMoveForm(task.id);
    const reopened = new WorkspaceMovePage(testPage);
    await reopened.choose(
      fixture.destination.id,
      fixture.destinationWorkflow.id,
      fixture.destinationStep.id,
    );
    await reopened.submit();
    await expect(reopened.outcome).toHaveAttribute("data-status", "committed");
    const keys = transferKeys(requests);
    expect(keys).toHaveLength(2);
    expect(keys[1]).not.toBe(keys[0]);
    expect((await apiClient.getTask(task.id)).workspace_id).toBe(fixture.destination.id);
  });

  // @covers AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.1, 005.3
  test("settles a lost source_changed response before allowing a reviewed move", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const fixture = await seedWorkspaceMove(apiClient, seedData, "Lost source change");
    const task = await apiClient.createTask(seedData.workspaceId, "Lost source change task", {
      workflow_id: fixture.sourceWorkflow.id,
      workflow_step_id: fixture.sourceStep.id,
    });
    const requests = recordTransfers(testPage);
    let dropped = false;
    await testPage.route(TRANSFER_ROUTE, async (route) => {
      if (dropped) return route.continue();
      dropped = true;
      const response = await route.fetch(); // real 409 source_changed
      expect(response.status()).toBe(409);
      await route.abort("connectionreset");
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto(fixture.sourceWorkflow.id);
    await kanban.openWorkspaceMoveForm(task.id);
    const form = new WorkspaceMovePage(testPage);
    await expect(form.form.getByTestId("workspace-move-current")).toContainText("Work");
    // The form captured the task version at open; a title edit makes that version stale.
    await apiClient.updateTaskTitle(task.id, "Lost source change task, edited");
    await form.choose(
      fixture.destination.id,
      fixture.destinationWorkflow.id,
      fixture.destinationStep.id,
    );
    await form.submit();
    await expect(form.outcome).toHaveAttribute("data-status", "uncertain");

    await form.press(testPage.getByTestId("workspace-move-check-result"));
    await expect(form.outcome).toHaveAttribute("data-status", "operation-rejected");
    await expect(form.submitButton).toBeDisabled();
    const unchanged = await apiClient.getTask(task.id);
    expect(unchanged.workspace_id).toBe(seedData.workspaceId);
    expect(unchanged.title).toBe("Lost source change task, edited");

    await form.press(testPage.getByTestId("workspace-move-refresh-task"));
    await expect(testPage.getByTestId("workspace-move-refresh-task")).toHaveCount(0);
    await expect(form.outcome).toHaveCount(0);
    await form.chooseStep(fixture.destinationStep.id);
    await expect(form.form.getByTestId("workspace-move-step")).toContainText(
      fixture.destinationStep.name,
    );
    await form.submit();
    await expect(form.outcome).toHaveAttribute("data-status", "committed");
    const keys = transferKeys(requests);
    expect(keys).toHaveLength(3);
    expect(keys[1]).toBe(keys[0]);
    expect(keys[2]).not.toBe(keys[0]);
  });
});
