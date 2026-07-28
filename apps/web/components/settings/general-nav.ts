import { useTranslation } from "react-i18next";
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
  const { t } = useTranslation();
  const copy: Record<string, { label: string; description: string }> = {
    "/settings/general/appearance": {
      label: t("settings:appearance"),
      description: t("settings:themeMetricsAndChangesPanelPreferences"),
    },
    "/settings/general/layouts": {
      label: t("settings:layouts"),
      description: t("settings:taskWorkbenchLayoutProfilesAndDefaults"),
    },
    "/settings/general/terminal": {
      label: t("settings:terminal"),
      description: t("settings:shellTerminalFontsAndLinkBehavior"),
    },
    "/settings/general/notifications": {
      label: t("settings:notifications"),
      description: t("settings:providersAndNotificationEvents"),
    },
    "/settings/general/editors": {
      label: t("settings:editors"),
      description: t("settings:editorIntegrationsAndDefaults"),
    },
    "/settings/general/keyboard-shortcuts": {
      label: t("settings:keyboardShortcuts"),
      description: t("settings:chatInputAndCommandShortcuts"),
    },
    "/settings/general/task-actions": {
      label: t("settings:taskActions"),
      description: t("settings:mcpTaskDefaultsAndArchiveSafeguards"),
    },
  };
  return GENERAL_NAV_ITEMS.map((item) => ({ ...item, ...copy[item.href] }));
}
