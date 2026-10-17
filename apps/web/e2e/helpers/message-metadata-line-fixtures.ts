import type { Page, Locator } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";

export const MESSAGE_METADATA_PROMPT = "Message metadata line prompt";
export const MESSAGE_METADATA_RESPONSE = "Message metadata line response";
export const MESSAGE_METADATA_STARTED_AT = "2026-09-24T10:00:00.000Z";
export const MESSAGE_METADATA_COMPLETED_AT = "2026-09-24T10:00:02.000Z";

export async function seedCompletedMetadataTurn(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<{ taskId: string; sessionId: string; turnId: string }> {
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    description: "Seeded conversation for message metadata line E2E.",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, { state: "IDLE" });
  const prompt = await apiClient.seedSessionMessage(sessionId, {
    type: "message",
    authorType: "user",
    content: MESSAGE_METADATA_PROMPT,
    newTurn: true,
    turnStartedAt: MESSAGE_METADATA_STARTED_AT,
    createdAt: MESSAGE_METADATA_STARTED_AT,
    turnMetadata: { runtime_config_snapshot: { model: "gpt-5.6-sol" } },
  });
  if (!prompt.turnId) throw new Error("Seeded prompt did not create a turn");
  await apiClient.seedSessionMessage(sessionId, {
    type: "message",
    content: MESSAGE_METADATA_RESPONSE,
    createdAt: MESSAGE_METADATA_COMPLETED_AT,
  });
  await apiClient.completeSessionTurn(prompt.turnId);
  return { taskId: task.id, sessionId, turnId: prompt.turnId };
}

function metricTurn(turnId: string, sessionId: string, taskId: string) {
  return {
    id: turnId,
    session_id: sessionId,
    task_id: taskId,
    created_at: MESSAGE_METADATA_STARTED_AT,
    started_at: MESSAGE_METADATA_STARTED_AT,
    completed_at: MESSAGE_METADATA_COMPLETED_AT,
    updated_at: new Date(Date.now() + 60_000).toISOString(),
    metadata: {
      runtime_config_snapshot: { model: "gpt-5.6-sol" },
      timing: { ttft_ms: 350, generation_ms: 2000 },
    },
  };
}

function metricUsageTurn(turnId: string) {
  const direct = {
    input_tokens: 1200,
    input_complete: true,
    cached_read_tokens: 600,
    cached_write_tokens: 0,
    cached_read_tokens_present: true,
    cached_write_tokens_present: false,
    output_tokens: 42,
    thought_tokens: 0,
    total_tokens: 1242,
    output_complete: true,
    event_count: 1,
    unpriced_count: 0,
    cost_subcents: "120",
    overflow: false,
  };
  const empty = {
    input_tokens: 0,
    input_complete: true,
    cached_read_tokens: 0,
    cached_write_tokens: 0,
    cached_read_tokens_present: false,
    cached_write_tokens_present: false,
    output_tokens: 0,
    thought_tokens: 0,
    total_tokens: 0,
    output_complete: true,
    event_count: 0,
    unpriced_count: 0,
    overflow: false,
  };
  return {
    turn_id: turnId,
    completeness: "exact",
    direct,
    child: empty,
    total: direct,
    cost_sources: ["models_dev_list"],
  };
}

export async function installMetadataMetricRoutes(
  page: Page,
  fixture: { taskId: string; sessionId: string; turnId: string },
): Promise<() => number> {
  let usageRequests = 0;
  await page.route(`**/api/v1/task-sessions/${fixture.sessionId}/turns`, async (route) => {
    if (route.request().method() !== "GET") return route.continue();
    await route.fulfill({
      json: { turns: [metricTurn(fixture.turnId, fixture.sessionId, fixture.taskId)], total: 1 },
    });
  });
  await page.route(
    `**/api/v1/tasks/${fixture.taskId}/sessions/${fixture.sessionId}/usage/turns**`,
    async (route) => {
      usageRequests += 1;
      if (route.request().method() !== "GET") return route.continue();
      await route.fulfill({
        json: {
          session_id: fixture.sessionId,
          turns: [metricUsageTurn(fixture.turnId)],
        },
      });
    },
  );
  return () => usageRequests;
}

export function messageActionsRow(page: Page, content: string): Locator {
  return page
    .locator("[data-agent-message-body][data-message-id]")
    .filter({ hasText: content })
    .locator("xpath=..");
}

export function userMessageActionsRow(page: Page, content: string): Locator {
  return page.getByTestId("user-message-bubble").filter({ hasText: content }).locator("xpath=..");
}
