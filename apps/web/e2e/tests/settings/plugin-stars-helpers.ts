import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { BackendContext } from "../../fixtures/backend";
import { seedPersonalStarDeployment } from "../../fixtures/personal-star-deployment";
import { PLUGIN_ID } from "../../helpers/plugin-fixture";

/**
 * Shared fixture for the plugin-repository-star specs
 * (`REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001`).
 *
 * Every surface under test — marketplace catalog, installed row, installed
 * detail, Canvases detail, and `CanvasInstallDialog` — is seeded from here, so
 * the desktop fine-pointer, desktop coarse-pointer, Pixel 5, and 767px
 * fine-pointer specs assert the same repository, the same provider stub, and
 * the same user-facing copy.
 */

/**
 * The fixture plugin's manifest `repo_url`, and the repository every surface
 * queries. The installed row and detail read it from the packaged manifest, so
 * the value also proves the shipped fixture package carries a canonical GitHub
 * repository URL end to end.
 */
export const REPOSITORY_URL = "https://github.com/kdlbs/kandev-plugin-template";

/** Canonical `owner/name` key the backend answers star states under. */
export const REPOSITORY_KEY = "kdlbs/kandev-plugin-template";

export const STAR_TOGGLE = "plugin-star-toggle";
export const STAR_CTA = "plugin-star-connect";
export const REPO_LINK = "plugin-repo-link";
export const MARKETPLACE_ENTRY = `marketplace-entry-${PLUGIN_ID}`;
export const INSTALLED_ROW = `plugin-row-${PLUGIN_ID}`;
export const INSTALLED_ROW_LINK = `plugin-row-link-${PLUGIN_ID}`;
export const INSTALLED_DETAIL = `plugin-detail-${PLUGIN_ID}`;

/** English copy from `src/locales/en/plugins.json`, the default E2E locale. */
export const STAR_LABEL = "Star";
export const UNSTAR_LABEL = "Unstar";
export const STAR_UPDATING_LABEL = "Updating star…";
export const CONNECT_LABEL = "Connect GitHub";
export const RECONNECT_LABEL = "Reconnect GitHub";

export const CONNECTION_STATUS_PATH = /^\/api\/v1\/github\/user\/starred\/connection$/;
export const CONNECTION_START_PATH = /^\/api\/v1\/github\/user\/starred\/connection\/start$/;
export const STAR_STATUS_PATH = /^\/api\/v1\/github\/user\/starred\/status$/;
export const STAR_MUTATION_PATH = /^\/api\/v1\/github\/user\/starred$/;

/** Compact non-mobile fine-pointer target, and the touch-context floor. */
export const COMPACT_STAR_HEIGHT = 24;
export const TOUCH_STAR_MIN_HEIGHT = 44;

/**
 * Exposes the deployment-owned secret material the GitHub provider needs, via a
 * one-shot `backend.restart(seed.env)`. Without it the deployment reports
 * `available: false` and every surface correctly renders no control at all.
 */
export async function useCapableDeployment(backend: BackendContext): Promise<void> {
  const seed = seedPersonalStarDeployment(backend);
  await backend.restart(seed.env);
}

/** Seeds a capable connection and one confirmed repository state. */
export async function seedConnectedRepository(
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

/** The fixture plugin's marketplace catalog entry, with its GitHub repo URL. */
export function pluginCatalogEntry() {
  return {
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
  };
}

/** A registry canvas whose repository is the same one the plugin rows read. */
export function canvasCatalogEntry() {
  return {
    id: "canvas-star-board",
    kind: "canvas",
    name: "Star board",
    description: "A portable board with a repository of its own.",
    author: "Kandev E2E",
    categories: ["planning"],
    icon_url: "",
    repo_url: REPOSITORY_URL,
    version: "1.2.0",
    min_kandev_version: "0.94.0",
    license: "MIT",
    package_url: "https://downloads.example.test/canvas-star-board-1.2.0.tar.gz",
    package_sha256: "c".repeat(64),
    previews: [],
    permissions: { reads: ["tasks"], writes: [], events: [], shared_state: false },
    stars: 3,
    updated_at: "2026-09-01T00:00:00.000Z",
    install_state: "available",
    source_id: "official",
    source_name: "Kandev Official",
  };
}

function officialSource() {
  return {
    id: "official",
    name: "Kandev Official",
    url: "https://example.invalid",
    enabled: true,
    builtin: true,
    healthy: true,
  };
}

export type CatalogContents = {
  plugins?: unknown[];
  canvases?: unknown[];
};

/** Serves one marketplace catalog with the requested plugin/canvas entries. */
export async function mockMarketplaceCatalog(page: Page, contents: CatalogContents): Promise<void> {
  await page.route("**/api/plugins/marketplace*", async (route) => {
    if (route.request().method() !== "GET") {
      await route.fallback();
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        plugins: contents.plugins ?? [],
        canvases: contents.canvases ?? [],
        sources: [officialSource()],
      }),
    });
  });
}

/** Opens Settings > Plugins > Browse and waits for the mocked catalog entry. */
export async function openMarketplaceEntry(page: Page): Promise<Locator> {
  await page.goto("/settings/plugins");
  await page.getByTestId("plugins-tab-browse").click();
  const row = page.getByTestId(MARKETPLACE_ENTRY);
  await expect(row).toBeVisible();
  return row;
}

/** Opens Settings > Plugins > Canvases and waits for the mocked canvas card. */
export async function openCanvasMarketplaceCard(page: Page, canvasId: string): Promise<Locator> {
  await page.goto("/settings/plugins");
  await page.getByTestId("plugins-tab-canvases").click();
  const card = page.getByTestId(`canvas-marketplace-entry-${canvasId}`);
  await expect(card).toBeVisible();
  return card;
}
