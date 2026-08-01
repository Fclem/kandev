import { test, expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { NIGHTLY_VERSION, useManagedNPMUpdates } from "./updates-channel-helpers";

test.describe("System update channel", () => {
  test("selects and persists Nightly before offering the exact target", async ({
    backend,
    testPage,
  }, testInfo) => {
    test.setTimeout(60_000);
    const capture = new PrAssetCapture(testPage, testInfo.file);
    const fixture = await useManagedNPMUpdates(backend);
    try {
      await testPage.goto("/settings/system/updates");
      const stable = testPage.getByRole("radio", { name: /^Stable/ });
      const nightly = testPage.getByRole("radio", { name: /^Nightly/ });
      await expect(stable).toBeChecked();
      await expect(nightly).toBeEnabled();

      const saved = testPage.waitForResponse(
        (response) =>
          response.request().method() === "PATCH" &&
          new URL(response.url()).pathname === "/api/v1/system/updates/channel",
      );
      await nightly.click();
      await testPage
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Save changes" })
        .click();
      expect((await saved).status()).toBe(200);

      await expect(nightly).toBeChecked();
      await expect(testPage.getByTestId("system-updates-latest")).toHaveText(NIGHTLY_VERSION);
      expect(fixture.registryRequests()).toBeGreaterThanOrEqual(1);

      await testPage.reload();
      await expect(nightly).toBeChecked();
      await expect(testPage.getByTestId("system-updates-latest")).toHaveText(NIGHTLY_VERSION);

      const desktopViewport = testPage.viewportSize();
      await capture.screenshot("desktop-nightly-update-channel", {
        caption: "Desktop: managed npm service following the Nightly channel",
      });
      await testPage.setViewportSize({ width: 393, height: 851 });
      await expect(testPage.getByTestId("system-updates-channel")).toBeVisible();
      await capture.screenshot("mobile-nightly-update-channel", {
        caption: "Mobile: the same saved Nightly channel and exact target",
      });
      if (desktopViewport) await testPage.setViewportSize(desktopViewport);
      capture.flush();

      await makeExactNightlyAvailable(testPage);
      let applyBody: unknown;
      await testPage.route("**/api/v1/system/updates/apply", async (route) => {
        applyBody = route.request().postDataJSON();
        await route.fulfill({
          status: 202,
          contentType: "application/json",
          body: JSON.stringify({ job_id: "nightly-update-1" }),
        });
      });
      await testPage.getByTestId("system-updates-apply").click();
      await expect(testPage.getByRole("alertdialog")).toContainText(NIGHTLY_VERSION);
      await testPage.getByTestId("system-updates-apply-confirm").click();
      await expect.poll(() => applyBody).toEqual({ confirm: "UPDATE" });
    } finally {
      await fixture.release();
    }
  });

  test("keeps unsupported installs on Stable with no Nightly mutation path", async ({
    testPage,
  }) => {
    let channelMutations = 0;
    testPage.on("request", (request) => {
      if (
        request.method() === "PATCH" &&
        new URL(request.url()).pathname === "/api/v1/system/updates/channel"
      ) {
        channelMutations += 1;
      }
    });

    await testPage.goto("/settings/system/updates");

    await expect(testPage.getByRole("radio", { name: /^Stable/ })).toBeChecked();
    await expect(testPage.getByRole("radio", { name: /^Nightly/ })).toBeDisabled();
    await expect(testPage.getByTestId("system-updates-channel-reason")).toContainText(
      /managed npm or npx user service/i,
    );
    await expect(testPage.getByTestId("settings-floating-save")).toHaveCount(0);
    expect(channelMutations).toBe(0);
  });
});

async function makeExactNightlyAvailable(testPage: Page): Promise<void> {
  await testPage.route("**/api/v1/system/updates/check", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        current: "v1.0.0",
        latest: NIGHTLY_VERSION,
        latest_url: `https://www.npmjs.com/package/kandev/v/${NIGHTLY_VERSION}`,
        latest_checked_at: new Date().toISOString(),
        update_available: true,
        channel: "nightly",
        channel_editable: true,
        channel_unsupported_reason: "",
        install: {
          running_as_service: true,
          managed_service: true,
          mode: "user",
          manager: "systemd",
          kind: "npm",
        },
        apply_supported: true,
      }),
    });
  });
  await testPage.getByTestId("system-updates-check").click();
  await expect(testPage.getByTestId("system-updates-apply")).toBeVisible();
}
