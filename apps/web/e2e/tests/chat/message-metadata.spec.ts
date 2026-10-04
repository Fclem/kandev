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

const DEFAULT_FIELDS = ["session_config", "timestamp", "duration"];
const ALL_FIELDS = [...DEFAULT_FIELDS, "ttft", "tps", "tokens", "cost"];

async function expectItemOrder(row: ReturnType<typeof messageActionsRow>, testIds: string[]) {
  const order = await row.evaluate((element, ids) => {
    const items = ids.map((id) => element.querySelector(`[data-testid="${id}"]`));
    return {
      allPresent: items.every(Boolean),
      inOrder: items.every(
        (item, index) =>
          index === items.length - 1 ||
          Boolean(
            item &&
            items[index + 1] &&
            item.compareDocumentPosition(items[index + 1]!) & Node.DOCUMENT_POSITION_FOLLOWING,
          ),
      ),
    };
  }, testIds);
  expect(order).toEqual({ allPresent: true, inOrder: true });
}

test.describe("chat message metadata line", () => {
  test("keeps the default line without an extra turn-usage read", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const fixture = await seedCompletedMetadataTurn(apiClient, seedData, "Metadata default line");
    await apiClient.saveUserSettings({ message_metadata_fields: DEFAULT_FIELDS });
    const usageRequests = await installMetadataMetricRoutes(testPage, fixture);

    await testPage.goto(`/t/${fixture.taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const promptRow = userMessageActionsRow(testPage, MESSAGE_METADATA_PROMPT);
    const responseRow = messageActionsRow(testPage, MESSAGE_METADATA_RESPONSE);
    await expect(promptRow.getByTestId("message-metadata-timestamp")).toBeVisible();
    await expect(promptRow.getByTestId("message-turn-duration")).toBeVisible();
    await expect(responseRow.getByTestId("message-metadata-session-config")).toHaveText(
      "gpt-5.6-sol",
    );
    await expect(responseRow.getByTestId("message-metadata-timestamp")).toBeVisible();
    await expect(responseRow.getByTestId("message-metadata-ttft")).toHaveCount(0);
    await expect.poll(() => usageRequests()).toBe(1);
  });

  test("updates both open clients, renders selected fields in order, and persists an empty selection", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const fixture = await seedCompletedMetadataTurn(apiClient, seedData, "Metadata selected line");
    const usageRequestsA = await installMetadataMetricRoutes(testPage, fixture);
    await testPage.goto(`/t/${fixture.taskId}`);
    const sessionA = new SessionPage(testPage);
    await sessionA.waitForLoad();

    const secondPage = await testPage.context().newPage();
    const usageRequestsB = await installMetadataMetricRoutes(secondPage, fixture);
    await secondPage.goto(`/t/${fixture.taskId}`);
    const sessionB = new SessionPage(secondPage);
    await sessionB.waitForLoad();
    await expect.poll(() => usageRequestsA()).toBe(1);
    await expect.poll(() => usageRequestsB()).toBe(1);

    await apiClient.saveUserSettings({ message_metadata_fields: ALL_FIELDS });

    const promptRowA = userMessageActionsRow(testPage, MESSAGE_METADATA_PROMPT);
    const responseRowA = messageActionsRow(testPage, MESSAGE_METADATA_RESPONSE);
    const promptRowB = userMessageActionsRow(secondPage, MESSAGE_METADATA_PROMPT);
    const responseRowB = messageActionsRow(secondPage, MESSAGE_METADATA_RESPONSE);
    for (const row of [promptRowA, promptRowB]) {
      await expect(row.getByTestId("message-metadata-timestamp")).toBeVisible();
      await expect(row.getByTestId("message-turn-duration")).toBeVisible();
      await expectItemOrder(row, ["message-metadata-timestamp", "message-turn-duration"]);
    }
    for (const row of [responseRowA, responseRowB]) {
      await expect(row.getByTestId("message-metadata-session-config")).toBeVisible();
      await expect(row.getByTestId("message-metadata-timestamp")).toBeVisible();
      await expect(row.getByTestId("message-metadata-ttft")).toBeVisible();
      await expect(row.getByTestId("message-metadata-tps")).toBeVisible();
      await expect(row.getByTestId("message-metadata-tokens")).toContainText("Input 1,200");
      await expect(row.getByTestId("message-metadata-cost")).toContainText("$0.01");
      await expectItemOrder(row, [
        "message-metadata-session-config",
        "message-metadata-timestamp",
        "message-metadata-ttft",
        "message-metadata-tps",
        "message-metadata-tokens",
        "message-metadata-cost",
      ]);
    }
    const ttftTrigger = responseRowA.getByTestId("message-metadata-ttft");
    await ttftTrigger.hover();
    const ttftTooltip = testPage.getByRole("tooltip");
    await expect(ttftTooltip).toContainText("turn_started");
    await ttftTrigger.focus();
    await expect(ttftTooltip).toBeVisible();
    await expect.poll(() => usageRequestsA()).toBe(2);
    await expect.poll(() => usageRequestsB()).toBe(2);

    await apiClient.saveUserSettings({ message_metadata_fields: [] });
    for (const row of [promptRowA, responseRowA, promptRowB, responseRowB]) {
      await expect(row.getByTestId("message-metadata-session-config")).toHaveCount(0);
      await expect(row.getByTestId("message-metadata-timestamp")).toHaveCount(0);
      await expect(row.getByTestId("message-turn-duration")).toHaveCount(0);
      await expect(row.getByTestId("message-metadata-ttft")).toHaveCount(0);
      await expect(row.getByTestId("message-metadata-tps")).toHaveCount(0);
      await expect(row.getByTestId("message-metadata-tokens")).toHaveCount(0);
      await expect(row.getByTestId("message-metadata-cost")).toHaveCount(0);
    }

    await testPage.reload();
    await sessionA.waitForLoad();
    const reloadedResponseRow = messageActionsRow(testPage, MESSAGE_METADATA_RESPONSE);
    await expect(reloadedResponseRow.getByTestId("message-metadata-timestamp")).toHaveCount(0);
    await expect(reloadedResponseRow.getByTestId("message-metadata-ttft")).toHaveCount(0);
    const settings = await apiClient.getUserSettings();
    expect(settings.settings.message_metadata_fields).toEqual([]);
  });

  test("shows live TTFT for a slow mock turn without reloading", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await apiClient.saveUserSettings({ message_metadata_fields: ["ttft"] });
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Live metadata timing",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    let mainNavigations = 0;
    testPage.on("framenavigated", (frame) => {
      if (frame === testPage.mainFrame()) mainNavigations += 1;
    });
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    const navigationsBeforeTurn = mainNavigations;
    await session.sendMessage("/slow 2s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    const slowResponse = session.chat.getByText("Running slow response (2s total)...", {
      exact: true,
    });
    await expect(slowResponse).toBeVisible({ timeout: 20_000 });

    const progressRow = messageActionsRow(testPage, "Running slow response (2s total)...");
    await expect(session.agentStatus()).toBeVisible();
    await expect(progressRow.getByTestId("message-metadata-ttft")).toHaveCount(0);
    const responseRow = messageActionsRow(testPage, "Slow response complete after 2s.");
    await expect(responseRow.getByTestId("message-metadata-ttft")).toBeVisible({
      timeout: 20_000,
    });
    await expect(responseRow.getByTestId("message-metadata-tps")).toHaveCount(0);
    expect(mainNavigations).toBe(navigationsBeforeTurn);
  });
});
