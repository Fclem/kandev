import { expect, type Page, type TestInfo } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";
import type { ApiClient } from "./api-client";
import { seedIdleSession } from "./session";

type ProviderLimitScenario = {
  page: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  backend: BackendContext;
  phone: boolean;
  testInfo: TestInfo;
};

export async function exerciseProviderLimitAutomaticLaunchWait({
  page,
  apiClient,
  seedData,
  backend,
  phone,
  testInfo,
}: ProviderLimitScenario): Promise<void> {
  await backend.restart({
    KANDEV_MOCK_PROVIDERS: "claude-acp",
    KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING: "false",
  });
  const { agents } = await apiClient.listAgents();
  const family = agents.find((agent) => agent.name === "claude-acp");
  if (!family) throw new Error("canonical Claude mock provider was not registered");
  const profile = await apiClient.createAgentProfile(family.id, "Provider limit automatic launch", {
    model: "mock-fast",
    limit_fallback: false,
    resume_after_reset: true,
    auto_fallback: false,
    require_exact_model: true,
  });
  const limitedSession = await seedIdleSession(
    page,
    apiClient,
    { ...seedData, agentProfileId: profile.id },
    "Provider limit automatic launch mark",
  );
  if (phone) await limitedSession.sendMessageViaButton("/provider-quota mock-fast 3600");
  else await limitedSession.sendMessage("/provider-quota mock-fast 3600");
  await expect
    .poll(
      async () => {
        const response = await apiClient.rawRequest("GET", "/api/v1/agent-profiles/limits");
        if (!response.ok) return false;
        const limits = (await response.json()) as Array<{ profile_id: string; model: string }>;
        return limits.some(
          (limit) => limit.profile_id === profile.id && limit.model === "mock-fast",
        );
      },
      { timeout: 30_000 },
    )
    .toBe(true);

  const workflow = await apiClient.createWorkflow(
    seedData.workspaceId,
    "Provider limit auto-start",
  );
  const waitingStep = await apiClient.createWorkflowStep(workflow.id, "Waiting", 0, {
    is_start_step: true,
  });
  const launch = await apiClient.createWorkflowStep(workflow.id, "Implement", 1, {
    agent_profile_id: profile.id,
    events: { on_enter: [{ type: "auto_start_agent" }] },
  });
  const task = await apiClient.createTask(seedData.workspaceId, "Provider limit auto-start wait", {
    workflow_id: workflow.id,
    workflow_step_id: waitingStep.id,
    agent_profile_id: profile.id,
    executor_profile_id: seedData.worktreeExecutorProfileId,
    repository_ids: [seedData.repositoryId],
    description: "Wait for the active provider reset before starting.",
  });
  await apiClient.moveTask(task.id, workflow.id, launch.id);
  try {
    await expect
      .poll(
        async () => {
          const current = await apiClient.getTask(task.id);
          const deferred = current.metadata?.deferred_launch as Record<string, unknown> | undefined;
          return deferred?.provider_limit_deferred === true && current.state === "SCHEDULING";
        },
        { timeout: 30_000 },
      )
      .toBe(true);
  } catch (error) {
    const failedTask = await apiClient.getTask(task.id);
    await testInfo.attach("provider-limit-auto-start-task", {
      body: JSON.stringify(failedTask),
      contentType: "application/json",
    });
    await testInfo.attach("provider-limit-auto-start-backend-log", {
      path: backend.logPath,
      contentType: "text/plain",
    });
    throw new Error(
      `Automatic launch wait was not persisted: ${JSON.stringify({
        state: failedTask.state,
        metadata: failedTask.metadata,
      })}`,
      { cause: error },
    );
  }
  await page.goto(`/t/${task.id}`);
  const waiting = page.getByText(/Waiting for mock-fast limit reset at/);
  await expect(waiting).toBeVisible({ timeout: 15_000 });
  if (phone) await assertNoDocumentHorizontalOverflow(page, "provider limit automatic launch wait");
  await page.screenshot({
    path: testInfo.outputPath(`auto-start-wait-${phone ? "phone" : "desktop"}.png`),
  });
}
export async function exerciseProviderLimitManualNotice({
  page,
  apiClient,
  seedData,
  backend,
  phone,
}: ProviderLimitScenario): Promise<void> {
  await backend.restart({
    KANDEV_MOCK_PROVIDERS: "claude-acp",
    KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING: "false",
  });
  const { agents } = await apiClient.listAgents();
  const family = agents.find((agent) => agent.name === "claude-acp");
  if (!family) throw new Error("canonical Claude mock provider was not registered");
  const profile = await apiClient.createAgentProfile(family.id, "Provider limit manual notice", {
    model: "mock-fast",
    fallback_model: "mock-slow",
    limit_fallback: true,
    resume_after_reset: false,
    auto_fallback: false,
    require_exact_model: true,
  });
  const session = await seedIdleSession(
    page,
    apiClient,
    { ...seedData, agentProfileId: profile.id },
    "Provider limit manual notice",
  );
  const taskId = new URL(page.url()).pathname.split("/").at(-1);
  if (!taskId) throw new Error("seeded task ID is missing from the task URL");
  const before = await apiClient.getTask(taskId);
  if (!before.primary_session_id) throw new Error("seeded task has no active session");
  if (phone) await session.sendMessageViaButton("/provider-quota mock-fast 3600");
  else await session.sendMessage("/provider-quota mock-fast 3600");
  await session.waitForChatIdle({ timeout: 30_000 });
  if (phone) await session.sendMessageViaButton("Continue the existing task.");
  else await session.sendMessage("Continue the existing task.");
  await session.waitForChatIdle({ timeout: 30_000 });
  await expect(session.chat).toContainText("mock-fast is limited.");
  await expect(session.chat).toContainText("Sending anyway.");
  const after = await apiClient.getTask(taskId);
  expect(after.primary_session_id).toBe(before.primary_session_id);
  await page.reload();
  await session.waitForLoad();
  await expect(session.chat).toContainText("Continue the existing task.");
  if (phone) await assertNoDocumentHorizontalOverflow(page, "provider limit manual notice");
}

export async function exerciseProviderLimitWait({
  page,
  apiClient,
  seedData,
  backend,
  phone,
  testInfo,
}: ProviderLimitScenario): Promise<void> {
  await backend.restart({
    KANDEV_MOCK_PROVIDERS: "claude-acp",
    KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING: "false",
  });
  const { agents } = await apiClient.listAgents();
  const family = agents.find((agent) => agent.name === "claude-acp");
  if (!family) throw new Error("canonical Claude mock provider was not registered");
  const profile = await apiClient.createAgentProfile(family.id, "Provider limit reset wait", {
    model: "mock-fast",
    limit_fallback: false,
    resume_after_reset: true,
    auto_fallback: false,
    require_exact_model: true,
  });
  const session = await seedIdleSession(
    page,
    apiClient,
    { ...seedData, agentProfileId: profile.id },
    "Provider limit reset wait",
  );
  const taskId = new URL(page.url()).pathname.split("/").at(-1);
  if (!taskId) throw new Error("seeded task ID is missing from the task URL");
  if (phone) await session.sendMessageViaButton("/provider-quota mock-fast 3600");
  else await session.sendMessage("/provider-quota mock-fast 3600");
  await session.waitForChatIdle({ timeout: 30_000 });
  const card = page.getByTestId("provider-limit-wait-card");
  await expect(card).toBeVisible({ timeout: 30_000 });
  await expect(card.getByRole("time")).toHaveAttribute("datetime", /^\d{4}-/);
  if (phone) {
    await assertNoDocumentHorizontalOverflow(page, "provider limit reset wait");
    const cancelBounds = await card.getByTestId("recovery-cancel-retry-button").boundingBox();
    expect(cancelBounds?.height).toBeGreaterThanOrEqual(44);
  }
  await page.reload();
  await session.waitForLoad();
  await expect(page.getByTestId("provider-limit-wait-card")).toBeVisible({ timeout: 15_000 });
  await page.getByTestId("recovery-cancel-retry-button").click();
  await expect(page.getByTestId("provider-limit-wait-card")).toBeHidden({ timeout: 15_000 });
  await testInfo.attach("provider-limit-wait-task", {
    body: JSON.stringify(await apiClient.getTask(taskId)),
  });
  const resumeProfile = await apiClient.createAgentProfile(
    family.id,
    "Provider limit exact reset",
    {
      model: "mock-fast",
      limit_fallback: false,
      resume_after_reset: true,
      auto_fallback: false,
      require_exact_model: true,
    },
  );
  const resumedSession = await seedIdleSession(
    page,
    apiClient,
    { ...seedData, agentProfileId: resumeProfile.id },
    "Provider limit exact reset",
  );
  const resumedTaskId = new URL(page.url()).pathname.split("/").at(-1);
  if (!resumedTaskId) throw new Error("resumed task ID is missing from the task URL");
  const beforeResume = await apiClient.getTask(resumedTaskId);
  if (!beforeResume.primary_session_id) throw new Error("resumed task has no active session");
  if (phone) await resumedSession.sendMessageViaButton("/provider-quota mock-fast 15");
  else await resumedSession.sendMessage("/provider-quota mock-fast 15");
  const resumedCard = page.getByTestId("provider-limit-wait-card");
  await expect(resumedCard).toBeVisible({ timeout: 30_000 });
  await expect(resumedCard.getByRole("time")).toHaveAttribute("datetime", /^\d{4}-/);
  await expect(resumedCard).toBeHidden({ timeout: 45_000 });
  await resumedSession.waitForChatIdle({ timeout: 45_000 });
  const afterResume = await apiClient.getTask(resumedTaskId);
  expect(afterResume.primary_session_id).toBe(beforeResume.primary_session_id);
  await expect(resumedSession.chat).toContainText(
    "This is a simple mock response for e2e testing.",
  );
}
