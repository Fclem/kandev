"use client";
import { useState } from "react";
import { IconRefresh } from "@tabler/icons-react";
import { t } from "@/lib/i18n";
import { useTranslation } from "react-i18next";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

type BranchRefreshButtonProps = {
  onRefresh: () => void;
  refreshing?: boolean;
  fetchedAt?: string;
  fetchError?: string;
  label?: string;
  testId?: string;
  touchTarget?: boolean;
};

export function BranchRefreshButton({
  onRefresh,
  refreshing,
  fetchedAt,
  fetchError,
  label = "branches",
  testId = "branch-refresh-button",
  touchTarget = false,
}: BranchRefreshButtonProps) {
  // Controlled open so the tooltip only reacts to hover, not focus.
  // Radix Popover auto-focuses the first focusable child when it opens, which
  // would otherwise trigger this tooltip the moment the dropdown is opened.
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const hasError = Boolean(fetchError);
  const tooltip = formatRefreshTooltip(label, fetchedAt, refreshing, fetchError);
  return (
    <Tooltip open={open} onOpenChange={setOpen}>
      <TooltipTrigger asChild>
        <button
          type="button"
          aria-label={t("common:refresh", { label })}
          data-testid={testId}
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
            onRefresh();
          }}
          onMouseEnter={() => setOpen(true)}
          onMouseLeave={() => setOpen(false)}
          disabled={refreshing}
          className={`inline-flex ${touchTarget ? "h-12 w-12" : "h-6 w-6"} items-center justify-center rounded-md hover:bg-muted/40 ${
            hasError
              ? "text-amber-500 hover:text-amber-600"
              : "text-muted-foreground hover:text-foreground"
          } ${refreshing ? "cursor-not-allowed opacity-50" : "cursor-pointer"}`}
        >
          <IconRefresh className={`h-3.5 w-3.5 ${refreshing ? "animate-spin" : ""}`} />
        </button>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

function formatRefreshTooltip(
  label: string,
  fetchedAt: string | undefined,
  refreshing: boolean | undefined,
  fetchError: string | undefined,
) {
  if (refreshing) return t("common:refreshing", { label });
  if (fetchError) return t("common:lastRefreshFailed", { fetchError });
  if (!fetchedAt) return initialRefreshTooltip(label);
  const date = new Date(fetchedAt);
  if (Number.isNaN(date.getTime())) return initialRefreshTooltip(label);
  const fetchedTime = date.toLocaleTimeString();
  return t("common:refreshLastFetched", { label, fetchedTime });
}

function initialRefreshTooltip(label: string) {
  // "branches" is a logic sentinel for the default label, not display copy.
  return label === "branches"
    ? t("common:refreshBranchesGitFetch")
    : t("common:refresh", { label });
}
