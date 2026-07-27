import { useLingui } from "@lingui/react/macro";
import {
  IconArchive,
  IconBell,
  IconCommand,
  IconCode,
  IconLayoutDashboard,
  IconPalette,
  IconTerminal2,
} from "@tabler/icons-react";
import type { Icon as TablerIcon } from "@tabler/icons-react";

export type GeneralNavItem = {
  href: string;
  label: string;
  description: string;
  icon: TablerIcon;
};

export const GENERAL_NAV_ITEMS: GeneralNavItem[] = [
  {
    href: "/settings/general/appearance",
    label: "Appearance",
    description: "Theme, metrics, and changes panel preferences",
    icon: IconPalette,
  },
  {
    href: "/settings/general/layouts",
    label: "Layouts",
    description: "Task workbench layout profiles and defaults",
    icon: IconLayoutDashboard,
  },
  {
    href: "/settings/general/terminal",
    label: "Terminal",
    description: "Shell, terminal fonts, and link behavior",
    icon: IconTerminal2,
  },
  {
    href: "/settings/general/notifications",
    label: "Notifications",
    description: "Providers and notification events",
    icon: IconBell,
  },
  {
    href: "/settings/general/editors",
    label: "Editors",
    description: "Editor integrations and defaults",
    icon: IconCode,
  },
  {
    href: "/settings/general/keyboard-shortcuts",
    label: "Keyboard Shortcuts",
    description: "Chat input and command shortcuts",
    icon: IconCommand,
  },
  {
    href: "/settings/general/task-actions",
    label: "Task Actions",
    description: "MCP task defaults and archive safeguards",
    icon: IconArchive,
  },
];

/**
 * Localized copy for {@link GENERAL_NAV_ITEMS}. The base list is a module-level
 * constant (evaluated once at import time), so translated labels have to be
 * resolved at render time through this hook instead of baked into the const.
 */
export function useGeneralNavItems(): GeneralNavItem[] {
  const { t } = useLingui();
  const copy: Record<string, { label: string; description: string }> = {
    "/settings/general/appearance": {
      label: t`Appearance`,
      description: t`Theme, metrics, and changes panel preferences`,
    },
    "/settings/general/layouts": {
      label: t`Layouts`,
      description: t`Task workbench layout profiles and defaults`,
    },
    "/settings/general/terminal": {
      label: t`Terminal`,
      description: t`Shell, terminal fonts, and link behavior`,
    },
    "/settings/general/notifications": {
      label: t`Notifications`,
      description: t`Providers and notification events`,
    },
    "/settings/general/editors": {
      label: t`Editors`,
      description: t`Editor integrations and defaults`,
    },
    "/settings/general/keyboard-shortcuts": {
      label: t`Keyboard Shortcuts`,
      description: t`Chat input and command shortcuts`,
    },
    "/settings/general/task-actions": {
      label: t`Task Actions`,
      description: t`MCP task defaults and archive safeguards`,
    },
  };
  return GENERAL_NAV_ITEMS.map((item) => ({ ...item, ...copy[item.href] }));
}
