import { expect, type BrowserContext } from "@playwright/test";
import path from "node:path";
import { backendFixture as test } from "../../fixtures/backend";
import { seedPersonalStarDeployment } from "../../fixtures/personal-star-deployment";
import { acceptInvite, createInviteToken, login, setupAdmin } from "../../helpers/auth";
import { expectControlHeight } from "../../helpers/control-sizing";
import { installFixturePlugin } from "../../helpers/plugin-fixture";
import {
  COMPACT_STAR_HEIGHT,
  INSTALLED_DETAIL,
  INSTALLED_ROW,
  INSTALLED_ROW_LINK,
  MARKETPLACE_ENTRY,
  REPOSITORY_KEY,
  REPOSITORY_URL,
  STAR_MUTATION_PATH,
  STAR_TOGGLE,
  canvasCatalogEntry,
  mockMarketplaceCatalog,
  openCanvasMarketplaceCard,
  openMarketplaceEntry,
  pluginCatalogEntry,
} from "../settings/plugin-stars-helpers";

/**
 * The dedicated personal star flow (`REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001`)
 * under **authentication** — the desktop `auth` Playwright project.
 *
 * `chromium` assumes an open backend and a tokenless ApiClient, so this file
 * runs in its own gated CI job: the worker's backend is restarted with
 * `KANDEV_FEATURES_AUTH=true`, every request is made through a real
 * session-cookie browser context, and the test-base fixtures (`testPage`,
 * `apiClient`, `seedData`) are deliberately unused because their automatic
 * setup/cleanup endpoints require a session before the first admin exists.
 */

const ADMIN = { email: "stars-admin@e2e.dev", password: "adminpass123", displayName: "Ada" };
const MEMBER = { email: "stars-member@e2e.dev", password: "memberpass123", displayName: "Sam" };
const WORKSPACE_NAME = "Shared Star Workspace";
const FOREIGN_WORKSPACE_NAME = "Admin Only Star Workspace";

/** Auth is opt-in per restart: the baseline (auth off) is restored in afterAll. */
let authEnv: Record<string, string> = {};

async function userIdOf(context: BrowserContext, baseUrl: string): Promise<string> {
  const response = await context.request.get(`${baseUrl}/api/v1/auth/me`);
  expect(response.ok(), await response.text()).toBeTruthy();
  const body = (await response.json()) as { user?: { id?: string } };
  expect(body.user?.id, "an authenticated context must report its user id").toBeTruthy();
  return body.user?.id as string;
}

async function createWorkspaceAs(
  context: BrowserContext,
  baseUrl: string,
  name: string,
): Promise<{ id: string; name: string }> {
  const response = await context.request.post(`${baseUrl}/api/v1/workspaces`, { data: { name } });
  expect(response.ok(), await response.text()).toBeTruthy();
  return (await response.json()) as { id: string; name: string };
}

/** One user's dedicated personal-star connection, as the E2E control needs it. */
interface StarConnectionSeed {
  context: BrowserContext;
  baseUrl: string;
  workspaceId: string;
  userId: string;
  login: string;
  starred: boolean;
}

/** Seeds one user's dedicated personal-star connection through the E2E control. */
async function seedStarConnection(seed: StarConnectionSeed): Promise<void> {
  const { context, baseUrl, workspaceId, userId, starred } = seed;
  const connection = await context.request.put(
    `${baseUrl}/api/v1/github/mock/personal-star-connection`,
    {
      data: {
        workspace_id: workspaceId,
        user_id: userId,
        login: seed.login,
        public_repo: true,
        outcome: "connected",
      },
    },
  );
  expect(connection.ok(), await connection.text()).toBeTruthy();
  const state = await context.request.put(`${baseUrl}/api/v1/github/mock/personal-star-state`, {
    data: {
      workspace_id: workspaceId,
      user_id: userId,
      repository_url: REPOSITORY_URL,
      starred,
    },
  });
  expect(state.ok(), await state.text()).toBeTruthy();
}

async function connectionTuple(
  context: BrowserContext,
  baseUrl: string,
  workspaceId: string,
): Promise<{ user_id: string; active: boolean; capable: boolean; credential_generation: number }> {
  const response = await context.request.get(
    `${baseUrl}/api/v1/github/user/starred/connection?workspace_id=${encodeURIComponent(
      workspaceId,
    )}`,
  );
  expect(response.ok(), await response.text()).toBeTruthy();
  return (await response.json()) as {
    user_id: string;
    active: boolean;
    capable: boolean;
    credential_generation: number;
  };
}

async function repositoryStates(
  context: BrowserContext,
  baseUrl: string,
  workspaceId: string,
): Promise<Record<string, boolean | null>> {
  const response = await context.request.post(`${baseUrl}/api/v1/github/user/starred/status`, {
    data: { workspace_id: workspaceId, repository_urls: [REPOSITORY_URL] },
  });
  expect(response.ok(), await response.text()).toBeTruthy();
  return ((await response.json()) as { states: Record<string, boolean | null> }).states;
}

/** The `InstallReview` the mocked canvas-install preparation returns. */
function installReview(workspaceId: string) {
  const entry = canvasCatalogEntry();
  return {
    preparation_id: "preparation-auth-star-e2e",
    workspace_id: workspaceId,
    metadata: {
      package_id: entry.id,
      version: entry.version,
      display_name: entry.name,
      description: entry.description,
      author: entry.author,
      license: entry.license,
      source_mode: "project",
      min_kandev_version: entry.min_kandev_version,
      repo_url: REPOSITORY_URL,
    },
    sha256: "f".repeat(64),
    permissions: {
      reads: ["tasks"],
      writes: [],
      events: [],
      external_origins: [],
      shared_state: false,
    },
    origin_kind: "registry",
    source_id: "official",
    expires_at: "2026-12-31T00:00:00Z",
  };
}

test.describe.serial("authenticated plugin repository stars", () => {
  test.beforeAll(async ({ backend }) => {
    authEnv = {
      KANDEV_FEATURES_AUTH: "true",
      KANDEV_FEATURES_CANVASES: "true",
      KANDEV_DATABASE_PATH: path.join(backend.tmpDir, "kandev-auth-plugin-stars.db"),
    };
    await backend.restart(authEnv);
  });

  test.afterAll(async ({ backend }) => {
    // Revert to the fixture's baseline env (auth off) for any later suites.
    await backend.restart();
  });

  test("closes the star API to unauthenticated visitors before any flow runs", async ({
    browser,
    backend,
  }) => {
    const anonymous = await browser.newContext({ baseURL: backend.frontendUrl });
    try {
      expect((await anonymous.request.get(`${backend.baseUrl}/api/v1/workspaces`)).status()).toBe(
        401,
      );
      for (const response of [
        await anonymous.request.get(
          `${backend.baseUrl}/api/v1/github/user/starred/connection?workspace_id=any`,
        ),
        await anonymous.request.post(`${backend.baseUrl}/api/v1/github/user/starred/status`, {
          data: { workspace_id: "any", repository_urls: [REPOSITORY_URL] },
        }),
        await anonymous.request.put(`${backend.baseUrl}/api/v1/github/user/starred`, {
          data: { workspace_id: "any", repository_url: REPOSITORY_URL },
        }),
      ]) {
        expect(response.status(), "the dedicated star routes require a session").toBe(401);
      }
    } finally {
      await anonymous.close();
    }
  });

  test("lets an active member star the shared repository without operator controls", async ({
    browser,
    backend,
  }) => {
    test.setTimeout(240_000);

    // --- Accounts and the target workspace, created after the auth restart ---
    const adminContext = await browser.newContext({ baseURL: backend.frontendUrl });
    await setupAdmin(adminContext, backend.baseUrl, ADMIN);
    await login(adminContext, backend.baseUrl, ADMIN);
    const adminId = await userIdOf(adminContext, backend.baseUrl);

    const memberContext = await browser.newContext({ baseURL: backend.frontendUrl });
    const inviteToken = await createInviteToken(adminContext, backend.baseUrl, {
      email: MEMBER.email,
      role: "member",
    });
    await acceptInvite(memberContext, backend.baseUrl, inviteToken, MEMBER);
    const memberId = await userIdOf(memberContext, backend.baseUrl);

    // The fixture plugin must be installed before the member signs in, so the
    // installed row and detail surfaces exist for them too.
    const adminPage = await adminContext.newPage();
    await installFixturePlugin(adminPage);

    const workspace = await createWorkspaceAs(adminContext, backend.baseUrl, WORKSPACE_NAME);
    const foreign = await createWorkspaceAs(adminContext, backend.baseUrl, FOREIGN_WORKSPACE_NAME);

    // Active membership: the member holds an explicit collaborator row.
    const added = await adminContext.request.put(
      `${backend.baseUrl}/api/v1/workspaces/${workspace.id}/members/${memberId}`,
      { data: { role: "collaborator" } },
    );
    expect(added.ok(), await added.text()).toBeTruthy();
    expect(
      (
        await memberContext.request.get(`${backend.baseUrl}/api/v1/workspaces/${workspace.id}`)
      ).ok(),
      "the member must reach the workspace they are an active member of",
    ).toBeTruthy();

    // --- Deployment capability requires its own restart, then re-login ---
    const deployment = seedPersonalStarDeployment(backend);
    await backend.restart({ ...authEnv, ...deployment.env });
    await login(adminContext, backend.baseUrl, ADMIN);
    await login(memberContext, backend.baseUrl, MEMBER);

    await seedStarConnection({
      context: adminContext,
      baseUrl: backend.baseUrl,
      workspaceId: workspace.id,
      userId: adminId,
      login: "e2e-star-admin",
      starred: true,
    });
    await seedStarConnection({
      context: memberContext,
      baseUrl: backend.baseUrl,
      workspaceId: workspace.id,
      userId: memberId,
      login: "e2e-star-member",
      starred: false,
    });

    // Each identity owns its own connection for the same workspace.
    const adminTuple = await connectionTuple(adminContext, backend.baseUrl, workspace.id);
    const memberTuple = await connectionTuple(memberContext, backend.baseUrl, workspace.id);
    expect(adminTuple).toMatchObject({ user_id: adminId, active: true, capable: true });
    expect(memberTuple).toMatchObject({ user_id: memberId, active: true, capable: true });

    // The member's active workspace must be the shared one for the plugin
    // surfaces (they scope the star control by the active workspace).
    const saveSettings = await memberContext.request.patch(
      `${backend.baseUrl}/api/v1/user/settings`,
      { data: { workspace_id: workspace.id } },
    );
    expect(saveSettings.ok(), await saveSettings.text()).toBeTruthy();

    const memberPage = await memberContext.newPage();
    const canvasEntry = canvasCatalogEntry();
    await mockMarketplaceCatalog(memberPage, {
      plugins: [pluginCatalogEntry()],
      canvases: [canvasEntry],
    });
    await memberPage.route("**/api/v1/canvases/install-preparations", async (route) => {
      if (route.request().method() !== "POST") {
        await route.fallback();
        return;
      }
      const request = route.request().postDataJSON() as { workspace_id?: string };
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(installReview(request.workspace_id ?? "")),
      });
    });

    // --- Marketplace surface: star, then confirm the mutation causally ---
    const card = await openMarketplaceEntry(memberPage);
    const cardToggle = card.getByTestId(STAR_TOGGLE);
    await expect(cardToggle).toHaveAttribute("aria-pressed", "false");
    await expectControlHeight(cardToggle, COMPACT_STAR_HEIGHT);

    const starred = memberPage.waitForResponse((response) => {
      const url = new URL(response.url());
      return response.request().method() === "PUT" && STAR_MUTATION_PATH.test(url.pathname);
    });
    await cardToggle.click();
    expect((await starred).ok()).toBe(true);
    await expect(cardToggle).toHaveAttribute("aria-pressed", "true");
    expect(
      (await repositoryStates(memberContext, backend.baseUrl, workspace.id))[REPOSITORY_KEY],
    ).toBe(true);

    // --- Installed row and detail: the same confirmed state, no second read ---
    await memberPage.goto("/settings/plugins");
    const row = memberPage.getByTestId(INSTALLED_ROW);
    await expect(row.getByTestId(STAR_TOGGLE)).toHaveAttribute("aria-pressed", "true");
    await row.getByTestId(INSTALLED_ROW_LINK).click();
    await expect(memberPage.getByTestId(INSTALLED_DETAIL).getByTestId(STAR_TOGGLE)).toHaveAttribute(
      "aria-pressed",
      "true",
    );

    // Plugin-management controls stay absent for a member.
    const settingsPanel = memberPage.locator('[data-testid="settings-scroll-container"]:visible');
    await expect(settingsPanel.getByTestId("install-plugin-trigger")).toHaveCount(0);
    await expect(settingsPanel.getByTestId("plugins-sync-button")).toHaveCount(0);
    await expect(
      settingsPanel.getByRole("button", { name: /Enable|Disable|Uninstall|Update/ }),
    ).toHaveCount(0);

    // --- Canvases detail and the install dialog share the same workspace ---
    const canvasCard = await openCanvasMarketplaceCard(memberPage, canvasEntry.id);
    await canvasCard.getByRole("button").first().click();
    const canvasDetail = memberPage.getByTestId(`canvas-marketplace-detail-${canvasEntry.id}`);
    await expect(canvasDetail.getByTestId(STAR_TOGGLE)).toHaveAttribute("aria-pressed", "true");

    await canvasDetail.getByRole("button", { name: "Review and install", exact: true }).click();
    const dialog = memberPage.getByRole("dialog").last();
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: "Review bundle", exact: true }).click();
    const review = dialog.getByTestId("canvas-install-review");
    await expect(review).toBeVisible();
    await expect(review.getByText(`${workspace.name} (${workspace.id})`)).toBeVisible();
    await expect(dialog.getByTestId(STAR_TOGGLE)).toHaveAttribute("aria-pressed", "true");
    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();

    // --- Reload hydration, then unstar through the UI ---
    await openMarketplaceEntry(memberPage);
    await expect(
      memberPage.getByTestId(MARKETPLACE_ENTRY).getByTestId(STAR_TOGGLE),
    ).toHaveAttribute("aria-pressed", "true");

    const marketplaceToggle = memberPage.getByTestId(MARKETPLACE_ENTRY).getByTestId(STAR_TOGGLE);
    const unstarred = memberPage.waitForResponse((response) => {
      const url = new URL(response.url());
      return response.request().method() === "DELETE" && STAR_MUTATION_PATH.test(url.pathname);
    });
    await marketplaceToggle.click();
    expect((await unstarred).ok()).toBe(true);
    await expect(marketplaceToggle).toHaveAttribute("aria-pressed", "false");

    await memberPage.reload();
    await memberPage.getByTestId("plugins-tab-browse").click();
    await expect(
      memberPage.getByTestId(MARKETPLACE_ENTRY).getByTestId(STAR_TOGGLE),
    ).toHaveAttribute("aria-pressed", "false");
    expect(
      (await repositoryStates(memberContext, backend.baseUrl, workspace.id))[REPOSITORY_KEY],
    ).toBe(false);

    // --- Foreign workspace is denied before any credential/provider use ---
    //
    // A foreign workspace is indistinguishable from a missing one: every
    // dedicated star route answers 404 without touching credentials or the
    // provider.
    for (const response of [
      await memberContext.request.get(
        `${backend.baseUrl}/api/v1/github/user/starred/connection?workspace_id=${foreign.id}`,
      ),
      await memberContext.request.post(`${backend.baseUrl}/api/v1/github/user/starred/status`, {
        data: { workspace_id: foreign.id, repository_urls: [REPOSITORY_URL] },
      }),
      await memberContext.request.put(`${backend.baseUrl}/api/v1/github/user/starred`, {
        data: { workspace_id: foreign.id, repository_url: REPOSITORY_URL },
      }),
      await memberContext.request.delete(`${backend.baseUrl}/api/v1/github/user/starred`, {
        data: { workspace_id: foreign.id, repository_url: REPOSITORY_URL },
      }),
    ]) {
      expect(
        response.status(),
        `a foreign workspace must be denied with 404, got ${response.status()}`,
      ).toBe(404);
    }
    // The member's own connection and confirmed state are untouched by the
    // denied requests, and the admin's separate connection still stands.
    expect(await connectionTuple(memberContext, backend.baseUrl, workspace.id)).toMatchObject({
      user_id: memberId,
      active: true,
      capable: true,
    });
    expect(await connectionTuple(adminContext, backend.baseUrl, workspace.id)).toMatchObject({
      user_id: adminId,
      active: true,
      capable: true,
    });

    await memberPage.close();
    await adminPage.close();
    await adminContext.close();
    await memberContext.close();
  });
});
