import type { Page } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";
import { z } from "zod";

export async function installEffortFixture(page: Page, api: ApiClient, workspaceId: string) {
  await installFixturePlugin(page);
  const context = await api.rawRequest(
    "GET",
    `/api/plugins/${PLUGIN_ID}/capability-approvals?workspace_id=${workspaceId}`,
  );
  expect(context.ok).toBe(true);
  const approval = z.object({ manifest_digest: z.string() }).parse(await context.json());
  const saved = await api.rawRequest("PUT", `/api/plugins/${PLUGIN_ID}/capability-approvals`, {
    workspace_id: workspaceId,
    expected_revision: 0,
    manifest_digest: approval.manifest_digest,
    capability_ids: ["host.v2.read:change_requests", "host.v2.read:usage"],
    reason: "Read effort signals in the fixture",
    audit_id: `effort-${Date.now()}`,
  });
  expect(saved.ok).toBe(true);
}
export async function readEffort(
  api: ApiClient,
  workspaceId: string,
  taskId: string,
): Promise<{ active_ms: number }> {
  const response = await api.rawRequest("POST", `/api/plugins/${PLUGIN_ID}/actions/summary`, {
    workspaceId,
    body: { task_id: taskId },
  });
  expect(response.ok, await response.clone().text()).toBe(true);
  return z.object({ active_ms: z.number() }).parse(await response.json());
}
export async function checkEffortRoute(page: Page, taskId: string, touch = false) {
  await page.goto(`/plugins/e2e-effort?task_id=${taskId}`);
  if (touch) await page.getByTestId("effort-summary").tap();
  else await page.getByTestId("effort-summary").click();
  await expect(page.getByTestId("effort-result")).toBeVisible();
  await expect(page.getByTestId("effort-provider")).toHaveCount(2);
  await expect(
    page.getByTestId("effort-provider").filter({ hasText: "github: available" }),
  ).toBeVisible();
  await expect(page.getByTestId("effort-feedback-item")).toHaveText("Fixture review feedback");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
}

export async function seedEffortFeedback(
  api: ApiClient,
  workspaceId: string,
  repositoryId: string,
  taskId: string,
) {
  await api.mockGitHubSetWorkspaceConnection(workspaceId, {
    source: "legacy_shared",
    status: "active",
  });
  await api.mockGitHubAssociateTaskPR({
    task_id: taskId,
    workspace_id: workspaceId,
    repository_id: repositoryId,
    owner: "effort",
    repo: "fixture",
    pr_number: 42,
    pr_url: "https://github.com/effort/fixture/pull/42",
    pr_title: "Effort fixture review",
    head_branch: "effort-fixture",
    base_branch: "main",
    author_login: "fixture",
  });
  await api.mockGitHubSeedPRFeedback({
    owner: "effort",
    repo: "fixture",
    pr_number: 42,
    reviews: [
      {
        id: 7,
        author: "reviewer",
        state: "APPROVED",
        body: "Fixture review feedback",
        created_at: "2026-10-03T12:00:00Z",
      },
    ],
  });
}
