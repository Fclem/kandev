import { i18n, type Messages } from "@lingui/core";

import { writeLocaleCookie } from "./cookie";

/**
 * i18n runtime for the Kandev web SPA (Lingui).
 *
 * `en` is the source locale; `pseudo` is a QA-only accented/padded locale used
 * to visually prove every string was externalized. `pseudo` is filtered out of
 * the language switcher in production builds (see LanguageSettings), but the
 * runtime can still activate it if a `kandev_locale=pseudo` cookie is present.
 */
export const DEFAULT_LOCALE = "en";
export const SUPPORTED_LOCALES = ["en", "pseudo"] as const;
export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number];

/** Human-readable labels for the language switcher. */
export const LOCALE_LABELS: Record<SupportedLocale, string> = {
  en: "English",
  pseudo: "Pseudo (QA)",
};

export function isSupportedLocale(value: unknown): value is SupportedLocale {
  return typeof value === "string" && (SUPPORTED_LOCALES as readonly string[]).includes(value);
}

/** Coerce any value to a supported locale, defaulting to `en`. */
export function normalizeLocale(value: unknown): SupportedLocale {
  return isSupportedLocale(value) ? value : DEFAULT_LOCALE;
}

/**
 * Locales offered in the language switcher. The `pseudo` QA locale is hidden in
 * production builds.
 */
export function selectableLocales(isProd: boolean): SupportedLocale[] {
  return SUPPORTED_LOCALES.filter((locale) => locale !== "pseudo" || !isProd);
}

/**
 * Catalog loader. The Vite plugin compiles `.po` imports on the fly; the glob
 * template import lets the pseudo catalog be code-split out of production. A
 * fake loader can be injected in tests.
 */
export type CatalogLoader = (locale: SupportedLocale) => Promise<{ messages: Messages }>;

const defaultLoader: CatalogLoader = (locale) => import(`../../src/locales/${locale}/messages.po`);

/**
 * Load and activate a locale: fetch its compiled catalog, activate it on the
 * shared `i18n` instance, reflect it on `<html lang>`, and persist the cookie.
 * Unknown locales coerce to `en`. Returns the locale actually activated.
 */
export async function activateLocale(
  locale: string,
  loader: CatalogLoader = defaultLoader,
): Promise<SupportedLocale> {
  const normalized = normalizeLocale(locale);
  const { messages } = await loader(normalized);
  i18n.loadAndActivate({ locale: normalized, messages });
  if (typeof document !== "undefined") {
    document.documentElement.lang = normalized;
  }
  writeLocaleCookie(normalized);
  return normalized;
}

export { i18n };
