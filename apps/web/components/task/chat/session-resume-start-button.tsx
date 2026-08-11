"use client";

import { useCallback, useState } from "react";
import { IconPlayerPlay } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildResumeRequest } from "@/lib/services/session-launch-helpers";

/**
 * "Start agent" affordance for a resume-skipped session (prevent-auto-start-on-
 * open preference): the session exists with history (its agent process is not
 * running) and the open-time auto-resume was skipped, so the session stayed
 * stopped. Clicking resumes the agent.
 *
 * Rendered in the message-list footer when the session has messages (the
 * task-description start button only renders for empty sessions). The resume
 * is NOT gated by the preference — it is the explicit manual action.
 */
export function SessionResumeStartButton({ sessionId }: { sessionId: string }) {
  const { t } = useTranslation();
  const [isStarting, setIsStarting] = useState(false);
  const taskId = useAppStore((state) => state.taskSessions.items[sessionId]?.task_id ?? null);
  const setResumeSkipped = useAppStore((state) => state.setResumeSkipped);

  const handleStart = useCallback(async () => {
    if (!taskId) return;
    setIsStarting(true);
    try {
      const { request } = buildResumeRequest(taskId, sessionId);
      const response = await launchSession(request);
      // Only confirmed RUNNING clears the resume-skipped marker: a resume
      // response commonly reports STARTING (launch accepted, agent starting),
      // and the WS RUNNING transition clears it later. Failed launches keep
      // the button as a retry affordance.
      if (response.state === "RUNNING") {
        setResumeSkipped(sessionId, false);
      }
    } catch (error) {
      console.error("Failed to resume agent:", error);
    } finally {
      setIsStarting(false);
    }
  }, [taskId, sessionId, setResumeSkipped]);

  return (
    <div className="flex justify-end mt-1.5">
      <Button
        size="sm"
        variant="default"
        className="cursor-pointer gap-1.5"
        onClick={handleStart}
        disabled={isStarting}
        data-testid="task-description-start-button"
      >
        <IconPlayerPlay className="h-3.5 w-3.5" />
        {isStarting ? t("task:startingEllipsis") : t("task:startAgent")}
      </Button>
    </div>
  );
}
