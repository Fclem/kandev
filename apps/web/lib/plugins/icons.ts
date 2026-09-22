/**
 * Curated icon-name → Tabler icon map for plugin registrations.
 *
 * Registrations may use a curated opaque name or a plugin-owned component.
 * The host maps known names onto first-party glyphs and passes component icons
 * through; unknown or missing names retain each surface's existing fallback
 * (puzzle piece in the sidebar, no icon in the page topbar).
 */
import {
  IconBell,
  IconBolt,
  IconBook,
  IconBug,
  IconCalendar,
  IconChartBar,
  IconChecklist,
  IconCloud,
  IconDatabase,
  IconFlask,
  IconGlobe,
  IconMessage,
  IconPuzzle,
  IconRobot,
  IconRocket,
  IconSettings,
  IconTicket,
  IconUsers,
} from "@tabler/icons-react";
import type { Icon as TablerIcon } from "@tabler/icons-react";
import type { PluginIcon } from "./types";

type PluginIconComponent = Exclude<PluginIcon, string>;
type ResolvedPluginIcon = TablerIcon | PluginIconComponent;

/** Curated icon names a plugin may reference instead of supplying its own component. */
export const PLUGIN_ICONS: Record<string, TablerIcon> = {
  bell: IconBell,
  bolt: IconBolt,
  book: IconBook,
  bug: IconBug,
  calendar: IconCalendar,
  chart: IconChartBar,
  checklist: IconChecklist,
  cloud: IconCloud,
  database: IconDatabase,
  flask: IconFlask,
  globe: IconGlobe,
  message: IconMessage,
  puzzle: IconPuzzle,
  robot: IconRobot,
  rocket: IconRocket,
  settings: IconSettings,
  ticket: IconTicket,
  users: IconUsers,
};

/**
 * A component value a plugin may hand us, including React's exotic components:
 * `memo`, `forwardRef` and `lazy` return *objects* (callable per their types,
 * not at run time), and an icon set -- which `host-api` explicitly tells
 * plugins to bundle -- is built from `forwardRef`. A plain `typeof` check
 * would therefore reject the host's own icon shapes.
 */
export function isPluginIconComponent(icon: unknown): icon is ResolvedPluginIcon {
  if (typeof icon === "function") return true;
  if (typeof icon !== "object" || icon === null) return false;
  try {
    return "$$typeof" in icon;
  } catch {
    return false;
  }
}

/**
 * Strict lookup: the named icon, or undefined when the name is unknown/missing.
 * Only a string names a curated icon: any other value (a stray object or a
 * ready-made element from a JavaScript bundle) has no name to look up, and
 * indexing the map with it would coerce it to a key -- which a hostile
 * `toString` turns into a throw out of whatever is rendering. A component,
 * exotic or not, needs no lookup at all.
 */
export function lookupPluginIcon(icon?: PluginIcon): ResolvedPluginIcon | undefined {
  if (isPluginIconComponent(icon)) return icon;
  return typeof icon === "string" ? PLUGIN_ICONS[icon] : undefined;
}

/** Sidebar lookup: always renders something — unknown/missing names get the puzzle glyph. */
export function resolvePluginIcon(icon?: PluginIcon): ResolvedPluginIcon {
  return lookupPluginIcon(icon) ?? IconPuzzle;
}
