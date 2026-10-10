"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { useAppStore } from "@/components/state-provider";
import { formatDateTime } from "@/lib/i18n/formats";

const maximumTimerDelay = 2_147_483_647;

export function ProfileLimitBadge({ profileId }: { profileId: string }) {
  const limit = useAppStore((state) => state.agentProfileLimits.byProfileId[profileId]);
  const { t } = useTranslation();
  const [expiryCheck, setExpiryCheck] = useState(0);
  const expiry = limit ? Date.parse(limit.until) : 0;
  useEffect(() => {
    const remaining = expiry - Date.now();
    if (remaining <= 0) return;
    const timer = window.setTimeout(
      () => setExpiryCheck((current) => current + 1),
      Math.min(remaining, maximumTimerDelay),
    );
    return () => window.clearTimeout(timer);
  }, [expiry, expiryCheck]);
  if (!limit || expiry <= Date.now()) return null;
  return (
    <Badge
      variant="outline"
      className="h-auto min-h-5 max-w-full min-w-0 whitespace-pre-wrap break-words border-amber-500/40 text-left text-amber-700 dark:text-amber-400"
      data-testid={`profile-limit-${profileId}`}
    >
      {t("agents:limitedUntil", { time: formatDateTime(limit.until) })}
    </Badge>
  );
}
