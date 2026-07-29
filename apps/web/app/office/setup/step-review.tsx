"use client";

import { Badge } from "@kandev/ui/badge";
import { Card } from "@kandev/ui/card";
import { useAppStore } from "@/components/state-provider";
import { useTranslation } from "react-i18next";

type StepReviewProps = {
  workspaceName: string;
  taskPrefix: string;
  agentName: string;
  agentProfileLabel: string;
  executorPreference: string;
  taskTitle: string;
};

// Fallback used only when meta has not been hydrated yet (graceful degradation).
const FALLBACK_EXECUTOR_LABELS: Record<string, string> = {
  local_pc: "Local (standalone)",
  local_docker: "Local Docker",
  sprites: "Sprites (remote sandbox)",
};

export function StepReview({
  workspaceName,
  taskPrefix,
  agentName,
  agentProfileLabel,
  executorPreference,
  taskTitle,
}: StepReviewProps) {
  const { t } = useTranslation();
  const meta = useAppStore((s) => s.office.meta);

  const executorLabel =
    meta?.executorTypes.find((e) => e.id === executorPreference)?.label ??
    FALLBACK_EXECUTOR_LABELS[executorPreference] ??
    executorPreference;

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">{t("office:reviewAndLaunch")}</h2>
        <p className="text-sm text-muted-foreground mt-1">
          {t("office:confirmTheDetailsBelowEverythingCan")}
        </p>
      </div>
      <Card className="divide-y divide-border">
        <ReviewRow label="Workspace" value={workspaceName || "Default Workspace"}>
          <Badge variant="secondary" className="ml-2">
            {taskPrefix || "KAN"}
          </Badge>
        </ReviewRow>
        <ReviewRow label="Coordinator agent" value={agentName || "CEO"}>
          {agentProfileLabel && (
            <span className="text-xs text-muted-foreground ml-2">({agentProfileLabel})</span>
          )}
        </ReviewRow>
        <ReviewRow label="Executor" value={executorLabel} />
        <ReviewRow label="First task" value={taskTitle || "No initial task"} />
      </Card>
    </div>
  );
}

function ReviewRow({
  label,
  value,
  children,
}: {
  label: string;
  value: string;
  children?: React.ReactNode;
}) {
  return (
    <div className="flex items-center justify-between px-4 py-3">
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className="text-sm font-medium flex items-center">
        {value}
        {children}
      </span>
    </div>
  );
}
