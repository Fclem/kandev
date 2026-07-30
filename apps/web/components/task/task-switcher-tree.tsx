"use client";

// Recursive sidebar task tree: one task row (with its context menu) plus the
// nested subtask levels it expands into, and the modifier-aware row-click
// dispatch they share. TaskSwitcher owns grouping and prop plumbing.

import { TaskItem } from "./task-item";
import { TaskItemWithContextMenu, type StepDef } from "./task-switcher-context-menu";
import { countGroupTasks } from "@/lib/sidebar/apply-view";
import { type TaskMoveWorkflow } from "@/components/task/task-move-context-menu";
import { SortableTaskLevel, SortableTaskNode } from "./task-switcher-subtask-dnd";
import type { TaskSwitcherItem } from "./task-switcher";

export type TaskLinkHandler = (taskId: string, taskTitle?: string) => void;

/**
 * Modifier-aware sidebar row click: cmd/ctrl toggles one task, shift extends a
 * range, a plain click toggles while a selection is active and otherwise
 * navigates to the task.
 */
/** @internal Exported for unit testing the modifier-aware click dispatch. */
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

type TaskLinkHandlerProps = {
  onLinkPullRequest?: TaskLinkHandler;
  onLinkIssue?: TaskLinkHandler;
  onLinkMergeRequest?: TaskLinkHandler;
  onLinkJiraTicket?: TaskLinkHandler;
  onLinkLinearIssue?: TaskLinkHandler;
  onLinkSentryIssue?: TaskLinkHandler;
};

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
  const isSelected = task.id === selectedTaskId || task.id === activeTaskId;
  const taskSteps = task.workflowId ? stepsByWorkflowId?.[task.workflowId] : undefined;
  const stepId = task.workflowStepId;
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
        onSelect={(e) =>
          dispatchSidebarRowClick(e, task.id, (selectedTaskIds?.size ?? 0) > 0, {
            onSelectTask,
            onToggleSelectTask,
            onSelectTaskRange,
          })
        }
        title={task.title}
        state={task.state}
        sessionState={task.sessionState}
        foregroundActivity={task.foregroundActivity}
        isArchived={task.isArchived}
        isSelected={isSelected}
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
        isOnLastWorkflowStep={!!stepId && taskSteps?.at(-1)?.id === stepId}
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

// Shared, per-render context threaded through the recursive task tree so each
// node can look up its children, collapse state, and reorder callbacks without
// drilling a dozen props through every level.
export type TaskTreeContext = {
  subTasksByParentId: Map<string, TaskSwitcherItem[]>;
  collapsedSubs: Set<string>;
  onToggleSubtasks?: (parentTaskId: string) => void;
  pinnedSet: Set<string>;
  rowProps: Omit<TaskRowProps, "task" | "subtaskToggle" | "isPinned" | "isSubTask" | "depth">;
  onReorderGroup?: (groupTaskIds: string[]) => void;
  onReorderSubtasks?: (parentTaskId: string, orderedSubtaskIds: string[]) => void;
};

// One task row plus — when expanded — its nested subtree. Mutually recursive
// with TaskTreeLevel, so it renders arbitrarily deep hierarchies.
function TaskTreeNode({
  task,
  depth,
  ctx,
  isDraggable,
}: {
  task: TaskSwitcherItem;
  depth: number;
  ctx: TaskTreeContext;
  isDraggable: boolean;
}) {
  const subs = ctx.subTasksByParentId.get(task.id);
  const hasSubs = !!subs?.length;
  const subsHidden = hasSubs && !!ctx.onToggleSubtasks && ctx.collapsedSubs.has(task.id);
  const toggleInfo: SubtaskToggleInfo | undefined =
    hasSubs && ctx.onToggleSubtasks
      ? {
          subtaskCount: countGroupTasks(subs!, ctx.subTasksByParentId),
          subtasksCollapsed: subsHidden,
          onToggleSubtasks: () => ctx.onToggleSubtasks!(task.id),
        }
      : undefined;
  const isRoot = depth === 0;
  const handle = (
    <TaskRow
      task={task}
      depth={depth}
      isSubTask={!isRoot}
      subtaskToggle={toggleInfo}
      isPinned={isRoot && ctx.pinnedSet.has(task.id)}
      {...ctx.rowProps}
      // Only root tasks are pinnable — `floatPinnedToTop` reorders root tasks
      // only, so a pin on a nested row would show an icon but never move it.
      onTogglePin={isRoot ? ctx.rowProps.onTogglePin : undefined}
    />
  );
  const nested =
    !subsHidden && hasSubs ? (
      <TaskTreeLevel parentTaskId={task.id} tasks={subs!} depth={depth + 1} ctx={ctx} />
    ) : undefined;
  return (
    <SortableTaskNode
      taskId={task.id}
      depth={depth}
      handle={handle}
      nested={nested}
      isDraggable={isDraggable}
    />
  );
}

// One level of sibling tasks. `parentTaskId === null` is the group root (whose
// reorder maps to onReorderGroup); deeper levels reorder via onReorderSubtasks
// scoped to that parent's children.
export function TaskTreeLevel({
  parentTaskId,
  tasks,
  depth,
  ctx,
}: {
  parentTaskId: string | null;
  tasks: TaskSwitcherItem[];
  depth: number;
  ctx: TaskTreeContext;
}) {
  let onReorder: ((orderedTaskIds: string[]) => void) | undefined;
  if (parentTaskId === null) {
    onReorder = ctx.onReorderGroup;
  } else if (ctx.onReorderSubtasks) {
    const pid = parentTaskId;
    onReorder = (ids: string[]) => ctx.onReorderSubtasks!(pid, ids);
  }
  return (
    <SortableTaskLevel
      tasks={tasks}
      onReorder={onReorder}
      renderNode={(task, levelDraggable) => (
        <TaskTreeNode
          key={task.id}
          task={task}
          depth={depth}
          ctx={ctx}
          isDraggable={levelDraggable}
        />
      )}
    />
  );
}
