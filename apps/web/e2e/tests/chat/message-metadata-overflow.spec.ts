// Regression guard: the chat message "Message Metadata" dialog must scroll
// its entries when they exceed the dialog's height cap instead of clipping
// the tail. The reported bug: `turn_metadata` (the last of ten debug fields,
// carrying a large `runtime_config_snapshot`) rendered below the dialog's
// visible area with no scrollbar anywhere (see message-actions.tsx).
import { test, expect, type Locator, type Page } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const SEEDED_MESSAGE = "Message metadata overflow fixture";

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

async function seedOverflowMessage(
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<{ taskId: string; sessionId: string }> {
  const task = await apiClient.createTask(seedData.workspaceId, "Metadata Overflow", {
    description: "seeded message metadata overflow fixture",
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
  return { taskId: task.id, sessionId };
}

/** Opens the metadata dialog for the seeded message and returns its parts. */
async function openMetadataDialog(
  page: Page,
  taskId: string,
): Promise<{
  dialog: Locator;
  entries: Locator;
  turnLabel: Locator;
  title: Locator;
}> {
  await page.goto(`/t/${taskId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();

  const chat = session.activeChat();
  await expect(chat.getByText(SEEDED_MESSAGE)).toBeVisible({ timeout: 15_000 });

  // Scope to the seeded message's own action row (mirrors the favorite spec).
  const messageBody = chat
    .locator("[data-agent-message-body][data-message-id]")
    .filter({ hasText: SEEDED_MESSAGE });
  const actionsRow = messageBody.locator("xpath=..");

  await actionsRow.getByRole("button", { name: "Show message metadata" }).click();

  const dialog = page.locator('[data-slot="dialog-content"]');
  await expect(dialog).toBeVisible();
  // The entries container is the dialog's only direct `div.grid` child (the
  // header carries data-slot="dialog-header").
  const entries = dialog.locator("> div.grid").first();
  await expect(entries).toBeVisible();
  return {
    dialog,
    entries,
    turnLabel: dialog.getByText("turn_metadata", { exact: true }),
    title: dialog.getByText("Message Metadata", { exact: true }),
  };
}

test.describe("Chat message metadata dialog overflow", () => {
  test("scrolls the entries area and keeps turn_metadata reachable", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { taskId } = await seedOverflowMessage(apiClient, seedData);
    const { dialog, entries, turnLabel, title } = await openMetadataDialog(testPage, taskId);

    // RED assertion: the entries area must actually be scrollable. The bug
    // made scrollHeight == clientHeight (grid auto-rows grow to content), so
    // the tail was clipped by the dialog's overflow-hidden with no scroller.
    const scrollable = await entries.evaluate((el) => {
      const container = el as HTMLElement;
      return { scrollHeight: container.scrollHeight, clientHeight: container.clientHeight };
    });
    expect(scrollable.scrollHeight).toBeGreaterThan(scrollable.clientHeight);

    // Scroll the entries area to the bottom and prove the last field
    // (turn_metadata) is fully inside the dialog viewport.
    await entries.evaluate((el) => {
      (el as HTMLElement).scrollTop = (el as HTMLElement).scrollHeight;
    });

    const dialogBox = await dialog.boundingBox();
    const turnBox = await turnLabel.boundingBox();
    expect(dialogBox).not.toBeNull();
    expect(turnBox).not.toBeNull();
    expect(turnBox!.y).toBeGreaterThanOrEqual(dialogBox!.y);
    expect(turnBox!.y + turnBox!.height).toBeLessThanOrEqual(dialogBox!.y + dialogBox!.height);

    // The dialog title stays visible while the entries area scrolls.
    const titleBox = await title.boundingBox();
    expect(titleBox).not.toBeNull();
    expect(titleBox!.y).toBeGreaterThanOrEqual(dialogBox!.y);
    expect(titleBox!.y + titleBox!.height).toBeLessThanOrEqual(dialogBox!.y + dialogBox!.height);

    // The close control is an absolute sibling outside the entries scroller;
    // it must stay fully inside the dialog while the entries are scrolled to
    // the bottom, and it must still close the dialog.
    const close = dialog.locator('[data-slot="dialog-close"]');
    await expect(close).toBeVisible();
    const closeBox = await close.boundingBox();
    expect(closeBox).not.toBeNull();
    expect(closeBox!.y).toBeGreaterThanOrEqual(dialogBox!.y);
    expect(closeBox!.y + closeBox!.height).toBeLessThanOrEqual(dialogBox!.y + dialogBox!.height);

    await close.click();
    await expect(dialog).toBeHidden();
  });
});
