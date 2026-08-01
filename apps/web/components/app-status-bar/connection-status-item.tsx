"use client";
import { t } from "@/lib/i18n";
import { useTranslation } from "react-i18next";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@kandev/ui/lib/utils";
import { useAppStore } from "@/components/state-provider";
import type { ConnectionIssueSeverity, ConnectionStatus } from "@/lib/types/connection";

export type ConnectionStatusDetails = {
  label: string;
  description: string;
  dotClass: string;
  animate: boolean;
};

export type ConnectionIssueDetails = {
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

export function connectionIssueDetails(
  severity: Exclude<ConnectionIssueSeverity, "none">,
): ConnectionIssueDetails {
  switch (severity) {
    case "unstable":
      return {
        label: t("statusBar:connectionUnstable"),
        description: t("statusBar:connectionUnstableReconnectingToKandev"),
        dotClass: "bg-amber-500",
        animate: false,
      };
    case "lost":
      return {
        label: t("statusBar:connectionLost"),
        description: t("statusBar:connectionLostForAtLeast10"),
        dotClass: "bg-destructive",
        animate: false,
      };
  }
}

export function ConnectionStatusItem({ presentation }: { presentation: "bar" | "mobile-drawer" }) {
  const { t } = useTranslation();
  const status = useAppStore((state) => state.connection.status);
  const error = useAppStore((state) => state.connection.error);
  const severity = useAppStore((state) => state.connection.issueSeverity);
  if (severity === "none" && presentation === "bar") return null;
  const issueActive = severity !== "none";
  const details = issueActive
    ? connectionIssueDetails(severity)
    : connectionStatusDetails(status, error);

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "inline-flex items-center leading-none",
            presentation === "bar" ? "h-full w-5 justify-center" : "min-h-11 gap-3 px-1 text-sm",
          )}
          role="status"
          aria-label={details.description}
          tabIndex={presentation === "bar" ? 0 : undefined}
          data-testid="app-status-connection"
          data-connection-severity={issueActive ? severity : undefined}
        >
          <span
            className={`size-1.5 shrink-0 rounded-full ${details.dotClass} ${issueActive || details.animate ? "animate-pulse" : ""}`}
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
