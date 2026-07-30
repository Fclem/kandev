"use client";
import { Trans, useTranslation } from "react-i18next";

import { cn } from "@/lib/utils";

type LiveAgentIndicatorProps = {
  count: number;
  className?: string;
};

/**
 * Pulsing emerald dot + "{N} live" badge shown next to an agent in the
 * sidebar when one or more of its task sessions are actively working
 * (RUNNING / WAITING_FOR_INPUT). Returns null when count is 0 so callers
 * can fall back to a static status dot without layout shift.
 *
 * Styling matches the existing live indicators in `agent-cards-panel`
 * and `execution-indicator` (emerald-400/500, animate-ping halo).
 */
export function LiveAgentIndicator({ count, className }: LiveAgentIndicatorProps) {
  const { t } = useTranslation();
  if (count <= 0) return null;
  return (
    <div
      className={cn("flex items-center gap-1.5", className)}
      aria-label={t("office:activeSession", { count })}
    >
      <span className="relative flex h-2 w-2 shrink-0">
        <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75" />
        <span className="relative inline-flex h-2 w-2 rounded-full bg-emerald-500" />
      </span>
      <span className="text-[11px] font-medium text-emerald-500">
        <Trans i18nKey="office:live" values={{ count }}>
          {count} live
        </Trans>
      </span>
    </div>
  );
}
