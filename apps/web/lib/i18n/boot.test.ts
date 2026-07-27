import { afterEach, describe, expect, it } from "vitest";

import type { BootPayload } from "@/src/boot-payload";

import { resolveInitialLocale } from "./boot";
import { LOCALE_COOKIE } from "./cookie";

function clearLocaleCookie() {
  document.cookie = `${LOCALE_COOKIE}=; path=/; max-age=0`;
}

function payloadWithLocale(locale?: string): BootPayload {
  return { runtime: locale ? { locale } : {}, initialState: {} };
}

describe("resolveInitialLocale", () => {
  afterEach(clearLocaleCookie);

  it("prefers the boot payload locale", () => {
    document.cookie = `${LOCALE_COOKIE}=en; path=/`;
    expect(resolveInitialLocale(payloadWithLocale("pseudo"))).toBe("pseudo");
  });

  it("falls back to the cookie when the payload has no locale", () => {
    document.cookie = `${LOCALE_COOKIE}=pseudo; path=/`;
    expect(resolveInitialLocale(payloadWithLocale())).toBe("pseudo");
  });

  it("defaults to en when neither payload nor cookie is present", () => {
    expect(resolveInitialLocale(undefined)).toBe("en");
  });

  it("coerces an unknown payload locale to en", () => {
    expect(resolveInitialLocale(payloadWithLocale("klingon"))).toBe("en");
  });
});
