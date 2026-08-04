import { useTranslation } from "react-i18next";
import { IconAlertCircle } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { TaskSessionState, TaskState } from "@/lib/types/http";

// Terminal states keep their existing icons even when an interrupted marker
// lingers (e.g. manually seeded metadata); the red interruption affordance only
// replaces the idle/done readings of a task that has not been resumed.
export function isTerminalInterruptedState(
  state?: TaskState,
  sessionState?: TaskSessionState,
): boolean {
  return (
    state === "COMPLETED" ||
    state === "FAILED" ||
    state === "CANCELLED" ||
    sessionState === "COMPLETED" ||
    sessionState === "FAILED" ||
    sessionState === "CANCELLED"
  );
}

/** Red alert icon for a task whose session was mid-turn when the backend died. */
export function InterruptedTaskIcon() {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          aria-label={t("common:task.interruptedByRestart")}
          tabIndex={0}
          className="mt-[1px] flex shrink-0 rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500 focus-visible:ring-offset-1"
        >
          <IconAlertCircle
            aria-hidden="true"
            data-testid="task-state-interrupted"
            className="h-3.5 w-3.5 shrink-0 text-red-500"
          />
        </span>
      </TooltipTrigger>
      <TooltipContent side="right">{t("common:task.interruptedByRestart")}</TooltipContent>
    </Tooltip>
  );
}
