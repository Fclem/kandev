import { describe, expect, it } from "vitest";
import { IconPuzzle, IconTicket } from "@tabler/icons-react";
import { lookupPluginIcon, resolvePluginIcon } from "./icons";

describe("plugin icons", () => {
  it("looks up a known icon name", () => {
    expect(lookupPluginIcon("ticket")).toBe(IconTicket);
    expect(resolvePluginIcon("ticket")).toBe(IconTicket);
  });

  it("does not require host-owned provider brand icons", () => {
    expect(lookupPluginIcon("bitbucket")).toBeUndefined();
    expect(resolvePluginIcon("bitbucket")).toBe(IconPuzzle);
  });

  it("passes through a plugin-owned icon component", () => {
    const PluginIcon = () => null;

    expect(lookupPluginIcon(PluginIcon as unknown as string)).toBe(PluginIcon);
    expect(resolvePluginIcon(PluginIcon as unknown as string)).toBe(PluginIcon);
  });

  it("returns undefined from lookupPluginIcon for unknown or missing names", () => {
    expect(lookupPluginIcon("not-an-icon")).toBeUndefined();
    expect(lookupPluginIcon(undefined)).toBeUndefined();
  });

  // Regression: a JavaScript bundle can pass any value. Indexing PLUGIN_ICONS
  // with a non-string coerced it to a key, so an object whose toString threw
  // took the rendering surface down, and any other object silently became the
  // puzzle glyph by way of a "[object Object]" lookup miss.
  it("passes through an exotic component, which memo/forwardRef produce", () => {
    // React's memo and forwardRef return objects that are callable per their
    // types but not at run time, and @tabler/icons-react -- the set
    // PLUGIN_ICONS maps names onto -- builds every icon with forwardRef.
    // memo() returns a non-callable object; a function carrying $$typeof would
    // not reproduce the shape that `typeof icon === "function"` accepts.
    const ExoticIcon = { $$typeof: Symbol.for("react.memo"), type: () => null };

    expect(lookupPluginIcon(ExoticIcon as never)).toBe(ExoticIcon);
    expect(resolvePluginIcon(ExoticIcon as never)).toBe(ExoticIcon);
  });

  it("treats a non-string, non-function icon as absent instead of coercing it to a name", () => {
    const hostile = {
      toString() {
        throw new Error("key coercion");
      },
    };

    expect(() => lookupPluginIcon(hostile as unknown as string)).not.toThrow();
    expect(lookupPluginIcon(hostile as unknown as string)).toBeUndefined();
    expect(resolvePluginIcon(hostile as unknown as string)).toBe(IconPuzzle);
    expect(lookupPluginIcon({} as unknown as string)).toBeUndefined();
  });

  it("falls back to the puzzle glyph from resolvePluginIcon", () => {
    expect(resolvePluginIcon("not-an-icon")).toBe(IconPuzzle);
    expect(resolvePluginIcon(undefined)).toBe(IconPuzzle);
  });
});
