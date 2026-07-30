"use client";

import { IconRobot } from "@tabler/icons-react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { WorkflowStep } from "@/lib/types/http";
import { useHealthyAgentProfiles } from "@/hooks/domains/settings/use-healthy-agent-profiles";
import { HelpTip } from "./workflow-pipeline-editor-helpers";
import { isWorkflowStepValueDirty } from "./workflow-dirty-state";

// StepAgentProfileSelect overrides the agent profile used while a workflow step
// is active.
export function StepAgentProfileSelect({
  step,
  savedStep,
  onUpdate,
  readOnly,
}: {
  step: WorkflowStep;
  savedStep?: WorkflowStep;
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  readOnly: boolean;
}) {
  const healthyProfiles = useHealthyAgentProfiles(step.agent_profile_id);

  return (
    <div className="flex w-full min-w-0 items-center gap-1.5 sm:w-auto">
      <Select
        value={step.agent_profile_id || "none"}
        onValueChange={(value) => {
          if (readOnly) return;
          onUpdate({ agent_profile_id: value === "none" ? "" : value });
        }}
        disabled={readOnly}
      >
        <SelectTrigger
          className="h-8 w-full min-w-0 cursor-pointer sm:w-[220px]"
          data-testid="step-agent-profile-select"
          data-settings-dirty={isWorkflowStepValueDirty(
            step,
            savedStep,
            (item) => item.agent_profile_id ?? "",
          )}
        >
          <IconRobot className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          <SelectValue placeholder="No profile override" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="none" className="cursor-pointer">
            No profile override
          </SelectItem>
          {healthyProfiles.map((p) => (
            <SelectItem key={p.id} value={p.id} className="cursor-pointer">
              {p.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <HelpTip text="Override the agent profile for this step. A different profile creates a new session with fresh context when entering this step." />
    </div>
  );
}
