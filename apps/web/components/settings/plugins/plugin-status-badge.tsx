"use client";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import type { PluginStatus } from "@/lib/types/plugins";

const STATUS_LABEL: Record<PluginStatus, string> = {
  active: "common:active",
  error: "settings:error",
  disabled: "settings:disabled",
  registered: "settings:registered",
  uninstalled: "settings:uninstalled",
};

// green=active, red=error, gray=disabled, amber=registered, per task-20 acceptance.
const STATUS_CLASS: Record<PluginStatus, string> = {
  active: "border-green-500/40 bg-green-500/10 text-green-600 dark:text-green-400",
  error: "border-red-500/40 bg-red-500/10 text-red-600 dark:text-red-400",
  disabled: "border-border bg-muted text-muted-foreground",
  registered: "border-amber-500/40 bg-amber-500/10 text-amber-600 dark:text-amber-400",
  uninstalled: "border-border bg-muted text-muted-foreground",
};

export function PluginStatusBadge({ status }: { status: PluginStatus }) {
  const { t } = useTranslation();
  return (
    <Badge variant="outline" className={STATUS_CLASS[status]}>
      {t(STATUS_LABEL[status])}
    </Badge>
  );
}
