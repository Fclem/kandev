import { expect, test } from "../../fixtures/test-base";

test.describe("bfcache restore reload", () => {
  test("reloads the app when the page is restored from a frozen snapshot", async ({ testPage }) => {
    await testPage.goto("/");

    // A normal load is not a restore: the document must not reload by itself.
    expect(await testPage.evaluate(() => performance.getEntriesByType("navigation")[0]?.type)).toBe(
      "navigate",
    );

    // Chrome's "Duplicate tab" (and any bfcache restore) brings the page back
    // from a frozen snapshot and fires pageshow with persisted=true. The app
    // reloads so the no-store boot payload is re-fetched instead of showing
    // the frozen state.
    await testPage
      .evaluate(() => {
        window.dispatchEvent(new PageTransitionEvent("pageshow", { persisted: true }));
      })
      .catch(() => undefined); // the reload may tear down the evaluate context

    // The reload destroys the execution context mid-flight; each evaluate may
    // land on a dying document. Retry until a fresh document reports the
    // reload navigation type.
    await expect(async () => {
      const type = await testPage
        .evaluate(() => performance.getEntriesByType("navigation")[0]?.type)
        .catch(() => undefined);
      expect(type).toBe("reload");
    }).toPass({ timeout: 15_000 });
  });
});
