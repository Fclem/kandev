import { expect, type Locator, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";

useRegularMode();

type DialogRenderingMetrics = {
  animationName: string;
  transform: string;
  translate: string;
  zIndex: string;
  overlayZIndex: string;
  centerX: number;
  centerY: number;
  width: number;
  height: number;
  contentOverOverlay: boolean;
};

async function openCreateTaskDialog(testPage: Page): Promise<Locator> {
  const kanban = new KanbanPage(testPage);
  await kanban.goto();
  await kanban.createTaskButton.first().click();
  const dialog = testPage.getByTestId("create-task-dialog");
  await expect(dialog).toBeVisible();
  return dialog;
}

async function readDialogRenderingMetrics(dialog: Locator): Promise<DialogRenderingMetrics> {
  await dialog.evaluate(async (element) => {
    await Promise.all(element.getAnimations().map((animation) => animation.finished));
  });

  return dialog.evaluate((element: HTMLElement) => {
    const style = getComputedStyle(element);
    const box = element.getBoundingClientRect();
    const center = document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2);
    const overlay = document.querySelector<HTMLElement>('[data-slot="dialog-overlay"]');
    return {
      animationName: style.animationName,
      transform: style.transform,
      translate: style.translate,
      zIndex: style.zIndex,
      overlayZIndex: overlay ? getComputedStyle(overlay).zIndex : "",
      centerX: box.left + box.width / 2,
      centerY: box.top + box.height / 2,
      width: box.width,
      height: box.height,
      contentOverOverlay: center?.closest('[data-testid="create-task-dialog"]') === element,
    };
  });
}

test.describe("Create Task WebKit rendering", () => {
  test("keeps the existing Chromium motion and geometry", async ({ testPage, prCapture }) => {
    const dialog = await openCreateTaskDialog(testPage);
    await expect(testPage.locator("html")).toHaveAttribute("data-rendering-engine", "other");

    const metrics = await readDialogRenderingMetrics(dialog);
    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    expect(metrics.animationName).toBe("enter");
    expect(metrics.translate).toContain("-50%");
    expect(metrics.zIndex).toBe("50");
    expect(metrics.overlayZIndex).toBe("50");
    expect(metrics.width).toBe(900);
    expect(metrics.centerX).toBeCloseTo(viewport!.width / 2, 0);
    expect(metrics.centerY).toBeCloseTo(viewport!.height / 2, 0);
    expect(metrics.contentOverOverlay).toBe(true);
    await prCapture.screenshot("chromium-create-task-dialog", {
      caption: "Chromium keeps the existing Create Task dialog motion and geometry.",
    });
  });

  test("uses transform-free motion and centering for WebKit", async ({ testPage, prCapture }) => {
    await testPage.goto("/");
    await testPage.locator("html").evaluate((root) => {
      root.setAttribute("data-rendering-engine", "webkit");
    });

    const kanban = new KanbanPage(testPage);
    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    const metrics = await readDialogRenderingMetrics(dialog);
    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    expect(metrics.animationName).toBe("kandev-dialog-webkit-enter");
    expect(["none", "matrix(1, 0, 0, 1, 0, 0)"]).toContain(metrics.transform);
    expect(["none", "0px", "0px 0px"]).toContain(metrics.translate);
    expect(metrics.zIndex).toBe("50");
    expect(metrics.overlayZIndex).toBe("49");
    expect(metrics.width).toBe(900);
    expect(metrics.centerX).toBeCloseTo(viewport!.width / 2, 0);
    expect(metrics.centerY).toBeCloseTo(viewport!.height / 2, 0);
    expect(metrics.contentOverOverlay).toBe(true);
    await prCapture.screenshot("webkit-create-task-dialog", {
      caption: "The WebKit-safe Create Task dialog is centered without transformed text.",
    });
  });
});
