import { cloneElement, isValidElement } from "react";

import type { TaskSwitcherItem } from "./task-switcher";

import type { ContextMenuProps } from "./task-switcher-context-menu";

// Pure helpers for the task context menu: link-action factories and the
// menu-open clone. Extracted from task-switcher-context-menu.tsx to keep that
// file under the 600-line limit.

export function createTaskLinkSelectAction(
  task: Pick<TaskSwitcherItem, "id" | "title">,
  handler: ((taskId: string, taskTitle?: string) => void) | undefined,
  closeMenu: () => void,
) {
  if (!handler) return undefined;
  return () => {
    closeMenu();
    handler(task.id, task.title);
  };
}

export function selectTaskLinkActions(
  task: Pick<TaskSwitcherItem, "id" | "title">,
  closeMenu: () => void,
  handlers: Pick<
    ContextMenuProps,
    | "onLinkPullRequest"
    | "onLinkIssue"
    | "onLinkMergeRequest"
    | "onLinkJiraTicket"
    | "onLinkLinearIssue"
    | "onLinkSentryIssue"
  >,
) {
  return {
    onLinkPullRequest: createTaskLinkSelectAction(task, handlers.onLinkPullRequest, closeMenu),
    onLinkIssue: createTaskLinkSelectAction(task, handlers.onLinkIssue, closeMenu),
    onLinkMergeRequest: createTaskLinkSelectAction(task, handlers.onLinkMergeRequest, closeMenu),
    onLinkJiraTicket: createTaskLinkSelectAction(task, handlers.onLinkJiraTicket, closeMenu),
    onLinkLinearIssue: createTaskLinkSelectAction(task, handlers.onLinkLinearIssue, closeMenu),
    onLinkSentryIssue: createTaskLinkSelectAction(task, handlers.onLinkSentryIssue, closeMenu),
  };
}

export function cloneWithMenuOpen(
  children: React.ReactElement<{ menuOpen?: boolean }>,
  menuOpen: boolean,
): React.ReactNode {
  if (isValidElement(children)) return cloneElement(children, { menuOpen });
  return children;
}
