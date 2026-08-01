export type RenderingEngine = "webkit" | "other";

export type RenderingNavigator = {
  userAgent?: string;
  platform?: string;
  maxTouchPoints?: number;
};

const BLINK_USER_AGENT_TOKENS = /(?:Chrome|Chromium|HeadlessChrome|Edg|OPR|SamsungBrowser)\//i;
const IOS_USER_AGENT_TOKENS = /\b(?:iPad|iPhone|iPod)\b/i;

function browserNavigator(): RenderingNavigator | undefined {
  if (typeof navigator === "undefined") return undefined;
  return navigator;
}

function isIOSLike(navigatorLike: RenderingNavigator, userAgent: string): boolean {
  if (IOS_USER_AGENT_TOKENS.test(userAgent)) return true;

  return (
    navigatorLike.platform?.toLowerCase() === "macintel" && (navigatorLike.maxTouchPoints ?? 0) > 1
  );
}

/**
 * Identify WebKit without mistaking desktop Chromium's AppleWebKit token for
 * the rendering engine. iOS browser brands remain WebKit because iOS routes
 * all browser engines through the system WebKit runtime.
 */
export function detectRenderingEngine(
  navigatorLike: RenderingNavigator | null | undefined = browserNavigator(),
): RenderingEngine {
  if (!navigatorLike) return "other";

  const userAgent = navigatorLike.userAgent ?? "";
  if (!/AppleWebKit\//i.test(userAgent)) return "other";
  if (isIOSLike(navigatorLike, userAgent)) return "webkit";
  if (BLINK_USER_AGENT_TOKENS.test(userAgent)) return "other";

  return "webkit";
}

/** Apply the transient engine marker used by scoped rendering workarounds. */
export function markRenderingEngine(
  root: HTMLElement,
  navigatorLike: RenderingNavigator | null | undefined = browserNavigator(),
): RenderingEngine {
  const engine = detectRenderingEngine(navigatorLike);
  root.dataset.renderingEngine = engine;
  return engine;
}
