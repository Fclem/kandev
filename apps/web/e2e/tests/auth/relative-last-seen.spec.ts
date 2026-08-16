import { expect, type BrowserContext } from "@playwright/test";
import path from "node:path";
import { backendFixture as test } from "../../fixtures/backend";
import { login, setupAdmin } from "../../helpers/auth";
import { watchWs } from "../../helpers/causal-waits";

/**
 * Relative "Last seen" on the account security page (desktop + cross-tab).
 *
 * Runs in the `auth` project with the worker backend restarted to require
 * auth and use its own database. Serial: auth setup is single-shot per
 * database and the worker DB survives restarts, so the file owns its own
 * `KANDEV_DATABASE_PATH` (the users-self-actions isolation pattern) and
 * restores the baseline backend in afterAll.
 */
const ADMIN = { email: "admin@demo.dev", password: "adminpass123", displayName: "Ada Admin" };
const SECURITY_PATH = "/settings/account/security";

async function readLastSeenDisplay(ctx: BrowserContext, baseUrl: string): Promise<string> {
  const res = await ctx.request.get(`${baseUrl}/api/v1/user/settings`);
  expect(res.ok(), await res.text()).toBeTruthy();
  const body = (await res.json()) as { settings: { last_seen_display?: string } };
  return body.settings.last_seen_display ?? "absolute";
}

async function restoreLastSeenDisplay(
  ctx: BrowserContext,
  baseUrl: string,
  value: string,
): Promise<void> {
  const res = await ctx.request.patch(`${baseUrl}/api/v1/user/settings`, {
    data: { last_seen_display: value },
  });
  expect(res.ok(), await res.text()).toBeTruthy();
}

test.describe.serial("relative last seen (desktop)", () => {
  test.beforeAll(async ({ backend }) => {
    await backend.restart({
      KANDEV_FEATURES_AUTH: "true",
      KANDEV_DATABASE_PATH: path.join(backend.tmpDir, "kandev-relative-last-seen.db"),
    });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("switches Last seen to relative time and shows a relative label with an absolute tooltip", async ({
    browser,
    backend,
  }) => {
    const ctx = await browser.newContext({ baseURL: backend.frontendUrl });
    await setupAdmin(ctx, backend.baseUrl, ADMIN);
    await login(ctx, backend.baseUrl, ADMIN);
    const original = await readLastSeenDisplay(ctx, backend.baseUrl);

    try {
      const page = await ctx.newPage();
      await page.goto(SECURITY_PATH);

      const select = page.getByTestId("last-seen-display-select");
      await expect(select).toBeVisible({ timeout: 15_000 });

      await select.click();
      await page.getByRole("option", { name: "Relative time" }).click();

      const relative = page.getByTestId("last-seen-relative").first();
      await expect(relative).toBeVisible({ timeout: 15_000 });

      // Hover reveals the absolute timestamp in a tooltip, matching the
      // trigger's accessible name/native title exactly.
      const absolute = await relative.getAttribute("title");
      expect(absolute).toBeTruthy();
      await relative.hover();
      const tooltip = page.getByRole("tooltip");
      await expect(tooltip).toBeVisible();
      await expect(tooltip).toHaveText(absolute!);

      // The choice persists to the user-settings API.
      await expect
        .poll(() => readLastSeenDisplay(ctx, backend.baseUrl), { timeout: 15_000 })
        .toBe("relative");
    } finally {
      await restoreLastSeenDisplay(ctx, backend.baseUrl, original);
      await ctx.close();
    }
  });

  test("syncs a Last seen change to another tab over WebSocket without a reload", async ({
    browser,
    backend,
  }) => {
    const ctxA = await browser.newContext({ baseURL: backend.frontendUrl });
    const ctxB = await browser.newContext({ baseURL: backend.frontendUrl });
    await login(ctxA, backend.baseUrl, ADMIN);
    await login(ctxB, backend.baseUrl, ADMIN);
    const original = await readLastSeenDisplay(ctxA, backend.baseUrl);

    try {
      const pageA = await ctxA.newPage();
      // watchWs only observes sockets opened after it is armed, and the
      // subscription wait itself must be armed before navigation.
      const watcher = watchWs(pageA);
      const subscriptionAck = watcher.waitForResponse("user.subscribe");
      await pageA.goto(SECURITY_PATH);
      await subscriptionAck;

      const pageB = await ctxB.newPage();
      await pageB.goto(SECURITY_PATH);
      await pageB.getByTestId("last-seen-display-select").click();
      await pageB.getByRole("option", { name: "Relative time" }).click();
      await expect(pageB.getByTestId("last-seen-relative").first()).toBeVisible({
        timeout: 15_000,
      });

      // Tab A observes the change without a reload.
      await expect(pageA.getByTestId("last-seen-relative").first()).toBeVisible({
        timeout: 15_000,
      });
    } finally {
      await restoreLastSeenDisplay(ctxA, backend.baseUrl, original);
      await ctxA.close();
      await ctxB.close();
    }
  });
});
