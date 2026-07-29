"use client";

import { memo, useMemo } from "react";
import type { ForegroundActivity, TaskState, TaskSessionState } from "@/lib/types/http";
import { type StepDef } from "./task-switcher-context-menu";
import {
  countGroupTasks,
  type GroupedSidebarList,
  type SidebarGroup,
} from "@/lib/sidebar/apply-view";
import { type TaskMoveWorkflow } from "@/components/task/task-move-context-menu";
import { TaskTreeLevel, type TaskLinkHandler, type TaskTreeContext } from "./task-switcher-tree";
import { GroupHeader, TaskSwitcherSkeleton } from "./task-switcher-group";
import { useTranslation } from "react-i18next";

export type TaskSwitcherItem = {
  id: string;
  title: string;
  state?: TaskState;
  sessionState?: TaskSessionState;
  /** Task-level most-active-wins busy aggregate (ADR-0049) from the task record. */
  foregroundActivity?: ForegroundActivity | null;
  description?: string;
  workflowId?: string;
  workflowName?: string;
  workflowStepId?: string;
  workflowStepTitle?: string;
  repositoryPath?: string;
  repositories?: string[];
  diffStats?: { additions: number; deletions: number };
  isRemoteExecutor?: boolean;
  remoteExecutorType?: string;
  remoteExecutorName?: string;
  updatedAt?: string;
  createdAt?: string;
  isArchived?: boolean;
  primarySessionId?: string | null;
  hasPendingClarification?: boolean;
  hasPendingPermission?: boolean;
  parentTaskTitle?: string;
  parentTaskId?: string;
  workspaceMode?: "inherit_parent" | "new_workspace" | "shared_group";
  prInfo?: { number: number; state: string };
  isPRReview?: boolean;
  isIssueWatch?: boolean;
  issueInfo?: { url: string; number: number };
  agentErrorMessage?: string | null;
};

type TaskSwitcherProps = {
  grouped: GroupedSidebarList;
  workflows?: TaskMoveWorkflow[];
  stepsByWorkflowId?: Record<string, StepDef[]>;
  activeTaskId: string | null;
  selectedTaskId: string | null;
  collapsedGroupKeys?: string[];
  onToggleGroup?: (groupKey: string) => void;
  collapsedSubtaskParentIds?: string[];
  onToggleSubtasks?: (parentTaskId: string) => void;
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
  onReorderGroup?: (groupTaskIds: string[]) => void;
  onReorderSubtasks?: (parentTaskId: string, orderedSubtaskIds: string[]) => void;
  pinnedTaskIds?: string[];
  deletingTaskId?: string | null;
  isLoading?: boolean;
  totalTaskCount?: number;
  // Multi-select (cmd/shift click). When the selection is non-empty, plain
  // clicks toggle instead of navigating; the context menu acts on the selection.
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

type GroupSectionProps = {
  group: SidebarGroup;
  subTasksByParentId: Map<string, TaskSwitcherItem[]>;
  workflows?: TaskMoveWorkflow[];
  stepsByWorkflowId?: Record<string, StepDef[]>;
  activeTaskId: string | null;
  selectedTaskId: string | null;
  isCollapsed: boolean;
  onToggleCollapsed: () => void;
  collapsedSubtaskParentIds?: string[];
  onToggleSubtasks?: (parentTaskId: string) => void;
  showHeader: boolean;
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
  onReorderGroup?: (groupTaskIds: string[]) => void;
  onReorderSubtasks?: (parentTaskId: string, orderedSubtaskIds: string[]) => void;
  pinnedTaskIds?: string[];
  pinnedSet: Set<string>;
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

function GroupSection({
  group,
  subTasksByParentId,
  workflows,
  stepsByWorkflowId,
  activeTaskId,
  selectedTaskId,
  isCollapsed,
  onToggleCollapsed,
  collapsedSubtaskParentIds,
  onToggleSubtasks,
  showHeader,
  onSelectTask,
  onRenameTask,
  onArchiveTask,
  onCreateSubtask,
  onDeleteTask,
  onDetachTask,
  onLinkPullRequest,
  onLinkIssue,
  onLinkMergeRequest,
  onLinkJiraTicket,
  onLinkLinearIssue,
  onLinkSentryIssue,
  onMoveToStep,
  onTogglePin,
  onReorderGroup,
  onReorderSubtasks,
  pinnedTaskIds,
  pinnedSet,
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
}: GroupSectionProps) {
  const totalCount = countGroupTasks(group.tasks, subTasksByParentId);
  const ctx: TaskTreeContext = {
    subTasksByParentId,
    collapsedSubs: new Set(collapsedSubtaskParentIds ?? []),
    onToggleSubtasks,
    pinnedSet,
    rowProps: {
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
      onLinkPullRequest,
      onLinkIssue,
      onLinkMergeRequest,
      onLinkJiraTicket,
      onLinkLinearIssue,
      onLinkSentryIssue,
      onMoveToStep,
      onTogglePin,
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
    },
    onReorderGroup,
    onReorderSubtasks,
  };

  return (
    <div>
      {showHeader && (
        <GroupHeader
          label={group.label}
          groupKey={group.key}
          count={totalCount}
          isCollapsed={isCollapsed}
          onToggle={onToggleCollapsed}
        />
      )}
      {!isCollapsed && (
        <TaskTreeLevel parentTaskId={null} tasks={group.tasks} depth={0} ctx={ctx} />
      )}
    </div>
  );
}

export const TaskSwitcher = memo(function TaskSwitcher({
  grouped,
  workflows,
  stepsByWorkflowId,
  activeTaskId,
  selectedTaskId,
  collapsedGroupKeys = [],
  onToggleGroup,
  collapsedSubtaskParentIds,
  onToggleSubtasks,
  onSelectTask,
  onRenameTask,
  onArchiveTask,
  onCreateSubtask,
  onDeleteTask,
  onDetachTask,
  onLinkPullRequest,
  onLinkIssue,
  onLinkMergeRequest,
  onLinkJiraTicket,
  onLinkLinearIssue,
  onLinkSentryIssue,
  onMoveToStep,
  onTogglePin,
  onReorderGroup,
  onReorderSubtasks,
  pinnedTaskIds,
  deletingTaskId,
  isLoading = false,
  totalTaskCount,
  selectedTaskIds,
  onToggleSelectTask,
  onSelectTaskRange,
  onBulkArchive,
  onBulkDelete,
  onBulkPin,
  onBulkMove,
  onClearSelection,
  isMixedWorkflowSelection,
}: TaskSwitcherProps) {
  const { t } = useTranslation();
  const pinnedSet = useMemo(() => new Set(pinnedTaskIds ?? []), [pinnedTaskIds]);
  if (isLoading) return <TaskSwitcherSkeleton />;
  const totalTasks = totalTaskCount ?? grouped.groups.reduce((sum, g) => sum + g.tasks.length, 0);
  if (totalTasks === 0) {
    return <div className="px-3 py-3 text-xs text-muted-foreground">{t("task:noTasksYet")}</div>;
  }

  const collapsedSet = new Set(collapsedGroupKeys);
  const showHeaders =
    grouped.groups.length > 1 ||
    (grouped.groups.length === 1 && grouped.groups[0].key !== "__all__");

  return (
    <div>
      {grouped.groups.map((group) => (
        <GroupSection
          key={group.key}
          group={group}
          subTasksByParentId={grouped.subTasksByParentId}
          workflows={workflows}
          stepsByWorkflowId={stepsByWorkflowId}
          activeTaskId={activeTaskId}
          selectedTaskId={selectedTaskId}
          isCollapsed={collapsedSet.has(group.key)}
          onToggleCollapsed={() => onToggleGroup?.(group.key)}
          collapsedSubtaskParentIds={collapsedSubtaskParentIds}
          onToggleSubtasks={onToggleSubtasks}
          showHeader={showHeaders}
          onSelectTask={onSelectTask}
          onRenameTask={onRenameTask}
          onArchiveTask={onArchiveTask}
          onCreateSubtask={onCreateSubtask}
          onDeleteTask={onDeleteTask}
          onDetachTask={onDetachTask}
          onLinkPullRequest={onLinkPullRequest}
          onLinkIssue={onLinkIssue}
          onLinkMergeRequest={onLinkMergeRequest}
          onLinkJiraTicket={onLinkJiraTicket}
          onLinkLinearIssue={onLinkLinearIssue}
          onLinkSentryIssue={onLinkSentryIssue}
          onMoveToStep={onMoveToStep}
          onTogglePin={onTogglePin}
          onReorderGroup={onReorderGroup}
          onReorderSubtasks={onReorderSubtasks}
          pinnedTaskIds={pinnedTaskIds}
          pinnedSet={pinnedSet}
          deletingTaskId={deletingTaskId}
          selectedTaskIds={selectedTaskIds}
          onToggleSelectTask={onToggleSelectTask}
          onSelectTaskRange={onSelectTaskRange}
          onBulkArchive={onBulkArchive}
          onBulkDelete={onBulkDelete}
          onBulkPin={onBulkPin}
          onBulkMove={onBulkMove}
          onClearSelection={onClearSelection}
          isMixedWorkflowSelection={isMixedWorkflowSelection}
        />
      ))}
    </div>
  );
});
