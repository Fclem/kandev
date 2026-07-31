import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";

const PLUGIN_ID = "kandev-plugin-bitbucket";
const packagePath = process.env.KANDEV_BITBUCKET_PLUGIN_PACKAGE?.trim();

test.skip(!packagePath, "requires KANDEV_BITBUCKET_PLUGIN_PACKAGE from the attached plugin repo");

test.describe("Bitbucket packaged plugin", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("installs the real package and renders its desktop and mobile workbench", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(90_000);
    if (!packagePath) throw new Error("Bitbucket plugin package path is required");
    const capture = new PrAssetCapture(testPage, testInfo.file);

    await testPage.goto("/settings/plugins");
    await testPage.getByTestId("install-plugin-trigger").click();
    await testPage.getByTestId("install-plugin-tab-upload").click();
    await testPage
      .getByTestId("install-plugin-file-input")
      .setInputFiles(path.resolve(packagePath));
    await testPage.getByTestId("install-plugin-upload-submit").click();
    const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await expect(pluginRow).toBeVisible({ timeout: 15_000 });
    await expect(pluginRow.getByText("Active", { exact: true })).toBeVisible();

    const connection = await apiClient.rawRequest(
      "POST",
      `/api/plugins/${PLUGIN_ID}/actions/connection.get`,
      { workspaceId: seedData.workspaceId },
    );
    const connectionBody = await connection.text();
    expect(connection.status, connectionBody).toBe(200);

    await testPage.setViewportSize({ width: 1440, height: 900 });
    await testPage.goto("/bitbucket");
    await expect(testPage.getByTestId("bitbucket-workbench")).toBeVisible();
    await expect(testPage.getByTestId("bitbucket-connection-health")).toBeVisible();
    await capture.screenshot("desktop-bitbucket-workbench", {
      caption: "Desktop Bitbucket workbench from the packaged plugin",
    });

    await testPage.setViewportSize({ width: 393, height: 851 });
    await testPage.goto("/bitbucket");
    const filterButton = testPage.getByRole("button", { name: "Filter pull requests" });
    await expect(filterButton).toBeVisible();
    await filterButton.click();
    await expect(testPage.getByRole("heading", { name: "Queue filters" })).toBeVisible();
    await testPage.waitForTimeout(400);
    await capture.screenshot("mobile-bitbucket-filters", {
      caption: "Mobile Bitbucket workbench with its touch filter drawer",
    });
    capture.flush();
  });
});
