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
import { t } from "@lingui/core/macro";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";

const ROOT_HREF = "/settings/system";
const DEFAULT_HREF = `${ROOT_HREF}/status`;

type SystemNavItem = { href: string; label: string; icon: TablerIcon };

const baseItems = (): SystemNavItem[] => [
  { href: `${ROOT_HREF}/status`, label: t`Status`, icon: IconActivity },
  { href: `${ROOT_HREF}/feature-toggles`, label: t`Feature Toggles`, icon: IconFlask },
  { href: `${ROOT_HREF}/database`, label: t`Database`, icon: IconDatabase },
  { href: `${ROOT_HREF}/backups`, label: t`Backups`, icon: IconArchive },
  { href: `${ROOT_HREF}/storage`, label: t`Storage`, icon: IconTrash },
  { href: `${ROOT_HREF}/logs`, label: t`Logs`, icon: IconFileText },
  { href: `${ROOT_HREF}/updates`, label: t`Updates`, icon: IconRefresh },
  { href: `${ROOT_HREF}/about`, label: t`About`, icon: IconInfoCircle },
  { href: `${ROOT_HREF}/licenses`, label: t`Licenses`, icon: IconScale },
];

const authItems = (): SystemNavItem[] => [
  { href: `${ROOT_HREF}/users`, label: t`Users`, icon: IconUsers },
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
  const authEnabled = useFeature("auth");
  const isAdmin = useIsAdmin();
  const items = authEnabled && isAdmin ? [...baseItems(), ...authItems()] : baseItems();

  return (
    <SettingsGroup
      label={t`System`}
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
