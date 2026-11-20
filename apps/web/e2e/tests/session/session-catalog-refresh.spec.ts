import { expect } from "@playwright/test";
import type { Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import {
  assertCatalogNativeRetained,
  catalogMutationFrames,
  catalogRecord,
  cleanupSessionCatalogScenario,
  clickSavedCatalogRefresh,
  createSessionCatalogScenario,
  openCatalogProfilePage,
  writeSessionCatalogFile,
} from "../../helpers/session-catalog-refresh";
import { assertRestartRetained, captureRestartBaseline } from "./session-tab-restart-helpers";
import { pastePng, waitForReadyDraftAttachment } from "../chat/composer-attachment-scope-helpers";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { verifySessionCatalogDiscoveryDeadline } from "../../helpers/session-catalog-discovery-loss";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import {
  verifyCatalogCurrentLaunchInputs,
  verifyCatalogNativeRestoreFailure,
} from "../../helpers/session-catalog-launch-inputs";
import { verifyCatalogTerminalOutcome } from "../../helpers/session-catalog-terminal-outcomes";
import {
  verifyCatalogInterruptedTurn,
  verifyCatalogPreview,
  verifyCatalogQuickChat,
} from "../../helpers/session-catalog-additional-surfaces";

// @covers AC-AGENTS-SESSION-CATALOG-001.1, AC-AGENTS-SESSION-CATALOG-001.3, AC-AGENTS-SESSION-CATALOG-001.4, AC-AGENTS-SESSION-CATALOG-001.5, AC-AGENTS-SESSION-CATALOG-002.1, AC-AGENTS-SESSION-CATALOG-002.2, AC-AGENTS-SESSION-CATALOG-002.3, AC-AGENTS-SESSION-CATALOG-002.4, AC-AGENTS-SESSION-CATALOG-003.1
test("publishes saved catalog drift to the open composer and requires consent before executor rediscovery", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}, testInfo) => {
  test.setTimeout(150000);
  await testPage.setViewportSize({ width: 1440, height: 1000 });
  expect(await testPage.evaluate(() => matchMedia("(pointer: fine)").matches)).toBe(true);
  const fixture = await createSessionCatalogScenario(testPage, apiClient, seedData, backend);
  let settings: Page | undefined;
  try {
    const before = (await apiClient.getTaskSession(fixture.sessionId)).session;
    const retained = await captureRestartBaseline(apiClient, fixture.taskId, fixture.sessionId);
    const editor = await fixture.session.fillMessage("Unsent session catalog work");
    expect((await pastePng(testPage, editor, "catalog-retained.png")).ok()).toBe(true);
    await waitForReadyDraftAttachment(testPage, fixture.sessionId, "catalog-retained.png");
    const attachments = await testPage.evaluate(
      (id) => sessionStorage.getItem(`kandev.chatDraft.attachments.${id}`),
      fixture.sessionId,
    );
    writeSessionCatalogFile(fixture.catalogPath, "new-choices");
    settings = await openCatalogProfilePage(testPage, fixture);
    const notice = testPage.getByTestId("session-catalog-refresh-notice");
    await expect(notice).toHaveCount(0);
    const wireStart = fixture.traffic.frames.length;
    await clickSavedCatalogRefresh(settings, fixture.profile.id);
    await expect(notice).toBeVisible();
    const published = (await apiClient.getTaskSession(fixture.sessionId)).session;
    await testInfo.attach("published-catalog-observation", {
      body: JSON.stringify(catalogRecord(published)),
      contentType: "application/json",
    });
    expect(catalogRecord(published)).toMatchObject({
      disposition: "host_candidates",
      reason: "new_candidates",
      owner: { execution_id: before.agent_execution_id },
    });
    expect(published.agent_execution_id).toBe(before.agent_execution_id);
    expect(published.metadata?.runtime_config).toEqual(before.metadata?.runtime_config);
    expect(published.metadata?.acp_config_baseline).toEqual(before.metadata?.acp_config_baseline);
    await clickSavedCatalogRefresh(settings, fixture.profile.id);
    const repeated = (await apiClient.getTaskSession(fixture.sessionId)).session;
    expect(catalogRecord(repeated)).toEqual(catalogRecord(published));
    expect(repeated.agent_execution_id).toBe(before.agent_execution_id);
    await expect(notice).toHaveCount(1);

    const model = testPage.getByRole("button", { name: "Session model settings", exact: true });
    await model.click();
    await waitForFiniteAnimations(testPage.locator("body"));
    const host = testPage.getByRole("group", { name: "Fresh models", exact: true });
    await expect(host.getByRole("option")).toHaveCount(2);
    expect(
      await host
        .getByRole("option")
        .evaluateAll((rows) => rows.map((row) => row.getAttribute("aria-label"))),
    ).toEqual(["Catalog new model", "Mock Fast"]);
    await expect(testPage.getByRole("option", { name: "Mock Smart", exact: true })).toBeVisible();
    await waitForFiniteAnimations(testPage.locator("body"));
    await testInfo.attach("desktop-session-catalog-candidates", {
      body: await testPage.screenshot({
        path: testInfo.outputPath("desktop-session-catalog-candidates.png"),
      }),
      contentType: "image/png",
    });
    await testPage.getByRole("option", { name: "Catalog new model", exact: true }).click();
    const confirmation = testPage.getByTestId("session-catalog-refresh-confirmation");
    await expect(confirmation).toBeVisible();
    expect(fixture.httpMutations).toEqual([]);
    expect(catalogMutationFrames(fixture.traffic.frames.slice(wireStart))).toEqual([]);
    await confirmation.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(model).toBeFocused();
    await testPage.keyboard.press("Escape");
    await testPage.getByTestId("session-mode-selector").click();
    await testPage.getByRole("menuitem", { name: /Catalog plan/ }).click();
    await expect(confirmation).toBeVisible();
    expect(fixture.httpMutations).toEqual([]);
    expect(catalogMutationFrames(fixture.traffic.frames.slice(wireStart))).toEqual([]);
    await confirmation.getByRole("button", { name: "Cancel", exact: true }).click();

    await notice.getByRole("button", { name: "Refresh session", exact: true }).click();
    await expect(confirmation).toBeVisible();
    await waitForFiniteAnimations(confirmation);
    await testInfo.attach("desktop-session-catalog-consent", {
      body: await testPage.screenshot({
        path: testInfo.outputPath("desktop-session-catalog-consent.png"),
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
    await confirmation.getByRole("button", { name: "Refresh session", exact: true }).click();
    await Promise.all([stop, launch]);
    await assertRestartRetained(apiClient, fixture.taskId, fixture.sessionId, retained);
    await expect
      .poll(
        async () =>
          catalogRecord((await apiClient.getTaskSession(fixture.sessionId)).session)?.disposition,
      )
      .toBe("cleared");
    const after = (await apiClient.getTaskSession(fixture.sessionId)).session;
    expect(after.agent_execution_id).not.toBe(before.agent_execution_id);
    expect(catalogRecord(after)?.owner.execution_id).toBe(after.agent_execution_id);
    expect(catalogRecord(after)?.result?.outcome).toBe("complete");
    expect(after.metadata?.runtime_config).toEqual(before.metadata?.runtime_config);
    expect(after.metadata?.acp_config_baseline).toEqual(before.metadata?.acp_config_baseline);
    await expect(editor).toContainText("Unsent session catalog work");
    expect(
      await testPage.evaluate(
        (id) => sessionStorage.getItem(`kandev.chatDraft.attachments.${id}`),
        fixture.sessionId,
      ),
    ).toBe(attachments);
    assertCatalogNativeRetained(fixture);
    await expect(model).toContainText("Mock Fast");
    await testInfo.attach("desktop-session-catalog-complete", {
      body: await testPage.screenshot({
        path: testInfo.outputPath("desktop-session-catalog-complete.png"),
      }),
      contentType: "image/png",
    });
    await model.click();
    await expect(
      testPage.getByRole("option", { name: "Catalog new model", exact: true }),
    ).not.toHaveAttribute("aria-disabled", "true");
    await testPage.getByRole("option", { name: "Catalog new model", exact: true }).click();
    await expect
      .poll(
        async () =>
          (await apiClient.getTaskSession(fixture.sessionId)).session.metadata?.runtime_config,
      )
      .toMatchObject({ model: "catalog-new" });
    await expect(confirmation).toHaveCount(0);
    expect((await apiClient.getAgentProfile(fixture.profile.id)).model).toBe("mock-fast");
    await testPage.reload();
    await fixture.session.waitForLoad();
    await expect(model).toContainText("Catalog new model");
    await expect(notice.getByRole("button", { name: "Refresh session", exact: true })).toHaveCount(
      0,
    );
    await assertNoDocumentHorizontalOverflow(testPage, "desktop refreshed session catalog");
  } finally {
    await settings?.close();
    await cleanupSessionCatalogScenario(apiClient, fixture);
  }
});

for (const late of ["matching", "newer-notice"] as const) {
  // @covers AC-AGENTS-SESSION-CATALOG-002.5, AC-AGENTS-SESSION-CATALOG-003.1
  test(`bounds missing discovery after Resume ACK and handles ${late} late delivery`, async ({
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
      touch: false,
      late: late,
      testInfo: testInfo,
    });
  });
}

// @covers AC-AGENTS-SESSION-CATALOG-003.1, AC-AGENTS-SESSION-CATALOG-003.2
test("keeps refresh available at the 767px fine-pointer boundary", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}, testInfo) => {
  test.setTimeout(150000);
  await testPage.setViewportSize({ width: 767, height: 820 });
  expect(await testPage.evaluate(() => matchMedia("(pointer: fine)").matches)).toBe(true);
  await verifySessionCatalogDiscoveryDeadline({
    page: testPage,
    api: apiClient,
    seed: seedData,
    backend: backend,
    touch: false,
    late: "newer-notice",
    testInfo: testInfo,
  });
});

// @covers AC-AGENTS-SESSION-CATALOG-002.3, AC-AGENTS-SESSION-CATALOG-002.4
test("resolves current profile inputs edited while refresh consent is open", async ({
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
    touch: false,
    testInfo: testInfo,
  });
});

// @covers AC-AGENTS-SESSION-CATALOG-002.4, AC-AGENTS-SESSION-CATALOG-002.5
test("retains native conversation and exposes recovery after a real restore rejection", async ({
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
    touch: false,
    testInfo: testInfo,
  });
});

for (const variant of ["empty", "mode-only", "unsupported", "unchanged"] as const) {
  // @covers AC-AGENTS-SESSION-CATALOG-002.4, AC-AGENTS-SESSION-CATALOG-002.5
  test(`retires stale selectable models after native ${variant} discovery`, async ({
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
      touch: false,
      variant: variant,
      testInfo: testInfo,
    });
  });
}

// @covers AC-AGENTS-SESSION-CATALOG-002.1, AC-AGENTS-SESSION-CATALOG-002.2, AC-AGENTS-SESSION-CATALOG-003.1
test("keeps refresh bound to the preview session and its unsent draft", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}, testInfo) => {
  test.setTimeout(150000);
  await verifyCatalogPreview(testPage, apiClient, seedData, backend, testInfo);
});

// @covers AC-AGENTS-SESSION-CATALOG-002.3, AC-AGENTS-SESSION-CATALOG-002.4
test("surfaces uncertain interrupted-turn recovery without replaying the prompt", async ({
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
    touch: false,
    testInfo: testInfo,
  });
});

// @covers AC-AGENTS-SESSION-CATALOG-002.1, AC-AGENTS-SESSION-CATALOG-002.2, AC-AGENTS-SESSION-CATALOG-003.1
test("keeps Quick Chat refresh bound to its own conversation and draft", async ({
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
    touch: false,
    testInfo: testInfo,
  });
});
