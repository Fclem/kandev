import { describe, expect, it } from "vitest";
import { PANEL_REGISTRY, REUSABLE_PANEL_IDS } from "@/lib/state/layout-manager";
import { placeholderComponents } from "./layout-editor";

describe("layout editor placeholder components", () => {
  it("renders every reusable panel the Add panel menu offers", () => {
    for (const id of REUSABLE_PANEL_IDS) {
      const component = PANEL_REGISTRY[id].component;
      expect(
        placeholderComponents[component],
        `placeholder for reusable panel ${id} (component ${component})`,
      ).toBeDefined();
    }
  });
});
