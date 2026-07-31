import { afterEach, describe, expect, it, vi } from "vitest";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { loadPlugins } from "./host";
import { pluginRegistry } from "./registry";
import type { ActivePlugin, PluginHostApi, PluginRegistry } from "./types";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "" }),
}));

const PLUGIN_HANG_A_ID = "plugin-hang-a";
const PLUGIN_HANG_B_ID = "plugin-hang-b";

type FakeWindow = Window & {
  registerKandevPlugin: (id: string, plugin: unknown) => void;
};

function makeHostFactory(pluginId: string): PluginHostApi {
  return {
    pluginId,
    React: {} as PluginHostApi["React"],
    jsx: {} as PluginHostApi["jsx"],
    store: {
      getState: () => ({}) as never,
      setState: () => {},
      subscribe: () => () => {},
    },
    api: {
      fetch: async () => new Response(),
      invokeAction: async <TResponse>() => undefined as TResponse,
      baseUrl: "",
    },
    ui: {},
    useResponsiveBreakpoint,
    theme: "light",
    navigate: () => {},
    openModal: () => ({ close: () => {} }),
  };
}

function activePlugin(overrides: Partial<ActivePlugin> = {}): ActivePlugin {
  return {
    id: "plugin-a",
    name: "Plugin A",
    bundleUrl: "/api/plugins/plugin-a/bundle",
    ...overrides,
  };
}

function fakeImporterFor(
  bundles: Record<string, (win: Window) => void>,
): (url: string) => Promise<unknown> {
  return async (url: string) => {
    const run = bundles[url];
    if (!run) throw new Error(`no fake bundle for ${url}`);
    run(window);
    return {};
  };
}

afterEach(() => {
  pluginRegistry.unregisterPlugin(PLUGIN_HANG_A_ID);
  pluginRegistry.unregisterPlugin(PLUGIN_HANG_B_ID);
});

describe("loadPlugins — initialize() timeout isolation", () => {
  it("does not let a hung plugin block the next plugin", async () => {
    const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
    const secondInitialize = vi.fn((registry: PluginRegistry) => {
      registry.registerNavItem({ id: "nav-hang-b", label: "B", path: "/plugin-hang-b" });
    });
    const importer = fakeImporterFor({
      "/hang-bundle.js": (win) =>
        (win as unknown as FakeWindow).registerKandevPlugin(PLUGIN_HANG_A_ID, {
          initialize: () => new Promise<void>(() => {}),
        }),
      "/second-bundle.js": (win) =>
        (win as unknown as FakeWindow).registerKandevPlugin(PLUGIN_HANG_B_ID, {
          initialize: secondInitialize,
        }),
    });

    await loadPlugins(
      [
        activePlugin({ id: PLUGIN_HANG_A_ID, bundleUrl: "/hang-bundle.js" }),
        activePlugin({ id: PLUGIN_HANG_B_ID, bundleUrl: "/second-bundle.js" }),
      ],
      makeHostFactory,
      importer,
      window,
      10,
    );

    expect(secondInitialize).toHaveBeenCalledTimes(1);
    expect(pluginRegistry.getNavItems()).toContainEqual({
      id: "nav-hang-b",
      label: "B",
      path: "/plugin-hang-b",
    });
    expect(warnSpy).toHaveBeenCalledWith(expect.stringContaining(PLUGIN_HANG_A_ID));
    warnSpy.mockRestore();
  });

  it("fences registrations that arrive after initialize times out", async () => {
    let finishInitialize!: () => void;
    const initializeFinished = new Promise<void>((resolve) => {
      finishInitialize = resolve;
    });
    const importer = fakeImporterFor({
      "/late-registration.js": (win) =>
        (win as unknown as FakeWindow).registerKandevPlugin(PLUGIN_HANG_A_ID, {
          initialize: async (registry: PluginRegistry) => {
            await initializeFinished;
            registry.registerNavItem({ id: "late-nav", label: "Late", path: "/late" });
          },
        }),
    });

    await loadPlugins(
      [activePlugin({ id: PLUGIN_HANG_A_ID, bundleUrl: "/late-registration.js" })],
      makeHostFactory,
      importer,
      window,
      10,
    );
    finishInitialize();
    await Promise.resolve();

    expect(pluginRegistry.getNavItems().find((item) => item.id === "late-nav")).toBeUndefined();
  });
});
