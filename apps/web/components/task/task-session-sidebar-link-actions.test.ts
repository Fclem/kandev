import { describe, expect, it, vi } from "vitest";
import { runPluginTaskLinkAction } from "./task-session-sidebar-link-actions";

describe("runPluginTaskLinkAction", () => {
  it("closes the containing mobile drawer before invoking the plugin callback", () => {
    const calls: string[] = [];
    const closeSurface = vi.fn(() => calls.push("close"));
    const run = vi.fn(() => {
      calls.push("plugin");
      return Promise.resolve();
    });

    runPluginTaskLinkAction(closeSurface, run);

    expect(calls).toEqual(["close", "plugin"]);
  });
});
