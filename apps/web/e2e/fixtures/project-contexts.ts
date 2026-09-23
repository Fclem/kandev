import { devices, type BrowserContextOptions } from "@playwright/test";

/**
 * Browser-context options per Playwright project.
 *
 * `testPage` opens its own context, so a project's `use` block alone does not
 * decide what a spec sees. `playwright.config.ts` and `fixtures/test-base.ts`
 * both read this module, so the emulated viewport and pointer/touch
 * capabilities cannot drift between the project definition and the fixture
 * that actually opens the page.
 */

/**
 * The largest width `use-responsive-breakpoint` still classifies as `mobile`
 * (`width < 768`). A *fine* pointer at this width is the "mobile fine-pointer"
 * case: the plugin star control keeps its 44px target because the viewport is a
 * phone, not because the pointer is coarse.
 */
export const MOBILE_FINE_POINTER_VIEWPORT = { width: 767, height: 900 } as const;

/**
 * Desktop width with touch emulation. Chromium reports a coarse primary pointer
 * for a touch-enabled context at any width, which is what drives the ≥44px
 * target on this project — width alone proves nothing about the pointer.
 */
export const DESKTOP_COARSE_POINTER_VIEWPORT = { width: 1280, height: 900 } as const;

/**
 * The context options one project's pages must be opened with. Projects that
 * keep Playwright's own desktop fine-pointer defaults (including the default
 * viewport) return an empty object.
 */
export function projectContextOptions(projectName: string): BrowserContextOptions {
  switch (projectName) {
    case "mobile-chrome":
      // Pixel 5 emulation: isMobile, hasTouch, deviceScaleFactor, and a phone
      // user agent. Preserved verbatim so the phone project keeps behaving
      // exactly as it did before the fine/coarse desktop projects existed.
      return { ...devices["Pixel 5"] };
    case "mobile-fine-pointer":
      return {
        viewport: { ...MOBILE_FINE_POINTER_VIEWPORT },
        deviceScaleFactor: 1,
        hasTouch: false,
        isMobile: false,
      };
    case "desktop-coarse-pointer":
      return {
        viewport: { ...DESKTOP_COARSE_POINTER_VIEWPORT },
        deviceScaleFactor: 1,
        hasTouch: true,
        isMobile: false,
      };
    default:
      return {};
  }
}
