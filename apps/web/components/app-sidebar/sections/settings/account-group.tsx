"use client";

import { IconKey, IconShieldLock, IconUserCircle } from "@tabler/icons-react";
import { t } from "@lingui/core/macro";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";

const ROOT_HREF = "/settings/account";
const DEFAULT_HREF = `${ROOT_HREF}/security`;

const items = () => [
  { href: `${ROOT_HREF}/security`, label: t`Profile & Password`, icon: IconShieldLock },
  { href: `${ROOT_HREF}/tokens`, label: t`API Tokens`, icon: IconKey },
];

type AccountGroupProps = {
  pathname: string;
  expanded?: boolean;
  onToggle?: () => void;
};

export function AccountGroup({ pathname, expanded, onToggle }: AccountGroupProps) {
  return (
    <SettingsGroup
      label={t`Account`}
      icon={IconUserCircle}
      href={DEFAULT_HREF}
      isActive={pathname.startsWith(ROOT_HREF)}
      expanded={expanded}
      onToggle={onToggle}
    >
      {items().map(({ href, label, icon }) => (
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
