import fs from "node:fs";
import path from "node:path";
import { expect } from "@playwright/test";
import type { Locator, Page } from "@playwright/test";
import type {
  SessionCatalogBrowserContext,
  SessionCatalogScenario,
} from "./session-catalog-refresh";
import {
  assertCatalogNativeRetained,
  catalogMutationFrames,
  catalogRecord,
  cleanupSessionCatalogScenario,
  createSessionCatalogScenario,
  refreshSavedSessionCatalog,
  writeSessionCatalogFile,
} from "./session-catalog-refresh";
import { createGenericWrapper, readProfileProbeEvidence } from "./profile-capability-discovery";
import { readMockACPTrace } from "./transient-turn-runtime-continuity";
import { readSessionRuntimeIdentity } from "./session-resume-prompt-queue";
import {
  assertRestartDataRetained,
  assertRestartRetained,
  captureRestartBaseline,
} from "../tests/session/session-tab-restart-helpers";
import { waitForFiniteAnimations } from "./pr-capture";
import { activate } from "./builtin-mcp-management";

async function openConsent(page: Page, touch: boolean): Promise<Locator> {
  const notice = page.getByTestId("session-catalog-refresh-notice");
  await activate(notice.getByRole("button", { name: "Refresh session", exact: true }), touch);
  const consent = page.getByTestId("session-catalog-refresh-confirmation");
  await expect(consent).toBeVisible();
  return consent;
}

async function confirmConsent(
  consent: Locator,
  fixture: SessionCatalogScenario,
  touch: boolean,
): Promise<void> {
  const stopped = fixture.wire.waitForResponse("session.stop", {
    requestWhere: (payload) => payload.session_id === fixture.sessionId,
  });
  const resumed = fixture.wire.waitForResponse("session.launch", {
    timeout: 30000,
    timeoutAfter: stopped,
    requestWhere: (payload) =>
      payload.session_id === fixture.sessionId && payload.intent === "resume",
  });
  await activate(consent.getByRole("button", { name: "Refresh session", exact: true }), touch);
  await Promise.all([stopped, resumed]);
}

export async function verifyCatalogCurrentLaunchInputs({
  page,
  api,
  seed,
  backend,
  touch,
  testInfo,
}: SessionCatalogBrowserContext): Promise<void> {
  const fixture = await createSessionCatalogScenario(page, api, seed, backend);
  try {
    const retained = await captureRestartBaseline(api, fixture.taskId, fixture.sessionId);
    const identity = await readSessionRuntimeIdentity(api, fixture.taskId, fixture.sessionId);
    const before = (await api.getTaskSession(fixture.sessionId)).session;
    const editor = await fixture.session.fillMessage(
      "Retain work while saved launch inputs change",
    );
    writeSessionCatalogFile(fixture.catalogPath, "new-choices");
    await refreshSavedSessionCatalog(api, fixture.profile.id);
    const consent = await openConsent(page, touch);
    const evidencePath = path.join(
      path.dirname(fixture.catalogPath),
      "current-launch-evidence.jsonl",
    );
    fs.writeFileSync(evidencePath, "");
    const prefix = createGenericWrapper(backend);
    await api.updateAgentProfile(fixture.profile.id, {
      command_prefix: prefix,
      cli_flags: [
        {
          description: "Current launch input",
          flag: "--profile-catalog consent-cli",
          enabled: true,
        },
      ],
      env_vars: [
        ...(fixture.profile.envVars ?? []),
        { key: "MOCK_AGENT_PROFILE_CATALOG", value: "consent-account" },
        { key: "MOCK_AGENT_PROFILE_EVIDENCE_FILE", value: evidencePath },
      ],
    });
    expect((await api.getTaskSession(fixture.sessionId)).session.agent_execution_id).toBe(
      identity.executionId,
    );
    await waitForFiniteAnimations(consent);
    const name = `${touch ? "phone" : "desktop"}-catalog-current-input-consent`;
    await testInfo.attach(name, {
      body: await page.screenshot({ path: testInfo.outputPath(`${name}.png`) }),
      contentType: "image/png",
    });
    await confirmConsent(consent, fixture, touch);
    await assertRestartRetained(api, fixture.taskId, fixture.sessionId, retained);
    const resumed = await readSessionRuntimeIdentity(api, fixture.taskId, fixture.sessionId);
    expect(resumed.acpSessionId).toBe(identity.acpSessionId);
    expect(resumed.executionId).not.toBe(identity.executionId);
    const load = readMockACPTrace(fixture.tracePath).findLast(
      (event) => event.event === "session_load" && event.session_id === identity.acpSessionId,
    );
    expect(load).toBeDefined();
    const evidence = readProfileProbeEvidence(evidencePath);
    const wrapperIndex = evidence.findIndex(
      (row) => row.wrapper && String(row.launcher_pid) === load!.process_id,
    );
    expect(wrapperIndex).toBeGreaterThanOrEqual(0);
    expect(evidence[wrapperIndex + 1]).toMatchObject({
      wrapper: false,
      env_catalog: "consent-account",
      cli_catalog: "consent-cli",
      profile_args: ["--profile-catalog", "consent-cli"],
    });
    const after = (await api.getTaskSession(fixture.sessionId)).session;
    expect(after.metadata?.runtime_config).toEqual(before.metadata?.runtime_config);
    expect(after.metadata?.acp_config_baseline).toEqual(before.metadata?.acp_config_baseline);
    await expect(editor).toContainText("Retain work while saved launch inputs change");
    assertCatalogNativeRetained(fixture);
    expect(fixture.httpMutations).toEqual([]);
  } finally {
    await cleanupSessionCatalogScenario(api, fixture);
  }
}

export async function verifyCatalogNativeRestoreFailure({
  page,
  api,
  seed,
  backend,
  touch,
  testInfo,
}: SessionCatalogBrowserContext): Promise<void> {
  const fixture = await createSessionCatalogScenario(page, api, seed, backend);
  try {
    const retained = await captureRestartBaseline(api, fixture.taskId, fixture.sessionId);
    const identity = await readSessionRuntimeIdentity(api, fixture.taskId, fixture.sessionId);
    const editor = await fixture.session.fillMessage(
      "Keep the original conversation after restore rejection",
    );
    writeSessionCatalogFile(fixture.catalogPath, "new-choices");
    await refreshSavedSessionCatalog(api, fixture.profile.id);
    const consent = await openConsent(page, touch);
    await api.updateAgentProfile(fixture.profile.id, {
      cli_flags: [
        {
          description: "Reject native conversation restore",
          flag: "--fail-on-resume",
          enabled: true,
        },
      ],
    });
    const stopped = fixture.wire.waitForResponse("session.stop", {
      requestWhere: (payload) => payload.session_id === fixture.sessionId,
    });
    const resumed = fixture.wire
      .waitForResponse("session.launch", {
        timeout: 60000,
        timeoutAfter: stopped,
        requestWhere: (payload) =>
          payload.session_id === fixture.sessionId && payload.intent === "resume",
      })
      .catch((error: unknown) => {
        if (
          !(error instanceof Error) ||
          !error.message.startsWith('watchWs.waitForResponse: "session.launch" was rejected: ')
        )
          throw error;
      });
    const requestStart = fixture.traffic.frames.length;
    await activate(consent.getByRole("button", { name: "Refresh session", exact: true }), touch);
    await Promise.all([stopped, resumed]);
    await expect
      .poll(async () => (await api.getTaskSession(fixture.sessionId)).session.state, {
        timeout: 60000,
      })
      .toBe("FAILED");
    const requests = catalogMutationFrames(fixture.traffic.frames.slice(requestStart));
    expect(requests.map((frame) => frame.action)).toEqual(["session.stop", "session.launch"]);
    await assertRestartDataRetained(api, fixture.taskId, fixture.sessionId, retained);
    expect(
      await page.evaluate(
        (id) => JSON.parse(sessionStorage.getItem(`kandev.chatDraft.text.${id}`) ?? "null"),
        fixture.sessionId,
      ),
    ).toBe("Keep the original conversation after restore rejection");
    await expect(page.getByTestId("session-catalog-refresh-notice")).toBeVisible();
    await expect(fixture.session.recoveryResumeButton()).toBeVisible();
    await expect
      .poll(
        async () =>
          catalogRecord((await api.getTaskSession(fixture.sessionId)).session)?.result?.outcome,
      )
      .toBe("failed");
    assertCatalogNativeRetained(fixture);
    await waitForFiniteAnimations(page.locator("body"));
    const name = `${touch ? "phone" : "desktop"}-catalog-native-restore-failure`;
    await testInfo.attach(name, {
      body: await page.screenshot({ path: testInfo.outputPath(`${name}.png`) }),
      contentType: "image/png",
    });
    await api.updateAgentProfile(fixture.profile.id, { cli_flags: [] });
    expect((await api.getTaskSession(fixture.sessionId)).session.state).toBe("FAILED");
    const recovery = fixture.wire.waitForResponse("session.recover", {
      timeout: 60000,
      requestWhere: (payload) =>
        payload.session_id === fixture.sessionId && payload.action === "resume",
    });
    await activate(fixture.session.recoveryResumeButton(), touch);
    await recovery;
    await assertRestartRetained(api, fixture.taskId, fixture.sessionId, retained);
    await expect(editor).toBeEditable();
    await expect(editor).toContainText("Keep the original conversation after restore rejection");
    expect(
      (await readSessionRuntimeIdentity(api, fixture.taskId, fixture.sessionId)).acpSessionId,
    ).toBe(identity.acpSessionId);
    assertCatalogNativeRetained(fixture);
    expect(fixture.httpMutations).toEqual([]);
  } finally {
    await cleanupSessionCatalogScenario(api, fixture);
  }
}
