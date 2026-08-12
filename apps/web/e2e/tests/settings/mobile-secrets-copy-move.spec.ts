import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

const WORKSPACE_VALUE = "e2e-mobile-copy-move-value";

test.describe("mobile-secrets-copy-move", () => {
  test("moves a workspace secret to General and copies one back at phone width", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    const sourceName = "E2E Mobile Copy Move";
    const movedName = "E2E Mobile Moved";
    const copiedName = "E2E Mobile Copied";

    // Create a workspace secret through the mobile flow.
    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/secrets`);
    await testPage.getByRole("button", { name: "Add secret", exact: true }).click();
    await testPage.getByPlaceholder("Name (e.g. OpenAI Production Key)").fill(sourceName);
    await testPage.getByPlaceholder("Secret value").fill(WORKSPACE_VALUE);
    const secretSave = testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" });
    await expect(secretSave).toBeEnabled();
    await secretSave.click();
    await expect(testPage.getByText(sourceName, { exact: true })).toBeVisible();

    // Move it to General through the dialog's own controls.
    await testPage.getByRole("button", { name: `Copy or move ${sourceName}` }).click();
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByRole("radio", { name: /Move/ }).click();
    await dialog.getByLabel("Name").fill(movedName);

    for (const control of [
      dialog.getByLabel("Name"),
      dialog.getByRole("radio", { name: /Move/ }),
      dialog.getByRole("button", { name: "Move", exact: true }),
      dialog.getByRole("button", { name: "Close", exact: true }),
    ]) {
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
    }
    await assertNoDocumentHorizontalOverflow(testPage, "mobile copy/move dialog");

    await dialog.getByRole("button", { name: "Move", exact: true }).click();
    await expect(dialog).toBeHidden();
    await expect(testPage.getByText(sourceName, { exact: true })).toHaveCount(0);
    await expect(testPage.locator("body")).not.toContainText(WORKSPACE_VALUE);

    // The moved secret now lives in General.
    await testPage.goto("/settings/general/secrets");
    await expect(testPage.getByText(movedName, { exact: true })).toBeVisible();

    // Copy it back to the workspace using the destination picker.
    await testPage.getByRole("button", { name: `Copy or move ${movedName}` }).click();
    const copyDialog = testPage.getByRole("dialog");
    await expect(copyDialog).toBeVisible();
    await copyDialog.getByLabel("Name").fill(copiedName);

    const workspaces = await apiClient.listWorkspaces();
    const workspaceName = workspaces.workspaces.find(
      (workspace) => workspace.id === seedData.workspaceId,
    )?.name;
    expect(workspaceName).toBeTruthy();
    await copyDialog.getByRole("combobox", { name: "Destination" }).click();
    await testPage.getByRole("option", { name: workspaceName!, exact: true }).click();
    await copyDialog.getByRole("button", { name: "Copy", exact: true }).click();
    await expect(copyDialog).toBeHidden();

    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/secrets`);
    await expect(testPage.getByText(copiedName, { exact: true })).toBeVisible();
    await expect(testPage.locator("body")).not.toContainText(WORKSPACE_VALUE);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile secrets after copy/move");
    await prCapture.screenshot("mobile-secrets-copy-move", {
      caption: "Mobile secrets copy/move flow",
    });
  });
});
