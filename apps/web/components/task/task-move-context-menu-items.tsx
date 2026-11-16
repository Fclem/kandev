"use client";

import { IconBuildingWarehouse, IconLogicBuffer, IconArrowRight } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import {
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuSub,
  ContextMenuSubTrigger,
} from "@kandev/ui/context-menu";
import type { WorkflowMoveEntryOptions } from "@/lib/api/domains/kanban-api";
import type { WorkflowStepProgress } from "@/hooks/domains/kanban/use-workflow-step-progress";
import { TaskContextMenuSubContent as ContextMenuSubContent } from "./task-context-menu-sub-content";
import {
  MoveToCurrentWorkflowSubmenu,
  StepMenuItem,
  TASK_MOVE_MENU_WIDTH,
  taskMoveOptions,
  type TaskMoveStep,
  type TaskMoveWorkflow,
} from "./task-move-context-menu";

type TaskMoveContextMenuItemsProps = {
  currentWorkflowId?: string | null;
  currentStepId?: string | null;
  workflows: TaskMoveWorkflow[];
  stepsByWorkflowId: Record<string, TaskMoveStep[]>;
  disabled?: boolean;
  showSeparator?: boolean;
  onMoveToStep?: (stepId: string) => void;
  onMoveToStepWithOptions?: (stepId: string) => void;
  onSubmitWithOptions?: (
    stepId: string,
    entryOptions: WorkflowMoveEntryOptions | undefined,
  ) => Promise<boolean>;
  isMoving?: boolean;
  isBulkSelection?: boolean;
  progressByStepId?: Readonly<Record<string, WorkflowStepProgress>>;
  agentLabelsByProfileId?: Readonly<Record<string, string>>;
  onChangeWorkflow?: () => void;
  /** Opens the single-task workspace transfer form; ignored for bulk selection. */
  onChangeWorkspace?: () => void;
  onSendToWorkflow?: (workflowId: string, stepId: string) => void;
};

function WorkflowTargetItem({
  workflow,
  steps,
  disabled,
  onSendToWorkflow,
}: {
  workflow: TaskMoveWorkflow;
  steps: TaskMoveStep[];
  disabled?: boolean;
  onSendToWorkflow?: (workflowId: string, stepId: string) => void;
}) {
  const { t } = useTranslation();
  if (steps.length === 0 || !onSendToWorkflow) {
    return (
      <ContextMenuItem
        data-testid={`task-context-workflow-${workflow.id}`}
        disabled
        aria-disabled="true"
      >
        <span className="min-w-0 flex-1 whitespace-normal break-words">{workflow.name}</span>
        <span data-testid="task-context-disabled-reason" className="ml-2 text-[10px]">
          {t("task:noSteps")}
        </span>
      </ContextMenuItem>
    );
  }

  return (
    <ContextMenuSub>
      <ContextMenuSubTrigger
        className="[@media(pointer:coarse)]:min-h-11"
        data-testid={`task-context-workflow-${workflow.id}`}
        disabled={disabled}
      >
        <span className="min-w-0 whitespace-normal break-words">{workflow.name}</span>
      </ContextMenuSubTrigger>
      <ContextMenuSubContent className={TASK_MOVE_MENU_WIDTH}>
        {steps.map((step) => (
          <StepMenuItem
            key={step.id}
            step={step}
            onSelect={(stepId) => onSendToWorkflow(workflow.id, stepId)}
          />
        ))}
      </ContextMenuSubContent>
    </ContextMenuSub>
  );
}

function SendToWorkflowSubmenu({
  workflows,
  stepsByWorkflowId,
  disabled,
  onSendToWorkflow,
}: {
  workflows: TaskMoveWorkflow[];
  stepsByWorkflowId: Record<string, TaskMoveStep[]>;
  disabled?: boolean;
  onSendToWorkflow?: (workflowId: string, stepId: string) => void;
}) {
  const { t } = useTranslation();
  if (!onSendToWorkflow || workflows.length === 0) return null;
  return (
    <ContextMenuSub>
      <ContextMenuSubTrigger
        className="[@media(pointer:coarse)]:min-h-11"
        data-testid="task-context-change-workflow-selection"
        disabled={disabled}
      >
        <IconLogicBuffer className="mr-2 h-4 w-4" />
        {t("task:changeWorkflowForSelectedTasks")}
      </ContextMenuSubTrigger>
      <ContextMenuSubContent className="w-56">
        {workflows.map((workflow) => (
          <WorkflowTargetItem
            key={workflow.id}
            workflow={workflow}
            steps={stepsByWorkflowId[workflow.id] ?? []}
            disabled={disabled}
            onSendToWorkflow={onSendToWorkflow}
          />
        ))}
      </ContextMenuSubContent>
    </ContextMenuSub>
  );
}

function moveAvailability(props: TaskMoveContextMenuItemsProps) {
  const options = taskMoveOptions(
    props.currentWorkflowId,
    props.workflows,
    props.stepsByWorkflowId,
  );
  const canMoveStep = Boolean(
    (props.onMoveToStep || props.onMoveToStepWithOptions || props.onSubmitWithOptions) &&
    options.canMove,
  );
  const crossWorkflow = props.isBulkSelection ? props.onSendToWorkflow : props.onChangeWorkflow;
  return { ...options, canMoveStep, canChangeWorkflow: Boolean(options.canSend && crossWorkflow) };
}

export function TaskMoveContextMenuItems(props: TaskMoveContextMenuItemsProps) {
  const { stepsByWorkflowId, showSeparator = true } = props;
  const { currentSteps, targets, canMoveStep, canChangeWorkflow } = moveAvailability(props);
  const canChangeWorkspace = !props.isBulkSelection && Boolean(props.onChangeWorkspace);
  if (!canMoveStep && !canChangeWorkflow && !canChangeWorkspace) return null;
  const separator = showSeparator ? <ContextMenuSeparator /> : null;
  if (props.isBulkSelection) {
    return (
      <>
        {separator}
        <StepSubmenu {...props} steps={currentSteps} />
        {canChangeWorkflow && (
          <SendToWorkflowSubmenu
            workflows={targets}
            stepsByWorkflowId={stepsByWorkflowId}
            disabled={props.disabled}
            onSendToWorkflow={props.onSendToWorkflow}
          />
        )}
      </>
    );
  }
  return (
    <>
      {separator}
      <SingleTaskMoveSubmenu
        {...props}
        steps={currentSteps}
        hasWorkflowChange={canChangeWorkflow}
      />
    </>
  );
}

function StepSubmenu(
  props: TaskMoveContextMenuItemsProps & { steps: TaskMoveStep[]; nested?: boolean },
) {
  return (
    <MoveToCurrentWorkflowSubmenu
      steps={props.steps}
      currentStepId={props.currentStepId}
      disabled={props.disabled}
      onMoveToStep={props.onMoveToStep}
      onMoveToStepWithOptions={props.onMoveToStepWithOptions}
      onSubmitWithOptions={props.onSubmitWithOptions}
      isMoving={props.isMoving}
      progressByStepId={props.progressByStepId}
      agentLabelsByProfileId={props.agentLabelsByProfileId}
      nested={props.nested}
    />
  );
}

/**
 * Single-task movement: one `Move to` group with a workflow step, a different
 * workflow, and a different workspace. The workspace choice is offered even
 * when the current workflow has one step or no other workflow exists.
 */
function SingleTaskMoveSubmenu(
  props: TaskMoveContextMenuItemsProps & { steps: TaskMoveStep[]; hasWorkflowChange: boolean },
) {
  const { t } = useTranslation();
  return (
    <ContextMenuSub>
      <ContextMenuSubTrigger
        className="[@media(pointer:coarse)]:min-h-11"
        data-testid="task-context-move-to"
        disabled={props.disabled}
      >
        <IconArrowRight className="mr-2 h-4 w-4" />
        {t("task:moveTo")}
      </ContextMenuSubTrigger>
      <ContextMenuSubContent className="w-56">
        <StepSubmenu {...props} nested />
        {props.hasWorkflowChange && (
          <ContextMenuItem
            data-testid="task-context-change-workflow"
            className="[@media(pointer:coarse)]:min-h-11"
            disabled={props.disabled}
            onSelect={props.onChangeWorkflow}
          >
            <IconLogicBuffer className="mr-2 h-4 w-4" />
            {t("task:moveToDifferentWorkflow")}
          </ContextMenuItem>
        )}
        {props.onChangeWorkspace && (
          <ContextMenuItem
            data-testid="task-context-change-workspace"
            className="[@media(pointer:coarse)]:min-h-11"
            disabled={props.disabled}
            onSelect={props.onChangeWorkspace}
          >
            <IconBuildingWarehouse className="mr-2 h-4 w-4" />
            {t("task:moveToDifferentWorkspace")}
          </ContextMenuItem>
        )}
      </ContextMenuSubContent>
    </ContextMenuSub>
  );
}
