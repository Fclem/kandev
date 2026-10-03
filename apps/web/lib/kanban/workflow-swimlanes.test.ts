import { describe, expect, it } from "vitest";
import {
  selectMobileNavigatorWorkflows,
  selectWorkflowSwimlanes,
  selectVisibleWorkflows,
} from "./workflow-swimlanes";

const IMPROVE_WORKFLOW_NAME = "Improve Kandev";

describe("selectWorkflowSwimlanes — hidden workflow filter resolution", () => {
  const workflows = [
    { id: "dev", name: "Development", hidden: false },
    {
      id: "improve",
      name: IMPROVE_WORKFLOW_NAME,
      hidden: true,
      workflowTemplateId: "improve-kandev",
    },
    { id: "report", name: "Open issue", hidden: true, workflowTemplateId: "report-kandev-issue" },
    { id: "unrelated", name: "Internal", hidden: true, workflowTemplateId: "internal" },
  ];
  const snapshots = {
    dev: { workflowName: "Development" },
    improve: { workflowName: IMPROVE_WORKFLOW_NAME },
    report: { workflowName: "Open issue" },
    unrelated: { workflowName: "Internal" },
  };

  it("includes task-bearing Improve Kandev templates, but no unrelated hidden workflows", () => {
    expect(selectWorkflowSwimlanes(null, workflows, snapshots).map((w) => w.id)).toEqual([
      "dev",
      "improve",
      "report",
    ]);
  });

  it("keeps explicit workflow selection scoped to the selected lane", () => {
    expect(selectWorkflowSwimlanes("report", workflows, snapshots).map((w) => w.id)).toEqual([
      "report",
    ]);
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
    {
      id: "improve",
      name: IMPROVE_WORKFLOW_NAME,
      hidden: true,
      workflowTemplateId: "improve-kandev",
    },
    { id: "report", name: "Open issue", hidden: true, workflowTemplateId: "report-kandev-issue" },
    { id: "other", name: "Internal", hidden: true, workflowTemplateId: "internal" },
  ];
  const visibleOrdered = [{ id: "dev", name: "Development", hidden: false }];
  const noTasks = () => [];
  const tasks = (workflowId: string) => (workflowId === "improve" ? [{ id: "t1" }] : []);

  it("lists visible workflows and Improve templates regardless of filtered task counts", () => {
    const entries = selectMobileNavigatorWorkflows(visibleOrdered, workflows, tasks);
    expect(entries.map((entry) => entry.workflow.id)).toEqual(["dev", "improve", "report"]);
  });

  it("keeps empty Improve templates available but excludes other empty hidden workflows", () => {
    expect(
      selectMobileNavigatorWorkflows(visibleOrdered, workflows, noTasks).map(
        (entry) => entry.workflow.id,
      ),
    ).toEqual(["dev", "improve", "report"]);
  });
  it("keeps both Improve workflows in the navigator even when task filters empty them", () => {
    expect(
      selectMobileNavigatorWorkflows(visibleOrdered, workflows, noTasks).map(
        (entry) => entry.workflow.id,
      ),
    ).toEqual(["dev", "improve", "report"]);
  });

  it("keeps a hidden workflow whose live steps are all hidden", () => {
    type NavigatorSelectorWithHiddenSteps = (
      visibleWorkflows: typeof visibleOrdered,
      allWorkflows: typeof workflows,
      getTasks: (workflowId: string) => unknown[],
      hasLiveHiddenSteps: (workflowId: string) => boolean,
    ) => Array<{ workflow: (typeof workflows)[number]; tasks: unknown[] }>;

    // The fourth argument is the regression contract. The cast keeps this
    // test executable before the implementation grows that argument, so the
    // red phase fails on the assertion rather than on TypeScript arity.
    const selectWithHiddenSteps =
      selectMobileNavigatorWorkflows as unknown as NavigatorSelectorWithHiddenSteps;
    const entries = selectWithHiddenSteps(
      visibleOrdered,
      workflows,
      noTasks,
      (workflowId) => workflowId === "improve",
    );

    expect(entries.map((entry) => entry.workflow.id)).toEqual(["dev", "improve", "report"]);
  });

  it("returns the filtered tasks alongside each workflow so callers reuse the result", () => {
    const entries = selectMobileNavigatorWorkflows(visibleOrdered, workflows, tasks);
    expect(entries.find((entry) => entry.workflow.id === "improve")?.tasks).toEqual([{ id: "t1" }]);
  });

  // `both` marks the hidden improve workflow as already visible. `withTasks`
  // gives every workflow tasks, so the hidden `report` workflow from the
  // module-scope fixture is added too — the point is that improve is not
  // duplicated just because it is both hidden and already in the visible list.
  it("does not duplicate templates already in visible order", () => {
    const both = [
      { id: "dev", name: "Development", hidden: false },
      { id: "improve", name: IMPROVE_WORKFLOW_NAME, hidden: true },
    ];
    expect(
      selectMobileNavigatorWorkflows(both, workflows, noTasks).map((entry) => entry.workflow.id),
    ).toEqual(["dev", "improve", "report"]);
  });
});

describe("selectVisibleWorkflows — lane retention for hidden columns", () => {
  const WF_A = { id: "wf-a", name: "A" };
  const WF_B = { id: "wf-b", name: "B" };
  const ordered = [WF_A, WF_B];
  const noneHidden = () => false;

  it("drops a task-less workflow whose hidden set is empty, as before the feature", () => {
    const result = selectVisibleWorkflows({
      workflowFilter: null,
      orderedWorkflows: ordered,
      hasTasks: (id) => id === WF_B.id,
      hasLiveHiddenSteps: noneHidden,
      showEmptyBoard: false,
    });

    expect(result.map((w) => w.id)).toEqual([WF_B.id]);
  });

  it("retains a task-less workflow that has a live hidden step, so its menu stays reachable", () => {
    const result = selectVisibleWorkflows({
      workflowFilter: null,
      orderedWorkflows: ordered,
      hasTasks: (id) => id === WF_B.id,
      hasLiveHiddenSteps: (id) => id === WF_A.id,
      showEmptyBoard: false,
    });

    expect(result.map((w) => w.id)).toEqual([WF_A.id, WF_B.id]);
  });

  it("preserves the board's workflow order when retaining", () => {
    const result = selectVisibleWorkflows({
      workflowFilter: null,
      orderedWorkflows: ordered,
      hasTasks: () => false,
      hasLiveHiddenSteps: () => true,
      showEmptyBoard: false,
    });

    expect(result.map((w) => w.id)).toEqual([WF_A.id, WF_B.id]);
  });

  it("does not retain on a stale hidden id — the predicate is the live intersection", () => {
    // `hasLiveHiddenSteps` is defined as H ∩ liveStepIds, so a hidden set made
    // only of deleted step ids reports false and cannot hold a lane open.
    const result = selectVisibleWorkflows({
      workflowFilter: null,
      orderedWorkflows: ordered,
      hasTasks: (id) => id === WF_B.id,
      hasLiveHiddenSteps: noneHidden,
      showEmptyBoard: false,
    });

    expect(result.map((w) => w.id)).toEqual([WF_B.id]);
  });

  it("returns every workflow when an explicit workflow filter is set", () => {
    const result = selectVisibleWorkflows({
      workflowFilter: WF_A.id,
      orderedWorkflows: ordered,
      hasTasks: () => false,
      hasLiveHiddenSteps: noneHidden,
      showEmptyBoard: false,
    });

    expect(result.map((w) => w.id)).toEqual([WF_A.id, WF_B.id]);
  });

  it("falls back to every workflow on an empty mobile board", () => {
    const result = selectVisibleWorkflows({
      workflowFilter: null,
      orderedWorkflows: ordered,
      hasTasks: () => false,
      hasLiveHiddenSteps: noneHidden,
      showEmptyBoard: true,
    });

    expect(result.map((w) => w.id)).toEqual([WF_A.id, WF_B.id]);
  });
});
