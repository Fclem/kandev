import { afterEach, describe, expect, it, vi } from "vitest";

import {
  activateLocale,
  DEFAULT_LOCALE,
  i18n,
  isSupportedLocale,
  normalizeLocale,
  selectableLocales,
  SUPPORTED_LOCALES,
} from "./index";
import { LOCALE_COOKIE, readLocaleCookie } from "./cookie";

const noopLoader = vi.fn(async () => ({ messages: {} }));

afterEach(() => {
  document.cookie = `${LOCALE_COOKIE}=; path=/; max-age=0`;
  vi.clearAllMocks();
});

describe("locale predicates", () => {
  it("recognizes supported locales", () => {
    expect(isSupportedLocale("en")).toBe(true);
    expect(isSupportedLocale("pseudo")).toBe(true);
    expect(isSupportedLocale("fr")).toBe(false);
    expect(isSupportedLocale(42)).toBe(false);
  });

  it("normalizes unknown values to the default", () => {
    expect(normalizeLocale("pseudo")).toBe("pseudo");
    expect(normalizeLocale("nope")).toBe(DEFAULT_LOCALE);
    expect(normalizeLocale(undefined)).toBe(DEFAULT_LOCALE);
  });

  it("exposes en as the default and lists both locales", () => {
    expect(DEFAULT_LOCALE).toBe("en");
    expect([...SUPPORTED_LOCALES]).toEqual(["en", "pseudo"]);
  });

  it("hides the pseudo locale in production builds only", () => {
    expect(selectableLocales(false)).toEqual(["en", "pseudo"]);
    expect(selectableLocales(true)).toEqual(["en"]);
  });
});

describe("activateLocale", () => {
  it("activates the locale, sets <html lang>, and writes the cookie", async () => {
    const result = await activateLocale("pseudo", noopLoader);
    expect(result).toBe("pseudo");
    expect(noopLoader).toHaveBeenCalledWith("pseudo");
    expect(i18n.locale).toBe("pseudo");
    expect(document.documentElement.lang).toBe("pseudo");
    expect(readLocaleCookie()).toBe("pseudo");
  });

  it("coerces an invalid locale to en", async () => {
    const result = await activateLocale("klingon", noopLoader);
    expect(result).toBe("en");
    expect(noopLoader).toHaveBeenCalledWith("en");
    expect(document.documentElement.lang).toBe("en");
  });
});
