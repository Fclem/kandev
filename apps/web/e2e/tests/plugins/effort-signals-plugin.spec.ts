import { expect, test } from "../../fixtures/test-base";
import { uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import { dwell } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";
import {
  installEffortFixture,
  readEffort,
  checkEffortRoute,
  seedEffortFeedback,
} from "./effort-signals-helpers";
import { KanbanPage } from "../../pages/kanban-page";

declare global {
  interface Window {
    setEffortHidden(hidden: boolean): void;
  }
}

test("AC-PLUGINS-EFFORT-SIGNALS-005.2: fresh Host activity stops under emulated visibility and resumes", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(180000);
  await backend.restart({ KANDEV_FEATURES_SESSION_USER_ACTIVITY: "true" });
  try {
    await testPage.addInitScript(() => {
      const visibility = Object.getOwnPropertyDescriptor(Document.prototype, "visibilityState");
      const focus = document.hasFocus;
      window.setEffortHidden = (hidden) => {
        if (hidden) {
          Object.defineProperty(document, "visibilityState", {
            configurable: true,
            get: () => "hidden",
          });
          document.hasFocus = () => false;
        } else {
          Reflect.deleteProperty(document, "visibilityState");
          document.hasFocus = focus;
        }
        document.dispatchEvent(new Event("visibilitychange"));
        window.dispatchEvent(new Event(hidden ? "blur" : "focus"));
      };
      if (!visibility) throw new Error("Browser visibility descriptor unavailable");
    });
    await installEffortFixture(testPage, apiClient, seedData.workspaceId);
    const task = await apiClient.createTask(seedData.workspaceId, "Effort capture fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.seedTaskSession(task.id, {
      state: "IDLE",
      agentProfileId: seedData.agentProfileId,
      repositoryId: seedData.repositoryId,
    });
    await seedEffortFeedback(apiClient, seedData.workspaceId, seedData.repositoryId, task.id);
    let beats = 0;
    testPage.on("websocket", (socket) =>
      socket.on("framesent", (frame) => {
        if (String(frame.payload).includes("session.user_activity.beat")) beats++;
      }),
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.bringToFront();
    await session.activeChat().dispatchEvent("pointerdown");
    await expect.poll(() => beats).toBeGreaterThan(0);
    await dwell(
      testPage,
      20000,
      "negative-assertion",
      "Capture two activity beats including the immediate engagement beat",
    );
    const baseline = await readEffort(apiClient, seedData.workspaceId, task.id);
    expect(baseline.active_ms).toBeGreaterThan(0);
    await testPage.evaluate(() => window.setEffortHidden(true));
    await expect.poll(() => testPage.evaluate(() => document.visibilityState)).toBe("hidden");
    const hiddenBeats = beats;
    await dwell(
      testPage,
      30000,
      "negative-assertion",
      "Observe two beat periods while visibility and focus are explicitly emulated",
    );
    expect(beats).toBe(hiddenBeats);
    expect((await readEffort(apiClient, seedData.workspaceId, task.id)).active_ms).toBe(
      baseline.active_ms,
    );
    await testPage.evaluate(() => window.setEffortHidden(false));
    await session.activeChat().dispatchEvent("pointerdown");
    await expect.poll(() => beats).toBeGreaterThan(hiddenBeats);
    await checkEffortRoute(testPage, task.id);
    await testPage.screenshot({ path: test.info().outputPath("effort-desktop.png") });
    await testPage.goto(`/t/${task.id}`);
    await session.waitForLoad();
    await session.addPanelButton().click();
    await session.addPanelPluginItem("kandev-plugin-e2e", "effort").click();
    await testPage.getByTestId("effort-summary").click();
    await expect(testPage.getByTestId("effort-result")).toBeVisible();
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await new KanbanPage(testPage).goto(seedData.workflowId);
    await expect(
      testPage.locator(`[data-testid="effort-badge"][data-task-id="${task.id}"]`),
    ).toBeVisible();
  } finally {
    await test
      .info()
      .attach("effort-backend-log", { path: backend.logPath, contentType: "text/plain" });
    await uninstallFixturePlugin(apiClient);
    await backend.restart();
  }
});
