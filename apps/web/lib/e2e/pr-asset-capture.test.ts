import type { Page } from "@playwright/test";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it } from "vitest";

import { PrAssetCapture } from "../../e2e/helpers/pr-asset-capture";

const originalCaptureSetting = process.env.CAPTURE_PR_ASSETS;

afterEach(() => {
  if (originalCaptureSetting === undefined) {
    delete process.env.CAPTURE_PR_ASSETS;
    return;
  }
  process.env.CAPTURE_PR_ASSETS = originalCaptureSetting;
});

describe("PrAssetCapture", () => {
  it("does not remove assets created by another worker", () => {
    process.env.CAPTURE_PR_ASSETS = "1";
    const outputDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-pr-assets-"));
    try {
      const existingAsset = path.join(outputDir, "desktop.png");
      fs.writeFileSync(existingAsset, "existing asset");

      new PrAssetCapture({} as Page, "/tmp/mobile-capture.spec.ts", { outputDir });

      expect(fs.existsSync(existingAsset)).toBe(true);
    } finally {
      fs.rmSync(outputDir, { recursive: true, force: true });
    }
  });

  it("retains assets flushed by separate captures", async () => {
    process.env.CAPTURE_PR_ASSETS = "1";
    const outputDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-pr-assets-"));
    const page = {
      screenshot: async ({ path: screenshotPath }: { path: string }) => {
        fs.writeFileSync(screenshotPath, "screenshot");
      },
    } as unknown as Page;
    try {
      const desktop = new PrAssetCapture(page, "/tmp/desktop-capture.spec.ts", { outputDir });
      const mobile = new PrAssetCapture(page, "/tmp/mobile-capture.spec.ts", { outputDir });
      await desktop.screenshot("desktop");
      await mobile.screenshot("mobile");

      desktop.flush();
      mobile.flush();

      const manifest = JSON.parse(fs.readFileSync(path.join(outputDir, "manifest.json"), "utf-8"));
      expect(manifest.assets.map((asset: { file: string }) => asset.file).sort()).toEqual([
        "desktop-capture--desktop.png",
        "mobile-capture--mobile.png",
      ]);
    } finally {
      fs.rmSync(outputDir, { recursive: true, force: true });
    }
  });
});
