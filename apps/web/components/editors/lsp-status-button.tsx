"use client";

import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  IconLoader2,
  IconPlugConnected,
  IconPlugOff,
  IconAlertTriangle,
} from "@tabler/icons-react";
import type { LspStatus } from "@/lib/lsp/lsp-client-manager";
import { useTranslation } from "react-i18next";

type StaticConfig = { tooltipKey: string; clickable: boolean };

const ICON_CLS = "h-3.5 w-3.5";

const ICONS: Record<string, React.ReactNode> = {
  disabled: <IconPlugOff className={`${ICON_CLS} text-muted-foreground/50`} />,
  connecting: <IconLoader2 className={`${ICON_CLS} animate-spin text-muted-foreground`} />,
  installing: <IconLoader2 className={`${ICON_CLS} animate-spin text-amber-500`} />,
  starting: <IconLoader2 className={`${ICON_CLS} animate-spin text-blue-500`} />,
  ready: <IconPlugConnected className={`${ICON_CLS} text-emerald-500`} />,
  stopping: <IconLoader2 className={`${ICON_CLS} animate-spin text-muted-foreground`} />,
  unavailable: <IconPlugOff className={`${ICON_CLS} text-muted-foreground`} />,
  error: <IconAlertTriangle className={`${ICON_CLS} text-yellow-500`} />,
};

const STATIC_CONFIGS: Record<string, StaticConfig> = {
  disabled: { tooltipKey: "editors:lspOffClickToStart", clickable: true },
  connecting: { tooltipKey: "editors:lspConnecting", clickable: true },
  installing: { tooltipKey: "editors:lspInstallingLanguageServer", clickable: false },
  starting: { tooltipKey: "editors:lspStartingLanguageServer", clickable: true },
  ready: { tooltipKey: "editors:lspConnectedClickToStop", clickable: true },
  stopping: { tooltipKey: "editors:lspStopping", clickable: false },
};

function getConfig(
  status: LspStatus,
  t: (key: string, vars?: Record<string, string>) => string,
): { icon: React.ReactNode; tooltip: string; clickable: boolean } | null {
  const icon = ICONS[status.state];
  if (!icon) return null;

  const sc = STATIC_CONFIGS[status.state];
  if (sc) return { icon, tooltip: t(sc.tooltipKey), clickable: sc.clickable };

  // Dynamic tooltip for unavailable/error states
  const reason = "reason" in status ? status.reason : null;
  if (status.state === "unavailable")
    return {
      icon,
      tooltip: t("editors:lspStatusReason", { reason: reason ?? t("editors:unavailable") }),
      clickable: true,
    };
  if (status.state === "error")
    return {
      icon,
      tooltip: t("editors:lspStatusReason", { reason: reason ?? t("editors:error") }),
      clickable: true,
    };

  return null;
}

export function LspStatusButton({
  status,
  lspLanguage,
  onToggle,
}: {
  status: LspStatus;
  lspLanguage: string | null;
  onToggle: () => void;
}) {
  const { t } = useTranslation();
  if (!lspLanguage) return null;

  const c = getConfig(status, t);
  if (!c) return null;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="h-6 w-6 p-0 cursor-pointer"
          onClick={c.clickable ? onToggle : undefined}
          disabled={!c.clickable}
        >
          {c.icon}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{c.tooltip}</TooltipContent>
    </Tooltip>
  );
}
