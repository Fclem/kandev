import { expect, test } from "../../fixtures/test-base";
import { injectLatency, waitForHttp } from "../../helpers/causal-waits";
import { expectTouchControl } from "../../helpers/control-sizing";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { MOBILE_FINE_POINTER_VIEWPORT } from "../../fixtures/project-contexts";
import {
  STAR_LABEL,
  STAR_MUTATION_PATH,
  STAR_TOGGLE,
  STAR_UPDATING_LABEL,
  UNSTAR_LABEL,
  mockMarketplaceCatalog,
  openMarketplaceEntry,
  pluginCatalogEntry,
  seedConnectedRepository,
  useCapableDeployment,
} from "./plugin-stars-helpers";

/**
 * The repository-star control at 767px with a **fine pointer** — the largest
 * width `use-responsive-breakpoint` still classifies as mobile.
 *
 * The 44px target here comes from the viewport, not from touch emulation, which
 * is exactly what the guards below pin down: the same fine pointer as the
 * desktop `chromium` project, one pixel narrower than its mobile breakpoint.
 */

const MUTATION_HOLD_MS = 750;

test.describe("Plugin repository stars on a fine-pointer phone-width viewport", () => {
  test.afterEach(async ({ backend, apiClient }) => {
    await backend.restart();
    await apiClient.mockGitHubSetPersonalStarFailures({});
  });

  test("keeps a 44px star target on a fine pointer at 767px and stars causally", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    expect(await testPage.evaluate(() => window.innerWidth)).toBe(
      MOBILE_FINE_POINTER_VIEWPORT.width,
    );
    expect(await testPage.evaluate(() => matchMedia("(pointer: fine)").matches)).toBe(true);
    expect(await testPage.evaluate(() => matchMedia("(any-pointer: coarse)").matches)).toBe(false);

    await useCapableDeployment(backend);
    await seedConnectedRepository(apiClient, seedData.workspaceId, false);
    await mockMarketplaceCatalog(testPage, { plugins: [pluginCatalogEntry()] });

    const row = await openMarketplaceEntry(testPage);
    const toggle = row.getByTestId(STAR_TOGGLE);
    await expect(toggle).toHaveText(STAR_LABEL);
    await expect(toggle).toHaveAttribute("aria-pressed", "false");
    // Fine pointer, but still the touch-sized target: width decides here.
    await expectTouchControl(toggle);
    await assertNoDocumentHorizontalOverflow(testPage);

    const starred = waitForHttp(testPage, "PUT", STAR_MUTATION_PATH);
    await toggle.click();
    expect((await starred).ok()).toBe(true);
    await expect(toggle).toHaveAttribute("aria-pressed", "true");
    await expect(toggle).toHaveText(UNSTAR_LABEL);
    await expectTouchControl(toggle);

    const unstarred = waitForHttp(testPage, "DELETE", STAR_MUTATION_PATH);
    await toggle.click();
    expect((await unstarred).ok()).toBe(true);
    await expect(toggle).toHaveAttribute("aria-pressed", "false");
    await expect(toggle).toHaveText(STAR_LABEL);
    await assertNoDocumentHorizontalOverflow(testPage);
  });

  test("announces the in-flight mutation and keeps the 44px target while it runs", async ({
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
        "the star mutation is held in flight so the busy geometry and the localized live status can be observed at phone width",
      );
      await route.fallback();
    });
    await mockMarketplaceCatalog(testPage, { plugins: [pluginCatalogEntry()] });

    const row = await openMarketplaceEntry(testPage);
    const toggle = row.getByTestId(STAR_TOGGLE);
    const liveStatus = row.getByRole("status");
    await expect(liveStatus).toHaveText("");

    const starred = waitForHttp(testPage, "PUT", STAR_MUTATION_PATH);
    await toggle.click();

    await expect(toggle).toBeDisabled();
    await expect(toggle).toHaveText(STAR_LABEL);
    await expect(liveStatus).toHaveText(STAR_UPDATING_LABEL);
    await expectTouchControl(toggle);
    await assertNoDocumentHorizontalOverflow(testPage);

    expect((await starred).ok()).toBe(true);
    expect(intercepted, "the mutation route must intercept the PUT to hold it in flight").toBe(
      true,
    );
    await expect(toggle).toBeEnabled();
    await expect(toggle).toHaveAttribute("aria-pressed", "true");
    await expect(liveStatus).toHaveText("");
  });
});
