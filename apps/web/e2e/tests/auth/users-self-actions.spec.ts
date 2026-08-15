import { expect } from "@playwright/test";
import { backendFixture as test } from "../../fixtures/backend";
import { login, setupAdmin } from "../../helpers/auth";

/**
 * The Users page must grey out the role/status toggles on an active-admin row
 * when no other active admin exists, mirroring the backend's existing
 * last-admin guard (ErrLastAdmin -> 409). The backend itself is unchanged:
 * self-demotion stays allowed while another active admin exists.
 *
 * Runs in the `auth` project (backend restarted with auth required). Serial:
 * shares the admin and users created inside the single test.
 */
const ADMIN = { email: "admin@demo.dev", password: "adminpass123", displayName: "Ada Admin" };
const MEMBER = { email: "sam@demo.dev", password: "memberpass123", displayName: "Sam Member" };
const SECOND_ADMIN = { email: "bob@demo.dev", password: "bobpass123", displayName: "Bob Admin" };

/** Reads `user.id` from an API response body, narrowing the unknown shape. */
function responseUserId(body: unknown): string {
  if (body && typeof body === "object" && "user" in body) {
    const user = body.user;
    if (user && typeof user === "object" && "id" in user) {
      return String(user.id);
    }
  }
  throw new Error("API response is missing user.id");
}

test.describe.serial("users self-actions guard", () => {
  test.beforeAll(async ({ backend }) => {
    // The features.auth flag turns authentication on (setup mode) and reveals
    // the admin surfaces.
    await backend.restart({ KANDEV_FEATURES_AUTH: "true" });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("own-row toggles track the last-admin guard; backend unchanged", async ({
    browser,
    backend,
  }) => {
    const ctx = await browser.newContext({ baseURL: backend.frontendUrl });
    await setupAdmin(ctx, backend.baseUrl, ADMIN);
    await login(ctx, backend.baseUrl, ADMIN);

    // The setup helper does not return the user; fetch the admin id.
    const meRes = await ctx.request.get(`${backend.baseUrl}/api/v1/auth/me`);
    expect(meRes.ok(), await meRes.text()).toBeTruthy();
    const adminId = responseUserId(await meRes.json());

    // A member row provides the enabled baseline.
    const memberRes = await ctx.request.post(`${backend.baseUrl}/api/v1/users`, {
      data: {
        email: MEMBER.email,
        password: MEMBER.password,
        display_name: MEMBER.displayName,
        role: "member",
      },
    });
    expect(memberRes.status(), await memberRes.text()).toBe(201);
    const memberId = responseUserId(await memberRes.json());

    const page = await ctx.newPage();
    await page.goto("/settings/system/users");
    const ownRow = page.locator(`[data-user-id="${adminId}"]`);
    const memberRow = page.locator(`[data-user-id="${memberId}"]`);
    await expect(ownRow).toBeVisible({ timeout: 15_000 });

    // Sole active admin: the own row's demote/disable toggles are disabled,
    // exactly where the backend would reject the action.
    await expect(ownRow.getByTestId("users-table-toggle-role")).toBeDisabled();
    await expect(ownRow.getByTestId("users-table-toggle-status")).toBeDisabled();
    // A member row is never gated by the last-admin guard.
    await expect(memberRow.getByTestId("users-table-toggle-role")).toBeEnabled();
    await expect(memberRow.getByTestId("users-table-toggle-status")).toBeEnabled();

    // Existing guard still enforced at the API: the sole admin cannot
    // self-demote.
    const selfDemote = await ctx.request.patch(`${backend.baseUrl}/api/v1/users/${adminId}`, {
      data: { role: "member" },
    });
    expect(selfDemote.status(), await selfDemote.text()).toBe(409);

    // A second active admin lifts the guard.
    const secondRes = await ctx.request.post(`${backend.baseUrl}/api/v1/users`, {
      data: {
        email: SECOND_ADMIN.email,
        password: SECOND_ADMIN.password,
        display_name: SECOND_ADMIN.displayName,
        role: "admin",
      },
    });
    expect(secondRes.status(), await secondRes.text()).toBe(201);

    await page.reload();
    await expect(ownRow.getByTestId("users-table-toggle-role")).toBeEnabled();
    await expect(ownRow.getByTestId("users-table-toggle-status")).toBeEnabled();

    // No new backend behavior: self-demotion is allowed again once another
    // active admin exists.
    const selfDemoteAfter = await ctx.request.patch(`${backend.baseUrl}/api/v1/users/${adminId}`, {
      data: { role: "member" },
    });
    expect(selfDemoteAfter.status(), await selfDemoteAfter.text()).toBe(200);

    await ctx.close();
  });
});
