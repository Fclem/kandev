import { describe, expect, it } from "vitest";
import { formatDollars } from "@/lib/utils";

describe("cost measurement availability", () => {
  it("does not turn an absent monetary measurement into a measured free result", () => {
    expect(formatDollars(null)).toBe("-");
    expect(formatDollars(undefined)).toBe("-");
  });

  it("preserves measured zero and independently recorded spend", () => {
    expect(formatDollars(0)).toBe("$0.00");
    expect(formatDollars(12500)).toBe("$1.25");
  });
});
