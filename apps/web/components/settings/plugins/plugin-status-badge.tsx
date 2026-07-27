"use client";

import { useLingui } from "@lingui/react/macro";
import { msg } from "@lingui/core/macro";
import type { MessageDescriptor } from "@lingui/core";
import { Badge } from "@kandev/ui/badge";
import type { PluginStatus } from "@/lib/types/plugins";

const STATUS_LABEL: Record<PluginStatus, MessageDescriptor> = {
  active: msg`Active`,
  error: msg`Error`,
  disabled: msg`Disabled`,
  registered: msg`Registered`,
  uninstalled: msg`Uninstalled`,
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
  const { t } = useLingui();
  return (
    <Badge variant="outline" className={STATUS_CLASS[status]}>
      {t(STATUS_LABEL[status])}
    </Badge>
  );
}
