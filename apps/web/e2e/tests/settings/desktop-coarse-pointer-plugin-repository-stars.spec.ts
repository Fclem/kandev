import { expect, test } from "../../fixtures/test-base";
import { injectLatency, waitForHttp } from "../../helpers/causal-waits";
import { expectTouchControl } from "../../helpers/control-sizing";
import {
  assertNoDocumentHorizontalOverflow,
  expectTopmostAtCenter,
} from "../../helpers/layout-assertions";
import { installFixturePlugin, uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import {
  INSTALLED_DETAIL,
  INSTALLED_ROW,
  INSTALLED_ROW_LINK,
  REPO_LINK,
  REPOSITORY_URL,
  STAR_LABEL,
  STAR_MUTATION_PATH,
  STAR_TOGGLE,
  STAR_UPDATING_LABEL,
  TOUCH_STAR_MIN_HEIGHT,
  UNSTAR_LABEL,
  mockMarketplaceCatalog,
  openMarketplaceEntry,
  pluginCatalogEntry,
  seedConnectedRepository,
  useCapableDeployment,
} from "./plugin-stars-helpers";

/**
 * The repository-star control on a **desktop-width coarse pointer** — the
 * tablet/touch-laptop case where width alone would wrongly suggest desktop
 * density.
 *
 * This spec runs in the `desktop-coarse-pointer` Playwright project, whose
 * context emulates touch at 1280x900. The in-page guards below fail loudly if
 * that emulation is ever dropped, because a fine-pointer 1280px viewport would
 * silently render the compact control and make every 44px assertion vacuous.
 */

const MUTATION_HOLD_MS = 750;

test.describe("Plugin repository stars on a desktop-width coarse pointer", () => {
  test.afterEach(async ({ backend, apiClient }) => {
    // Deployment capability is a per-restart fact, not a stored one, and the
    // injected provider failures are process state: revert both so sibling
    // specs in this worker never inherit them.
    await backend.restart();
    await apiClient.mockGitHubSetPersonalStarFailures({});
  });

  test("keeps a 44px star target at desktop width and taps through to confirmed state", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    // Guard the emulation this spec exists for: desktop width, coarse pointer.
    expect(await testPage.evaluate(() => window.innerWidth)).toBe(1280);
    expect(await testPage.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);
    expect(await testPage.evaluate(() => matchMedia("(pointer: fine)").matches)).toBe(false);

    await useCapableDeployment(backend);
    await seedConnectedRepository(apiClient, seedData.workspaceId, false);
    await mockMarketplaceCatalog(testPage, { plugins: [pluginCatalogEntry()] });

    const row = await openMarketplaceEntry(testPage);
    const toggle = row.getByTestId(STAR_TOGGLE);
    await expect(toggle).toHaveText(STAR_LABEL);
    await expect(toggle).toHaveAttribute("aria-pressed", "false");
    await expectTouchControl(toggle);
    expect((await toggle.boundingBox())?.height).toBeGreaterThanOrEqual(TOUCH_STAR_MIN_HEIGHT);
    // The touch target is really the topmost element at its own center.
    await expectTopmostAtCenter(toggle, "marketplace star toggle");
    await assertNoDocumentHorizontalOverflow(testPage);

    const starred = waitForHttp(testPage, "PUT", STAR_MUTATION_PATH);
    await toggle.tap();
    expect((await starred).ok()).toBe(true);
    await expect(toggle).toHaveAttribute("aria-pressed", "true");
    await expect(toggle).toHaveText(UNSTAR_LABEL);
    await expectTouchControl(toggle);

    // Starring the touched control must not widen the catalog list.
    await expect(row).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage);
  });

  test("holds the 44px target, its accessible name, and the live status during a mutation", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    await useCapableDeployment(backend);
    await seedConnectedRepository(apiClient, seedData.workspaceId, false);

    let intercepted = false;
    await testPage.route("**/api/v1/github/user/starred", async (route) => {
      if (route.request().method() !== "PUT") {
        await route.fallback();
        return;
      }
      intercepted = true;
      await injectLatency(
        MUTATION_HOLD_MS,
        "the star mutation is held in flight so the busy geometry, accessible name, and live status can be observed before the confirmed state lands",
      );
      await route.fallback();
    });
    await mockMarketplaceCatalog(testPage, { plugins: [pluginCatalogEntry()] });

    const row = await openMarketplaceEntry(testPage);
    const toggle = row.getByTestId(STAR_TOGGLE);
    const liveStatus = row.getByRole("status");
    await expect(toggle).toHaveText(STAR_LABEL);
    await expect(liveStatus).toHaveText("");

    const starred = waitForHttp(testPage, "PUT", STAR_MUTATION_PATH);
    await toggle.tap();

    // Busy: same accessible name, still a 44px target, plus the localized
    // progress announcement — a renamed or shrunk control breaks both
    // screen-reader use and the tap area mid-flight.
    await expect(toggle).toBeDisabled();
    await expect(toggle).toHaveText(STAR_LABEL);
    await expect(liveStatus).toHaveText(STAR_UPDATING_LABEL);
    await expectTouchControl(toggle);

    expect((await starred).ok()).toBe(true);
    expect(intercepted, "the mutation route must intercept the PUT to hold it in flight").toBe(
      true,
    );
    await expect(toggle).toBeEnabled();
    await expect(toggle).toHaveAttribute("aria-pressed", "true");
    await expect(toggle).toHaveText(UNSTAR_LABEL);
    await expect(liveStatus).toHaveText("");
    await expectTouchControl(toggle);
  });

  test("keeps the star tap its own click without swallowing the row or repository link", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await useCapableDeployment(backend);
    await seedConnectedRepository(apiClient, seedData.workspaceId, false);
    await installFixturePlugin(testPage);

    try {
      const row = testPage.getByTestId(INSTALLED_ROW);
      const toggle = row.getByTestId(STAR_TOGGLE);
      await expect(toggle).toHaveAttribute("aria-pressed", "false");
      await expectTouchControl(toggle);

      // The row's overlay link owns the whole card; the star must sit above it
      // and must not be nested inside it.
      await expect(row.getByTestId(INSTALLED_ROW_LINK)).toHaveCount(1);
      expect(await toggle.evaluate((element) => element.closest("a") !== null)).toBe(false);
      await expectTopmostAtCenter(toggle, "installed row star toggle");

      const before = testPage.url();
      const starred = waitForHttp(testPage, "PUT", STAR_MUTATION_PATH);
      await toggle.tap();
      expect((await starred).ok()).toBe(true);
      await expect(toggle).toHaveAttribute("aria-pressed", "true");
      // Tapping the star never activates the overlay navigation.
      expect(testPage.url()).toBe(before);
      expect(new URL(testPage.url()).pathname).toBe("/settings/plugins");

      // The guarded repository link is likewise above the overlay, and still
      // points at the repository it advertises.
      const repoLink = row.getByTestId(REPO_LINK);
      await expect(repoLink).toHaveAttribute("href", REPOSITORY_URL);
      await expect(repoLink).toHaveAttribute("target", "_blank");
      await expectTopmostAtCenter(repoLink, "installed row repository link");

      // Row navigation itself is intact.
      await row.getByTestId(INSTALLED_ROW_LINK).click();
      await expect(testPage.getByTestId(INSTALLED_DETAIL)).toBeVisible();
    } finally {
      await uninstallFixturePlugin(apiClient);
    }
  });
});
