import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  installMetadataMetricRoutes,
  MESSAGE_METADATA_PROMPT,
  MESSAGE_METADATA_RESPONSE,
  messageActionsRow,
  userMessageActionsRow,
  seedCompletedMetadataTurn,
} from "../../helpers/message-metadata-line-fixtures";

const ALL_FIELDS = ["session_config", "timestamp", "duration", "ttft", "tps", "tokens", "cost"];

test.describe("chat message metadata line on mobile", () => {
  test("wraps selected metadata without horizontal document overflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const fixture = await seedCompletedMetadataTurn(apiClient, seedData, "Mobile metadata line");
    await apiClient.saveUserSettings({ message_metadata_fields: ALL_FIELDS });
    await installMetadataMetricRoutes(testPage, fixture);

    await testPage.goto(`/t/${fixture.taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const promptRow = userMessageActionsRow(testPage, MESSAGE_METADATA_PROMPT);
    const responseRow = messageActionsRow(testPage, MESSAGE_METADATA_RESPONSE);
    await expect(promptRow.getByTestId("message-metadata-timestamp")).toBeVisible();
    await expect(responseRow.getByTestId("message-metadata-session-config")).toBeVisible();
    await expect(responseRow.getByTestId("message-metadata-timestamp")).toBeVisible();
    await expect(promptRow.getByTestId("message-turn-duration")).toBeVisible();
    for (const field of ["ttft", "tps", "tokens", "cost"]) {
      await expect(responseRow.getByTestId(`message-metadata-${field}`)).toBeVisible();
    }

    const wrapped = await responseRow.evaluate((row) => {
      const ids = [
        "message-metadata-session-config",
        "message-metadata-timestamp",
        "message-metadata-ttft",
        "message-metadata-tps",
        "message-metadata-tokens",
        "message-metadata-cost",
      ];
      const rows = ids.map((id) => {
        const item = row.querySelector(`[data-testid="${id}"]`);
        return item ? Math.round(item.getBoundingClientRect().top) : null;
      });
      return {
        wraps: new Set(rows.filter((top): top is number => top !== null)).size > 1,
        documentFits: document.documentElement.scrollWidth <= window.innerWidth,
      };
    });
    expect(wrapped.wraps).toBe(true);
    expect(wrapped.documentFits).toBe(true);
  });
});
