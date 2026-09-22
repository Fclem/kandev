import { createElement, type ReactNode } from "react";
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

/** Plugin icons get the same sizing every neighbouring native menu icon uses. */
function pluginMenuIcon(icon?: PluginIcon): ReactNode {
  return icon ? createElement(resolvePluginIcon(icon), { className: "mr-2 h-4 w-4" }) : undefined;
}

/**
 * The children of an action that declares `items`, or null when the action
 * has no usable submenu — leaving `run` as the entry's behavior. `items()` is
 * a plugin callback running inside the host's menu build, so an empty or
 * malformed result and a throw all degrade to the flat fallback rather than
 * rendering a submenu trigger nothing can open; a throw is logged.
 */
function pluginSubItems(
  action: PluginTaskMenuActionRegistration,
  context: PluginTaskMenuContext,
): readonly TaskMenuSubItemRegistration[] | null {
  if (typeof action.items !== "function") return null;
  try {
    const items = action.items(context);
    return Array.isArray(items) && items.length > 0 ? items : null;
  } catch (error: unknown) {
    console.error(
      `[plugins] task menu action "${action.pluginId}:${action.id}" items() threw`,
      error,
    );
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
