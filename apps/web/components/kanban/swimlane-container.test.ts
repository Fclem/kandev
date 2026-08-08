import { describe, expect, it, vi } from "vitest";
import type { Task } from "@/components/kanban-card";
import {
  filterTasks,
  selectMobileNavigatorWorkflows,
  selectWorkflowSwimlanes,
} from "./swimlane-container";
import { mapSelectedRepositoryIds } from "@/lib/kanban/filters";

const IMPROVE_WORKFLOW_NAME = "Improve Kandev";

function makeTask(overrides: Partial<Task> & { id: string }): Task {
  return {
    title: "Task",
    description: "",
    repositoryId: undefined,
    ...overrides,
  } as Task;
}

describe("filterTasks — plugin task filter predicate", () => {
  const NO_REPO_FILTER = mapSelectedRepositoryIds([], []);

  it("keeps every task when no predicate is supplied", () => {
    const snapshots = {
      wf: { tasks: [makeTask({ id: "1" }), makeTask({ id: "2" })] },
    };

    const result = filterTasks(snapshots, "wf", NO_REPO_FILTER);

    expect(result.map((t) => t.id)).toEqual(["1", "2"]);
  });

  it("excludes tasks the predicate rejects", () => {
    const snapshots = {
      wf: { tasks: [makeTask({ id: "1" }), makeTask({ id: "2" })] },
    };
    const matches = vi.fn((taskId: string) => taskId === "1");

    const result = filterTasks(snapshots, "wf", NO_REPO_FILTER, undefined, matches);

    expect(result.map((t) => t.id)).toEqual(["1"]);
    expect(matches).toHaveBeenCalledWith("1");
    expect(matches).toHaveBeenCalledWith("2");
  });

  it("composes with search and repository filtering (all must pass)", () => {
    const snapshots = {
      wf: {
        tasks: [
          makeTask({ id: "1", title: "Fix bug", repositoryId: "repo-a" }),
          makeTask({ id: "2", title: "Fix bug", repositoryId: "repo-b" }),
        ],
      },
    };
    const repoFilter = mapSelectedRepositoryIds(
      [{ id: "repo-a", name: "A" } as never, { id: "repo-b", name: "B" } as never],
      ["repo-a", "repo-b"],
    );
    const matches = (taskId: string) => taskId === "1";

    const result = filterTasks(snapshots, "wf", repoFilter, "fix", matches);

    expect(result.map((t) => t.id)).toEqual(["1"]);
  });
});

describe("selectWorkflowSwimlanes — hidden workflow filter resolution", () => {
  const workflows = [
    { id: "dev", name: "Development", hidden: false },
    { id: "improve", name: IMPROVE_WORKFLOW_NAME, hidden: true },
  ];
  const snapshots = {
    dev: { workflowName: "Development" },
    improve: { workflowName: IMPROVE_WORKFLOW_NAME },
  };

  it("keeps hidden workflows off the All Workflows board", () => {
    expect(selectWorkflowSwimlanes(null, workflows, snapshots).map((w) => w.id)).toEqual(["dev"]);
  });

  it("renders a hidden workflow the user explicitly selects", () => {
    expect(selectWorkflowSwimlanes("improve", workflows, snapshots).map((w) => w.id)).toEqual([
      "improve",
    ]);
  });

  it("does not render a hidden workflow before its snapshot has loaded", () => {
    expect(selectWorkflowSwimlanes("improve", workflows, {})).toEqual([]);
  });

  it("still renders a visible workflow under an explicit filter", () => {
    expect(selectWorkflowSwimlanes("dev", workflows, snapshots).map((w) => w.id)).toEqual(["dev"]);
  });

  it("returns no workflows for a filter that does not exist in the store", () => {
    expect(selectWorkflowSwimlanes("missing", workflows, snapshots)).toEqual([]);
  });
});

describe("selectMobileNavigatorWorkflows — mobile board navigator options", () => {
  const workflows = [
    { id: "dev", name: "Development", hidden: false },
    { id: "improve", name: IMPROVE_WORKFLOW_NAME, hidden: true },
    { id: "report", name: "Report Kandev Issue", hidden: true },
  ];
  const visibleOrdered = [{ id: "dev", name: "Development", hidden: false }];
  const noTasks = () => [];
  const tasks = (workflowId: string) => (workflowId === "improve" ? [{ id: "t1" }] : []);

  it("lists visible workflows plus hidden workflows that have tasks", () => {
    expect(
      selectMobileNavigatorWorkflows(visibleOrdered, workflows, tasks).map((w) => w.id),
    ).toEqual(["dev", "improve"]);
  });

  it("keeps empty hidden workflows out of the navigator", () => {
    expect(
      selectMobileNavigatorWorkflows(visibleOrdered, workflows, noTasks).map((w) => w.id),
    ).toEqual(["dev"]);
  });

  it("does not duplicate a workflow already in the visible list", () => {
    const both = [
      { id: "dev", name: "Development", hidden: false },
      { id: "improve", name: "Improve Kandev", hidden: true },
    ];
    const withTasks = () => [{ id: "t1" }];
    expect(selectMobileNavigatorWorkflows(both, workflows, withTasks).map((w) => w.id)).toEqual([
      "dev",
      "improve",
      "report",
    ]);
  });
});
