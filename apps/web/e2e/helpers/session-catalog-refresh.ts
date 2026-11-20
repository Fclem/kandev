import fs from "node:fs";
import path from "node:path";
import { expect } from "@playwright/test";
import type { Page, TestInfo } from "@playwright/test";
import type { AgentProfile, TaskSession } from "../../lib/types/http";
import type { SessionCatalogRecord } from "../../lib/types/session-catalog";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import type { WsWatcher } from "./causal-waits";
import type { GatewayTrafficFrame } from "./ws-traffic";
import { watchWs } from "./causal-waits";
import { attachGatewayTrafficCapture } from "./ws-traffic";
import { readMockACPTrace } from "./transient-turn-runtime-continuity";
import { SessionPage } from "../pages/session-page";

export type CatalogVariant =
  | "baseline"
  | "new-choices"
  | "newer-choices"
  | "empty"
  | "mode-only"
  | "unsupported";
type WireChoice = { value: string; name: string };
type WireOption = {
  id: string;
  name: string;
  category: string;
  type: "select";
  currentValue: string;
  options: WireChoice[] | { group: string; name: string; options: WireChoice[] }[];
};

function catalogModelChoices(variant: CatalogVariant): WireOption["options"] {
  if (variant === "empty") return [];
  if (variant === "new-choices" || variant === "newer-choices")
    return [
      {
        group: "fresh-models",
        name: "Fresh models",
        options: [
          {
            value: variant === "newer-choices" ? "catalog-newer" : "catalog-new",
            name: variant === "newer-choices" ? "Catalog newer model" : "Catalog new model",
          },
          { value: "mock-fast", name: "Mock Fast" },
        ],
      },
    ];
  return [
    { value: "mock-fast", name: "Mock Fast" },
    { value: "mock-smart", name: "Mock Smart" },
  ];
}

function catalogModeChoices(variant: CatalogVariant) {
  if (variant === "empty") return [];
  if (variant === "new-choices" || variant === "newer-choices")
    return [
      { id: "default", name: "Default" },
      { id: "catalog-plan", name: "Catalog plan" },
    ];
  return [
    { id: "default", name: "Default" },
    { id: "plan-mock", name: "Plan Mock" },
  ];
}

export type SessionCatalogScenario = {
  taskId: string;
  sessionId: string;
  profile: AgentProfile;
  agentName: string;
  catalogPath: string;
  tracePath: string;
  nativeId: string;
  session: SessionPage;
  wire: WsWatcher;
  traffic: { frames: GatewayTrafficFrame[] };
  httpMutations: string[];
  releaseBackend: () => Promise<void>;
};

export type SessionCatalogBrowserContext = {
  page: Page;
  api: ApiClient;
  seed: SeedData;
  backend: BackendContext;
  touch: boolean;
  testInfo: TestInfo;
};

export function writeSessionCatalogFile(file: string, variant: CatalogVariant): void {
  if (variant === "unsupported") {
    fs.writeFileSync(file, "{}");
    return;
  }
  const configOptions: WireOption[] = [];
  if (variant !== "mode-only")
    configOptions.push({
      id: "model",
      name: "Model",
      category: "model",
      type: "select",
      currentValue: "mock-fast",
      options: catalogModelChoices(variant),
    });
  const modes = catalogModeChoices(variant);
  configOptions.push({
    id: "mode",
    name: "Mode",
    category: "mode",
    type: "select",
    currentValue: "default",
    options: modes.map((mode) => ({ value: mode.id, name: mode.name })),
  });
  configOptions.push({
    id: "effort",
    name: "Effort",
    category: "thought_level",
    type: "select",
    currentValue: "medium",
    options: [
      { value: "low", name: "Low" },
      { value: "medium", name: "Medium" },
      { value: "high", name: "High" },
    ],
  });
  fs.writeFileSync(
    file,
    JSON.stringify({ configOptions, modes: { currentModeId: "default", availableModes: modes } }),
  );
}

export async function createSessionCatalogScenario(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  backend: BackendContext,
): Promise<SessionCatalogScenario> {
  const releaseBackend = await backend.useEnv({ KANDEV_MOCK_GITLAB: "true" });
  try {
    const fixtureRoot = fs.mkdtempSync(path.join(backend.tmpDir, "session-catalog-"));
    const catalogPath = path.join(fixtureRoot, "catalog.json");
    const tracePath = path.join(fixtureRoot, "acp.jsonl");
    writeSessionCatalogFile(catalogPath, "baseline");
    fs.writeFileSync(tracePath, "");
    const host = "https://gitlab.catalog-fixture.invalid";
    const token = `catalog-fixture:${seed.workspaceId}`;
    await api.configureGitLab(seed.workspaceId, host, token);
    const { agents } = await api.listAgents();
    const agent = agents.find((candidate) => candidate.name === "mock-agent");
    if (!agent) throw new Error("Mock agent is required for catalog refresh");
    const profile = await api.createAgentProfile(agent.id, "Session catalog refresh", {
      model: "mock-fast",
      mode: "default",
      config_options: { effort: "high" },
      auto_fallback: false,
      env_vars: [
        { key: "MOCK_AGENT_SESSION_CATALOG_FILE", value: catalogPath },
        { key: "E2E_MOCK_AGENT_ACP_TRACE_FILE", value: tracePath },
        { key: "GITLAB_HOST", value: host },
        { key: "GITLAB_TOKEN", value: token },
      ],
    });
    const wire = watchWs(page);
    const traffic = attachGatewayTrafficCapture(page);
    const httpMutations: string[] = [];
    page.on("request", (request) => {
      const url = new URL(request.url());
      if (
        request.method() === "POST" &&
        /\/api\/v1\/task-sessions\/[^/]+\/(set-model|set-mode|set-config-option|resolve-model-config)$/.test(
          url.pathname,
        )
      ) {
        httpMutations.push(url.pathname);
      }
    });
    const { executors } = await api.listExecutors();
    const local = executors.find(
      (executor) => executor.type === "local" || executor.type === "local_pc",
    );
    const executorProfile = local?.profiles?.[0];
    if (!local || !executorProfile)
      throw new Error("Catalog candidates require a concrete Local executor profile");
    const task = await api.createTaskWithAgent(
      seed.workspaceId,
      "Session catalog refresh",
      profile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seed.workflowId,
        workflow_step_id: seed.startStepId,
        repository_ids: [seed.repositoryId],
        executor_profile_id: executorProfile.id,
      },
    );
    if (!task.session_id) throw new Error("Catalog fixture has no session");
    const sessionId = task.session_id;
    await expect
      .poll(async () => (await api.getTaskSession(sessionId)).session.state, {
        timeout: 30000,
        message: "Wait for the catalog fixture's initial turn",
      })
      .toBe("WAITING_FOR_INPUT");
    const environment = await api.getTaskEnvironment(task.id);
    expect(environment?.executor_type).toBe(local.type);
    expect(environment?.executor_profile_id).toBe(executorProfile.id);
    const nativeId = readMockACPTrace(tracePath).find(
      (event) => event.event === "prompt",
    )?.session_id;
    if (!nativeId) throw new Error("Catalog fixture has no observed native conversation");
    await page.goto(`/t/${task.id}`);
    const session = new SessionPage(page);
    await session.waitForLoad();
    return {
      taskId: task.id,
      sessionId,
      profile,
      agentName: agent.name,
      catalogPath,
      tracePath,
      nativeId,
      session,
      wire,
      traffic,
      httpMutations,
      releaseBackend,
    };
  } catch (error) {
    await releaseBackend();
    throw error;
  }
}

export function catalogRecord(session: TaskSession): SessionCatalogRecord | undefined {
  return session.metadata?.profile_catalog_refresh as SessionCatalogRecord | undefined;
}

export async function refreshSavedSessionCatalog(api: ApiClient, profileId: string): Promise<void> {
  const response = await api.rawRequest(
    "POST",
    `/api/v1/agent-profiles/${profileId}/model-catalog/refresh`,
  );
  expect(response.status).toBe(200);
  const result = await response.json();
  expect(result.status).toBe("ok");
  expect(result.publication).toEqual({ status: "published" });
}

export async function openCatalogProfilePage(
  page: Page,
  fixture: SessionCatalogScenario,
): Promise<Page> {
  const settings = await page.context().newPage();
  await settings.goto(`/settings/agents/${fixture.agentName}/profiles/${fixture.profile.id}`);
  await expect(settings.getByTestId("profile-capability-status")).toHaveAttribute(
    "data-status",
    "ready",
    { timeout: 20000 },
  );
  return settings;
}

export async function clickSavedCatalogRefresh(settings: Page, profileId: string): Promise<void> {
  const response = settings.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      response.url().endsWith(`/api/v1/agent-profiles/${profileId}/model-catalog/refresh`),
  );
  await settings.getByTestId("profile-refresh-capabilities").click();
  const refreshed = await response;
  expect(refreshed.request().postData()).toBeNull();
  expect(refreshed.status()).toBe(200);
  expect((await refreshed.json()).publication).toEqual({ status: "published" });
}

export function catalogMutationFrames(frames: GatewayTrafficFrame[]): GatewayTrafficFrame[] {
  return frames.filter(
    (frame) =>
      frame.direction === "sent" &&
      frame.activationSource !== "session_focus" &&
      [
        "session.stop",
        "session.launch",
        "session.set_model",
        "session.set_mode",
        "session.set_config_option",
        "message.add",
        "message.queue.add",
      ].includes(frame.action ?? ""),
  );
}

export function assertCatalogNativeRetained(
  fixture: SessionCatalogScenario,
  expectedPrompts = 1,
): void {
  const events = readMockACPTrace(fixture.tracePath);
  const loads = events.filter(
    (event) => event.event === "session_load" && event.session_id === fixture.nativeId,
  );
  expect(loads.length).toBeGreaterThan(0);
  const resumedProcesses = new Set(loads.map((event) => event.process_id));
  expect(
    events.filter(
      (event) => event.event === "session_new" && resumedProcesses.has(event.process_id),
    ),
  ).toEqual([]);
  expect(
    events.filter((event) => event.event === "prompt" && event.session_id === fixture.nativeId),
  ).toHaveLength(expectedPrompts);
}

export async function cleanupSessionCatalogScenario(
  api: ApiClient,
  fixture: SessionCatalogScenario,
): Promise<void> {
  try {
    await api.deleteTask(fixture.taskId);
    await api.deleteAgentProfile(fixture.profile.id, true);
  } finally {
    await fixture.releaseBackend();
  }
}
