import { IconLoader, IconUnlink } from "@tabler/icons-react";
import { t } from "@/lib/i18n";
import { buildLinkSubmenu } from "./kanban-card-link-submenu";
import {
  buildArchiveEntry,
  buildCurrentWorkflowMoveEntry,
  buildDeleteEntry,
  buildGroupedMenuEntries,
  buildPriorityMenuEntry,
  buildSingleTaskMoveGroup,
  buildWorkflowMenuEntry,
  resolveCardPluginEntries,
  type BuildKanbanCardMenuEntriesArgs,
  type KanbanCardMenuEntry,
  type KanbanCardMenuEntryGroup,
} from "./kanban-card-menu-items";

function buildMoveMenuEntries(
  args: Pick<
    BuildKanbanCardMenuEntriesArgs,
    | "currentWorkflowId"
    | "currentStepId"
    | "workflows"
    | "stepsByWorkflowId"
    | "moveDisabled"
    | "onMoveToStep"
    | "onChangeWorkflow"
    | "onChangeWorkspace"
    | "onSendToWorkflow"
    | "isBulkSelection"
  >,
  isProcessing: boolean,
): KanbanCardMenuEntry[] {
  const currentSteps = args.currentWorkflowId
    ? (args.stepsByWorkflowId[args.currentWorkflowId] ?? [])
    : [];
  const moveDisabled = Boolean(args.moveDisabled || isProcessing);
  const single = !args.isBulkSelection;
  const stepEntry = buildCurrentWorkflowMoveEntry(
    currentSteps,
    args.currentStepId,
    moveDisabled,
    args.onMoveToStep,
    single,
  );
  const workflowEntry = buildWorkflowMenuEntry({
    currentWorkflowId: args.currentWorkflowId,
    workflows: args.workflows,
    stepsByWorkflowId: args.stepsByWorkflowId,
    disabled: moveDisabled,
    onChangeWorkflow: args.onChangeWorkflow,
    onSendToWorkflow: args.onSendToWorkflow,
    isBulkSelection: args.isBulkSelection,
  });
  // Bulk selections keep their plural step and workflow actions and never
  // offer a workspace transfer.
  const entries = single
    ? [
        buildSingleTaskMoveGroup({
          stepEntry,
          workflowEntry,
          disabled: moveDisabled,
          onChangeWorkspace: args.onChangeWorkspace,
        }),
      ]
    : [stepEntry, workflowEntry];
  return entries.filter((entry): entry is KanbanCardMenuEntry => entry !== null);
}

function buildDetachEntry({
  parentTaskId,
  onDetach,
  isDetaching,
  isProcessing,
}: Pick<BuildKanbanCardMenuEntriesArgs, "parentTaskId" | "onDetach" | "isDetaching"> & {
  isProcessing: boolean;
}): KanbanCardMenuEntry | null {
  if (!parentTaskId || !onDetach) return null;
  return {
    kind: "item",
    key: "detach",
    testId: "task-context-detach",
    icon: isDetaching ? (
      <IconLoader className="mr-2 h-4 w-4 animate-spin" />
    ) : (
      <IconUnlink className="mr-2 h-4 w-4" />
    ),
    label: t("kanban:detachFromParent"),
    disabled: isProcessing,
    onSelect: onDetach,
  };
}

function buildRelationshipMenuEntries(
  args: Pick<
    BuildKanbanCardMenuEntriesArgs,
    | "parentTaskId"
    | "onDetach"
    | "isDetaching"
    | "onLinkPullRequest"
    | "onLinkIssue"
    | "onLinkMergeRequest"
    | "onLinkJiraTicket"
    | "onLinkLinearIssue"
    | "onLinkSentryIssue"
    | "pluginLinkActions"
  >,
  isProcessing: boolean,
) {
  const linkEntry = buildLinkSubmenu({
    disabled: isProcessing,
    onLinkPullRequest: args.onLinkPullRequest,
    onLinkIssue: args.onLinkIssue,
    onLinkMergeRequest: args.onLinkMergeRequest,
    onLinkJiraTicket: args.onLinkJiraTicket,
    onLinkLinearIssue: args.onLinkLinearIssue,
    onLinkSentryIssue: args.onLinkSentryIssue,
    pluginLinkActions: args.pluginLinkActions,
  });
  const detachEntry = buildDetachEntry({
    parentTaskId: args.parentTaskId,
    onDetach: args.onDetach,
    isDetaching: args.isDetaching,
    isProcessing,
  });
  return [linkEntry, detachEntry].filter((entry): entry is KanbanCardMenuEntry => entry !== null);
}

function buildCardMenuGroups(args: {
  priorityEntry: ReturnType<typeof buildPriorityMenuEntry>;
  editEntry: KanbanCardMenuEntry;
  relationshipEntries: KanbanCardMenuEntry[];
  moveEntries: KanbanCardMenuEntry[];
  pluginEntries: KanbanCardMenuEntry[];
  archiveEntry: KanbanCardMenuEntry;
  deleteEntry: KanbanCardMenuEntry;
}): KanbanCardMenuEntryGroup[] {
  return [
    { key: "priority", entries: args.priorityEntry ? [args.priorityEntry] : [] },
    { key: "edit", entries: [args.editEntry] },
    { key: "relationships", entries: args.relationshipEntries },
    { key: "move", entries: args.moveEntries },
    { key: "plugins", entries: args.pluginEntries },
    { key: "remove", entries: [args.archiveEntry, args.deleteEntry] },
  ];
}

export function buildKanbanCardMenuEntries({
  currentWorkflowId,
  currentStepId,
  workflows,
  stepsByWorkflowId,
  moveDisabled,
  disabled,
  isDeleting,
  isArchiving,
  isDetaching,
  parentTaskId,
  currentPriority,
  onSelectPriority,
  onEdit,
  onArchive,
  onDelete,
  onDetach,
  onLinkPullRequest,
  onLinkIssue,
  onLinkMergeRequest,
  onLinkJiraTicket,
  onLinkLinearIssue,
  onLinkSentryIssue,
  pluginLinkActions,
  onMoveToStep,
  onChangeWorkflow,
  onChangeWorkspace,
  onSendToWorkflow,
  isBulkSelection,
  pluginMenuContext,
  pluginEntries,
  forceFlatEdit,
  nativeUnlinkEntries,
  loadingUnlinkLabel,
}: BuildKanbanCardMenuEntriesArgs): KanbanCardMenuEntry[] {
  const isProcessing = Boolean(disabled || isDeleting || isArchiving || isDetaching);
  const priorityEntry = buildPriorityMenuEntry({
    currentPriority,
    disabled: isProcessing,
    onSelectPriority,
  });
  const moveEntries = buildMoveMenuEntries(
    {
      currentWorkflowId,
      currentStepId,
      workflows,
      stepsByWorkflowId,
      moveDisabled,
      onMoveToStep,
      onChangeWorkflow,
      onChangeWorkspace,
      onSendToWorkflow,
      isBulkSelection,
    },
    isProcessing,
  );
  const pluginContributions = resolveCardPluginEntries({
    pluginEntries,
    forceFlatEdit,
    nativeUnlinkEntries,
    loadingUnlinkLabel,
    disabled: isProcessing,
    onEdit,
    pluginMenuContext,
  });
  const relationshipEntries = buildRelationshipMenuEntries(
    {
      parentTaskId,
      onDetach,
      isDetaching,
      onLinkPullRequest,
      onLinkIssue,
      onLinkMergeRequest,
      onLinkJiraTicket,
      onLinkLinearIssue,
      onLinkSentryIssue,
      pluginLinkActions,
    },
    isProcessing,
  );
  return buildGroupedMenuEntries(
    buildCardMenuGroups({
      priorityEntry,
      editEntry: pluginContributions.edit,
      relationshipEntries,
      moveEntries,
      pluginEntries: pluginContributions.primary,
      archiveEntry: buildArchiveEntry({ isArchiving, isProcessing, onArchive }),
      deleteEntry: buildDeleteEntry({ isDeleting, isProcessing, onDelete }),
    }),
  );
}
