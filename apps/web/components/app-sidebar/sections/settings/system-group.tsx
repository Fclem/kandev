"use client";
import {
  IconActivity,
  IconArchive,
  IconDatabase,
  IconFileText,
  IconFlask,
  IconInfoCircle,
  IconRefresh,
  IconScale,
  IconServerCog,
  IconTrash,
  IconUsers,
} from "@tabler/icons-react";
import type { Icon as TablerIcon } from "@tabler/icons-react";
import { t } from "@/lib/i18n";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";
import { useTranslation } from "react-i18next";

const ROOT_HREF = "/settings/system";
const DEFAULT_HREF = `${ROOT_HREF}/status`;

type SystemNavItem = { href: string; label: string; icon: TablerIcon };

const baseItems = (): SystemNavItem[] => [
  { href: `${ROOT_HREF}/status`, label: t("common:status"), icon: IconActivity },
  { href: `${ROOT_HREF}/feature-toggles`, label: t("common:featureToggles"), icon: IconFlask },
  { href: `${ROOT_HREF}/database`, label: t("common:database"), icon: IconDatabase },
  { href: `${ROOT_HREF}/backups`, label: t("common:backups"), icon: IconArchive },
  { href: `${ROOT_HREF}/storage`, label: t("common:storage"), icon: IconTrash },
  { href: `${ROOT_HREF}/logs`, label: t("common:logs"), icon: IconFileText },
  { href: `${ROOT_HREF}/updates`, label: t("common:updates"), icon: IconRefresh },
  { href: `${ROOT_HREF}/about`, label: t("common:about"), icon: IconInfoCircle },
  { href: `${ROOT_HREF}/licenses`, label: t("common:licenses"), icon: IconScale },
];

const authItems = (): SystemNavItem[] => [
  { href: `${ROOT_HREF}/users`, label: t("common:users"), icon: IconUsers },
];

type SystemGroupProps = {
  pathname: string;
  expanded?: boolean;
  onToggle?: () => void;
};

/** null user (disabled/synthetic single-user mode) counts as admin for gating. */
function useIsAdmin(): boolean {
  const role = useAppStore((s) => s.auth.user?.role);
  return role === undefined || role === "admin";
}

export function SystemGroup({ pathname, expanded, onToggle }: SystemGroupProps) {
  const { t } = useTranslation();
  const authEnabled = useFeature("auth");
  const isAdmin = useIsAdmin();
  const items = authEnabled && isAdmin ? [...baseItems(), ...authItems()] : baseItems();

  return (
    <SettingsGroup
      label={t("common:system")}
      icon={IconServerCog}
      href={DEFAULT_HREF}
      isActive={pathname.startsWith(ROOT_HREF)}
      expanded={expanded}
      onToggle={onToggle}
    >
      {items.map(({ href, label, icon }) => (
        <SettingsLeaf
          key={href}
          href={href}
          label={label}
          icon={icon}
          isActive={pathname === href}
          depth={1}
        />
      ))}
    </SettingsGroup>
  );
}
