import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { t } from "i18next";
import type { Task } from "@/app/office/tasks/[id]/types";
import { TaskProperties } from "./task-properties";

const mockAppState = vi.hoisted(() => ({
  auth: {
    mode: "enabled",
    authenticated: true,
    user: { id: "user-1" } as { id: string } | null,
    ssoProviders: [],
  },
}));

const treeApi = vi.hoisted(() => ({ getTreeCostSummary: vi.fn() }));
vi.mock("@/lib/api/domains/tree-api", () => treeApi);

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockAppState) => unknown) => selector(mockAppState),
}));

vi.mock("./components/status-picker", () => ({ StatusPicker: () => null }));
vi.mock("./components/priority-picker", () => ({ PriorityPicker: () => null }));
vi.mock("./components/labels-picker", () => ({ LabelsPicker: () => null }));
vi.mock("./components/assignee-picker", () => ({ AssigneePicker: () => null }));
vi.mock("./components/human-assignee-picker", () => ({
  HumanAssigneePicker: () => <span data-testid="human-assignee-picker-trigger" />,
}));
vi.mock("./components/project-picker", () => ({ ProjectPicker: () => null }));
vi.mock("./components/parent-picker", () => ({ ParentPicker: () => null }));
vi.mock("./components/blockers-picker", () => ({ BlockersPicker: () => null }));
vi.mock("./components/sub-issues-row", () => ({ SubIssuesRow: () => null }));
vi.mock("./components/reviewers-picker", () => ({ ReviewersPicker: () => null }));
vi.mock("./components/approvers-picker", () => ({ ApproversPicker: () => null }));
vi.mock("./components/pending-approval-badge", () => ({ PendingApprovalBadge: () => null }));
vi.mock("./components/quorum-status-badge", () => ({ QuorumStatusBadge: () => null }));

const TASK = {
  id: "task-1",
  workspaceId: "workspace-1",
  identifier: "TASK-1",
  title: "Auth gate task",
  status: "todo",
  priority: "medium",
  labels: [],
  blockedBy: [],
  blocking: [],
  children: [],
  reviewers: [],
  approvers: [],
  decisions: [],
  createdBy: "user-1",
  createdAt: "2026-09-05T12:00:00Z",
  updatedAt: "2026-09-05T12:00:00Z",
} satisfies Task;

afterEach(() => {
  cleanup();
  mockAppState.auth.mode = "enabled";
  mockAppState.auth.user = { id: "user-1" };
});

describe("TaskProperties human assignee", () => {
  it.each(["disabled", "setup"] as const)("hides the row in %s auth mode", (mode) => {
    mockAppState.auth.mode = mode;
    mockAppState.auth.user = { id: "default-user" };

    render(<TaskProperties task={TASK} />);

    expect(screen.queryByText("Assigned to", { exact: true })).toBeNull();
  });

  it("hides the row when enabled auth has no signed-in user", () => {
    mockAppState.auth.user = null;

    render(<TaskProperties task={TASK} />);

    expect(screen.queryByText("Assigned to", { exact: true })).toBeNull();
  });

  it("shows the row when authentication is enabled for a signed-in user", () => {
    render(<TaskProperties task={TASK} />);

    expect(screen.getByText("Assigned to", { exact: true })).toBeTruthy();
    expect(screen.getByTestId("human-assignee-picker-trigger")).toBeTruthy();
  });
});

describe("TaskProperties subtree count precision", () => {
  it.each([
    ["9007199254740993", "-"],
    ["9007199254740991", "9,007,199,254,740,991"],
    ["0", "0"],
    ["-1", "-"],
    ["0.5", "-"],
    ["null", "-"],
  ])("does not present an invalid or rounded count as exact (%s)", async (wire, expected) => {
    treeApi.getTreeCostSummary.mockResolvedValue(
      JSON.parse(
        `{"task_id":"task-1","task_count":2,"include_descendants":true,"cost_subcents":12500,"tokens_in":${wire},"tokens_out":${wire},"tokens_cached_in":${wire}}`,
      ),
    );
    render(
      <TaskProperties
        task={{
          ...TASK,
          children: [{ id: "child", identifier: "CHILD-1", title: "Child", status: "todo" }],
        }}
      />,
    );
    await waitFor(() =>
      expect(screen.getByText("Input tokens").nextElementSibling?.textContent).toBe(expected),
    );
    expect(screen.getByText("Output tokens").nextElementSibling?.textContent).toBe(expected);
    expect(screen.getByText("Cached input").nextElementSibling?.textContent).toBe(expected);
    expect(screen.getByText("$1.25")).toBeTruthy();
  });
});

describe("TaskProperties subtree monetary coverage", () => {
  it.each([
    { name: "gap only", amount: null, degraded: true, expected: "-" },
    { name: "unpriced only", amount: null, degraded: true, expected: "-" },
    { name: "no observations", amount: null, degraded: false, expected: "-" },
    { name: "mixed known spend and gap", amount: 12500, degraded: true, expected: "$1.25" },
    { name: "measured zero", amount: 0, degraded: false, expected: "$0.00" },
    { name: "legacy measured spend", amount: 7500, degraded: false, expected: "$0.75" },
  ])("distinguishes monetary coverage for $name", async ({ amount, degraded, expected }) => {
    treeApi.getTreeCostSummary.mockResolvedValue({
      task_id: TASK.id,
      task_count: 2,
      include_descendants: true,
      cost_subcents: amount,
      coverage_degraded: degraded,
      tokens_in: null,
      tokens_out: null,
      tokens_cached_in: null,
    });
    render(
      <TaskProperties
        task={{
          ...TASK,
          children: [{ id: "child", identifier: "CHILD-1", title: "Child", status: "todo" }],
        }}
      />,
    );
    await waitFor(() =>
      expect(screen.getByText("Tree cost").nextElementSibling?.textContent).toContain(expected),
    );
    const notice = screen.queryByText(t("office:costCoverageIncomplete"));
    if (degraded) expect(notice).toBeTruthy();
    else expect(notice).toBeNull();
  });
});
