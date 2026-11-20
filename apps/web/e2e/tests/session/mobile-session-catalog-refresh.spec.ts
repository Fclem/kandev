import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import {
  assertCatalogNativeRetained,
  catalogMutationFrames,
  catalogRecord,
  cleanupSessionCatalogScenario,
  createSessionCatalogScenario,
  refreshSavedSessionCatalog,
  writeSessionCatalogFile,
} from "../../helpers/session-catalog-refresh";
import { assertRestartRetained, captureRestartBaseline } from "./session-tab-restart-helpers";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { closeModelConfigSheetIfOpen } from "../../helpers/model-config-sheet";
import { verifySessionCatalogDiscoveryDeadline } from "../../helpers/session-catalog-discovery-loss";
import { pastePng, waitForReadyDraftAttachment } from "../chat/composer-attachment-scope-helpers";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import {
  verifyCatalogCurrentLaunchInputs,
  verifyCatalogNativeRestoreFailure,
} from "../../helpers/session-catalog-launch-inputs";
import { verifyCatalogTerminalOutcome } from "../../helpers/session-catalog-terminal-outcomes";
import {
  verifyCatalogInterruptedTurn,
  verifyCatalogQuickChat,
} from "../../helpers/session-catalog-additional-surfaces";

// @covers AC-AGENTS-SESSION-CATALOG-002.1, AC-AGENTS-SESSION-CATALOG-002.2, AC-AGENTS-SESSION-CATALOG-002.3, AC-AGENTS-SESSION-CATALOG-002.4, AC-AGENTS-SESSION-CATALOG-003.1, AC-AGENTS-SESSION-CATALOG-003.2, AC-AGENTS-SESSION-CATALOG-003.3
test("mobile: keeps catalog consent in one picker sheet and retains the conversation and draft", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}, testInfo) => {
  test.setTimeout(150000);
  await testPage.setViewportSize({ width: 320, height: 780 });
  expect(await testPage.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);
  const fixture = await createSessionCatalogScenario(testPage, apiClient, seedData, backend);
  try {
    const before = (await apiClient.getTaskSession(fixture.sessionId)).session;
    const retained = await captureRestartBaseline(apiClient, fixture.taskId, fixture.sessionId);
    const editor = await fixture.session.fillMessage("Keep this phone catalog draft");
    expect((await pastePng(testPage, editor, "phone-catalog-retained.png")).ok()).toBe(true);
    await waitForReadyDraftAttachment(testPage, fixture.sessionId, "phone-catalog-retained.png");
    const attachments = await testPage.evaluate(
      (id) => sessionStorage.getItem(`kandev.chatDraft.attachments.${id}`),
      fixture.sessionId,
    );
    writeSessionCatalogFile(fixture.catalogPath, "new-choices");
    const wireStart = fixture.traffic.frames.length;
    await refreshSavedSessionCatalog(apiClient, fixture.profile.id);
    const notice = testPage.getByTestId("session-catalog-refresh-notice");
    await expect(notice).toBeVisible();
    const action = notice.getByRole("button", { name: "Refresh session", exact: true });
    expect((await action.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await testInfo.attach("phone-session-catalog-notice", {
      body: await testPage.screenshot({
        path: testInfo.outputPath("phone-session-catalog-notice.png"),
      }),
      contentType: "image/png",
    });

    const model = testPage.getByRole("button", { name: "Session model settings", exact: true });
    await model.tap();
    await expect(
      testPage.getByRole("option", { name: "Catalog new model", exact: true }),
    ).toBeVisible();
    await testInfo.attach("phone-session-catalog-candidates", {
      body: await testPage.screenshot({
        path: testInfo.outputPath("phone-session-catalog-candidates.png"),
      }),
      contentType: "image/png",
    });
    await testPage.getByRole("option", { name: "Catalog new model", exact: true }).tap();
    const confirmation = testPage.getByTestId("session-catalog-refresh-confirmation");
    await expect(confirmation).toBeVisible();
    await expect(testPage.getByRole("dialog")).toHaveCount(1);
    expect(fixture.httpMutations).toEqual([]);
    expect(catalogMutationFrames(fixture.traffic.frames.slice(wireStart))).toEqual([]);
    await testInfo.attach("phone-session-catalog-picker-consent", {
      body: await testPage.screenshot({
        path: testInfo.outputPath("phone-session-catalog-picker-consent.png"),
      }),
      contentType: "image/png",
    });
    await testPage.getByRole("dialog").getByRole("button", { name: "Cancel", exact: true }).tap();
    await expect(confirmation).toHaveCount(0);
    await expect(
      testPage.getByRole("option", { name: "Catalog new model", exact: true }),
    ).toBeVisible();
    await expect
      .poll(() =>
        testPage.getByRole("dialog").evaluate((dialog) => dialog.contains(document.activeElement)),
      )
      .toBe(true);
    await closeModelConfigSheetIfOpen(testPage);

    await testPage.getByTestId("session-mode-selector").tap();
    await testPage.getByRole("button", { name: "Catalog plan", exact: true }).tap();
    await expect(confirmation).toBeVisible();
    await expect(testPage.getByRole("dialog")).toHaveCount(1);
    await testPage.getByRole("dialog").getByRole("button", { name: "Cancel", exact: true }).tap();
    await expect(testPage.getByRole("button", { name: "Catalog plan", exact: true })).toBeVisible();
    await expect
      .poll(() =>
        testPage.getByRole("dialog").evaluate((dialog) => dialog.contains(document.activeElement)),
      )
      .toBe(true);
    const modeDialog = testPage.getByRole("dialog", { name: "Agent permission mode", exact: true });
    await modeDialog.getByRole("button", { name: "Close", exact: true }).tap();
    await expect(modeDialog).toHaveCount(0);

    await action.tap();
    await expect(confirmation).toBeVisible();
    const dialog = testPage.getByRole("dialog");
    const confirm = dialog.getByRole("button", { name: "Refresh session", exact: true });
    const cancel = dialog.getByRole("button", { name: "Cancel", exact: true });
    await waitForFiniteAnimations(dialog);
    for (const control of [confirm, cancel]) {
      const box = (await control.boundingBox())!;
      expect(box.height).toBeGreaterThanOrEqual(44);
      expect(box.x).toBeGreaterThanOrEqual(0);
      expect(box.x + box.width).toBeLessThanOrEqual(320);
      expect(box.y).toBeGreaterThanOrEqual(0);
      expect(box.y + box.height).toBeLessThanOrEqual(780);
    }
    await assertNoDocumentHorizontalOverflow(testPage, "phone catalog confirmation");
    await testInfo.attach("phone-session-catalog-consent", {
      body: await testPage.screenshot({
        path: testInfo.outputPath("phone-session-catalog-consent.png"),
      }),
      contentType: "image/png",
    });
    const stop = fixture.wire.waitForResponse("session.stop", {
      requestWhere: (payload) => payload.session_id === fixture.sessionId,
    });
    const launch = fixture.wire.waitForResponse("session.launch", {
      timeout: 30000,
      timeoutAfter: stop,
      requestWhere: (payload) =>
        payload.session_id === fixture.sessionId && payload.intent === "resume",
    });
    await confirm.tap();
    await Promise.all([stop, launch]);
    await assertRestartRetained(apiClient, fixture.taskId, fixture.sessionId, retained);
    await expect
      .poll(
        async () =>
          catalogRecord((await apiClient.getTaskSession(fixture.sessionId)).session)?.disposition,
      )
      .toBe("cleared");
    const after = (await apiClient.getTaskSession(fixture.sessionId)).session;
    expect(after.metadata?.runtime_config).toEqual(before.metadata?.runtime_config);
    expect(after.metadata?.acp_config_baseline).toEqual(before.metadata?.acp_config_baseline);
    await expect(editor).toContainText("Keep this phone catalog draft");
    expect(
      await testPage.evaluate(
        (id) => sessionStorage.getItem(`kandev.chatDraft.attachments.${id}`),
        fixture.sessionId,
      ),
    ).toBe(attachments);
    await expect(model).toContainText("Mock Fast");
    assertCatalogNativeRetained(fixture);
    await testInfo.attach("phone-session-catalog-complete", {
      body: await testPage.screenshot({
        path: testInfo.outputPath("phone-session-catalog-complete.png"),
      }),
      contentType: "image/png",
    });
    await model.tap();
    const candidate = testPage.getByRole("option", { name: "Catalog new model", exact: true });
    await expect(candidate).not.toHaveAttribute("aria-disabled", "true");
    await candidate.tap();
    await expect
      .poll(
        async () =>
          (await apiClient.getTaskSession(fixture.sessionId)).session.metadata?.runtime_config,
      )
      .toMatchObject({ model: "catalog-new" });
    await closeModelConfigSheetIfOpen(testPage);
    await testPage.getByTestId("session-mode-selector").tap();
    await testPage.getByRole("button", { name: "Catalog plan", exact: true }).tap();
    await expect
      .poll(
        async () =>
          (await apiClient.getTaskSession(fixture.sessionId)).session.metadata?.runtime_config,
      )
      .toMatchObject({ mode: "catalog-plan" });
    await expect(confirmation).toHaveCount(0);
    await assertNoDocumentHorizontalOverflow(testPage, "phone refreshed provider catalog");
  } finally {
    await cleanupSessionCatalogScenario(apiClient, fixture);
  }
});

for (const late of ["matching", "newer-notice"] as const) {
  // @covers AC-AGENTS-SESSION-CATALOG-002.5, AC-AGENTS-SESSION-CATALOG-003.1, AC-AGENTS-SESSION-CATALOG-003.3
  test(`mobile: bounds missing discovery after Resume ACK and handles ${late} late delivery`, async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }, testInfo) => {
    test.setTimeout(150000);
    await verifySessionCatalogDiscoveryDeadline({
      page: testPage,
      api: apiClient,
      seed: seedData,
      backend: backend,
      touch: true,
      late: late,
      testInfo: testInfo,
    });
  });
}

// @covers AC-AGENTS-SESSION-CATALOG-002.3, AC-AGENTS-SESSION-CATALOG-003.1
test("mobile: resolves current profile inputs edited while refresh consent is open", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}, testInfo) => {
  test.setTimeout(150000);
  await verifyCatalogCurrentLaunchInputs({
    page: testPage,
    api: apiClient,
    seed: seedData,
    backend: backend,
    touch: true,
    testInfo: testInfo,
  });
});

// @covers AC-AGENTS-SESSION-CATALOG-002.4, AC-AGENTS-SESSION-CATALOG-003.1
test("mobile: retains native conversation and exposes recovery after a real restore rejection", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}, testInfo) => {
  test.setTimeout(180000);
  await verifyCatalogNativeRestoreFailure({
    page: testPage,
    api: apiClient,
    seed: seedData,
    backend: backend,
    touch: true,
    testInfo: testInfo,
  });
});

for (const variant of ["empty", "mode-only", "unsupported", "unchanged"] as const) {
  // @covers AC-AGENTS-SESSION-CATALOG-002.4, AC-AGENTS-SESSION-CATALOG-002.5, AC-AGENTS-SESSION-CATALOG-003.1
  test(`mobile: retires stale selectable models after native ${variant} discovery`, async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }, testInfo) => {
    test.setTimeout(150000);
    await verifyCatalogTerminalOutcome({
      page: testPage,
      api: apiClient,
      seed: seedData,
      backend: backend,
      touch: true,
      variant: variant,
      testInfo: testInfo,
    });
  });
}

// @covers AC-AGENTS-SESSION-CATALOG-002.3, AC-AGENTS-SESSION-CATALOG-002.4, AC-AGENTS-SESSION-CATALOG-003.1
test("mobile: surfaces uncertain interrupted-turn recovery without replaying the prompt", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}, testInfo) => {
  test.setTimeout(150000);
  await verifyCatalogInterruptedTurn({
    page: testPage,
    api: apiClient,
    seed: seedData,
    backend: backend,
    touch: true,
    testInfo: testInfo,
  });
});

// @covers AC-AGENTS-SESSION-CATALOG-002.1, AC-AGENTS-SESSION-CATALOG-002.2, AC-AGENTS-SESSION-CATALOG-003.1
test("mobile: keeps Quick Chat refresh bound to its own conversation and draft", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}, testInfo) => {
  test.setTimeout(180000);
  await verifyCatalogQuickChat({
    page: testPage,
    api: apiClient,
    seed: seedData,
    backend: backend,
    touch: true,
    testInfo: testInfo,
  });
});
