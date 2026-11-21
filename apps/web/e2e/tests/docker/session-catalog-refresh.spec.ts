import { execFileSync } from "node:child_process";
import { test, expect } from "../../fixtures/docker-test-base";
import { SessionPage } from "../../pages/session-page";
import { watchWs } from "../../helpers/causal-waits";
import { catalogRecord, refreshSavedSessionCatalog } from "../../helpers/session-catalog-refresh";
import type { MockACPTrace } from "../../helpers/transient-turn-runtime-continuity";
import type { ApiClient } from "../../helpers/api-client";
import type { SessionCatalogRecord } from "../../../lib/types/session-catalog";

function readContainerCatalogTrace(containerId: string, workspacePath: string): MockACPTrace[] {
  const contents = execFileSync(
    "docker",
    ["exec", containerId, "cat", `${workspacePath}/catalog-acp.jsonl`],
    { encoding: "utf8" },
  ).trim();
  return contents
    .split("\n")
    .filter(Boolean)
    .map((line) => JSON.parse(line) as MockACPTrace);
}

async function requireCatalogContainer(api: ApiClient, taskId: string) {
  const environment = await api.getTaskEnvironment(taskId);
  expect(environment?.executor_type).toBe("local_docker");
  if (!environment?.container_id || !environment.workspace_path)
    throw new Error("Docker catalog environment is incomplete");
  return environment;
}

function expectExecutorScopedNotice(record: SessionCatalogRecord | undefined) {
  expect(record).toMatchObject({
    disposition: "rediscovery_required",
    reason: "executor_scoped",
  });
  expect(record?.models ?? []).toEqual([]);
  expect(record?.modes ?? []).toEqual([]);
  expect(record?.catalog).toBeUndefined();
  expect(
    record?.source,
    "isolated executors never receive an automatic task probe",
  ).toBeUndefined();
  expect(record?.probe).toBeUndefined();
  expect(JSON.stringify(record)).not.toContain("host-only-new");
}

// @covers AC-AGENTS-SESSION-CATALOG-001.2, AC-AGENTS-SESSION-CATALOG-001.5, AC-AGENTS-SESSION-CATALOG-002.3, AC-AGENTS-SESSION-CATALOG-002.4, AC-AGENTS-SESSION-CATALOG-003.1
test("keeps host refresh notice-only on a real Docker executor and rediscovers through ordinary resume", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(180000);
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((candidate) => candidate.name === "mock-agent");
  if (!agent) throw new Error("Mock agent is required for Docker catalog refresh");
  const traceEnv = { key: "E2E_MOCK_AGENT_ACP_TRACE_FILE", value: "catalog-acp.jsonl" };
  const profile = await apiClient.createAgentProfile(agent.id, "Docker catalog isolation", {
    model: "mock-fast",
    mode: "default",
    auto_fallback: false,
    config_options: { effort: "high" },
    env_vars: [{ key: "MOCK_AGENT_PROFILE_CATALOG", value: "remote-before" }, traceEnv],
  });
  const wire = watchWs(testPage);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Docker catalog refresh",
    profile.id,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      executor_profile_id: seedData.dockerExecutorProfileId,
    },
  );
  if (!task.session_id) throw new Error("Docker catalog task has no session");
  const sessionId = task.session_id;
  try {
    await expect
      .poll(async () => (await apiClient.getTaskSession(sessionId)).session.state, {
        timeout: 60000,
      })
      .toBe("WAITING_FOR_INPUT");
    const before = (await apiClient.getTaskSession(sessionId)).session;
    const environment = await requireCatalogContainer(apiClient, task.id);
    const initialTrace = readContainerCatalogTrace(
      environment.container_id!,
      environment.workspace_path!,
    );
    const nativeId = initialTrace.find((event) => event.event === "prompt")?.session_id;
    expect(nativeId).toBeTruthy();
    const messages = (await apiClient.listSessionMessages(sessionId)).messages;
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const editor = await session.fillMessage("Keep the Docker conversation draft");
    await apiClient.updateAgentProfile(profile.id, {
      env_vars: [{ key: "MOCK_AGENT_PROFILE_CATALOG", value: "host-only-new" }, traceEnv],
    });
    await refreshSavedSessionCatalog(apiClient, profile.id);
    const published = (await apiClient.getTaskSession(sessionId)).session;
    expectExecutorScopedNotice(catalogRecord(published));
    expect(published.agent_execution_id).toBe(before.agent_execution_id);
    const model = testPage.getByRole("button", { name: "Session model settings", exact: true });
    await model.click();
    await expect(
      testPage.getByRole("option", { name: "Profile env remote-before", exact: true }),
    ).toBeVisible();
    await expect(
      testPage.getByRole("option", { name: "Profile env host-only-new", exact: true }),
    ).toHaveCount(0);
    await testPage.keyboard.press("Escape");
    const notice = testPage.getByTestId("session-catalog-refresh-notice");
    await expect(notice).toBeVisible();
    await testInfo.attach("docker-catalog-executor-notice", {
      body: await testPage.screenshot({
        path: testInfo.outputPath("docker-catalog-executor-notice.png"),
      }),
      contentType: "image/png",
    });
    await notice.getByRole("button", { name: "Refresh session", exact: true }).click();
    const stop = wire.waitForResponse("session.stop", {
      requestWhere: (payload) => payload.session_id === sessionId,
    });
    const launch = wire.waitForResponse("session.launch", {
      timeout: 60000,
      timeoutAfter: stop,
      requestWhere: (payload) => payload.session_id === sessionId && payload.intent === "resume",
    });
    await testPage
      .getByTestId("session-catalog-refresh-confirmation")
      .getByRole("button", { name: "Refresh session", exact: true })
      .click();
    await Promise.all([stop, launch]);
    await expect
      .poll(
        async () => catalogRecord((await apiClient.getTaskSession(sessionId)).session)?.disposition,
        { timeout: 60000 },
      )
      .toBe("cleared");
    const after = (await apiClient.getTaskSession(sessionId)).session;
    const resumedEnvironment = await requireCatalogContainer(apiClient, task.id);
    expect(resumedEnvironment.id).toBe(environment.id);
    expect(resumedEnvironment.executor_profile_id).toBe(seedData.dockerExecutorProfileId);
    expect(after.metadata?.runtime_config).toEqual(before.metadata?.runtime_config);
    expect(after.metadata?.acp_config_baseline).toEqual(before.metadata?.acp_config_baseline);
    await expect(editor).toContainText("Keep the Docker conversation draft");
    const retainedIds = new Set(
      (await apiClient.listSessionMessages(sessionId)).messages.map((message) => message.id),
    );
    for (const message of messages) expect(retainedIds.has(message.id)).toBe(true);
    const trace = readContainerCatalogTrace(
      resumedEnvironment.container_id!,
      resumedEnvironment.workspace_path!,
    );
    expect(trace.filter((event) => event.event === "session_new")).toHaveLength(1);
    expect(
      trace.filter((event) => event.event === "session_load").map((event) => event.session_id),
    ).toEqual([nativeId]);
    expect(trace.filter((event) => event.event === "prompt")).toHaveLength(1);
    await model.click();
    const advertised = testPage.getByRole("option", {
      name: "Profile env host-only-new",
      exact: true,
    });
    await expect(advertised).not.toHaveAttribute("aria-disabled", "true");
    await advertised.click();
    await expect
      .poll(
        async () => (await apiClient.getTaskSession(sessionId)).session.metadata?.runtime_config,
      )
      .toMatchObject({ model: "profile-env-host-only-new" });
    await testInfo.attach("docker-catalog-provider-rediscovered", {
      body: await testPage.screenshot({
        path: testInfo.outputPath("docker-catalog-provider-rediscovered.png"),
      }),
      contentType: "image/png",
    });
  } finally {
    await apiClient.deleteTask(task.id);
    await apiClient.deleteAgentProfile(profile.id, true);
  }
});
