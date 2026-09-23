import { expect, test } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { expectTouchControl } from "../../helpers/control-sizing";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  installFixturePlugin,
  PLUGIN_ID,
  uninstallFixturePlugin,
} from "../../helpers/plugin-fixture";
import { enableCanvasFeature } from "../canvas/canvas-fixture";
import {
  INSTALLED_DETAIL,
  INSTALLED_ROW,
  INSTALLED_ROW_LINK,
  REPO_LINK,
  REPOSITORY_URL,
  STAR_LABEL,
  STAR_MUTATION_PATH,
  STAR_TOGGLE,
  UNSTAR_LABEL,
  canvasCatalogEntry,
  mockMarketplaceCatalog,
  openCanvasMarketplaceCard,
  openMarketplaceEntry,
  pluginCatalogEntry,
  seedConnectedRepository,
  useCapableDeployment,
} from "./plugin-stars-helpers";

/**
 * The repository-star control on the Pixel 5 `mobile-chrome` project: touch
 * activation, 44px targets on every surface it appears on, and no horizontal
 * overflow at phone width.
 */

const MOBILE_DEVICE_WIDTH = 393;

test.describe("Plugin repository stars on a phone", () => {
  test.afterEach(async ({ backend, apiClient }) => {
    await backend.restart();
    await apiClient.mockGitHubSetPersonalStarFailures({});
  });

  test("keeps 44px targets and a confirmed state across marketplace, row, and detail", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(90_000);
    // Guard the emulation this spec exists for: the Pixel 5 device (a phone
    // screen) and its coarse primary pointer. The emulated *screen* is what
    // proves the device was applied; the layout viewport is not a reliable
    // proxy for it under mobile emulation.
    expect(await testPage.evaluate(() => window.screen.width)).toBe(MOBILE_DEVICE_WIDTH);
    expect(await testPage.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);
    expect(await testPage.evaluate(() => matchMedia("(pointer: fine)").matches)).toBe(false);

    await useCapableDeployment(backend);
    await seedConnectedRepository(apiClient, seedData.workspaceId, false);
    await mockMarketplaceCatalog(testPage, { plugins: [pluginCatalogEntry()] });

    try {
      const card = await openMarketplaceEntry(testPage);
      const cardToggle = card.getByTestId(STAR_TOGGLE);
      await expect(cardToggle).toHaveText(STAR_LABEL);
      await expect(cardToggle).toHaveAttribute("aria-pressed", "false");
      await expectTouchControl(cardToggle);
      await assertNoDocumentHorizontalOverflow(testPage);

      const starred = waitForHttp(testPage, "PUT", STAR_MUTATION_PATH);
      await cardToggle.tap();
      expect((await starred).ok()).toBe(true);
      await expect(cardToggle).toHaveAttribute("aria-pressed", "true");
      await expect(cardToggle).toHaveText(UNSTAR_LABEL);
      await expectTouchControl(cardToggle);

      // Installed row: same repository, same touch target, and the confirmed
      // state is already there without a second provider read.
      await installFixturePlugin(testPage);
      const row = testPage.getByTestId(INSTALLED_ROW);
      const rowToggle = row.getByTestId(STAR_TOGGLE);
      await expect(rowToggle).toHaveAttribute("aria-pressed", "true");
      await expectTouchControl(rowToggle);
      await assertNoDocumentHorizontalOverflow(testPage);

      // At phone width the row's touch-sized action cluster sits above the
      // card's overlay link, so tapping the overlay's center is intercepted by
      // design. Follow the route the overlay points at instead of bypassing it.
      const rowHref = await row.getByTestId(INSTALLED_ROW_LINK).getAttribute("href");
      expect(rowHref).toBe(`/settings/plugins/${PLUGIN_ID}`);
      await testPage.goto(rowHref ?? "");
      const detail = testPage.getByTestId(INSTALLED_DETAIL);
      await expect(detail).toBeVisible();
      const detailToggle = detail.getByTestId(STAR_TOGGLE);
      await expect(detailToggle).toHaveAttribute("aria-pressed", "true");
      await expectTouchControl(detailToggle);
      await assertNoDocumentHorizontalOverflow(testPage);

      // Unstar from the detail surface and confirm the round trip.
      const unstarred = waitForHttp(testPage, "DELETE", STAR_MUTATION_PATH);
      await detailToggle.tap();
      expect((await unstarred).ok()).toBe(true);
      await expect(detailToggle).toHaveAttribute("aria-pressed", "false");
      await expect(detailToggle).toHaveText(STAR_LABEL);
      await expectTouchControl(detailToggle);
    } finally {
      await uninstallFixturePlugin(apiClient);
    }
  });

  test("keeps the Canvases detail star touch-sized on a phone", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const canvasEntry = canvasCatalogEntry();
    const releaseCanvasFeature = await enableCanvasFeature(
      backend,
      apiClient,
      seedData.workspaceId,
    );
    try {
      await useCapableDeployment(backend);
      await seedConnectedRepository(apiClient, seedData.workspaceId, false);
      await mockMarketplaceCatalog(testPage, { canvases: [canvasEntry] });

      const card = await openCanvasMarketplaceCard(testPage, canvasEntry.id);
      await card.getByRole("button").first().tap();

      const detail = testPage.getByTestId(`canvas-marketplace-detail-${canvasEntry.id}`);
      await expect(detail).toBeVisible();
      // Canvases detail belongs to the selected canvas workspace, and the
      // repository is the same one the plugin surfaces read.
      const toggle = detail.getByTestId(STAR_TOGGLE);
      await expect(toggle).toHaveText(STAR_LABEL);
      await expect(toggle).toHaveAttribute("aria-pressed", "false");
      await expectTouchControl(toggle);
      await expect(detail.getByTestId(REPO_LINK)).toHaveAttribute("href", REPOSITORY_URL);
      await assertNoDocumentHorizontalOverflow(testPage);

      const starred = waitForHttp(testPage, "PUT", STAR_MUTATION_PATH);
      await toggle.tap();
      expect((await starred).ok()).toBe(true);
      await expect(toggle).toHaveAttribute("aria-pressed", "true");
      await expectTouchControl(toggle);
      await assertNoDocumentHorizontalOverflow(testPage);
    } finally {
      await releaseCanvasFeature();
    }
  });
});
