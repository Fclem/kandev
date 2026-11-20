import { expect } from "@playwright/test";
import type { Locator, Page } from "@playwright/test";
import {
  sessionCatalogMetadataSchema,
  sessionCatalogOwnerSchema,
  sessionCatalogResumeAckSchema,
  sessionCatalogSourceOwnerSchema,
} from "../../lib/types/session-catalog";
import type { ApiClient } from "./api-client";
import {
  assertRestartDataRetained,
  assertRestartRetained,
  captureRestartBaseline,
} from "../tests/session/session-tab-restart-helpers";
import {
  assertCatalogNativeRetained,
  catalogMutationFrames,
  catalogRecord,
  cleanupSessionCatalogScenario,
  createSessionCatalogScenario,
  refreshSavedSessionCatalog,
  writeSessionCatalogFile,
} from "./session-catalog-refresh";
import { waitForFiniteAnimations } from "./pr-capture";
import { activate } from "./builtin-mcp-management";
import type {
  SessionCatalogBrowserContext,
  SessionCatalogScenario,
} from "./session-catalog-refresh";

export type TerminalCatalogVariant = "empty" | "mode-only" | "unsupported" | "unchanged";

export async function verifyCatalogTerminalOutcome({
  page,
  api,
  seed,
  backend,
  touch,
  variant,
  testInfo,
}: SessionCatalogBrowserContext & { variant: TerminalCatalogVariant }): Promise<void> {
  const fixture = await createSessionCatalogScenario(page, api, seed, backend);
  const unchanged = variant === "unchanged";
  const resumes = variant === "mode-only" || unchanged;
  try {
    const retained = await captureRestartBaseline(api, fixture.taskId, fixture.sessionId);
    const editor = await fixture.session.fillMessage(
      "Retain input after terminal catalog discovery",
    );
    writeSessionCatalogFile(fixture.catalogPath, "new-choices");
    await refreshSavedSessionCatalog(api, fixture.profile.id);
    const notice = page.getByTestId("session-catalog-refresh-notice");
    await expect(notice).toBeVisible();
    writeSessionCatalogFile(fixture.catalogPath, unchanged ? "baseline" : variant);
    const refresh = notice.getByRole("button", { name: "Refresh session", exact: true });
    await activate(refresh, touch);
    const consent = page.getByTestId("session-catalog-refresh-confirmation");
    await expect(consent).toBeVisible();
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
    const [, ack] = await Promise.all([stopped, resumed]);
    if (ack instanceof Error) {
      expect(resumes).toBe(false);
      await expect
        .poll(async () => (await api.getTaskSession(fixture.sessionId)).session.state)
        .toBe("FAILED");
      await assertRestartDataRetained(api, fixture.taskId, fixture.sessionId, retained);
    } else {
      expect(resumes).toBe(true);
      await assertRestartRetained(api, fixture.taskId, fixture.sessionId, retained);
    }
    await assertTerminalCatalogEvidence(api, fixture, variant, retained, ack);
    if (variant === "unsupported") {
      await expect(notice.getByRole("status")).toContainText(
        "does not support discovering session choices",
      );
    }
    if (resumes) {
      await expect(editor).toContainText("Retain input after terminal catalog discovery");
      await expect(fixture.session.activeChat().getByTestId("submit-message-button")).toBeEnabled();
    } else {
      expect(
        await page.evaluate(
          (id) => JSON.parse(sessionStorage.getItem(`kandev.chatDraft.text.${id}`) ?? "null"),
          fixture.sessionId,
        ),
      ).toBe("Retain input after terminal catalog discovery");
      await expect(fixture.session.recoveryResumeButton()).toBeVisible();
    }
    await assertTerminalModelControls(page, notice, touch, unchanged);
    assertCatalogNativeRetained(fixture);
    expect(
      catalogMutationFrames(fixture.traffic.frames.slice(start)).map((frame) => frame.action),
    ).toEqual(["session.stop", "session.launch"]);
    expect(fixture.httpMutations).toEqual([]);
    await waitForFiniteAnimations(page.locator("body"));
    const name = `${touch ? "phone" : "desktop"}-catalog-terminal-${variant}`;
    await testInfo.attach(name, {
      body: await page.screenshot({ path: testInfo.outputPath(`${name}.png`) }),
      contentType: "image/png",
    });
  } finally {
    await cleanupSessionCatalogScenario(api, fixture);
  }
}

async function assertTerminalCatalogEvidence(
  api: ApiClient,
  fixture: SessionCatalogScenario,
  variant: TerminalCatalogVariant,
  retained: Awaited<ReturnType<typeof captureRestartBaseline>>,
  ack: Awaited<ReturnType<SessionCatalogScenario["wire"]["waitForResponse"]>> | Error,
) {
  const unchanged = variant === "unchanged";
  const withModels = variant === "empty" || unchanged;
  const withModes = variant === "mode-only" || unchanged;
  const current = (await api.getTaskSession(fixture.sessionId)).session;
  const owner = sessionCatalogSourceOwnerSchema.parse(current.metadata?.acp_model_state);
  const marker = current.metadata?.profile_catalog_discovery_owner as
    | { owner?: unknown }
    | undefined;
  expect(owner).toEqual(sessionCatalogOwnerSchema.parse(marker?.owner));
  expect(owner.execution_id).not.toBe(retained.target.agent_execution_id);
  if (!(ack instanceof Error)) {
    expect(owner.execution_id).toBe(current.agent_execution_id);
    expect(owner).toEqual(sessionCatalogResumeAckSchema.parse(ack.payload).accepted_launch_owner);
  }
  const outcome = variant === "unsupported" ? "unsupported" : "complete";
  const availability = async () =>
    sessionCatalogMetadataSchema.parse(
      (await api.getTaskSession(fixture.sessionId)).session.metadata,
    ).acp_model_state?.catalog_discovery;
  await expect.poll(async () => (await availability())?.owner).toEqual(owner);
  await expect.poll(async () => (await availability())?.result.outcome).toBe(outcome);
  const result = (await availability())!.result;
  expect(result.models.present).toBe(withModels);
  expect(result.models.supported).toBe(withModels);
  expect(result.models.choices?.map((model) => model.id) ?? []).toEqual(
    unchanged ? ["mock-fast", "mock-smart"] : [],
  );
  expect(result.modes.supported).toBe(variant !== "unsupported");
  expect(result.modes.choices ?? []).toEqual(
    withModes
      ? [
          { id: "default", name: "Default" },
          { id: "plan-mock", name: "Plan Mock" },
        ]
      : [],
  );
  await expect
    .poll(
      async () => catalogRecord((await api.getTaskSession(fixture.sessionId)).session)?.disposition,
    )
    .toBe(variant === "unsupported" ? "rediscovery_required" : "cleared");
}

async function assertTerminalModelControls(
  page: Page,
  notice: Locator,
  touch: boolean,
  unchanged: boolean,
) {
  const selector = page.getByRole("button", { name: "Session model settings", exact: true });
  if (unchanged) {
    await expect(notice.getByRole("status")).toContainText("not advertised by this executor");
    await expect(selector).toBeEnabled();
  }
  if (!(await selector.count()) || !(await selector.isEnabled())) return;
  await activate(selector, touch);
  const providerOnly = page.getByRole("option", { name: "Mock Smart", exact: true });
  if (unchanged) await expect(providerOnly).toBeEnabled();
  else await expect(providerOnly).toHaveCount(0);
  await expect(page.getByRole("option", { name: "Catalog new model", exact: true })).toHaveCount(0);
  const selected = page.getByRole("option", { name: "Mock Fast", exact: true });
  if (unchanged) await expect(selected).toBeEnabled();
  else if (await selected.count()) await expect(selected).toBeDisabled();
}
