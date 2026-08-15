// Mobile companion to message-metadata-overflow.spec.ts. The metadata dialog
// is a centered modal on every viewport; on a phone the same large
// `turn_metadata` must stay reachable through the dialog's internal entries
// scroll, with the document itself never overflowing horizontally.
import { test, expect, type Locator, type Page } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const SEEDED_MESSAGE = "Mobile message metadata overflow fixture";

/** 40 config options make the runtime snapshot tall enough to overflow. */
function largeTurnMetadata(): Record<string, unknown> {
  return {
    runtime_config_snapshot: {
      config_baseline: {
        mode: "default",
        model: "anthropic/claude-sonnet-5",
        thinking: "auto",
      },
      config_options: Array.from({ length: 40 }, (_, i) => ({
        id: `opt_${i}`,
        name: `Option ${i}`,
        value: `v${i}`,
        value_name: `Value ${i}`,
      })),
    },
    prompt_usage: { input_tokens: 1234, output_tokens: 5678 },
    agent_id: "agent-123",
    agent_type: "task",
  };
}

async function openMetadataDialog(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<{
  dialog: Locator;
  entries: Locator;
  turnLabel: Locator;
}> {
  const task = await apiClient.createTask(seedData.workspaceId, "Mobile Metadata Overflow", {
    description: "seeded mobile message metadata overflow fixture",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "IDLE",
  });
  await apiClient.seedSessionMessage(sessionId, {
    type: "message",
    content: SEEDED_MESSAGE,
    turnMetadata: largeTurnMetadata(),
  });

  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();

  const chat = session.activeChat();
  await expect(chat.getByText(SEEDED_MESSAGE)).toBeVisible({ timeout: 15_000 });

  const messageBody = chat
    .locator("[data-agent-message-body][data-message-id]")
    .filter({ hasText: SEEDED_MESSAGE });
  const actionsRow = messageBody.locator("xpath=..");

  await actionsRow.getByRole("button", { name: "Show message metadata" }).tap();

  const dialog = page.locator('[data-slot="dialog-content"]');
  await expect(dialog).toBeVisible();
  const entries = dialog.locator("> div.grid").first();
  await expect(entries).toBeVisible();
  return { dialog, entries, turnLabel: dialog.getByText("turn_metadata", { exact: true }) };
}

test.describe("Chat message metadata dialog overflow (mobile)", () => {
  test("scrolls entries internally with no document horizontal overflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { dialog, entries, turnLabel } = await openMetadataDialog(testPage, apiClient, seedData);

    // The entries area must be scrollable on the phone viewport too.
    const scrollable = await entries.evaluate((el) => {
      const container = el as HTMLElement;
      return { scrollHeight: container.scrollHeight, clientHeight: container.clientHeight };
    });
    expect(scrollable.scrollHeight).toBeGreaterThan(scrollable.clientHeight);

    await entries.evaluate((el) => {
      (el as HTMLElement).scrollTop = (el as HTMLElement).scrollHeight;
    });

    const dialogBox = await dialog.boundingBox();
    const turnBox = await turnLabel.boundingBox();
    expect(dialogBox).not.toBeNull();
    expect(turnBox).not.toBeNull();
    expect(turnBox!.y).toBeGreaterThanOrEqual(dialogBox!.y);
    expect(turnBox!.y + turnBox!.height).toBeLessThanOrEqual(dialogBox!.y + dialogBox!.height);

    // The dialog's internal scroll must never spill into document overflow.
    const documentOverflow = await testPage.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
    }));
    expect(documentOverflow.scrollWidth).toBeLessThanOrEqual(documentOverflow.clientWidth);
  });
});
