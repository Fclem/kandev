import type { Page } from "@playwright/test";

export async function rewriteBackendHostOS(page: Page, hostOS: string): Promise<void> {
  await page.route("**/*", async (route) => {
    const request = route.request();
    if (request.resourceType() !== "document" || request.frame().parentFrame() !== null) {
      await route.continue();
      return;
    }
    const response = await route.fetch();
    const body = await response.text();
    const rewritten = body.replace(/"hostOS":"[^"]*"/, `"hostOS":"${hostOS}"`);
    if (rewritten === body) {
      await route.fulfill({ response, body });
      return;
    }
    const headers = { ...response.headers() };
    delete headers["content-encoding"];
    delete headers["content-length"];
    await route.fulfill({
      status: response.status(),
      headers,
      body: rewritten,
    });
  });
}
