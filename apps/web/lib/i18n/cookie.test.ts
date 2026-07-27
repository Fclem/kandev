import { afterEach, describe, expect, it } from "vitest";

import { LOCALE_COOKIE, readLocaleCookie, writeLocaleCookie } from "./cookie";

function clearCookies() {
  for (const part of document.cookie.split(";")) {
    const name = part.split("=")[0]?.trim();
    if (name) document.cookie = `${name}=; path=/; max-age=0`;
  }
}

describe("locale cookie", () => {
  afterEach(clearCookies);

  it("round-trips a written locale", () => {
    writeLocaleCookie("pseudo");
    expect(readLocaleCookie()).toBe("pseudo");
  });

  it("returns undefined when the cookie is absent", () => {
    expect(readLocaleCookie()).toBeUndefined();
  });

  it("uses the shared cookie name", () => {
    writeLocaleCookie("en");
    expect(document.cookie).toContain(`${LOCALE_COOKIE}=en`);
  });

  it("does not confuse a similarly-prefixed cookie", () => {
    document.cookie = `${LOCALE_COOKIE}_other=xx; path=/`;
    expect(readLocaleCookie()).toBeUndefined();
  });
});
