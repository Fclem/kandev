"use client";

import { createContext, createElement, useCallback, useContext, useMemo, useState } from "react";
import type { ReactNode } from "react";
import type { KanbanState } from "@/lib/state/slices";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import { usePluginRegistry } from "@/lib/plugins/registry";
import type { PluginTaskActionContext } from "@/lib/plugins/types";
import { useAppStore } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { usePathname } from "@/lib/routing/client-router";
import type { ExternalLinkProvider } from "./task-external-link-dialog";

type StoreApi = {
  getState: () => {
    kanbanMulti: { snapshots: Record<string, { tasks: KanbanState["tasks"] }> };
    kanban: { tasks: KanbanState["tasks"] };
  };
};

export type SidebarLinkTarget = {
  id: string;
  title: string;
  repositoryId?: string;
  issueUrl?: string;
  issueNumber?: number;
  repositories?: Array<{ id?: string; repository_id: string; position?: number }>;
};

export type SidebarExternalLinkTarget = {
  provider: ExternalLinkProvider;
  task: SidebarLinkTarget;
};

export type PluginLinkMenuAction = {
  id: string;
  label: string;
  onSelect: () => void;
};

const PluginTaskLinkActionSurfaceContext = createContext<(() => void) | undefined>(undefined);

/** Lets a mobile drawer dismiss before a plugin opens its own host-native surface. */
export function PluginTaskLinkActionSurfaceProvider({
  beforePluginRun,
  children,
}: {
  beforePluginRun?: () => void;
  children: ReactNode;
}) {
  return createElement(
    PluginTaskLinkActionSurfaceContext.Provider,
    { value: beforePluginRun },
    children,
  );
}

export function runPluginTaskLinkAction(
  beforePluginRun: (() => void) | undefined,
  run: () => Promise<void>,
): void {
  beforePluginRun?.();
  void run().catch(() => {
    // Plugin action owns visible failure UI; keep host menu lifecycle safe.
  });
}

/**
 * Registry actions receive only immutable, current task data. Menu callers
 * invoke this after closing their Radix surface, so plugins never run under an
 * open context menu or mobile bottom sheet.
 */
export function usePluginTaskLinkActions(
  context: PluginTaskActionContext | null,
): PluginLinkMenuAction[] {
  const registry = usePluginRegistry();
  const beforePluginRun = useContext(PluginTaskLinkActionSurfaceContext);
  return useMemo(() => {
    if (!context) return [];
    const actionContext: PluginTaskActionContext = Object.freeze({
      ...context,
      repositories: Object.freeze([...context.repositories]),
    });
    return registry
      .getTaskActions("link")
      .filter((action) => action.visible?.(actionContext) !== false)
      .map((action) => ({
        id: `${action.pluginId}:${action.id}`,
        label: action.label,
        onSelect: () => {
          runPluginTaskLinkAction(beforePluginRun, () => action.run(actionContext));
        },
      }));
  }, [beforePluginRun, context, registry]);
}

/** Builds the immutable action context shared by task-card and task-row menus. */
export function useTaskPluginLinkActions(taskId: string, repositories: readonly unknown[] = []) {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const pathname = usePathname() ?? "";
  const { isMobile } = useResponsiveBreakpoint();
  return usePluginTaskLinkActions(
    workspaceId
      ? {
          workspaceId,
          taskId,
          repositories,
          pathname,
          presentation: isMobile ? "mobile" : "desktop",
        }
      : null,
  );
}

export function useSidebarLinkActions(store: StoreApi) {
  const [linkingPullRequestTask, setLinkingPullRequestTask] = useState<SidebarLinkTarget | null>(
    null,
  );
  const [linkingIssueTask, setLinkingIssueTask] = useState<SidebarLinkTarget | null>(null);
  const [linkingMergeRequestTask, setLinkingMergeRequestTask] = useState<SidebarLinkTarget | null>(
    null,
  );
  const [linkingExternalIssueTask, setLinkingExternalIssueTask] =
    useState<SidebarExternalLinkTarget | null>(null);

  const getLinkTarget = useCallback(
    (taskId: string, fallbackTitle?: string): SidebarLinkTarget => {
      const state = store.getState();
      const task = findTaskInSnapshots(taskId, state.kanbanMulti.snapshots, state.kanban.tasks);
      return {
        id: taskId,
        title: task?.title ?? fallbackTitle ?? "this task",
        repositoryId: task?.repositoryId,
        issueUrl: task?.issueUrl,
        issueNumber: task?.issueNumber,
        repositories: task?.repositories,
      };
    },
    [store],
  );

  const handleLinkPullRequestTask = useCallback(
    (taskId: string, fallbackTitle?: string) => {
      setLinkingPullRequestTask(getLinkTarget(taskId, fallbackTitle));
    },
    [getLinkTarget],
  );

  const handleLinkIssueTask = useCallback(
    (taskId: string, fallbackTitle?: string) => {
      setLinkingIssueTask(getLinkTarget(taskId, fallbackTitle));
    },
    [getLinkTarget],
  );

  const handleLinkMergeRequestTask = useCallback(
    (taskId: string, fallbackTitle?: string) => {
      setLinkingMergeRequestTask(getLinkTarget(taskId, fallbackTitle));
    },
    [getLinkTarget],
  );

  const handleLinkExternalIssueTask = useCallback(
    (provider: ExternalLinkProvider, taskId: string, fallbackTitle?: string) => {
      setLinkingExternalIssueTask({ provider, task: getLinkTarget(taskId, fallbackTitle) });
    },
    [getLinkTarget],
  );

  const handleLinkJiraTicketTask = useCallback(
    (taskId: string, fallbackTitle?: string) =>
      handleLinkExternalIssueTask("jira", taskId, fallbackTitle),
    [handleLinkExternalIssueTask],
  );

  const handleLinkLinearIssueTask = useCallback(
    (taskId: string, fallbackTitle?: string) =>
      handleLinkExternalIssueTask("linear", taskId, fallbackTitle),
    [handleLinkExternalIssueTask],
  );

  const handleLinkSentryIssueTask = useCallback(
    (taskId: string, fallbackTitle?: string) =>
      handleLinkExternalIssueTask("sentry", taskId, fallbackTitle),
    [handleLinkExternalIssueTask],
  );

  return {
    linkingPullRequestTask,
    setLinkingPullRequestTask,
    handleLinkPullRequestTask,
    linkingIssueTask,
    setLinkingIssueTask,
    handleLinkIssueTask,
    linkingMergeRequestTask,
    setLinkingMergeRequestTask,
    handleLinkMergeRequestTask,
    linkingExternalIssueTask,
    setLinkingExternalIssueTask,
    handleLinkJiraTicketTask,
    handleLinkLinearIssueTask,
    handleLinkSentryIssueTask,
  };
}
