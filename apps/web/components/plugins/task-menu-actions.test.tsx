import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { createElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@kandev/ui/dropdown-menu";
import { pluginRegistry } from "@/lib/plugins/registry";
import type {
  PluginIcon,
  PluginTaskMenuContext,
  TaskMenuSubItemRegistration,
} from "@/lib/plugins/types";
import { KanbanCardDropdownMenuItems, type KanbanCardMenuEntry } from "../kanban-card-menu-items";
import { buildPrimaryPluginEntries } from "./task-menu-actions";

const PLUGIN_ID = "kandev-plugin-tags";
const ACTION_ID = "add-tag";
const ACTION_LABEL = "Add tag";
const ACTION_KEY = `plugin-primary-${PLUGIN_ID}-${ACTION_ID}`;

const CONTEXT: PluginTaskMenuContext = {
  workspaceId: "ws-1",
  taskId: "task-1",
  taskTitle: "Fix the bug",
  workflowStepId: "step-1",
  presentation: "desktop",
};

function registerAction(
  overrides: {
    id?: string;
    label?: string;
    group?: "edit" | "primary";
    icon?: PluginIcon;
    run?: (context: PluginTaskMenuContext) => void | Promise<void>;
    items?: (context: PluginTaskMenuContext) => readonly TaskMenuSubItemRegistration[];
  } = {},
) {
  const run = overrides.run ?? vi.fn();
  pluginRegistry.forPlugin(PLUGIN_ID).registerTaskMenuAction({
    id: overrides.id ?? ACTION_ID,
    label: overrides.label ?? ACTION_LABEL,
    group: overrides.group ?? "primary",
    run,
    ...(overrides.icon ? { icon: overrides.icon } : {}),
    ...(overrides.items ? { items: overrides.items } : {}),
  });
  return run;
}

function renderEntries(entries: KanbanCardMenuEntry[]) {
  render(
    <DropdownMenu defaultOpen>
      <DropdownMenuTrigger>open</DropdownMenuTrigger>
      <DropdownMenuContent>
        <KanbanCardDropdownMenuItems entries={entries} />
      </DropdownMenuContent>
    </DropdownMenu>,
  );
}

/** Radix opens a submenu on a mouse pointer-move over its trigger. */
async function openSubmenu(label: string, childLabel: string) {
  fireEvent.pointerMove(screen.getByRole("menuitem", { name: label }), { pointerType: "mouse" });
  return screen.findByRole("menuitem", { name: childLabel });
}

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
});

describe("buildPrimaryPluginEntries — action without items", () => {
  it("stays a flat item that runs run(context)", async () => {
    const run = registerAction();
    renderEntries(buildPrimaryPluginEntries({ context: CONTEXT }));

    fireEvent.click(screen.getByRole("menuitem", { name: ACTION_LABEL }));

    await Promise.resolve();
    expect(run).toHaveBeenCalledWith(CONTEXT);
  });
});

describe("buildPrimaryPluginEntries — action declaring items", () => {
  it("renders a submenu whose children are the resolved items, in order", () => {
    registerAction({
      items: () => [
        { id: "more", label: "More tags", run: vi.fn() },
        { id: "blocked", label: "Blocked", run: vi.fn() },
      ],
    });

    const entries = buildPrimaryPluginEntries({ context: CONTEXT });

    expect(entries.map((entry) => entry.kind)).toEqual(["submenu"]);
    const entry = entries[0];
    if (entry.kind !== "submenu") return;
    expect(entry.key).toBe(ACTION_KEY);
    expect(entry.label).toBe(ACTION_LABEL);
    expect(entry.children.map((child) => child.key)).toEqual([
      `${ACTION_KEY}-more`,
      `${ACTION_KEY}-blocked`,
    ]);
    // Every child is a selectable item, not a separator or a nested submenu.
    expect(
      entry.children.map((child) => (child.kind === "item" ? child.label : child.kind)),
    ).toEqual(["More tags", "Blocked"]);
  });

  it("runs the selected child with the menu context, not the action's fallback", async () => {
    const more = vi.fn();
    const blocked = vi.fn();
    const run = registerAction({
      items: () => [
        { id: "more", label: "More tags", run: more },
        { id: "blocked", label: "Blocked", run: blocked },
      ],
    });
    renderEntries(buildPrimaryPluginEntries({ context: CONTEXT }));

    const blockedItem = await openSubmenu(ACTION_LABEL, "Blocked");
    fireEvent.click(blockedItem);

    await Promise.resolve();
    expect(blocked).toHaveBeenCalledWith(CONTEXT);
    expect(more).not.toHaveBeenCalled();
    expect(run).not.toHaveBeenCalled();
  });

  it("keeps a disabled child unselectable", async () => {
    const disabledRun = vi.fn();
    registerAction({
      items: () => [{ id: "blocked", label: "Blocked", disabled: true, run: disabledRun }],
    });
    renderEntries(buildPrimaryPluginEntries({ context: CONTEXT }));

    const child = await openSubmenu(ACTION_LABEL, "Blocked");
    fireEvent.click(child);

    await Promise.resolve();
    expect(disabledRun).not.toHaveBeenCalled();
  });

  it("disables every child while the entry itself is disabled", async () => {
    const childRun = vi.fn();
    registerAction({ items: () => [{ id: "blocked", label: "Blocked", run: childRun }] });

    const entry = buildPrimaryPluginEntries({ context: CONTEXT, disabled: true })[0];
    expect(entry.kind).toBe("submenu");
    if (entry.kind !== "submenu") return;
    // A disabled trigger cannot be opened at all; rendering its children
    // directly is what proves the disabled flag reached them.
    expect(entry.disabled).toBe(true);
    renderEntries(entry.children);

    fireEvent.click(screen.getByRole("menuitem", { name: "Blocked" }));

    await Promise.resolve();
    expect(childRun).not.toHaveBeenCalled();
  });

  // A plugin's items() runs inside the host's menu build, so an unusable list
  // must never produce a trigger with nothing behind it: the action's run() is
  // what a host predating `items` would have used anyway.
  it("logs and renders the flat fallback when items() throws", () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    registerAction({
      items: () => {
        throw new Error("items() blew up");
      },
    });

    expect(buildPrimaryPluginEntries({ context: CONTEXT }).map((entry) => entry.kind)).toEqual([
      "item",
    ]);
    expect(consoleErrorSpy).toHaveBeenCalled();
    consoleErrorSpy.mockRestore();
  });

  it("renders the flat fallback when items() yields nothing", () => {
    registerAction({ items: () => [] });

    expect(buildPrimaryPluginEntries({ context: CONTEXT }).map((entry) => entry.kind)).toEqual([
      "item",
    ]);
  });

  it("logs a rejecting child run without throwing", async () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    registerAction({
      items: () => [
        { id: "blocked", label: "Blocked", run: () => Promise.reject(new Error("boom")) },
      ],
    });
    renderEntries(buildPrimaryPluginEntries({ context: CONTEXT }));

    const child = await openSubmenu(ACTION_LABEL, "Blocked");
    expect(() => fireEvent.click(child)).not.toThrow();

    await vi.waitFor(() => expect(consoleErrorSpy).toHaveBeenCalled());
    consoleErrorSpy.mockRestore();
  });
});

describe("buildPrimaryPluginEntries — malformed plugin results", () => {
  // Plugin bundles are plain JavaScript: the registration types are a promise,
  // not a guarantee. Every shape a broken plugin can produce must degrade to
  // the flat fallback (or drop just the unusable child) instead of crashing the
  // card render or handing React children it cannot key. Each case registers a
  // distinct action id, since one broken action is reported once -- the host
  // rebuilds entries on every render, so a per-build log would spam.
  const fallbackCases: [string, () => unknown][] = [
    ["a null child", () => [null]],
    ["a child without an id or label", () => [{ run: vi.fn() }]],
    ["a child with a blank id", () => [{ id: "   ", label: "X", run: vi.fn() }]],
    ["a child without a run", () => [{ id: "x", label: "X" }]],
    ["a child with an unusable icon", () => [{ id: "x", label: "X", icon: {}, run: vi.fn() }]],
    ["a non-array result", () => "nope"],
    ["a promise", () => Promise.resolve([{ id: "x", label: "X", run: vi.fn() }])],
    [
      "an array whose methods were replaced",
      () =>
        new Proxy([{ id: "x", label: "X", run: vi.fn() }], {
          get() {
            throw new Error("trap");
          },
        }),
    ],
    [
      "a thenable that throws on lookup",
      () =>
        Object.defineProperty({}, "then", {
          get() {
            throw new Error("then getter");
          },
        }),
    ],
  ];

  for (const [index, [label, items]] of fallbackCases.entries()) {
    it(`falls back to the flat item and reports ${label} once`, async () => {
      const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
      registerAction({ id: `broken-${index}`, items: items as never });

      expect(buildPrimaryPluginEntries({ context: CONTEXT }).map((entry) => entry.kind)).toEqual([
        "item",
      ]);
      // A second build must not log again: the defect is permanent for a
      // loaded bundle, and this callback runs on every render.
      expect(buildPrimaryPluginEntries({ context: CONTEXT }).map((entry) => entry.kind)).toEqual([
        "item",
      ]);
      expect(consoleErrorSpy).toHaveBeenCalledTimes(1);

      consoleErrorSpy.mockRestore();
      // A rejected promise must not escape as an unhandled rejection; one
      // macrotask is enough for vitest to surface it if it were unobserved.
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }

  it("observes a rejected promise, so it cannot escape as an unhandled rejection", async () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    registerAction({ id: "rejecting", items: () => Promise.reject(new Error("boom")) });

    expect(buildPrimaryPluginEntries({ context: CONTEXT }).map((entry) => entry.kind)).toEqual([
      "item",
    ]);
    // Vitest fails the file on an unhandled rejection, so reaching the end of
    // this test at all proves the rejection was observed -- but the log has to
    // say so too, which the deleted-observation mutation otherwise hides.
    await vi.waitFor(() =>
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        expect.stringContaining("items() rejected"),
        expect.any(Error),
      ),
    );
    consoleErrorSpy.mockRestore();
  });
});

describe("buildPrimaryPluginEntries — unreadable and repeated children", () => {
  it("reports a repeated child id and keeps the first entry", () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const first = { id: "dup", label: "First", run: vi.fn() };
    registerAction({
      id: "duplicated",
      items: () => [first, { id: "dup", label: "Second", run: vi.fn() }],
    });

    const entry = buildPrimaryPluginEntries({ context: CONTEXT })[0];

    expect(entry.kind).toBe("submenu");
    if (entry.kind !== "submenu") return;
    expect(entry.children.map((child) => child.key)).toEqual([
      `plugin-primary-${PLUGIN_ID}-duplicated-dup`,
    ]);
    expect(consoleErrorSpy).toHaveBeenCalledWith(
      expect.stringContaining("repeated a child id"),
      "dup",
    );
    consoleErrorSpy.mockRestore();
  });

  it("reports a later defect of a different kind on the same action", () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    let impl: () => unknown = () => [null];
    registerAction({ id: "varies", items: () => impl() });

    buildPrimaryPluginEntries({ context: CONTEXT });
    impl = () => {
      throw new Error("now it throws");
    };
    buildPrimaryPluginEntries({ context: CONTEXT });

    expect(consoleErrorSpy.mock.calls.map((call) => String(call[0]))).toEqual([
      expect.stringContaining("unusable child"),
      expect.stringContaining("could not be read"),
    ]);
    consoleErrorSpy.mockRestore();
  });

  it("falls back when a child cannot be read at all", () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const hostile = { id: "x", label: "X", run: vi.fn() };
    Object.defineProperty(hostile, "icon", {
      enumerable: true,
      get() {
        throw new Error("getter blew up");
      },
    });
    registerAction({ id: "hostile-child", items: () => [hostile] });

    let entries: KanbanCardMenuEntry[] = [];
    expect(() => {
      entries = buildPrimaryPluginEntries({ context: CONTEXT });
    }).not.toThrow();
    expect(entries.map((entry) => entry.kind)).toEqual(["item"]);
    expect(consoleErrorSpy).toHaveBeenCalledTimes(1);
    consoleErrorSpy.mockRestore();
  });

  it("keeps the usable children and drops only the broken ones", () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    registerAction({
      id: "partly-broken",
      items: () => [
        { id: "gone", label: "Gone", run: vi.fn() },
        { id: "", label: "No id", run: vi.fn() },
        { id: "ok", label: "Ok", run: vi.fn() },
        { id: "gone", label: "Duplicate id", run: vi.fn() },
      ],
    });

    const entries = buildPrimaryPluginEntries({ context: CONTEXT });

    expect(entries.map((entry) => entry.kind)).toEqual(["submenu"]);
    const entry = entries[0];
    if (entry.kind !== "submenu") return;
    expect(entry.children.map((child) => child.key)).toEqual([
      `plugin-primary-${PLUGIN_ID}-partly-broken-gone`,
      `plugin-primary-${PLUGIN_ID}-partly-broken-ok`,
    ]);
    consoleErrorSpy.mockRestore();
  });
});

describe("buildPrimaryPluginEntries — mixed registrations", () => {
  it("renders a submenu action as one entry and keeps the rest flat", () => {
    registerAction({ id: "add-tag", items: () => [{ id: "more", label: "More", run: vi.fn() }] });
    registerAction({ id: "other", label: "Other action" });

    const entries = buildPrimaryPluginEntries({ context: CONTEXT });

    expect(entries.map((entry) => entry.kind)).toEqual(["submenu", "item"]);
    expect(entries.map((entry) => entry.key)).toEqual([
      ACTION_KEY,
      `plugin-primary-${PLUGIN_ID}-other`,
    ]);
  });
});

describe("buildPrimaryPluginEntries — icon resolution", () => {
  // Regression: icons registered before the name/component resolution landed
  // are ready-made elements (kandev-plugin-tags ships one). Reading one as a
  // name looked up `PLUGIN_ICONS["[object Object]"]` and rendered the puzzle
  // fallback glyph instead of the plugin's own icon.
  it("renders an element-form icon instead of the fallback glyph", () => {
    const elementIcon = createElement(
      "svg",
      { "data-testid": "plugin-menu-icon", viewBox: "0 0 24 24" },
      createElement("path", { d: "M4 4h16" }),
    );
    registerAction({ icon: elementIcon as unknown as PluginIcon });

    renderEntries(buildPrimaryPluginEntries({ context: CONTEXT }));

    expect(screen.getByTestId("plugin-menu-icon")).toBeTruthy();
  });
});
