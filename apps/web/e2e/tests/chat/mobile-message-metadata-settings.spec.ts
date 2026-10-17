import { test, expect } from "../../fixtures/test-base";

test.describe("message metadata settings on mobile", () => {
  test("keeps every field label touch-sized without horizontal overflow", async ({ testPage }) => {
    await testPage.goto("/settings/preferences/task-behavior?tab=conversation");
    const card = testPage.getByTestId("message-metadata-settings");
    await expect(card).toBeVisible();
    const labels = card.locator("label");
    await expect(labels).toHaveCount(7);
    await card.scrollIntoViewIfNeeded();

    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    for (let index = 0; index < 7; index += 1) {
      const label = labels.nth(index);
      await label.scrollIntoViewIfNeeded();
      const box = await label.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.x).toBeGreaterThanOrEqual(0);
      expect(box!.x + box!.width).toBeLessThanOrEqual(viewport!.width);
    }
    await expect
      .poll(() =>
        testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      )
      .toBe(true);
    const cardFits = await card.evaluate((el) => el.scrollWidth <= el.clientWidth);
    expect(cardFits).toBe(true);
  });
});
