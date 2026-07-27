import { beforeAll, describe, expect, it } from "vitest";

import { formatDate, formatNumber, formatRelative } from "./formats";
import { i18n } from "./index";

beforeAll(() => {
  // Activate en with an empty catalog; the `t` macro then falls back to its
  // baked-in English source, matching the former timeAgo behavior.
  i18n.loadAndActivate({ locale: "en", messages: {} });
});

describe("formatRelative (en, timeAgo-compatible)", () => {
  const now = new Date("2026-07-27T12:00:00Z").getTime();
  const ago = (ms: number) => new Date(now - ms).toISOString();

  it("returns empty string for empty or invalid input", () => {
    expect(formatRelative("", now)).toBe("");
    expect(formatRelative("not-a-date", now)).toBe("");
  });

  it("returns 'just now' under a minute", () => {
    expect(formatRelative(ago(30_000), now)).toBe("just now");
  });

  it("formats minutes, hours, and days", () => {
    expect(formatRelative(ago(5 * 60_000), now)).toBe("5m ago");
    expect(formatRelative(ago(3 * 3_600_000), now)).toBe("3h ago");
    expect(formatRelative(ago(2 * 86_400_000), now)).toBe("2d ago");
  });
});

describe("locale-aware Intl wrappers", () => {
  it("formats numbers using the active locale", () => {
    i18n.loadAndActivate({ locale: "en", messages: {} });
    expect(formatNumber(1234567.89)).toBe("1,234,567.89");
  });

  it("maps the pseudo locale to en for Intl", () => {
    i18n.loadAndActivate({ locale: "pseudo", messages: {} });
    // pseudo has no CLDR data; wrappers fall back to en formatting.
    expect(formatNumber(1000)).toBe("1,000");
    expect(formatDate("2026-07-27T00:00:00Z", { year: "numeric" })).toBe("2026");
    i18n.loadAndActivate({ locale: "en", messages: {} });
  });
});
