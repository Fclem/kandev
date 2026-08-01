import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { NIGHTLY_TAG, useManagedNPMUpdates } from "./updates-channel-helpers";

test.describe("System update channel on mobile", () => {
  test("selects, saves, and reloads Nightly with touch-safe rows", async ({
    backend,
    testPage,
  }) => {
    test.setTimeout(60_000);
    const fixture = await useManagedNPMUpdates(backend);
    try {
      await testPage.goto("/settings/system/updates");
      const nightly = testPage.getByRole("radio", { name: /^Nightly/ });
      const nightlyRow = testPage.getByTestId("system-updates-channel-nightly");
      const rowBox = await nightlyRow.boundingBox();
      expect(rowBox).not.toBeNull();
      expect(rowBox!.height).toBeGreaterThanOrEqual(44);
      await assertNoDocumentHorizontalOverflow(testPage, "Updates before channel selection");

      await nightlyRow.tap();
      await expect(nightly).toBeChecked();
      await testPage
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Save changes" })
        .tap();
      await expect(testPage.getByTestId("system-updates-latest")).toHaveText(NIGHTLY_TAG);

      await testPage.reload();
      await expect(nightly).toBeChecked();
      await expect(testPage.getByTestId("system-updates-latest")).toHaveText(NIGHTLY_TAG);
      await assertNoDocumentHorizontalOverflow(testPage, "Updates after Nightly reload");
    } finally {
      await fixture.release();
    }
  });

  test("explains an unsupported Nightly channel without horizontal overflow", async ({
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
    await expect(testPage.getByTestId("system-updates-channel-reason")).toBeVisible();
    await expect(testPage.getByTestId("settings-floating-save")).toHaveCount(0);
    expect(channelMutations).toBe(0);
    await assertNoDocumentHorizontalOverflow(testPage, "Unsupported Updates channel");
  });
});
