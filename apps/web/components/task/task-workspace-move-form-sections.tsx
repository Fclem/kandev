"use client";

import type { ReactNode } from "react";
import { IconLoader2, IconRefresh } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Combobox, type ComboboxOption } from "@/components/combobox";
import AppLink from "@/components/routing/app-link";
import type { TaskWorkspaceMoveState } from "@/hooks/domains/kanban/use-task-workspace-move";
import type { TransferRejectionCode } from "@/lib/kanban/task-workspace-move-operation";
import { linkToTask } from "@/lib/links";
import { parseWorkflowStepColor } from "@/lib/task-color-presentation";
import type { WorkflowStepDTO } from "@/lib/types/http";
import { cn } from "@/lib/utils";

type SectionProps = { state: TaskWorkspaceMoveState; isTouchSurface: boolean };

const REJECTION_KEYS: Record<TransferRejectionCode, string> = {
  source_changed: "task:workspaceMoveErrorSourceChanged",
  destination_unavailable: "task:workspaceMoveErrorDestinationUnavailable",
  step_incompatible: "task:workspaceMoveErrorStepIncompatible",
  destination_full: "task:workspaceMoveErrorDestinationFull",
  workspace_relation_unmapped: "task:workspaceMoveErrorRelationUnmapped",
  profile_mapping_required: "task:workspaceMoveErrorProfileMapping",
  pending_step_unmapped: "task:workspaceMoveErrorPendingStep",
  cleanup_active: "task:workspaceMoveErrorCleanupActive",
  idempotency_conflict: "task:workspaceMoveErrorIdempotency",
  transfer_unsupported: "task:workspaceMoveErrorUnsupported",
  relations_busy: "task:workspaceMoveErrorRelationsBusy",
  forbidden: "task:workspaceMoveErrorForbidden",
  unavailable: "task:workspaceMoveErrorUnavailable",
  invalid: "task:workspaceMoveErrorInvalid",
};

function controlClass(isTouchSurface: boolean) {
  return isTouchSurface ? "h-11 w-full" : "h-7 w-full";
}

function Status({ label, testId }: { label: string; testId?: string }) {
  return (
    <div
      role="status"
      data-testid={testId}
      className="flex min-h-9 items-center gap-2 text-sm text-muted-foreground"
    >
      <IconLoader2 className="size-4 animate-spin" aria-hidden="true" />
      {label}
    </div>
  );
}

function LoadFailed({
  label,
  onRetry,
  isTouchSurface,
  testId,
}: {
  label: string;
  onRetry: () => void;
  isTouchSurface: boolean;
  testId: string;
}) {
  const { t } = useTranslation();
  return (
    <div
      role="alert"
      data-testid={testId}
      className="grid justify-items-start gap-2 rounded-md border border-destructive/40 p-3 text-sm"
    >
      <span>{label}</span>
      <Button
        type="button"
        variant="outline"
        size="sm"
        className={cn("gap-2", isTouchSurface && "min-h-11")}
        onClick={onRetry}
      >
        <IconRefresh className="size-3.5" aria-hidden="true" />
        {t("task:workspaceMoveRetry")}
      </Button>
    </div>
  );
}

export function WorkspaceMoveCurrentPlacement({ state, isTouchSurface }: SectionProps) {
  const { t } = useTranslation();
  if (state.taskStatus === "loading" && !state.task)
    return <Status label={t("task:workspaceMoveLoadingTask")} />;
  if (state.taskStatus === "error" && !state.task)
    return (
      <LoadFailed
        label={t("task:workspaceMoveLoadTaskFailed")}
        onRetry={() => void state.retryTask()}
        isTouchSurface={isTouchSurface}
        testId="workspace-move-task-error"
      />
    );
  if (!state.task) return null;
  return (
    <section
      aria-label={t("task:workspaceMoveCurrentLabel")}
      data-testid="workspace-move-current"
      className="grid min-w-0 gap-1 rounded-md bg-muted/50 p-3"
    >
      <span className="break-words text-sm font-medium">{state.task.title}</span>
      <span className="break-words text-xs text-muted-foreground">
        {t("task:workspaceMoveFrom", {
          workspace: state.sourceWorkspaceName ?? state.task.workspace_id,
          workflow: state.sourceWorkflowName ?? state.task.workflow_id,
          step: state.sourceStepName ?? state.task.workflow_step_id,
        })}
      </span>
    </section>
  );
}

function stepOptions(steps: readonly WorkflowStepDTO[]): ComboboxOption[] {
  return steps.map((step) => {
    const color = parseWorkflowStepColor(step.color);
    return {
      value: step.id,
      label: step.name,
      renderLabel: () => (
        <span className="flex min-w-0 items-center gap-2">
          <span
            aria-hidden="true"
            className={cn("size-2 shrink-0 rounded-full", "className" in color && color.className)}
            style={"style" in color ? color.style : undefined}
          />
          <span className="min-w-0 break-words">{step.name}</span>
        </span>
      ),
    };
  });
}

type PickerProps = SectionProps & { id: string };

function WorkspacePicker({ state, isTouchSurface, id }: PickerProps) {
  const { t } = useTranslation();
  if (state.workspaces.length === 0)
    return (
      <p
        role="status"
        className="text-sm text-muted-foreground"
        data-testid="workspace-move-no-workspaces"
      >
        {t("task:workspaceMoveNoWorkspaces")}
      </p>
    );
  return (
    <Field label={t("task:workspaceMoveWorkspaceLabel")} isTouchSurface={isTouchSurface}>
      <Combobox
        options={state.workspaces.map((item) => ({ value: item.id, label: item.name }))}
        value={state.selectedWorkspaceId}
        onValueChange={state.changeWorkspace}
        ariaLabel={t("task:workspaceMoveWorkspaceLabel")}
        triggerId={id}
        placeholder={t("task:workspaceMoveWorkspacePlaceholder")}
        searchPlaceholder={t("task:workspaceMoveSearchWorkspaces")}
        emptyMessage={t("task:workspaceMoveNoWorkspaceFound")}
        dropdownLabel={t("task:workspaceMoveWorkspaceLabel")}
        disabled={state.frozen || !state.task}
        touchTarget={isTouchSurface}
        triggerClassName={controlClass(isTouchSurface)}
        testId="workspace-move-workspace"
        popoverPortal
      />
    </Field>
  );
}

function WorkflowPicker({ state, isTouchSurface, id }: PickerProps) {
  const { t } = useTranslation();
  if (!state.selectedWorkspaceId) return null;
  if (state.workflowsStatus === "loading")
    return (
      <Status
        label={t("task:workspaceMoveLoadingWorkflows")}
        testId="workspace-move-workflows-loading"
      />
    );
  if (state.workflowsStatus === "error")
    return (
      <LoadFailed
        label={t("task:workspaceMoveLoadWorkflowsFailed")}
        onRetry={state.retryWorkflows}
        isTouchSurface={isTouchSurface}
        testId="workspace-move-workflows-error"
      />
    );
  if (state.workflowsStatus === "success" && state.workflows.length === 0)
    return (
      <p
        role="status"
        className="text-sm text-muted-foreground"
        data-testid="workspace-move-no-workflows"
      >
        {t("task:workspaceMoveNoWorkflows")}
      </p>
    );
  return (
    <Field label={t("task:workspaceMoveWorkflowLabel")} isTouchSurface={isTouchSurface}>
      <Combobox
        options={state.workflows.map((workflow) => ({ value: workflow.id, label: workflow.name }))}
        value={state.selectedWorkflowId}
        onValueChange={state.changeWorkflow}
        ariaLabel={t("task:workspaceMoveWorkflowLabel")}
        triggerId={id}
        placeholder={t("task:workspaceMoveWorkflowPlaceholder")}
        searchPlaceholder={t("task:workspaceMoveSearchWorkflows")}
        emptyMessage={t("task:workspaceMoveNoWorkflowFound")}
        dropdownLabel={t("task:workspaceMoveWorkflowLabel")}
        disabled={state.frozen}
        touchTarget={isTouchSurface}
        triggerClassName={controlClass(isTouchSurface)}
        testId="workspace-move-workflow"
        popoverPortal
      />
    </Field>
  );
}

function StepPicker({ state, isTouchSurface, id }: PickerProps) {
  const { t } = useTranslation();
  if (!state.selectedWorkflowId) return null;
  if (state.stepsStatus === "loading")
    return (
      <Status label={t("task:workspaceMoveLoadingSteps")} testId="workspace-move-steps-loading" />
    );
  if (state.stepsStatus === "error")
    return (
      <LoadFailed
        label={t("task:workspaceMoveLoadStepsFailed")}
        onRetry={state.retrySteps}
        isTouchSurface={isTouchSurface}
        testId="workspace-move-steps-error"
      />
    );
  if (state.stepsStatus === "success" && state.steps.length === 0)
    return (
      <p
        role="status"
        className="text-sm text-muted-foreground"
        data-testid="workspace-move-no-steps"
      >
        {t("task:workspaceMoveNoSteps")}
      </p>
    );
  return (
    <Field label={t("task:workspaceMoveStepLabel")} isTouchSurface={isTouchSurface}>
      <Combobox
        options={stepOptions(state.steps)}
        value={state.selectedStepId}
        onValueChange={state.chooseStep}
        ariaLabel={t("task:workspaceMoveStepLabel")}
        triggerId={id}
        placeholder={t("task:workspaceMoveStepPlaceholder")}
        searchPlaceholder={t("task:workspaceMoveSearchSteps")}
        emptyMessage={t("task:workspaceMoveNoStepFound")}
        dropdownLabel={t("task:workspaceMoveStepLabel")}
        disabled={state.frozen}
        touchTarget={isTouchSurface}
        triggerClassName={controlClass(isTouchSurface)}
        testId="workspace-move-step"
        popoverPortal
      />
    </Field>
  );
}

function Field({
  label,
  isTouchSurface,
  children,
}: {
  label: string;
  isTouchSurface: boolean;
  children: ReactNode;
}) {
  return (
    <div
      className={cn("grid min-w-0 grid-cols-[minmax(0,1fr)] gap-1.5", isTouchSurface && "gap-2")}
    >
      <span className="text-sm font-medium">{label}</span>
      {children}
    </div>
  );
}

export function WorkspaceMoveSelectors(props: SectionProps & { ids: [string, string, string] }) {
  const [workspaceId, workflowId, stepId] = props.ids;
  return (
    <>
      <WorkspacePicker {...props} id={workspaceId} />
      <WorkflowPicker {...props} id={workflowId} />
      <StepPicker {...props} id={stepId} />
    </>
  );
}

export function WorkspaceMoveNotice() {
  const { t } = useTranslation();
  return (
    <p
      className="text-xs leading-relaxed text-muted-foreground"
      data-testid="workspace-move-notice"
    >
      {t("task:workspaceMoveNotice")}
    </p>
  );
}

function OutcomeAction({
  label,
  onClick,
  isTouchSurface,
  testId,
}: {
  label: string;
  onClick: () => void;
  isTouchSurface: boolean;
  testId: string;
}) {
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      className={cn("cursor-pointer", isTouchSurface && "min-h-11")}
      onClick={onClick}
      data-testid={testId}
    >
      {label}
    </Button>
  );
}

type OutcomeProps = SectionProps & { showOpenTask: boolean; onOpenTask: () => void };

function CommittedOutcome({ state, isTouchSurface, showOpenTask, onOpenTask }: OutcomeProps) {
  const { t } = useTranslation();
  const taskId = state.operation?.taskId ?? "";
  return (
    <>
      <span>{t("task:workspaceMoveSuccess")}</span>
      {showOpenTask && (
        <AppLink
          href={linkToTask(taskId)}
          onClick={onOpenTask}
          data-testid="workspace-move-open-task"
          className={cn("cursor-pointer font-medium underline", isTouchSurface && "min-h-11 py-3")}
        >
          {t("task:workspaceMoveOpenTask")}
        </AppLink>
      )}
    </>
  );
}

function CheckOutcome({
  state,
  isTouchSurface,
  unavailable,
  refused,
}: SectionProps & { unavailable: boolean; refused: boolean }) {
  const { t } = useTranslation();
  let message = t("task:workspaceMoveUncertain");
  if (unavailable) message = t("task:workspaceMoveCheckUnavailable");
  if (refused) message = t("task:workspaceMoveCheckRejected");
  return (
    <>
      <span>{message}</span>
      <OutcomeAction
        label={unavailable ? t("task:workspaceMoveCheckAgain") : t("task:workspaceMoveCheckResult")}
        onClick={() => void state.check()}
        isTouchSurface={isTouchSurface}
        testId="workspace-move-check-result"
      />
    </>
  );
}

function OperationOutcome(props: OutcomeProps) {
  const { t } = useTranslation();
  const operation = props.state.operation;
  switch (operation?.status) {
    case "submitting":
      return (
        <span className="flex items-center gap-2">
          <IconLoader2 className="size-4 animate-spin" aria-hidden="true" />
          {t("task:workspaceMoveMoving")}
        </span>
      );
    case "committed":
      return <CommittedOutcome {...props} />;
    case "rejected":
      return (
        <span data-testid="workspace-move-rejection" data-code={operation.code}>
          {t(REJECTION_KEYS[operation.code])}
        </span>
      );
    case "uncertain":
    case "check-unavailable":
      return (
        <CheckOutcome
          {...props}
          unavailable={operation.status === "check-unavailable"}
          refused={operation.status === "check-unavailable" && operation.code !== undefined}
        />
      );
    case "operation-rejected":
      return <span>{t("task:workspaceMoveOperationRejected")}</span>;
    default:
      return null;
  }
}

/** One announced region for every operation outcome. */
export function WorkspaceMoveOutcome(props: OutcomeProps) {
  const { t } = useTranslation();
  const { state, isTouchSurface } = props;
  const status = state.operation?.status;
  const stale = state.sourceStale && status !== "submitting";
  if (!status && (state.restoreUnverifiable || state.storageUnavailable))
    return (
      <div
        role="status"
        data-testid="workspace-move-outcome"
        data-status={state.restoreUnverifiable ? "restore-unverifiable" : "storage-unavailable"}
        className="rounded-md border p-3 text-sm break-words"
      >
        {state.restoreUnverifiable
          ? t("task:workspaceMoveRestoreUnverifiable")
          : t("task:workspaceMoveStorageUnavailable")}
      </div>
    );
  if (!status && !stale) return null;
  const alert = status === "rejected" || status === "operation-rejected";
  return (
    <div
      role={alert ? "alert" : "status"}
      aria-live="polite"
      data-testid="workspace-move-outcome"
      data-status={status ?? "stale"}
      className={cn(
        "grid justify-items-start gap-2 rounded-md border p-3 text-sm break-words",
        alert ? "border-destructive/40" : "border-border",
      )}
    >
      <OperationOutcome {...props} />
      {stale && (
        <OutcomeAction
          label={t("task:workspaceMoveRefreshTask")}
          onClick={() => void state.refreshSource()}
          isTouchSurface={isTouchSurface}
          testId="workspace-move-refresh-task"
        />
      )}
    </div>
  );
}
