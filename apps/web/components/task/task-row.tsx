import { TaskItem } from "./task-item";
import { TaskItemWithContextMenu, type StepDef } from "./task-switcher-context-menu";
import type { TaskMoveWorkflow } from "@/components/task/task-move-context-menu";
import type { TaskSwitcherItem } from "./task-switcher";

type TaskLinkHandler = (taskId: string, taskTitle?: string) => void;

/**
 * Modifier-aware sidebar row click: cmd/ctrl toggles one task, shift extends a
 * range, a plain click toggles while a selection is active and otherwise
 * navigates to the task.
 *
 * @internal Exported for unit testing the modifier-aware click dispatch.
 */
export function dispatchSidebarRowClick(
  e: React.MouseEvent | React.KeyboardEvent,
  taskId: string,
  isSelecting: boolean,
  handlers: {
    onSelectTask: (taskId: string) => void;
    onToggleSelectTask?: (taskId: string) => void;
    onSelectTaskRange?: (taskId: string) => void;
  },
): void {
  // Only intercept a modifier click when the matching handler is wired (the
  // mobile switcher renders without selection handlers — there a Cmd/Shift click
  // must still navigate rather than become a no-op).
  if ((e.metaKey || e.ctrlKey) && handlers.onToggleSelectTask) {
    e.preventDefault();
    handlers.onToggleSelectTask(taskId);
    return;
  }
  if (e.shiftKey && handlers.onSelectTaskRange) {
    e.preventDefault();
    handlers.onSelectTaskRange(taskId);
    return;
  }
  if (isSelecting && handlers.onToggleSelectTask) {
    handlers.onToggleSelectTask(taskId);
    return;
  }
  handlers.onSelectTask(taskId);
}

type SubtaskToggleInfo = {
  subtaskCount: number;
  subtasksCollapsed: boolean;
  onToggleSubtasks: () => void;
};

type TaskRowProps = {
  task: TaskSwitcherItem;
  isSubTask?: boolean;
  depth?: number;
  subtaskToggle?: SubtaskToggleInfo;
  workflows?: TaskMoveWorkflow[];
  stepsByWorkflowId?: Record<string, StepDef[]>;
  activeTaskId: string | null;
  selectedTaskId: string | null;
  onSelectTask: (taskId: string) => void;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
  onArchiveTask?: (taskId: string) => void;
  onCreateSubtask?: (taskId: string, taskTitle: string) => void;
  onDeleteTask?: (taskId: string) => void;
  onDetachTask?: (taskId: string) => void;
  onLinkPullRequest?: TaskLinkHandler;
  onLinkIssue?: TaskLinkHandler;
  onLinkMergeRequest?: TaskLinkHandler;
  onLinkJiraTicket?: TaskLinkHandler;
  onLinkLinearIssue?: TaskLinkHandler;
  onLinkSentryIssue?: TaskLinkHandler;
  onMoveToStep?: (taskId: string, workflowId: string, targetStepId: string) => void;
  onTogglePin?: (taskId: string) => void;
  isPinned?: boolean;
  pinnedTaskIds?: string[];
  deletingTaskId?: string | null;
  selectedTaskIds?: Set<string>;
  onToggleSelectTask?: (taskId: string) => void;
  onSelectTaskRange?: (taskId: string) => void;
  onBulkArchive?: (taskIds: string[]) => void;
  onBulkDelete?: (taskIds: string[]) => void;
  onBulkPin?: (taskIds: string[]) => void;
  onBulkMove?: (taskIds: string[], targetWorkflowId: string, targetStepId: string) => void;
  onClearSelection?: () => void;
  isMixedWorkflowSelection?: boolean;
};

type TaskLinkHandlerProps = {
  onLinkPullRequest?: TaskLinkHandler;
  onLinkIssue?: TaskLinkHandler;
  onLinkMergeRequest?: TaskLinkHandler;
  onLinkJiraTicket?: TaskLinkHandler;
  onLinkLinearIssue?: TaskLinkHandler;
  onLinkSentryIssue?: TaskLinkHandler;
};

function taskLinkHandlerProps(props: Pick<TaskRowProps, keyof TaskLinkHandlerProps>) {
  return {
    onLinkPullRequest: props.onLinkPullRequest,
    onLinkIssue: props.onLinkIssue,
    onLinkMergeRequest: props.onLinkMergeRequest,
    onLinkJiraTicket: props.onLinkJiraTicket,
    onLinkLinearIssue: props.onLinkLinearIssue,
    onLinkSentryIssue: props.onLinkSentryIssue,
  };
}

// Modifier-aware row-selection handler for TaskRow, kept module-scoped so the
// row component stays under the per-function line limit.
function taskRowSelectHandler(
  task: Pick<TaskSwitcherItem, "id">,
  selectedTaskIds: Set<string> | undefined,
  handlers: Pick<TaskRowProps, "onSelectTask" | "onToggleSelectTask" | "onSelectTaskRange">,
) {
  return (e: React.MouseEvent | React.KeyboardEvent) =>
    dispatchSidebarRowClick(e, task.id, (selectedTaskIds?.size ?? 0) > 0, handlers);
}

function TaskRow({
  task,
  isSubTask,
  depth,
  subtaskToggle,
  workflows,
  stepsByWorkflowId,
  activeTaskId,
  selectedTaskId,
  onSelectTask,
  onRenameTask,
  onArchiveTask,
  onCreateSubtask,
  onDeleteTask,
  onDetachTask,
  onMoveToStep,
  onTogglePin,
  isPinned,
  pinnedTaskIds,
  deletingTaskId,
  selectedTaskIds,
  onToggleSelectTask,
  onSelectTaskRange,
  onBulkArchive,
  onBulkDelete,
  onBulkPin,
  onBulkMove,
  onClearSelection,
  isMixedWorkflowSelection,
  ...props
}: TaskRowProps) {
  const taskSteps = task.workflowId ? stepsByWorkflowId?.[task.workflowId] : undefined;
  return (
    <TaskItemWithContextMenu
      task={task}
      workflows={workflows}
      stepsByWorkflowId={stepsByWorkflowId}
      steps={taskSteps}
      onRenameTask={onRenameTask}
      onArchiveTask={onArchiveTask}
      onCreateSubtask={onCreateSubtask}
      onDeleteTask={onDeleteTask}
      onDetachTask={onDetachTask}
      {...taskLinkHandlerProps(props)}
      onMoveToStep={onMoveToStep}
      onTogglePin={onTogglePin}
      isPinned={isPinned}
      pinnedTaskIds={pinnedTaskIds}
      isDeleting={deletingTaskId === task.id}
      selectedTaskIds={selectedTaskIds}
      onBulkArchive={onBulkArchive}
      onBulkDelete={onBulkDelete}
      onBulkPin={onBulkPin}
      onBulkMove={onBulkMove}
      onClearSelection={onClearSelection}
      isMixedWorkflowSelection={isMixedWorkflowSelection}
    >
      <TaskItem
        isMultiSelected={selectedTaskIds?.has(task.id) ?? false}
        onSelect={taskRowSelectHandler(task, selectedTaskIds, {
          onSelectTask,
          onToggleSelectTask,
          onSelectTaskRange,
        })}
        title={task.title}
        state={task.state}
        sessionState={task.sessionState}
        foregroundActivity={task.foregroundActivity}
        interrupted={task.interrupted}
        isArchived={task.isArchived}
        isSelected={task.id === selectedTaskId || task.id === activeTaskId}
        diffStats={task.diffStats}
        isRemoteExecutor={task.isRemoteExecutor}
        remoteExecutorType={task.remoteExecutorType}
        remoteExecutorName={task.remoteExecutorName}
        taskId={task.id}
        primarySessionId={task.primarySessionId ?? null}
        hasPendingClarification={task.hasPendingClarification}
        hasPendingPermission={task.hasPendingPermission}
        updatedAt={task.updatedAt}
        repositories={task.repositories}
        prInfo={task.prInfo}
        issueInfo={task.issueInfo}
        agentErrorMessage={task.agentErrorMessage}
        isSubTask={isSubTask}
        isOnLastWorkflowStep={
          !!task.workflowStepId && taskSteps?.at(-1)?.id === task.workflowStepId
        }
        depth={depth}
        subtaskCount={subtaskToggle?.subtaskCount}
        subtasksCollapsed={subtaskToggle?.subtasksCollapsed}
        onToggleSubtasks={subtaskToggle?.onToggleSubtasks}
        onClick={() => onSelectTask(task.id)}
        isDeleting={deletingTaskId === task.id}
        isPinned={isPinned}
      />
    </TaskItemWithContextMenu>
  );
}

export type { TaskRowProps, TaskLinkHandlerProps, SubtaskToggleInfo, TaskLinkHandler };
export { TaskRow, taskLinkHandlerProps };
