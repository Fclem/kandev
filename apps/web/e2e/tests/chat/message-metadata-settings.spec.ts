import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  MESSAGE_METADATA_RESPONSE,
  installMetadataMetricRoutes,
  messageActionsRow,
  seedCompletedMetadataTurn,
} from "../../helpers/message-metadata-line-fixtures";

const SETTINGS_PATH = "/settings/preferences/task-behavior?tab=conversation";
const DEFAULT_FIELDS = ["session_config", "timestamp", "duration"];

test.describe("message metadata settings editor", () => {
  test.beforeEach(async ({ apiClient }) => {
    await apiClient.saveUserSettings({ message_metadata_fields: DEFAULT_FIELDS });
  });

  test("saves toggled fields, persists across reload, and shows them in the line", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const fixture = await seedCompletedMetadataTurn(apiClient, seedData, "Metadata settings line");
    await testPage.goto(SETTINGS_PATH);
    const card = testPage.getByTestId("message-metadata-settings");
    const ttft = card.getByRole("checkbox", { name: "TTFT" });
    const timestamp = card.getByRole("checkbox", { name: "Timestamp" });
    await expect(timestamp).toBeChecked();
    await expect(ttft).not.toBeChecked();

    await ttft.check();
    await timestamp.uncheck();
    // Nothing is persisted until Save is pressed.
    expect((await apiClient.getUserSettings()).settings.message_metadata_fields).toEqual(
      DEFAULT_FIELDS,
    );
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.message_metadata_fields)
      .toEqual(["session_config", "duration", "ttft"]);

    await testPage.reload();
    const reloadedCard = testPage.getByTestId("message-metadata-settings");
    await expect(reloadedCard.getByRole("checkbox", { name: "TTFT" })).toBeChecked();
    await expect(reloadedCard.getByRole("checkbox", { name: "Timestamp" })).not.toBeChecked();

    await installMetadataMetricRoutes(testPage, fixture);
    await testPage.goto(`/t/${fixture.taskId}`);
    await new SessionPage(testPage).waitForLoad();
    const responseRow = messageActionsRow(testPage, MESSAGE_METADATA_RESPONSE);
    await expect(responseRow.getByTestId("message-metadata-ttft")).toBeVisible();
    await expect(responseRow.getByTestId("message-metadata-session-config")).toBeVisible();
    await expect(responseRow.getByTestId("message-metadata-timestamp")).toHaveCount(0);
  });

  test("keeps the stored selection and the dirty draft when the save fails", async ({
    testPage,
    apiClient,
  }) => {
    await testPage.route("**/api/v1/user/settings", async (route) => {
      if (route.request().method() !== "PATCH") return route.continue();
      await route.fulfill({ status: 500, json: { error: "forced save failure" } });
    });
    await testPage.goto(SETTINGS_PATH);
    const ttft = testPage
      .getByTestId("message-metadata-settings")
      .getByRole("checkbox", { name: "TTFT" });
    await ttft.check();
    const savePanel = testPage.getByTestId("settings-floating-save");
    await savePanel.getByRole("button", { name: "Save changes" }).click();

    await expect(savePanel).toContainText("Couldn't save");
    await expect(ttft).toBeChecked();
    await expect(ttft).toHaveAttribute("data-settings-dirty", "true");
    expect((await apiClient.getUserSettings()).settings.message_metadata_fields).toEqual(
      DEFAULT_FIELDS,
    );

    await testPage.unroute("**/api/v1/user/settings");
    await testPage.reload();
    await expect(
      testPage.getByTestId("message-metadata-settings").getByRole("checkbox", { name: "TTFT" }),
    ).not.toBeChecked();
  });
});
