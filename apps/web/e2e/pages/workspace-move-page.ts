import { expect, type Locator, type Page } from "@playwright/test";
import { waitForFiniteAnimations } from "../helpers/animations";

/** The app-level form that moves one task to another workspace. */
export class WorkspaceMovePage {
  readonly form: Locator;
  readonly desktopDialog: Locator;
  readonly phoneDrawer: Locator;
  readonly outcome: Locator;
  readonly submitButton: Locator;

  constructor(
    readonly page: Page,
    readonly mobile = false,
  ) {
    this.form = page.getByTestId("workspace-move-form");
    this.desktopDialog = page.getByTestId("workspace-move-dialog");
    this.phoneDrawer = page.getByTestId("workspace-move-drawer");
    this.outcome = page.getByTestId("workspace-move-outcome");
    this.submitButton = page.getByTestId("workspace-move-submit");
  }

  async choose(workspaceId: string, workflowId: string, stepId: string) {
    await this.select("workspace-move-workspace", workspaceId);
    await this.select("workspace-move-workflow", workflowId);
    await this.select("workspace-move-step", stepId);
  }

  async chooseWorkspace(workspaceId: string) {
    await this.select("workspace-move-workspace", workspaceId);
  }

  async chooseStep(stepId: string) {
    await this.select("workspace-move-step", stepId);
  }

  async submit() {
    await expect(this.submitButton).toBeEnabled();
    await this.press(this.submitButton);
  }

  async press(target: Locator) {
    if (this.mobile) await target.tap();
    else await target.click();
  }

  private async select(testId: string, value: string) {
    const trigger = this.form.getByTestId(testId);
    await expect(trigger).toBeEnabled();
    if (this.mobile) await waitForFiniteAnimations(this.phoneDrawer);
    if ((await trigger.getAttribute("aria-expanded")) !== "true") await this.press(trigger);
    await expect(trigger).toHaveAttribute("aria-expanded", "true");
    const option = this.page.locator(`[role="option"][data-value="${value}"]`);
    // The shared Combobox clears a value when its selected option is chosen again,
    // so an already-selected choice is confirmed in place rather than re-picked.
    const optionText = (await option.innerText()).trim();
    if (optionText === (await trigger.innerText()).trim()) {
      await this.page.keyboard.press("Escape");
      await expect(trigger).toHaveAttribute("aria-expanded", "false");
      return;
    }
    await this.press(option);
    await expect(option).toBeHidden();
  }
}
