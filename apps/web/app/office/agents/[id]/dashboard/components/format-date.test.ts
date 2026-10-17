import { describe, expect, it } from "vitest";
import { formatSubcents } from "./format-date";

describe("dashboard monetary availability", () => {
  it("keeps an unavailable measurement distinct from a measured zero", () => {
    expect(formatSubcents(null)).toBe("-");
    expect(formatSubcents(0)).toBe("$0.00");
  });

  it("retains independently recorded spend", () => {
    expect(formatSubcents(12500)).toBe("$1.25");
    expect(formatSubcents(7500)).toBe("$0.75");
  });
});
