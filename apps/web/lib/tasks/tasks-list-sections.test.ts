import { describe, expect, it } from "vitest";
import type { TaskListFacetValue } from "@/lib/plugins/types";
import { taskId, type Task } from "@/lib/types/http";
import { buildTaskSections, flattenTaskTree } from "./tasks-list-sections";

const FACET = "facet:plugin:tags";
const translate = (key: string) =>
  ({
    "sidebar:groupUnassigned": "Unassigned",
    "tasks:noWorkflow": "No workflow",
    "tasks:noRepository": "No repository",
  })[key] ?? key;

function task(id: string, parentId?: string): Task {
  return {
    id: taskId(id),
    title: id,
    state: "WAITING_FOR_INPUT",
    parent_id: parentId ? taskId(parentId) : undefined,
  } as Task;
}

function sections(tasks: Task[], values: Record<string, TaskListFacetValue[]>) {
  return buildTaskSections(tasks, {
    groupBy: FACET,
    workflowMap: new Map(),
    repoMap: new Map(),
    facetValues: values,
    translate,
  });
}

describe("facet value sections", () => {
  it("includes only carried values, keeps per-bucket trees, color and multi-membership", () => {
    const parent = task("parent");
    const child = task("child", "parent");
    const result = sections([parent, child], {
      [`${FACET}:parent`]: [
        { value: "alpha", label: "Alpha", color: "#abcdef" },
        { value: "beta", label: "Beta" },
        { value: "beta", label: "Beta" },
      ],
      [`${FACET}:child`]: [{ value: "alpha", label: "Alpha" }],
      [`${FACET}:other`]: [{ value: "unused", label: "Not on page" }],
    });
    expect(result.map((section) => section.key)).toEqual(["facet:value:alpha", "facet:value:beta"]);
    expect(result[0].color).toBe("#abcdef");
    expect(flattenTaskTree(result[0].nodes).map((node) => [node.task.id, node.level])).toEqual([
      [parent.id, 0],
      [child.id, 1],
    ]);
    expect(flattenTaskTree(result[1].nodes).map((node) => node.task.id)).toEqual([parent.id]);
  });

  it("sorts labels case-insensitively, retains same-base input order, then appends Unassigned", () => {
    const [a, b, c] = [task("a"), task("b"), task("c")];
    const result = sections([a, b, c], {
      [`${FACET}:a`]: [{ value: "upper", label: "Beta" }],
      [`${FACET}:b`]: [{ value: "lower", label: "beta" }],
      [`${FACET}:c`]: [{ value: "empty", label: "" }],
    });
    expect(result.map((section) => [section.key, section.title])).toEqual([
      ["facet:value:upper", "Beta"],
      ["facet:value:lower", "beta"],
      ["facet:host:ungrouped", "Unassigned"],
    ]);
    expect(flattenTaskTree(result[2].nodes).map((node) => node.task.id)).toEqual([c.id]);
  });

  it("keeps valid sibling labels and namespaced host keys separate from plugin values", () => {
    const [a, b] = [task("a"), task("b")];
    const result = sections([a, b], {
      [`${FACET}:a`]: [
        { value: "missing", label: "" },
        { value: "__host_ungrouped__", label: "Zulu" },
      ],
    });
    expect(result.map((section) => section.key)).toEqual([
      "facet:value:__host_ungrouped__",
      "facet:host:ungrouped",
    ]);
    expect(flattenTaskTree(result[0].nodes).map((node) => node.task.id)).toEqual([a.id]);
    expect(flattenTaskTree(result[1].nodes).map((node) => node.task.id)).toEqual([b.id]);
  });

  // AC-PLUGINS-TASKLIST-FACETS-002.4: a child without its parent in a value bucket is a root.
  it("renders a child at the section root when its parent carries another value", () => {
    const parent = task("parent");
    const child = task("child", "parent");
    const result = sections([parent, child], {
      [`${FACET}:parent`]: [{ value: "alpha", label: "Alpha" }],
      [`${FACET}:child`]: [{ value: "beta", label: "Beta" }],
    });

    expect(result.map((section) => section.title)).toEqual(["Alpha", "Beta"]);
    expect(flattenTaskTree(result[0].nodes).map((node) => [node.task.id, node.level])).toEqual([
      [parent.id, 0],
    ]);
    expect(flattenTaskTree(result[1].nodes).map((node) => [node.task.id, node.level])).toEqual([
      [child.id, 0],
    ]);
  });
});
