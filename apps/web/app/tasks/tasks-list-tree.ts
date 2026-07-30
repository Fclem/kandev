/**
 * Grouping and tree-shaping for the tasks list. Pure data helpers, split from
 * `tasks-list-view.tsx` to keep that file under the size limit — nothing here
 * renders, so it is also directly testable.
 */
import { t } from "@/lib/i18n";
import { primaryTaskRepository, type Task } from "@/lib/types/http";
import { formatTaskStateLabel } from "@/lib/ui/state-labels";
import { TASK_STATE_ORDER, type TasksListGroup } from "@/lib/tasks/tasks-list-options";

export type TaskTreeNode = {
  task: Task;
  children: TaskTreeNode[];
  level: number;
};

export type TaskListSection = {
  key: string;
  title: string | null;
  nodes: TaskTreeNode[];
};

export function buildTaskTree(tasks: Task[]): TaskTreeNode[] {
  const childrenByParent = new Map<string, Task[]>();
  const taskIds = new Set(tasks.map((task) => task.id));
  const roots: Task[] = [];

  for (const task of tasks) {
    if (task.parent_id && taskIds.has(task.parent_id)) {
      const siblings = childrenByParent.get(task.parent_id) ?? [];
      siblings.push(task);
      childrenByParent.set(task.parent_id, siblings);
    } else {
      roots.push(task);
    }
  }

  const visited = new Set<string>();

  const buildNode = (task: Task, level: number): TaskTreeNode | null => {
    if (visited.has(task.id)) return null;
    visited.add(task.id);
    return {
      task,
      level,
      children: (childrenByParent.get(task.id) ?? [])
        .map((child) => buildNode(child, level + 1))
        .filter((node): node is TaskTreeNode => node !== null),
    };
  };

  const nodes = roots
    .map((task) => buildNode(task, 0))
    .filter((node): node is TaskTreeNode => node !== null);
  for (const task of tasks) {
    const node = buildNode(task, 0);
    if (node) nodes.push(node);
  }

  return nodes;
}

export function groupForTask(
  task: Task,
  groupBy: TasksListGroup,
  workflowMap: Map<string, string>,
  repoMap: Map<string, string>,
) {
  if (groupBy === "workflow") {
    const title = workflowMap.get(task.workflow_id);
    if (!title) return { key: "workflow:none", title: t("common:noWorkflow") };
    return { key: `workflow:${task.workflow_id || "none"}`, title };
  }
  if (groupBy === "repository") {
    const primaryRepo = primaryTaskRepository(task.repositories);
    if (!primaryRepo) return { key: "repository:none", title: t("common:noRepository") };
    const repoId = primaryRepo?.repository_id ?? "none";
    const title = repoMap.get(repoId);
    if (!title) return { key: "repository:none", title: t("common:noRepository") };
    return { key: `repository:${repoId}`, title };
  }
  const title = formatTaskStateLabel(task.state);
  return { key: `state:${task.state}`, title };
}

export function compareSection(
  a: TaskListSection,
  b: TaskListSection,
  groupBy: TasksListGroup,
): number {
  if (groupBy === "state") {
    const aIndex = TASK_STATE_ORDER.indexOf(a.key.replace("state:", "") as Task["state"]);
    const bIndex = TASK_STATE_ORDER.indexOf(b.key.replace("state:", "") as Task["state"]);
    return (
      (aIndex === -1 ? Number.MAX_SAFE_INTEGER : aIndex) -
      (bIndex === -1 ? Number.MAX_SAFE_INTEGER : bIndex)
    );
  }
  return (a.title ?? "").localeCompare(b.title ?? "", undefined, { sensitivity: "base" });
}

export function flattenTaskTree(nodes: TaskTreeNode[]): TaskTreeNode[] {
  return nodes.flatMap((node) => [node, ...flattenTaskTree(node.children)]);
}
