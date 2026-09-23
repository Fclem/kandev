import { expect, test } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { expectControlHeight } from "../../helpers/control-sizing";
import {
  assertNoDocumentHorizontalOverflow,
  expectTopmostAtCenter,
} from "../../helpers/layout-assertions";
import { installFixturePlugin, uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import { enableCanvasFeature } from "../canvas/canvas-fixture";
import {
  CONNECTION_STATUS_PATH,
  CONNECTION_START_PATH,
  COMPACT_STAR_HEIGHT,
  INSTALLED_DETAIL,
  INSTALLED_ROW,
  INSTALLED_ROW_LINK,
  REPO_LINK,
  REPOSITORY_KEY,
  REPOSITORY_URL,
  STAR_CTA,
  STAR_MUTATION_PATH,
  STAR_STATUS_PATH,
  STAR_TOGGLE,
  canvasCatalogEntry,
  mockMarketplaceCatalog,
  openCanvasMarketplaceCard,
  openMarketplaceEntry,
  pluginCatalogEntry,
  seedConnectedRepository,
  useCapableDeployment,
} from "./plugin-stars-helpers";

/**
 * The dedicated personal star flow (`REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001`)
 * as the user sees it in Settings > Plugins — the desktop fine-pointer
 * `chromium` project.
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

/** The `InstallReview` the mocked canvas-install preparation returns. */
function installReview(workspaceId: string) {
  const entry = canvasCatalogEntry();
  return {
    preparation_id: "preparation-star-e2e",
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
    sha256: "d".repeat(64),
    archive_sha256: "e".repeat(64),
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
    await mockMarketplaceCatalog(testPage, { plugins: [pluginCatalogEntry()] });

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
    await mockMarketplaceCatalog(testPage, { plugins: [pluginCatalogEntry()] });

    const row = await openMarketplaceEntry(testPage);
    const toggle = row.getByTestId(STAR_TOGGLE);
    await expect(toggle).toHaveAttribute("aria-pressed", "false");

    const starred = waitForHttp(testPage, "PUT", STAR_MUTATION_PATH);
    await toggle.click();
    expect((await starred).ok()).toBe(true);
    await expect(toggle).toHaveAttribute("aria-pressed", "true");
    expect(
      (await apiClient.getPersonalStarStates(seedData.workspaceId, [REPOSITORY_URL])).states[
        REPOSITORY_KEY
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
        REPOSITORY_KEY
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
    const row = testPage.getByTestId(INSTALLED_ROW);
    const rowToggle = row.getByTestId(STAR_TOGGLE);
    await expect(rowToggle).toHaveAttribute("aria-pressed", "false");

    const starred = waitForHttp(testPage, "PUT", STAR_MUTATION_PATH);
    await rowToggle.click();
    await starred;
    await expect(rowToggle).toHaveAttribute("aria-pressed", "true");

    // The detail surface renders the same scoped key, so the confirmed state
    // is already there without a second provider read.
    await row.getByTestId(INSTALLED_ROW_LINK).click();
    await expect(testPage.getByTestId(INSTALLED_DETAIL)).toBeVisible();
    await expect(testPage.getByTestId(INSTALLED_DETAIL).getByTestId(STAR_TOGGLE)).toHaveAttribute(
      "aria-pressed",
      "true",
    );

    await uninstallFixturePlugin(apiClient);
  });

  test("keeps the compact 24px target on a desktop fine pointer without capturing row navigation", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(90_000);
    // The compact rule belongs to a *non-mobile fine pointer*: guard both, or a
    // dropped touch emulation would silently move this spec into the 44px case.
    expect(await testPage.evaluate(() => matchMedia("(pointer: fine)").matches)).toBe(true);
    expect(await testPage.evaluate(() => matchMedia("(any-pointer: coarse)").matches)).toBe(false);

    await useCapableDeployment(backend);
    await seedConnectedRepository(apiClient, seedData.workspaceId, false);
    await mockMarketplaceCatalog(testPage, { plugins: [pluginCatalogEntry()] });

    try {
      const card = await openMarketplaceEntry(testPage);
      const cardToggle = card.getByTestId(STAR_TOGGLE);
      await expect(cardToggle).toHaveAttribute("aria-pressed", "false");
      await expectControlHeight(cardToggle, COMPACT_STAR_HEIGHT);
      await expectTopmostAtCenter(cardToggle, "marketplace star toggle");
      await assertNoDocumentHorizontalOverflow(testPage);

      const starred = waitForHttp(testPage, "PUT", STAR_MUTATION_PATH);
      await cardToggle.click();
      expect((await starred).ok()).toBe(true);
      await expect(cardToggle).toHaveAttribute("aria-pressed", "true");
      await expectControlHeight(cardToggle, COMPACT_STAR_HEIGHT);

      await installFixturePlugin(testPage);
      const row = testPage.getByTestId(INSTALLED_ROW);
      const rowToggle = row.getByTestId(STAR_TOGGLE);
      await expect(rowToggle).toHaveAttribute("aria-pressed", "true");
      await expectControlHeight(rowToggle, COMPACT_STAR_HEIGHT);

      // The row's overlay link spans the whole card; the compact star keeps its
      // own click above it and is not nested inside it.
      expect(await rowToggle.evaluate((element) => element.closest("a") !== null)).toBe(false);
      await expectTopmostAtCenter(rowToggle, "installed row star toggle");
      await expectTopmostAtCenter(row.getByTestId(REPO_LINK), "installed row repository link");

      const before = testPage.url();
      const unstarred = waitForHttp(testPage, "DELETE", STAR_MUTATION_PATH);
      await rowToggle.click();
      expect((await unstarred).ok()).toBe(true);
      await expect(rowToggle).toHaveAttribute("aria-pressed", "false");
      expect(testPage.url()).toBe(before);

      await row.getByTestId(INSTALLED_ROW_LINK).click();
      await expect(testPage.getByTestId(INSTALLED_DETAIL)).toBeVisible();
    } finally {
      await uninstallFixturePlugin(apiClient);
    }
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
    await mockMarketplaceCatalog(testPage, { plugins: [pluginCatalogEntry()] });

    // Armed before navigation: hydration fires on mount.
    const statusRead = waitForHttp(testPage, "POST", STAR_STATUS_PATH);
    const row = await openMarketplaceEntry(testPage);
    expect((await statusRead).ok()).toBe(true);

    // The backend really answered "unknown" for this repository…
    expect(
      (await apiClient.getPersonalStarStates(seedData.workspaceId, [REPOSITORY_URL])).states[
        REPOSITORY_KEY
      ],
    ).toBeNull();
    // …so the control asserts nothing: no unstarred toggle, and no CTA that
    // would imply the user could act on an unknown state.
    await expect(row.getByTestId(STAR_TOGGLE)).toHaveCount(0);
    await expect(row.getByTestId(STAR_CTA)).toHaveCount(0);
    await expect(row.getByTestId(REPO_LINK)).toBeVisible();
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
    await mockMarketplaceCatalog(testPage, { plugins: [pluginCatalogEntry()] });

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
    await expect(row.getByTestId(REPO_LINK)).toBeVisible();
    await expect(testPage.locator("[data-sonner-toast]")).toHaveCount(0);
    expect(
      (await apiClient.getPersonalStarStates(seedData.workspaceId, [REPOSITORY_URL])).states[
        REPOSITORY_KEY
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
    await mockMarketplaceCatalog(testPage, { plugins: [pluginCatalogEntry()] });

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
    await mockMarketplaceCatalog(testPage, { plugins: [pluginCatalogEntry()] });

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
    await mockMarketplaceCatalog(testPage, { plugins: [pluginCatalogEntry()] });

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
    await mockMarketplaceCatalog(testPage, { plugins: [pluginCatalogEntry()] });

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
          REPOSITORY_KEY
        ],
      ).toBeNull();

      // …while the other workspace keeps both its connection and its state.
      const otherAfter = await apiClient.getPersonalStarConnectionStatus(other.id);
      expect(otherAfter.credential_generation).toBe(otherBefore.credential_generation);
      expect(otherAfter).toMatchObject({ active: true, capable: true });
      expect(
        (await apiClient.getPersonalStarStates(other.id, [REPOSITORY_URL])).states[REPOSITORY_KEY],
      ).toBe(true);
    } finally {
      await apiClient.deleteWorkspace(other.id, other.name).catch(() => undefined);
    }
  });

  test("stars the shared repository from the Canvases detail surface", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const entry = canvasCatalogEntry();
    const releaseCanvasFeature = await enableCanvasFeature(
      backend,
      apiClient,
      seedData.workspaceId,
    );
    try {
      await useCapableDeployment(backend);
      await seedConnectedRepository(apiClient, seedData.workspaceId, true);
      await mockMarketplaceCatalog(testPage, { canvases: [entry] });

      const card = await openCanvasMarketplaceCard(testPage, entry.id);
      await card.getByRole("button").first().click();

      const detail = testPage.getByTestId(`canvas-marketplace-detail-${entry.id}`);
      await expect(detail).toBeVisible();
      const toggle = detail.getByTestId(STAR_TOGGLE);
      // Same repository as the plugin surfaces, so the confirmed state is
      // already hydrated — and the compact desktop geometry applies here too.
      await expect(toggle).toHaveAttribute("aria-pressed", "true");
      await expectControlHeight(toggle, COMPACT_STAR_HEIGHT);
      await expect(detail.getByTestId(REPO_LINK)).toHaveAttribute("href", REPOSITORY_URL);

      const unstarred = waitForHttp(testPage, "DELETE", STAR_MUTATION_PATH);
      await toggle.click();
      expect((await unstarred).ok()).toBe(true);
      await expect(toggle).toHaveAttribute("aria-pressed", "false");
      expect(
        (await apiClient.getPersonalStarStates(seedData.workspaceId, [REPOSITORY_URL])).states[
          REPOSITORY_KEY
        ],
      ).toBe(false);
    } finally {
      await releaseCanvasFeature();
    }
  });

  test("reads the CanvasInstallDialog star from its selected workspace, not the active one", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const entry = canvasCatalogEntry();
    const releaseCanvasFeature = await enableCanvasFeature(
      backend,
      apiClient,
      seedData.workspaceId,
    );
    const other = await apiClient.createWorkspace("Star Dialog Workspace");
    try {
      await useCapableDeployment(backend);
      // The active workspace A is starred; the dialog's selected workspace B is
      // not. A control that read the active workspace would show `true`.
      await seedConnectedRepository(apiClient, seedData.workspaceId, true);
      await seedConnectedRepository(apiClient, other.id, false);
      await mockMarketplaceCatalog(testPage, { canvases: [entry] });

      const preparedWorkspaces: string[] = [];
      await testPage.route("**/api/v1/canvases/install-preparations", async (route) => {
        if (route.request().method() !== "POST") {
          await route.fallback();
          return;
        }
        const request = route.request().postDataJSON() as { workspace_id?: string };
        preparedWorkspaces.push(request.workspace_id ?? "");
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(installReview(request.workspace_id ?? "")),
        });
      });

      // Workspace B only becomes selectable once the app has booted with it.
      await testPage.reload();
      const card = await openCanvasMarketplaceCard(testPage, entry.id);
      await testPage.getByTestId("canvas-marketplace-workspace").click();
      await testPage.getByRole("option", { name: other.name }).click();
      await expect(testPage.getByTestId("canvas-marketplace-workspace")).toHaveText(other.name);

      await card.getByRole("button").first().click();
      const detail = testPage.getByTestId(`canvas-marketplace-detail-${entry.id}`);
      await detail.getByRole("button", { name: "Review and install", exact: true }).click();

      const dialog = testPage.getByRole("dialog").last();
      await expect(dialog).toBeVisible();
      await dialog.getByRole("button", { name: "Review bundle", exact: true }).click();

      // The inspected review names the selected workspace, and the preparation
      // was requested for it.
      const review = dialog.getByTestId("canvas-install-review");
      await expect(review).toBeVisible();
      await expect(review.getByText(`${other.name} (${other.id})`)).toBeVisible();
      expect(preparedWorkspaces).toEqual([other.id]);

      const toggle = dialog.getByTestId(STAR_TOGGLE);
      await expect(toggle).toHaveAttribute("aria-pressed", "false");
      await expectControlHeight(toggle, COMPACT_STAR_HEIGHT);

      const starred = waitForHttp(testPage, "PUT", STAR_MUTATION_PATH);
      await toggle.click();
      const starResponse = await starred;
      expect((starResponse.request().postDataJSON() as { workspace_id: string }).workspace_id).toBe(
        other.id,
      );
      await expect(toggle).toHaveAttribute("aria-pressed", "true");

      const unstarred = waitForHttp(testPage, "DELETE", STAR_MUTATION_PATH);
      await toggle.click();
      const unstarResponse = await unstarred;
      expect(
        (unstarResponse.request().postDataJSON() as { workspace_id: string }).workspace_id,
      ).toBe(other.id);
      await expect(toggle).toHaveAttribute("aria-pressed", "false");

      // Selected workspace B moved; the global active workspace A did not.
      expect(
        (await apiClient.getPersonalStarStates(other.id, [REPOSITORY_URL])).states[REPOSITORY_KEY],
      ).toBe(false);
      expect(
        (await apiClient.getPersonalStarStates(seedData.workspaceId, [REPOSITORY_URL])).states[
          REPOSITORY_KEY
        ],
      ).toBe(true);
    } finally {
      await apiClient.deleteWorkspace(other.id, other.name).catch(() => undefined);
      await releaseCanvasFeature();
    }
  });

  test("omits the CanvasInstallDialog star control when the provider status is unknown", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const entry = canvasCatalogEntry();
    const releaseCanvasFeature = await enableCanvasFeature(
      backend,
      apiClient,
      seedData.workspaceId,
    );
    try {
      await useCapableDeployment(backend);
      await apiClient.mockGitHubSetPersonalStarConnection(seedData.workspaceId);
      await apiClient.mockGitHubSetPersonalStarFailures({ status: true });
      await mockMarketplaceCatalog(testPage, { canvases: [entry] });

      await testPage.route("**/api/v1/canvases/install-preparations", async (route) => {
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

      const card = await openCanvasMarketplaceCard(testPage, entry.id);
      await card.getByRole("button").first().click();
      const detail = testPage.getByTestId(`canvas-marketplace-detail-${entry.id}`);
      await detail.getByRole("button", { name: "Review and install", exact: true }).click();

      const dialog = testPage.getByRole("dialog").last();
      await dialog.getByRole("button", { name: "Review bundle", exact: true }).click();

      // The review still renders in full — the omission is the star control
      // asserting a state it cannot know, not a broken dialog.
      const review = dialog.getByTestId("canvas-install-review");
      await expect(review).toBeVisible();
      await expect(review.getByText(REPOSITORY_URL)).toBeVisible();
      await expect(dialog.getByTestId(STAR_TOGGLE)).toHaveCount(0);
      await expect(dialog.getByTestId(STAR_CTA)).toHaveCount(0);
    } finally {
      await releaseCanvasFeature();
    }
  });
});
