import { expect } from "@playwright/test";
import type { Page, TestInfo } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import type { TaskSession } from "../../lib/types/http";
import type { SessionCatalogBrowserContext } from "./session-catalog-refresh";
import { activate } from "./builtin-mcp-management";
import { KanbanPage } from "../pages/kanban-page";
import {
  assertRestartDataRetained,
  assertRestartRetained,
  captureRestartBaseline,
} from "../tests/session/session-tab-restart-helpers";
import { readMockACPTrace } from "./transient-turn-runtime-continuity";
import { readSessionRuntimeIdentity } from "./session-resume-prompt-queue";
import {
  openQuickChatSetup,
  selectAgentIfNeeded,
  startQuickChatFromSetup,
  waitForSessionSettledBaseline,
} from "../tests/chat/quick-chat-helpers";
import { openMobileQuickChatSetup } from "../tests/chat/quick-chat-saved-prompt-delivery-helpers";
import {
  assertCatalogNativeRetained,
  catalogRecord,
  catalogMutationFrames,
  cleanupSessionCatalogScenario,
  createSessionCatalogScenario,
  openCatalogProfilePage,
  clickSavedCatalogRefresh,
  refreshSavedSessionCatalog,
  writeSessionCatalogFile,
} from "./session-catalog-refresh";
import { waitForFiniteAnimations } from "./pr-capture";

export async function verifyCatalogPreview(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  backend: BackendContext,
  testInfo: TestInfo,
): Promise<void> {
  const fixture = await createSessionCatalogScenario(page, api, seed, backend);
  const priorPreviewSetting = (await api.getUserSettings()).settings.enable_preview_on_click;
  try {
    await api.saveUserSettings({ enable_preview_on_click: true });
    const kanban = new KanbanPage(page);
    await kanban.goto(seed.workflowId);
    await kanban.taskCard(fixture.taskId).click();
    await expect(page.getByTestId(`preview-session-tab-${fixture.sessionId}`)).toBeVisible();
    const retained = await captureRestartBaseline(api, fixture.taskId, fixture.sessionId);
    const editor = await fixture.session.fillMessage("Retain the preview composer draft");
    writeSessionCatalogFile(fixture.catalogPath, "new-choices");
    await refreshSavedSessionCatalog(api, fixture.profile.id);
    const notice = page.getByTestId("session-catalog-refresh-notice");
    await expect(notice).toBeVisible();
    const start = fixture.traffic.frames.length;
    await notice.getByRole("button", { name: "Refresh session", exact: true }).click();
    const consent = page.getByTestId("session-catalog-refresh-confirmation");
    await expect(consent).toBeVisible();
    await consent.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(consent).not.toBeVisible();
    expect(catalogMutationFrames(fixture.traffic.frames.slice(start))).toEqual([]);
    await notice.getByRole("button", { name: "Refresh session", exact: true }).click();
    const stopped = fixture.wire.waitForResponse("session.stop", {
      requestWhere: (payload) => payload.session_id === fixture.sessionId,
    });
    const resumed = fixture.wire.waitForResponse("session.launch", {
      timeout: 30000,
      timeoutAfter: stopped,
      requestWhere: (payload) =>
        payload.session_id === fixture.sessionId && payload.intent === "resume",
    });
    await consent.getByRole("button", { name: "Refresh session", exact: true }).click();
    await Promise.all([stopped, resumed]);
    await assertRestartRetained(api, fixture.taskId, fixture.sessionId, retained);
    await expect(editor).toContainText("Retain the preview composer draft");
    await expect(page.getByTestId(`preview-session-tab-${fixture.sessionId}`)).toBeVisible();
    assertCatalogNativeRetained(fixture);
    expect(
      catalogMutationFrames(fixture.traffic.frames.slice(start)).map((frame) => [
        frame.action,
        frame.sessionId,
      ]),
    ).toEqual([
      ["session.stop", fixture.sessionId],
      ["session.launch", fixture.sessionId],
    ]);
    expect(fixture.httpMutations).toEqual([]);
    await waitForFiniteAnimations(page.locator("body"));
    await testInfo.attach("preview-session-catalog-refresh", {
      body: await page.screenshot({
        path: testInfo.outputPath("preview-session-catalog-refresh.png"),
      }),
      contentType: "image/png",
    });
  } finally {
    try {
      await api.saveUserSettings({ enable_preview_on_click: priorPreviewSetting });
    } finally {
      await cleanupSessionCatalogScenario(api, fixture);
    }
  }
}

export async function verifyCatalogInterruptedTurn({
  page,
  api,
  seed,
  backend,
  touch,
  testInfo,
}: SessionCatalogBrowserContext): Promise<void> {
  const releaseCancellation = await backend.useEnv({
    KANDEV_E2E_CANCEL_HOLD_DURATION: "120s",
    KANDEV_E2E_PROMPT_CANCEL_JOIN_TIMEOUT: "1s",
  });
  const fixture = await createSessionCatalogScenario(page, api, seed, backend).catch(
    async (error: unknown) => {
      await releaseCancellation();
      throw error;
    },
  );
  try {
    const sibling = await api.launchSession({
      task_id: fixture.taskId,
      agent_profile_id: fixture.profile.id,
      prompt: "/e2e:simple-message",
    });
    expect(seed.agentProfileId).not.toBe(fixture.profile.id);
    const otherProfile = await api.launchSession({
      task_id: fixture.taskId,
      agent_profile_id: seed.agentProfileId,
      prompt: "/e2e:simple-message",
    });
    await waitForSessionSettledBaseline(api, fixture.taskId, sibling.session_id);
    await waitForSessionSettledBaseline(api, fixture.taskId, otherProfile.session_id);
    const siblingBefore = (await api.getTaskSession(sibling.session_id)).session;
    const otherBefore = (await api.getTaskSession(otherProfile.session_id)).session;
    if (touch) {
      await page.getByTestId("mobile-sessions-pill").tap();
      await page.getByTestId(`mobile-session-row-${fixture.sessionId}`).tap();
      await expect(page.getByRole("dialog", { name: "Sessions", exact: true })).not.toBeVisible();
    } else {
      await fixture.session.sessionTabBySessionId(fixture.sessionId).click();
    }
    await fixture.session.sendMessageViaButton("/e2e:cancel-hold Catalog running turn witness");
    await expect(
      fixture.session.activeChat().getByText("Catalog running turn witness", { exact: true }),
    ).toBeVisible();
    const retained = await captureRestartBaseline(api, fixture.taskId, fixture.sessionId);
    expect(retained.target.state).toBe("RUNNING");
    const runningBefore = (await api.getTaskSession(fixture.sessionId)).session;
    const editor = fixture.session
      .activeChat()
      .locator('.tiptap.ProseMirror[contenteditable="true"]')
      .first();
    await editor.fill("Keep input while interrupted outcome is uncertain");
    writeSessionCatalogFile(fixture.catalogPath, "new-choices");
    const beforeRefresh = fixture.traffic.frames.length;
    const settings = await openCatalogProfilePage(page, fixture);
    try {
      await clickSavedCatalogRefresh(settings, fixture.profile.id);
    } finally {
      await settings.close();
    }
    const siblingPublished = (await api.getTaskSession(sibling.session_id)).session;
    expect(catalogRecord(siblingPublished)?.disposition).toBe("host_candidates");
    expect(siblingPublished.state).toBe("WAITING_FOR_INPUT");
    expectCatalogLaunchInputsUnchanged(siblingPublished, siblingBefore);
    const otherAfter = (await api.getTaskSession(otherProfile.session_id)).session;
    expect(catalogRecord(otherAfter)).toEqual(catalogRecord(otherBefore));
    expect(otherAfter.state).toBe(otherBefore.state);
    expectCatalogLaunchInputsUnchanged(otherAfter, otherBefore);
    const runningPublished = (await api.getTaskSession(fixture.sessionId)).session;
    expect(catalogRecord(runningPublished)?.disposition).toBe("host_candidates");
    expect(runningPublished.state).toBe("RUNNING");
    expectCatalogLaunchInputsUnchanged(runningPublished, runningBefore);
    const notice = page.getByTestId("session-catalog-refresh-notice");
    const refresh = notice.getByRole("button", { name: "Refresh session", exact: true });
    await expect(refresh).toBeEnabled();
    await activate(refresh, touch);
    const consent = page.getByTestId("session-catalog-refresh-confirmation");
    await expect(consent).toBeVisible();
    expect(catalogMutationFrames(fixture.traffic.frames.slice(beforeRefresh))).toEqual([]);
    await api.updateAgentProfile(fixture.profile.id, {
      cli_flags: [
        {
          description: "Keep uncertain delivery unresolved",
          flag: "--fail-on-resume",
          enabled: true,
        },
      ],
    });
    const start = fixture.traffic.frames.length;
    const stopped = fixture.wire.waitForResponse("session.stop", {
      requestWhere: (payload) => payload.session_id === fixture.sessionId,
    });
    const resumed = fixture.wire
      .waitForResponse("session.launch", {
        timeout: 30000,
        timeoutAfter: stopped,
        requestWhere: (payload) =>
          payload.session_id === fixture.sessionId &&
          payload.intent === "resume" &&
          !payload.activation_source,
      })
      .catch((error: unknown) => {
        if (
          !(error instanceof Error) ||
          !error.message.startsWith('watchWs.waitForResponse: "session.launch" was rejected: ')
        )
          throw error;
        return error;
      });
    const confirm = consent.getByRole("button", { name: "Refresh session", exact: true });
    await activate(confirm, touch);
    await Promise.all([stopped, resumed]);
    await expect
      .poll(async () =>
        (await api.getTaskSession(fixture.sessionId)).session.session_recovery_blocks?.some(
          (block) => block.reason === "unknown_prompt_outcome",
        ),
      )
      .toBe(true);
    await expect(page.getByTestId("user-message-delivery-blocked")).toBeVisible();
    await expect(fixture.session.recoveryResumeButton()).toBeVisible();
    await expect(fixture.session.recoveryResumeButton()).toBeEnabled();
    await assertRestartDataRetained(api, fixture.taskId, fixture.sessionId, retained);
    expect(
      await page.evaluate(
        (id) => JSON.parse(sessionStorage.getItem(`kandev.chatDraft.text.${id}`) ?? "null"),
        fixture.sessionId,
      ),
    ).toBe("Keep input while interrupted outcome is uncertain");
    expect(
      readMockACPTrace(fixture.tracePath).filter(
        (event) => event.event === "prompt" && event.session_id === fixture.nativeId,
      ),
    ).toHaveLength(2);
    expect(
      catalogMutationFrames(fixture.traffic.frames.slice(start)).map((frame) => [
        frame.action,
        frame.sessionId,
      ]),
    ).toEqual([
      ["session.stop", fixture.sessionId],
      ["session.launch", fixture.sessionId],
    ]);
    expect(fixture.httpMutations).toEqual([]);
    await waitForFiniteAnimations(page.locator("body"));
    const name = `${touch ? "phone" : "desktop"}-catalog-interrupted-turn`;
    await testInfo.attach(name, {
      body: await page.screenshot({ path: testInfo.outputPath(`${name}.png`) }),
      contentType: "image/png",
    });
  } finally {
    try {
      await cleanupSessionCatalogScenario(api, fixture);
    } finally {
      await releaseCancellation();
    }
  }
}

export async function verifyCatalogQuickChat({
  page,
  api,
  seed,
  backend,
  touch,
  testInfo,
}: SessionCatalogBrowserContext): Promise<void> {
  const fixture = await createSessionCatalogScenario(page, api, seed, backend);
  let quickTaskId: string | undefined;
  try {
    const dialog = touch ? await openMobileQuickChatSetup(page) : await openQuickChatSetup(page);
    await selectAgentIfNeeded(dialog, page, fixture.profile.name);
    const started = await startQuickChatFromSetup(dialog, page, "/e2e:simple-message");
    quickTaskId = started.task_id;
    await waitForSessionSettledBaseline(api, started.task_id, started.session_id);
    const before = (await api.getTaskSession(started.session_id)).session;
    expect(before.agent_profile_id).toBe(fixture.profile.id);
    const environment = await api.getTaskEnvironment(started.task_id);
    const unknownContext =
      !environment?.executor_profile_id ||
      !["local", "worktree", "local_pc"].includes(environment.executor_type ?? "");
    const identity = await readSessionRuntimeIdentity(api, started.task_id, started.session_id);
    const retained = await captureRestartBaseline(api, started.task_id, started.session_id);
    const editor = dialog.locator('.tiptap.ProseMirror[contenteditable="true"]:visible').first();
    await editor.fill("Retain the Quick Chat catalog draft");
    writeSessionCatalogFile(fixture.catalogPath, "new-choices");
    await refreshSavedSessionCatalog(api, fixture.profile.id);
    if (unknownContext) {
      const record = catalogRecord((await api.getTaskSession(started.session_id)).session);
      expect(record?.disposition).toBe("rediscovery_required");
      expect(record?.models).toBeUndefined();
      expect(record?.modes).toBeUndefined();
    }
    const notice = dialog.getByTestId("session-catalog-refresh-notice");
    await expect(notice).toBeVisible();
    const start = fixture.traffic.frames.length;
    const refresh = notice.getByRole("button", { name: "Refresh session", exact: true });
    await activate(refresh, touch);
    const consent = page.getByTestId("session-catalog-refresh-confirmation");
    await expect(consent).toBeVisible();
    const cancel = consent.getByRole("button", { name: "Cancel", exact: true });
    await activate(cancel, touch);
    await expect(consent).not.toBeVisible();
    expect(catalogMutationFrames(fixture.traffic.frames.slice(start))).toEqual([]);
    await activate(refresh, touch);
    const stopped = fixture.wire.waitForResponse("session.stop", {
      requestWhere: (payload) => payload.session_id === started.session_id,
    });
    const resumed = fixture.wire.waitForResponse("session.launch", {
      timeout: 30000,
      timeoutAfter: stopped,
      requestWhere: (payload) =>
        payload.session_id === started.session_id && payload.intent === "resume",
    });
    const confirm = consent.getByRole("button", { name: "Refresh session", exact: true });
    await activate(confirm, touch);
    await Promise.all([stopped, resumed]);
    await assertRestartRetained(api, started.task_id, started.session_id, retained);
    const after = (await api.getTaskSession(started.session_id)).session;
    expect(after.agent_execution_id).not.toBe(before.agent_execution_id);
    expect(after.metadata?.runtime_config).toEqual(before.metadata?.runtime_config);
    expect(after.metadata?.acp_config_baseline).toEqual(before.metadata?.acp_config_baseline);
    expect(
      (await readSessionRuntimeIdentity(api, started.task_id, started.session_id)).acpSessionId,
    ).toBe(identity.acpSessionId);
    await expect(editor).toContainText("Retain the Quick Chat catalog draft");
    await expect(
      dialog.getByRole("button", { name: "Session model settings", exact: true }),
    ).toContainText("Mock Fast");
    assertCatalogNativeRetained({
      ...fixture,
      taskId: started.task_id,
      sessionId: started.session_id,
      nativeId: identity.acpSessionId,
    });
    expect(
      catalogMutationFrames(fixture.traffic.frames.slice(start)).map((frame) => [
        frame.action,
        frame.sessionId,
      ]),
    ).toEqual([
      ["session.stop", started.session_id],
      ["session.launch", started.session_id],
    ]);
    expect(fixture.httpMutations).toEqual([]);
    await waitForFiniteAnimations(page.locator("body"));
    const name = `${touch ? "phone" : "desktop"}-quick-chat-catalog-refresh`;
    await testInfo.attach(name, {
      body: await page.screenshot({ path: testInfo.outputPath(`${name}.png`) }),
      contentType: "image/png",
    });
  } finally {
    try {
      if (quickTaskId) await api.deleteTask(quickTaskId);
    } finally {
      await cleanupSessionCatalogScenario(api, fixture);
    }
  }
}

function expectCatalogLaunchInputsUnchanged(after: TaskSession, before: TaskSession) {
  expect(after.agent_execution_id).toBe(before.agent_execution_id);
  expect(after.metadata?.runtime_config).toEqual(before.metadata?.runtime_config);
  expect(after.metadata?.acp_config_baseline).toEqual(before.metadata?.acp_config_baseline);
}
