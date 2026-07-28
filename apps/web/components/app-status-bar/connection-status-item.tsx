"use client";
import { t } from "@/lib/i18n";
import { useTranslation } from "react-i18next";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@kandev/ui/lib/utils";
import { useAppStore } from "@/components/state-provider";
import type { ConnectionStatus } from "@/lib/types/connection";

export type ConnectionStatusDetails = {
  label: string;
  description: string;
  dotClass: string;
  animate: boolean;
};

export function connectionStatusDetails(
  status: ConnectionStatus,
  error: string | null,
): ConnectionStatusDetails {
  switch (status) {
    case "connected":
      return {
        label: t("common:connected"),
        description: t("statusBar:connectedToKandev"),
        dotClass: "bg-success",
        animate: false,
      };
    case "connecting":
      return {
        label: t("statusBar:connecting"),
        description: t("statusBar:connectingToKandev"),
        dotClass: "bg-muted-foreground",
        animate: true,
      };
    case "reconnecting":
      return {
        label: t("statusBar:reconnecting"),
        description: t("statusBar:reconnectingToKandev"),
        dotClass: "bg-amber-500",
        animate: true,
      };
    case "error":
      return {
        label: t("common:connectionError"),
        description: error
          ? t("statusBar:connectionError", { error })
          : t("common:connectionError"),
        dotClass: "bg-destructive",
        animate: false,
      };
    case "disconnected":
      return {
        label: t("statusBar:offline"),
        description: t("statusBar:connectionUnavailable"),
        dotClass: "bg-muted-foreground/50",
        animate: false,
      };
  }
}

export function ConnectionStatusItem({ presentation }: { presentation: "bar" | "mobile-drawer" }) {
  const { t } = useTranslation();
  const status = useAppStore((state) => state.connection.status);
  const error = useAppStore((state) => state.connection.error);
  const details = connectionStatusDetails(status, error);

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "inline-flex h-full items-center leading-none",
            presentation === "bar" ? "w-5 justify-center" : "min-h-11 gap-3 px-1 text-sm",
          )}
          role="status"
          aria-label={details.description}
          data-testid="app-status-connection"
        >
          <span
            className={`size-1.5 shrink-0 rounded-full ${details.dotClass} ${details.animate ? "animate-pulse" : ""}`}
            aria-hidden="true"
          />
          {presentation === "bar" ? (
            <span className="sr-only">{details.label}</span>
          ) : (
            <span className="flex min-w-0 flex-col gap-0.5">
              <span className="text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">
                {t("statusBar:connection")}
              </span>
              <span className="text-sm font-medium text-foreground">{details.description}</span>
            </span>
          )}
        </span>
      </TooltipTrigger>
      <TooltipContent>{details.description}</TooltipContent>
    </Tooltip>
  );
}
