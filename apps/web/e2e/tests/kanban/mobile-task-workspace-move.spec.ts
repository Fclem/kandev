import type { Locator, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { seedWorkspaceMove } from "../../helpers/workspace-move";
import { WorkspaceMovePage } from "../../pages/workspace-move-page";

async function openSourceBoard(page: Page, workflowId: string) {
  await page.goto(`/?workflowId=${encodeURIComponent(workflowId)}`);
  await page.getByTestId("mobile-kanban-layout").waitFor({ state: "visible" });
}

async function openMoveDrawer(page: Page, taskId: string) {
  await page.getByTestId(`task-card-${taskId}`).getByRole("button", { name: "More options" }).tap();
  await page.getByTestId("task-context-move-to").tap();
  await page.getByTestId("task-context-change-workspace").tap();
  const form = new WorkspaceMovePage(page, true);
  await expect(form.phoneDrawer).toBeVisible();
  await waitForFiniteAnimations(form.phoneDrawer);
  return form;
}

async function box(locator: Locator) {
  const value = await locator.boundingBox();
  if (!value) throw new Error("element has no layout box");
  return value;
}

/** Inset drawer within the dynamic viewport, fixed footer, touch-sized actions. */
async function expectPhoneGeometry(page: Page, form: WorkspaceMovePage) {
  const viewport = page.viewportSize()!;
  const drawer = await box(form.phoneDrawer);
  expect(drawer.y).toBeGreaterThanOrEqual(0);
  expect(drawer.y + drawer.height).toBeLessThanOrEqual(viewport.height + 1);
  expect(drawer.x).toBeGreaterThanOrEqual(0);
  expect(drawer.x + drawer.width).toBeLessThanOrEqual(viewport.width + 1);
  for (const testId of ["workspace-move-cancel", "workspace-move-submit", "workspace-move-close"]) {
    const target = await box(page.getByTestId(testId));
    expect(target.height, testId).toBeGreaterThanOrEqual(44);
    expect(target.y + target.height, testId).toBeLessThanOrEqual(viewport.height + 1);
  }
  // Long selected names truncate inside their triggers instead of overflowing the drawer.
  for (const testId of [
    "workspace-move-workspace",
    "workspace-move-workflow",
    "workspace-move-step",
  ]) {
    const trigger = page.getByTestId(testId);
    if (!(await trigger.isVisible())) continue;
    const triggerBox = await box(trigger);
    expect(triggerBox.x + triggerBox.width, testId).toBeLessThanOrEqual(
      drawer.x + drawer.width + 1,
    );
    // The selected label is clipped inside its trigger, never painted past it.
    const label = await trigger.evaluate((element) => {
      const text = element.querySelector("span");
      const clip = text?.closest("div");
      return {
        right: (clip ?? element).getBoundingClientRect().right,
        trigger: element.getBoundingClientRect().right,
      };
    });
    expect(label.right, testId).toBeLessThanOrEqual(label.trigger + 1);
    const body = await box(page.getByTestId("workspace-move-scroll"));
    expect(triggerBox.x + triggerBox.width, testId).toBeLessThanOrEqual(body.x + body.width + 1);
  }
  const scroll = page.getByTestId("workspace-move-scroll");
  expect(await scroll.evaluate((element) => getComputedStyle(element).overflowY)).toBe("auto");
  await assertNoDocumentHorizontalOverflow(page, "phone workspace move drawer");
}

test.describe("Phone task workspace move", () => {
  // @covers AC-TASKS-CROSS-WORKSPACE-TRANSFER-004.5, 004.6, 004.2, 004.4
  test("moves a task from a visible action in an inset drawer", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    const fixture = await seedWorkspaceMove(
      apiClient,
      seedData,
      "Phone move with a deliberately long workspace name for wrapping checks",
    );
    const task = await apiClient.createTask(seedData.workspaceId, "Phone workspace move", {
      workflow_id: fixture.sourceWorkflow.id,
      workflow_step_id: fixture.sourceStep.id,
    });
    await openSourceBoard(testPage, fixture.sourceWorkflow.id);
    await testPage
      .getByTestId(`task-card-${task.id}`)
      .getByRole("button", { name: "More options" })
      .tap();
    await testPage.getByTestId("task-context-move-to").tap();
    await expect(testPage.getByTestId("task-context-change-workspace")).toBeVisible();
    await testPage.screenshot({ path: testInfo.outputPath("ui-03-phone-move-menu.png") });
    await testPage.getByTestId("task-context-change-workspace").tap();
    const form = new WorkspaceMovePage(testPage, true);
    await expect(form.phoneDrawer).toBeVisible();
    await waitForFiniteAnimations(form.phoneDrawer);
    await expectPhoneGeometry(testPage, form);

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
    await expectPhoneGeometry(testPage, form);
    await testPage.screenshot({ path: testInfo.outputPath("ui-03-phone-rejected.png") });

    await form.chooseStep(fixture.destinationStep.id);
    await testPage.screenshot({ path: testInfo.outputPath("ui-03-phone-ready.png") });
    await form.submit();
    await expect(form.outcome).toHaveAttribute("data-status", "committed");
    await expect(testPage.getByTestId(`task-card-${task.id}`)).toHaveCount(0);
    await testPage.screenshot({ path: testInfo.outputPath("ui-04-phone-success.png") });
    await form.press(testPage.getByTestId("workspace-move-open-task"));
    await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}`));
    expect((await apiClient.getTask(task.id)).workspace_id).toBe(fixture.destination.id);
  });

  // @covers AC-TASKS-CROSS-WORKSPACE-TRANSFER-004.5
  test("switches between drawer and dialog at the 768px boundary without losing choices", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const fixture = await seedWorkspaceMove(apiClient, seedData, "Boundary move");
    const task = await apiClient.createTask(seedData.workspaceId, "Boundary workspace move", {
      workflow_id: fixture.sourceWorkflow.id,
      workflow_step_id: fixture.sourceStep.id,
    });
    await testPage.setViewportSize({ width: 767, height: 720 });
    await openSourceBoard(testPage, fixture.sourceWorkflow.id);
    const form = await openMoveDrawer(testPage, task.id);
    await form.choose(
      fixture.destination.id,
      fixture.destinationWorkflow.id,
      fixture.destinationStep.id,
    );
    await expectPhoneGeometry(testPage, form);

    await testPage.setViewportSize({ width: 768, height: 720 });
    await expect(form.desktopDialog).toBeVisible();
    await expect(form.phoneDrawer).toHaveCount(0);
    // A coarse pointer keeps touch-sized controls on the wider dialog.
    expect((await box(form.submitButton)).height).toBeGreaterThanOrEqual(44);
    await expect(form.submitButton).toBeEnabled();
    await assertNoDocumentHorizontalOverflow(testPage, "768px workspace move dialog");
  });
});
