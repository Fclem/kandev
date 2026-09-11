import { describe, expect, it } from "vitest";
import type { AutomationRun } from "@/lib/types/automation";
import { projectAutomationHistory } from "./automation-history";

function run(overrides: Partial<AutomationRun>): AutomationRun {
  return {
    id: "run-1",
    automation_id: "automation-1",
    trigger_id: "trigger-1",
    trigger_type: "scheduled",
    task_id: "task-1",
    status: "failed",
    dedup_key: "",
    trigger_data: {},
    error_message: "failed",
    created_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("projectAutomationHistory", () => {
  it("keeps every attempt in attempts mode", () => {
    const runs = [
      run({ id: "attempt-1", retry_group_id: "group-1", attempt_number: 1 }),
      run({ id: "attempt-2", retry_group_id: "group-1", attempt_number: 2 }),
    ];

    expect(projectAutomationHistory(runs, "attempts")).toEqual(runs);
  });

  it("collapses a retry group to its latest attempt in timeline mode", () => {
    const latest = run({
      id: "attempt-2",
      retry_group_id: "group-1",
      attempt_number: 2,
      created_at: "2026-01-01T00:01:00Z",
    });
    const runs = [
      run({ id: "attempt-1", retry_group_id: "group-1", attempt_number: 1 }),
      latest,
      run({ id: "standalone", attempt_number: 1, created_at: "2026-01-01T00:02:00Z" }),
    ];

    expect(projectAutomationHistory(runs, "timeline")).toEqual([
      run({ id: "standalone", attempt_number: 1, created_at: "2026-01-01T00:02:00Z" }),
      latest,
    ]);
  });
});
