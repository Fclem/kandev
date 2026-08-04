import type {
  GroupSectionProps,
  TaskRowProps,
  TaskSwitcherItem,
  TaskTreeContext,
} from "./task-switcher";

/**
 * Flattens one group's subtree (roots + every descendant, in rendered order)
 * so the group-spanning DndContext can resolve reorder levels and nest
 * targets without consulting other groups' children.
 */
export function flattenGroupTasks(
  roots: TaskSwitcherItem[],
  subTasksByParentId: Map<string, TaskSwitcherItem[]>,
): TaskSwitcherItem[] {
  const out: TaskSwitcherItem[] = [];
  const visit = (task: TaskSwitcherItem) => {
    out.push(task);
    const subs = subTasksByParentId.get(task.id);
    if (subs) {
      for (const sub of subs) visit(sub);
    }
  };
  for (const root of roots) visit(root);
  return out;
}

/**
 * Assembles the per-render context threaded through the recursive task tree
 * from the group's props plus the active drag state (nest targets + dragged
 * id). Kept out of `GroupSection` so that component stays under the function
 * length limit.
 */
export function buildTaskTreeContext(
  props: GroupSectionProps,
  drag: { nestTargetIds: Set<string>; activeDragId: string | null },
): TaskTreeContext {
  const rowProps: Omit<
    TaskRowProps,
    "task" | "subtaskToggle" | "isPinned" | "isSubTask" | "depth"
  > = {
    workflows: props.workflows,
    stepsByWorkflowId: props.stepsByWorkflowId,
    activeTaskId: props.activeTaskId,
    selectedTaskId: props.selectedTaskId,
    onSelectTask: props.onSelectTask,
    onEditTask: props.onEditTask,
    onRenameTask: props.onRenameTask,
    onArchiveTask: props.onArchiveTask,
    onCreateSubtask: props.onCreateSubtask,
    onDeleteTask: props.onDeleteTask,
    onDetachTask: props.onDetachTask,
    onLinkPullRequest: props.onLinkPullRequest,
    onLinkIssue: props.onLinkIssue,
    onLinkMergeRequest: props.onLinkMergeRequest,
    onLinkJiraTicket: props.onLinkJiraTicket,
    onLinkLinearIssue: props.onLinkLinearIssue,
    onLinkSentryIssue: props.onLinkSentryIssue,
    onMoveToStep: props.onMoveToStep,
    onTogglePin: props.onTogglePin,
    pinnedTaskIds: props.pinnedTaskIds,
    deletingTaskId: props.deletingTaskId,
    selectedTaskIds: props.selectedTaskIds,
    onToggleSelectTask: props.onToggleSelectTask,
    onSelectTaskRange: props.onSelectTaskRange,
    onBulkArchive: props.onBulkArchive,
    onBulkDelete: props.onBulkDelete,
    onBulkPin: props.onBulkPin,
    onBulkMove: props.onBulkMove,
    onClearSelection: props.onClearSelection,
    isMixedWorkflowSelection: props.isMixedWorkflowSelection,
  };
  return {
    subTasksByParentId: props.subTasksByParentId,
    collapsedSubs: new Set(props.collapsedSubtaskParentIds ?? []),
    onToggleSubtasks: props.onToggleSubtasks,
    pinnedSet: props.pinnedSet,
    rowProps,
    onReorderGroup: props.onReorderGroup,
    onReorderSubtasks: props.onReorderSubtasks,
    onNestTask: props.onNestTask,
    nestTargetIds: drag.nestTargetIds,
    activeDragId: drag.activeDragId,
    externalDragContext: true,
  };
}
