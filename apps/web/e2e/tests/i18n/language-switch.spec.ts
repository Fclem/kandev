import { test, expect } from "../../fixtures/test-base";

/**
 * i18n language switcher + pseudo-locale.
 *
 * The pseudo-locale accents/pads every extracted message (`Language` →
 * `Ĺàńĝũàĝē`), so it doubles as the completeness oracle for the string-
 * externalization sweep: any plain-ASCII user-facing text under `pseudo` is a
 * literal that was never wrapped in a Lingui macro.
 *
 * See docs/specs/platform/i18n.md and docs/i18n.md.
 */

const APPEARANCE_URL = "/settings/general/appearance";

/** Latin letters carrying diacritics, as produced by Lingui pseudolocalization. */
const ACCENTED = /[À-ɏ]/;

test.describe("i18n language switcher", () => {
  test("defaults to English with lang=en", async ({ testPage }) => {
    await testPage.goto(APPEARANCE_URL);

    await expect(testPage.locator("html")).toHaveAttribute("lang", "en");
    await expect(testPage.getByLabel("Display language")).toBeVisible({ timeout: 10_000 });
  });

  test("switching to pseudo re-renders accented copy and survives reload", async ({ testPage }) => {
    await testPage.goto(APPEARANCE_URL);

    const select = testPage.getByLabel("Display language");
    await expect(select).toBeVisible({ timeout: 10_000 });
    await select.click();
    await testPage.getByRole("option", { name: /Pseudo/ }).click();

    // Locale activates client-side: <html lang> flips and chrome is accented.
    await expect(testPage.locator("html")).toHaveAttribute("lang", "pseudo", { timeout: 10_000 });
    await expect(testPage.locator("body")).toHaveText(ACCENTED, { timeout: 10_000 });

    // The kandev_locale cookie is the source of truth, so the Go shell serves
    // lang="pseudo" on the very next request — no flash of English.
    await testPage.reload();
    await expect(testPage.locator("html")).toHaveAttribute("lang", "pseudo", { timeout: 10_000 });

    // Switch back so the persisted cookie does not leak into later tests.
    const selectAfter = testPage.getByLabel(/Display language|Ďĩśƥĺàŷ ĺàńĝũàĝē/);
    await selectAfter.click();
    await testPage.getByRole("option", { name: /English|Ēńĝĺĩśĥ/ }).click();
    await expect(testPage.locator("html")).toHaveAttribute("lang", "en", { timeout: 10_000 });
  });
});
