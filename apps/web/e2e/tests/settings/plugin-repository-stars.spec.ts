import type { Locator, Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import type { BackendContext } from "../../fixtures/backend";
import type { ApiClient } from "../../helpers/api-client";
import { waitForHttp } from "../../helpers/causal-waits";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";
import { seedPersonalStarDeployment } from "../../fixtures/personal-star-deployment";

/**
 * The dedicated personal star flow (`REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001`)
 * as the user sees it in Settings > Plugins.
 *
 * Every assertion here is about *what the deployment permits the UI to claim*:
 * an unsupported deployment renders no control at all, an inactive or
 * incapable connection renders a Connect/Reconnect CTA and never asserts
 * "unstarred", a provider-unknown status hides the toggle instead of showing
 * it unstarred, a failed mutation keeps the confirmed state and the list, and
 * a navigation-only OAuth outcome in the URL is removed rather than announced.
 *
 * Capable deployments are produced by `fixtures/personal-star-deployment.ts`,
 * which seeds the deployment-owned secret store the GitHub provider needs; each
 * test exposes it with a one-shot `backend.restart(seed.env)`.
 */

/**
 * The fixture plugin's manifest `repo_url`. The installed row and detail read
 * it from the packaged manifest, so this proves the shipped fixture package
 * carries a canonical GitHub repository URL end to end.
 */
const REPOSITORY_URL = "https://github.com/kdlbs/kandev-plugin-template";

const STAR_TOGGLE = "plugin-star-toggle";
const STAR_CTA = "plugin-star-connect";
const MARKETPLACE_ENTRY = `marketplace-entry-${PLUGIN_ID}`;
const CONNECTION_STATUS_PATH = /^\/api\/v1\/github\/user\/starred\/connection$/;
const CONNECTION_START_PATH = /^\/api\/v1\/github\/user\/starred\/connection\/start$/;
const STAR_STATUS_PATH = /^\/api\/v1\/github\/user\/starred\/status$/;
const STAR_MUTATION_PATH = /^\/api\/v1\/github\/user\/starred$/;

async function useCapableDeployment(backend: BackendContext): Promise<void> {
  const seed = seedPersonalStarDeployment(backend);
  await backend.restart(seed.env);
}

/** Serves one marketplace catalog entry carrying a canonical GitHub repo URL. */
async function mockMarketplaceCatalog(page: Page): Promise<void> {
  await page.route("**/api/plugins/marketplace*", async (route) => {
    if (route.request().method() !== "GET") {
      await route.fallback();
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        plugins: [
          {
            id: PLUGIN_ID,
            name: "E2E Hello",
            description: "",
            author: "kandev",
            categories: [],
            icon_url: "",
            repo_url: REPOSITORY_URL,
            version: "1.0.0",
            min_kandev_version: "",
            package_url: "https://example.invalid/kandev-plugin-e2e-1.0.0.tar.gz",
            package_sha256: "",
            stars: 0,
            updated_at: "2026-01-01T00:00:00.000Z",
            install_state: "available",
            source_id: "official",
            source_name: "Kandev Official",
          },
        ],
        sources: [
          {
            id: "official",
            name: "Kandev Official",
            url: "https://example.invalid",
            enabled: true,
            builtin: true,
            healthy: true,
          },
        ],
      }),
    });
  });
}

async function openMarketplaceEntry(page: Page): Promise<Locator> {
  await page.goto("/settings/plugins");
  await page.getByTestId("plugins-tab-browse").click();
  const row = page.getByTestId(MARKETPLACE_ENTRY);
  await expect(row).toBeVisible();
  return row;
}

/** Seeds a capable connection and one confirmed repository state. */
async function seedConnectedRepository(
  apiClient: ApiClient,
  workspaceId: string,
  starred: boolean,
): Promise<void> {
  await apiClient.mockGitHubSetPersonalStarConnection(workspaceId);
  await apiClient.mockGitHubSetPersonalStarState(workspaceId, {
    repositoryUrl: REPOSITORY_URL,
    starred,
  });
}

test.describe("Plugin repository stars", () => {
  test.afterEach(async ({ backend, apiClient }) => {
    // Deployment capability is a per-restart fact, not a stored one: revert the
    // one-shot environment so sibling specs in this worker never inherit a
    // deployment they did not seed.
    await backend.restart();
    await apiClient.mockGitHubSetPersonalStarFailures({});
  });

  test("shows the Connect GitHub CTA without asserting a star state", async ({
    testPage,
    backend,
  }) => {
    await useCapableDeployment(backend);
    await mockMarketplaceCatalog(testPage);

    const row = await openMarketplaceEntry(testPage);

    await expect(row.getByTestId(STAR_CTA)).toHaveText("Connect GitHub");
    // Non-assertive: an inactive connection is never rendered as unstarred.
    await expect(row.getByTestId(STAR_TOGGLE)).toHaveCount(0);
  });

  test("round-trips star and unstar through the mock provider on confirmed state", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    await useCapableDeployment(backend);
    await seedConnectedRepository(apiClient, seedData.workspaceId, false);
    await mockMarketplaceCatalog(testPage);

    const row = await openMarketplaceEntry(testPage);
    const toggle = row.getByTestId(STAR_TOGGLE);
    await expect(toggle).toHaveAttribute("aria-pressed", "false");

    const starred = waitForHttp(testPage, "PUT", STAR_MUTATION_PATH);
    await toggle.click();
    expect((await starred).ok()).toBe(true);
    await expect(toggle).toHaveAttribute("aria-pressed", "true");
    expect(
      (await apiClient.getPersonalStarStates(seedData.workspaceId, [REPOSITORY_URL])).states[
        "kdlbs/kandev-plugin-template"
      ],
    ).toBe(true);

    // Reload hydration is a fresh connection-status + status read, not a cache.
    await testPage.reload();
    await testPage.getByTestId("plugins-tab-browse").click();
    await expect(row.getByTestId(STAR_TOGGLE)).toHaveAttribute("aria-pressed", "true");

    const unstarred = waitForHttp(testPage, "DELETE", STAR_MUTATION_PATH);
    await row.getByTestId(STAR_TOGGLE).click();
    expect((await unstarred).ok()).toBe(true);
    await expect(row.getByTestId(STAR_TOGGLE)).toHaveAttribute("aria-pressed", "false");
    expect(
      (await apiClient.getPersonalStarStates(seedData.workspaceId, [REPOSITORY_URL])).states[
        "kdlbs/kandev-plugin-template"
      ],
    ).toBe(false);
  });

  test("shares confirmed star state across the installed row and detail surfaces", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await useCapableDeployment(backend);
    await seedConnectedRepository(apiClient, seedData.workspaceId, false);

    await installFixturePlugin(testPage);
    const row = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    const rowToggle = row.getByTestId(STAR_TOGGLE);
    await expect(rowToggle).toHaveAttribute("aria-pressed", "false");

    const starred = waitForHttp(testPage, "PUT", STAR_MUTATION_PATH);
    await rowToggle.click();
    await starred;
    await expect(rowToggle).toHaveAttribute("aria-pressed", "true");

    // The detail surface renders the same scoped key, so the confirmed state
    // is already there without a second provider read.
    await row.getByTestId(`plugin-row-link-${PLUGIN_ID}`).click();
    await expect(testPage.getByTestId(`plugin-detail-${PLUGIN_ID}`)).toBeVisible();
    await expect(
      testPage.getByTestId(`plugin-detail-${PLUGIN_ID}`).getByTestId(STAR_TOGGLE),
    ).toHaveAttribute("aria-pressed", "true");

    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("never renders a provider-unknown status as unstarred", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    await useCapableDeployment(backend);
    await apiClient.mockGitHubSetPersonalStarConnection(seedData.workspaceId);
    await apiClient.mockGitHubSetPersonalStarFailures({ status: true });
    await mockMarketplaceCatalog(testPage);

    // Armed before navigation: hydration fires on mount.
    const statusRead = waitForHttp(testPage, "POST", STAR_STATUS_PATH);
    const row = await openMarketplaceEntry(testPage);
    expect((await statusRead).ok()).toBe(true);

    // The backend really answered "unknown" for this repository…
    expect(
      (await apiClient.getPersonalStarStates(seedData.workspaceId, [REPOSITORY_URL])).states[
        "kdlbs/kandev-plugin-template"
      ],
    ).toBeNull();
    // …so the control asserts nothing: no unstarred toggle, and no CTA that
    // would imply the user could act on an unknown state.
    await expect(row.getByTestId(STAR_TOGGLE)).toHaveCount(0);
    await expect(row.getByTestId(STAR_CTA)).toHaveCount(0);
    await expect(row.getByTestId("plugin-repo-link")).toBeVisible();
  });

  test("keeps the confirmed state and the list when a mutation fails", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    await useCapableDeployment(backend);
    await seedConnectedRepository(apiClient, seedData.workspaceId, false);
    await apiClient.mockGitHubSetPersonalStarFailures({ mutation: true });
    await mockMarketplaceCatalog(testPage);

    const row = await openMarketplaceEntry(testPage);
    const toggle = row.getByTestId(STAR_TOGGLE);
    await expect(toggle).toHaveAttribute("aria-pressed", "false");

    const failed = waitForHttp(testPage, "PUT", STAR_MUTATION_PATH, {
      predicate: (response) => !response.ok(),
    });
    await toggle.click();
    await failed;

    // No optimistic flip, no error state replacing the list, no toast.
    await expect(toggle).toHaveAttribute("aria-pressed", "false");
    await expect(toggle).toBeEnabled();
    await expect(row).toBeVisible();
    await expect(row.getByTestId("plugin-repo-link")).toBeVisible();
    await expect(testPage.locator("[data-sonner-toast]")).toHaveCount(0);
    expect(
      (await apiClient.getPersonalStarStates(seedData.workspaceId, [REPOSITORY_URL])).states[
        "kdlbs/kandev-plugin-template"
      ],
    ).toBe(false);
  });

  test("shows Reconnect GitHub and no star state when the grant lost public_repo", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    await useCapableDeployment(backend);
    await apiClient.mockGitHubSetPersonalStarConnection(seedData.workspaceId, {
      public_repo: false,
      outcome: "insufficient_scope",
    });
    await mockMarketplaceCatalog(testPage);

    const statusRead = waitForHttp(testPage, "GET", CONNECTION_STATUS_PATH);
    const row = await openMarketplaceEntry(testPage);
    await statusRead;

    await expect(row.getByTestId(STAR_CTA)).toHaveText("Reconnect GitHub");
    await expect(row.getByTestId(STAR_TOGGLE)).toHaveCount(0);
  });

  test("keeps the actionable star control alongside Reconnect GitHub when re-consent is pending", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    await useCapableDeployment(backend);
    await seedConnectedRepository(apiClient, seedData.workspaceId, true);
    // A declined re-consent leaves the previous valid generation usable, so the
    // truth table keeps the star control and adds the reconnect entry point.
    await apiClient.mockGitHubSetPersonalStarOutcome(seedData.workspaceId, "declined");
    await mockMarketplaceCatalog(testPage);

    const row = await openMarketplaceEntry(testPage);
    await expect(row.getByTestId(STAR_TOGGLE)).toHaveAttribute("aria-pressed", "true");
    await expect(row.getByTestId(STAR_CTA)).toHaveText("Reconnect GitHub");
  });

  test("omits the control and issues no start request on an unsupported deployment", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    // Baseline restart: no deployment material settings, so the provider
    // reports available=false and mounts no start path.
    await backend.restart();

    const startRequests: string[] = [];
    testPage.on("request", (request) => {
      if (CONNECTION_START_PATH.test(new URL(request.url()).pathname)) {
        startRequests.push(request.url());
      }
    });
    await mockMarketplaceCatalog(testPage);

    const connectionRead = waitForHttp(testPage, "GET", CONNECTION_STATUS_PATH, {
      predicate: async (response) => {
        const body = (await response.json()) as { available?: boolean };
        return body.available === false;
      },
    });
    const row = await openMarketplaceEntry(testPage);
    await connectionRead;

    expect((await apiClient.getPersonalStarConnectionStatus(seedData.workspaceId)).available).toBe(
      false,
    );
    await expect(row.getByTestId(STAR_TOGGLE)).toHaveCount(0);
    await expect(row.getByTestId(STAR_CTA)).toHaveCount(0);
    expect(startRequests).toEqual([]);
    expect(new URL(testPage.url()).pathname).toBe("/settings/plugins");
  });

  test("removes the callback outcome query instead of announcing it", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    await useCapableDeployment(backend);
    await mockMarketplaceCatalog(testPage);

    // A stale "declined" value cannot manufacture a reconnect state: with no
    // connection yet, the truth table still says Connect.
    await testPage.goto("/settings/plugins?star_connection_outcome=declined");
    await expect
      .poll(() => new URL(testPage.url()).searchParams.has("star_connection_outcome"))
      .toBe(false);
    const connectRow = await openMarketplaceEntry(testPage);
    await expect(connectRow.getByTestId(STAR_CTA)).toHaveText("Connect GitHub");

    // The freshly fetched tuple decides, never the query: the seeded, confirmed
    // unstarred state stands even though the URL claimed "connected".
    await seedConnectedRepository(apiClient, seedData.workspaceId, false);
    await testPage.goto("/settings/plugins?star_connection_outcome=connected");
    await expect
      .poll(() => new URL(testPage.url()).searchParams.has("star_connection_outcome"))
      .toBe(false);
    expect(new URL(testPage.url()).pathname).toBe("/settings/plugins");
    await expect(testPage.getByTestId("toast-message")).toHaveCount(0);
    await expect(testPage.locator("[data-sonner-toast]")).toHaveCount(0);

    const row = await openMarketplaceEntry(testPage);
    await expect(row.getByTestId(STAR_TOGGLE)).toHaveAttribute("aria-pressed", "false");
  });

  test("keeps another workspace's connection and star state through a reset", async ({
    apiClient,
    backend,
    seedData,
  }) => {
    await useCapableDeployment(backend);
    // This test drives the API directly instead of through `testPage`, so
    // nothing has reset workspace A yet, and the E2E seed control cannot
    // replace an existing connection (its replace path fails the lifecycle
    // fence). Start from the same clean state a reset leaves behind.
    await apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]);
    const other = await apiClient.createWorkspace("Star Isolation Workspace");
    try {
      await apiClient.mockGitHubSetPersonalStarConnection(seedData.workspaceId);
      await apiClient.mockGitHubSetPersonalStarConnection(other.id);
      await apiClient.mockGitHubSetPersonalStarState(seedData.workspaceId, {
        repositoryUrl: REPOSITORY_URL,
        starred: true,
      });
      await apiClient.mockGitHubSetPersonalStarState(other.id, {
        repositoryUrl: REPOSITORY_URL,
        starred: true,
      });
      const otherBefore = await apiClient.getPersonalStarConnectionStatus(other.id);
      expect(otherBefore).toMatchObject({ available: true, active: true, capable: true });

      await apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]);

      // The reset workspace loses its connection, and its repository state
      // becomes unknown rather than asserted.
      expect(await apiClient.getPersonalStarConnectionStatus(seedData.workspaceId)).toMatchObject({
        available: true,
        active: false,
        capable: false,
        reconnect_required: false,
      });
      expect(
        (await apiClient.getPersonalStarStates(seedData.workspaceId, [REPOSITORY_URL])).states[
          "kdlbs/kandev-plugin-template"
        ],
      ).toBeNull();

      // …while the other workspace keeps both its connection and its state.
      const otherAfter = await apiClient.getPersonalStarConnectionStatus(other.id);
      expect(otherAfter.credential_generation).toBe(otherBefore.credential_generation);
      expect(otherAfter).toMatchObject({ active: true, capable: true });
      expect(
        (await apiClient.getPersonalStarStates(other.id, [REPOSITORY_URL])).states[
          "kdlbs/kandev-plugin-template"
        ],
      ).toBe(true);
    } finally {
      await apiClient.deleteWorkspace(other.id, other.name).catch(() => undefined);
    }
  });
});
