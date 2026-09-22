import { createElement, isValidElement, type ReactNode } from "react";
import { pluginRegistry } from "@/lib/plugins/registry";
import { resolvePluginIcon } from "@/lib/plugins/icons";
import type { PluginTaskMenuActionRegistration } from "@/lib/plugins/registry-registration-types";
import type {
  PluginIcon,
  PluginTaskMenuContext,
  TaskMenuSubItemRegistration,
} from "@/lib/plugins/types";
import type { KanbanCardMenuEntry } from "../kanban-card-menu-items";

/**
 * Registered actions for `group`, filtered by `visible(context)` (default
 * visible when omitted). A `visible` that throws is caught, logged, and
 * treated as hidden — the same defensive handling as `run`'s rejection.
 */
export function visiblePluginMenuActions(
  group: PluginTaskMenuActionRegistration["group"],
  context: PluginTaskMenuContext,
): PluginTaskMenuActionRegistration[] {
  return pluginRegistry.getTaskMenuActions(group).filter((action) => {
    if (!action.visible) return true;
    try {
      return action.visible(context);
    } catch (error: unknown) {
      console.error(
        `[plugins] task menu action "${action.pluginId}:${action.id}" visible() threw`,
        error,
      );
      return false;
    }
  });
}

/**
 * Plugin icons get the same sizing every neighbouring native menu icon uses.
 * A ready-made element — the shape registered before icon names/components
 * were resolved, and still shipped by kandev-plugin-tags — renders as-is:
 * passing it to the resolver would treat it as a name, miss the curated map,
 * and silently replace the plugin's own glyph with the fallback puzzle.
 */
function pluginMenuIcon(icon?: PluginIcon): ReactNode {
  if (!icon) return undefined;
  if (isValidElement(icon)) return icon;
  return createElement(resolvePluginIcon(icon), { className: "mr-2 h-4 w-4" });
}

/**
 * Plugin bundles are plain JavaScript, so the registration types are a
 * promise, not a guarantee. One log per action *and kind* is enough: `items()`
 * runs on every menu build, so a permanently malformed registration would
 * otherwise log on every render, while a later defect of a different kind on
 * the same action still reports.
 */
const loggedSubItemDefects = new Set<string>();

function logSubItemDefect(
  action: PluginTaskMenuActionRegistration,
  kind: string,
  detail?: unknown,
): void {
  const key = `${action.pluginId}:${action.id}:${kind}`;
  if (loggedSubItemDefects.has(key)) return;
  loggedSubItemDefects.add(key);
  console.error(`[plugins] task menu action "${action.pluginId}:${action.id}" ${kind}`, detail);
}

/** One child, read once into the plain values the menu actually renders. */
type SubItemSnapshot = {
  id: string;
  label: string;
  icon?: PluginIcon;
  disabled?: boolean;
  run: (context: PluginTaskMenuContext) => void | Promise<void>;
};

/**
 * Reads one plugin-supplied child into a snapshot the render can trust: a
 * non-blank id and label, a callable run, and optional fields of the shapes
 * the entry builder understands (a curated name, a component, or a ready-made
 * element -- never an arbitrary object, which the icon resolver would coerce
 * into a key). Everything the render will read is read here, inside the
 * catch, so a throwing getter or a Proxy fails this child instead of the
 * card's render, and nothing is read twice.
 */
function readSubItem(item: unknown): SubItemSnapshot | null {
  if (!item || typeof item !== "object") return null;
  try {
    const { id, label, icon, disabled, run } = item as Partial<TaskMenuSubItemRegistration>;
    if (typeof id !== "string" || id.trim().length === 0) return null;
    if (typeof label !== "string" || label.trim().length === 0) return null;
    if (typeof run !== "function") return null;
    if (disabled !== undefined && typeof disabled !== "boolean") return null;
    if (
      icon !== undefined &&
      typeof icon !== "string" &&
      typeof icon !== "function" &&
      !isValidElement(icon)
    ) {
      return null;
    }
    return { id, label, icon, disabled, run };
  } catch {
    return null;
  }
}

/**
 * The children of an action that declares `items`, or null when the action
 * has no usable submenu — leaving `run` as the entry's behavior.
 *
 * That fallback is the whole defensive boundary, because `items()` is a
 * plugin callback running inside the host's menu build, and every way its
 * result can be unreadable is the same answer: a throw, a thenable (the
 * contract is synchronous), a non-array, a result whose entries cannot be
 * rendered (blank/absent id or label, no callable run, an icon of the wrong
 * shape), a child that cannot even be read (a throwing getter, a Proxy), and
 * a repeated id all degrade to the flat item — or drop just that child —
 * instead of crashing the card's render, producing a trigger nothing can
 * open, or handing React children it cannot key. The body is one `try` on
 * purpose: a plugin object is free to throw from any property access,
 * including the array methods and the `then` lookup this function would
 * otherwise perform on it.
 */
function pluginSubItems(
  action: PluginTaskMenuActionRegistration,
  context: PluginTaskMenuContext,
): readonly SubItemSnapshot[] | null {
  if (typeof action.items !== "function") return null;

  try {
    const result: unknown = action.items(context);

    if (result && typeof (result as { then?: unknown }).then === "function") {
      logSubItemDefect(action, "items() returned a promise; it must be synchronous");
      // Observe the rejection so an out-of-contract async callback cannot
      // surface as an unhandled rejection out of the host's menu build.
      void Promise.resolve(result).catch((error: unknown) => {
        logSubItemDefect(action, "items() rejected", error);
      });
      return null;
    }

    if (!Array.isArray(result)) {
      // null/undefined means "no children"; anything else is out of contract.
      if (result !== null && result !== undefined) {
        logSubItemDefect(action, "items() must return an array");
      }
      return null;
    }

    // Array.from copies through the array's own length and indices, so no
    // array method is looked up on the plugin's object.
    const list = Array.from(result as readonly unknown[]);
    const seen = new Set<string>();
    const usable: SubItemSnapshot[] = [];
    list.forEach((item) => {
      const child = readSubItem(item);
      if (!child) {
        logSubItemDefect(action, "items() returned an unusable child", item);
        return;
      }
      if (seen.has(child.id)) {
        logSubItemDefect(action, "items() repeated a child id", child.id);
        return;
      }
      seen.add(child.id);
      usable.push(child);
    });
    return usable.length > 0 ? usable : null;
  } catch (error: unknown) {
    logSubItemDefect(action, "items() could not be read", error);
    return null;
  }
}

/**
 * `Promise.resolve().then(() => run(context))` — not
 * `Promise.resolve(run(context))` — so a *synchronous* throw inside run()
 * also lands in the `.catch()` below. Calling run directly as the
 * Promise.resolve() argument still throws before that expression finishes
 * evaluating, escaping past .catch() entirely and straight out of this
 * onSelect handler. `subject` names the failing part of a submenu action.
 */
function runPluginCallback(
  action: PluginTaskMenuActionRegistration,
  run: (context: PluginTaskMenuContext) => void | Promise<void>,
  context: PluginTaskMenuContext,
  subject = "",
): void {
  Promise.resolve()
    .then(() => run(context))
    .catch((error: unknown) => {
      console.error(
        `[plugins] task menu action "${action.pluginId}:${action.id}"${subject} failed`,
        error,
      );
    });
}

/**
 * Builds the menu entry for one plugin task menu action: a submenu when the
 * action declares a non-empty `items(context)`, otherwise the flat item it
 * has always been (and still is on a host that predates submenus, which
 * ignores `items` and reads only `label`/`icon`/`run`).
 */
export function pluginMenuEntry(
  action: PluginTaskMenuActionRegistration,
  context: PluginTaskMenuContext,
  disabled?: boolean,
  keyPrefix = "plugin-edit",
): KanbanCardMenuEntry {
  const key = `${keyPrefix}-${action.pluginId}-${action.id}`;
  const items = pluginSubItems(action, context);

  if (items) {
    return {
      kind: "submenu",
      key,
      icon: pluginMenuIcon(action.icon),
      label: action.label,
      disabled,
      children: items.map((item) => ({
        kind: "item",
        key: `${key}-${item.id}`,
        icon: pluginMenuIcon(item.icon),
        label: item.label,
        disabled: disabled || item.disabled,
        onSelect: () => runPluginCallback(action, item.run, context, ` item "${item.id}"`),
      })),
    };
  }

  return {
    kind: "item",
    key,
    icon: pluginMenuIcon(action.icon),
    label: action.label,
    disabled,
    onSelect: () => runPluginCallback(action, action.run, context),
  };
}

/**
 * Builds flat, top-level menu entries for group "primary" task menu
 * actions, in registration order. Unlike group "edit" (nested in the Edit
 * submenu), these render directly in the card menu — after the movement
 * group and before the Archive/Delete removal group. An action that declares
 * `items` is the one exception: it renders as its own submenu.
 */
export function buildPrimaryPluginEntries({
  disabled,
  context,
}: {
  disabled?: boolean;
  context: PluginTaskMenuContext;
}): KanbanCardMenuEntry[] {
  return visiblePluginMenuActions("primary", context).map((action) =>
    pluginMenuEntry(action, context, disabled, "plugin-primary"),
  );
}
