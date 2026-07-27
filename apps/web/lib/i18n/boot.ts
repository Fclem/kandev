import type { BootPayload } from "@/src/boot-payload";

import { readLocaleCookie } from "./cookie";
import { DEFAULT_LOCALE, normalizeLocale, type SupportedLocale } from "./index";

/**
 * Resolve the locale to activate before first paint. Precedence:
 *   boot payload (set by the Go shell from the cookie) → cookie → default.
 * Any unknown value coerces to `en`.
 */
export function resolveInitialLocale(
  payload: BootPayload | undefined,
  doc: Document | undefined = typeof document === "undefined" ? undefined : document,
): SupportedLocale {
  const fromPayload = payload?.runtime?.locale;
  if (fromPayload) {
    return normalizeLocale(fromPayload);
  }
  const fromCookie = doc ? readLocaleCookie(doc) : undefined;
  if (fromCookie) {
    return normalizeLocale(fromCookie);
  }
  return DEFAULT_LOCALE;
}
