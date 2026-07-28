"use client";
import { IconKey, IconShieldLock, IconUserCircle } from "@tabler/icons-react";
import { t } from "@/lib/i18n";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";
import { useTranslation } from "react-i18next";

const ROOT_HREF = "/settings/account";
const DEFAULT_HREF = `${ROOT_HREF}/security`;

const items = () => [
  { href: `${ROOT_HREF}/security`, label: t("sidebar:profilePassword"), icon: IconShieldLock },
  { href: `${ROOT_HREF}/tokens`, label: t("sidebar:apiTokens"), icon: IconKey },
];

type AccountGroupProps = {
  pathname: string;
  expanded?: boolean;
  onToggle?: () => void;
};

export function AccountGroup({ pathname, expanded, onToggle }: AccountGroupProps) {
  const { t } = useTranslation();
  return (
    <SettingsGroup
      label={t("sidebar:account")}
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
