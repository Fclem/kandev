import { devices, expect } from "@playwright/test";
import path from "node:path";
import { backendFixture as test } from "../../fixtures/backend";
import { login, setupAdmin } from "../../helpers/auth";

/**
 * Mobile parity for the relative "Last seen" option: the select is operated
 * with touch-native `tap()` (mouse-semantics clicks would not prove touch),
 * relative labels render without hover, and the trigger/option meet the 44px
 * active-dimension touch target at the Pixel 5 viewport.
 *
 * Named `mobile-*` so the `mobile-chrome` project routes it away from the
 * desktop `auth` project. Manual contexts do not inherit project device
 * options, so the Pixel 5 viewport is spread explicitly.
 */
const ADMIN = { email: "admin@demo.dev", password: "adminpass123", displayName: "Ada Admin" };
const SECURITY_PATH = "/settings/account/security";

test.describe.serial("relative last seen (mobile)", () => {
  test.beforeAll(async ({ backend }) => {
    await backend.restart({
      KANDEV_FEATURES_AUTH: "true",
      KANDEV_DATABASE_PATH: path.join(backend.tmpDir, "kandev-mobile-relative-last-seen.db"),
    });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("operates the Last seen select with touch and renders relative labels without hover", async ({
    browser,
    backend,
  }) => {
    const ctx = await browser.newContext({
      ...devices["Pixel 5"],
      baseURL: backend.frontendUrl,
    });
    await setupAdmin(ctx, backend.baseUrl, ADMIN);
    await login(ctx, backend.baseUrl, ADMIN);
    const originalRes = await ctx.request.get(`${backend.baseUrl}/api/v1/user/settings`);
    expect(originalRes.ok(), await originalRes.text()).toBeTruthy();
    const originalBody = (await originalRes.json()) as {
      settings: { last_seen_display?: string };
    };
    const original = originalBody.settings.last_seen_display ?? "absolute";

    try {
      const page = await ctx.newPage();
      // Manual contexts do not inherit project device options; pin the width.
      expect((await page.viewportSize())?.width).toBe(393);

      await page.goto(SECURITY_PATH);

      const trigger = page.getByTestId("last-seen-display-select");
      await expect(trigger).toBeVisible({ timeout: 15_000 });

      // The trigger meets the 44px active-dimension touch target.
      const triggerBox = await trigger.boundingBox();
      expect(triggerBox).not.toBeNull();
      expect(triggerBox!.height).toBeGreaterThanOrEqual(44);

      await trigger.tap();
      const option = page.getByRole("option", { name: "Relative time" });
      await expect(option).toBeVisible();
      const optionBox = await option.boundingBox();
      expect(optionBox).not.toBeNull();
      expect(Math.round(optionBox!.height)).toBeGreaterThanOrEqual(44);
      await option.tap();

      // Relative labels render without hover, with no horizontal overflow.
      const relative = page.getByTestId("last-seen-relative").first();
      await expect(relative).toBeVisible({ timeout: 15_000 });
      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
      );
      expect(overflow).toBe(false);

      // The absolute stamp stays reachable without a tooltip, via the
      // trigger's accessible name / native title.
      const absolute = await relative.getAttribute("title");
      expect(absolute).toBeTruthy();
      await expect(relative).toHaveAttribute("aria-label", absolute!);
    } finally {
      const res = await ctx.request.patch(`${backend.baseUrl}/api/v1/user/settings`, {
        data: { last_seen_display: original },
      });
      expect(res.ok(), await res.text()).toBeTruthy();
      await ctx.close();
    }
  });
});
