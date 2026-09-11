import type { AutomationRun, RetryHistoryMode } from "@/lib/types/automation";

export function projectAutomationHistory(
  runs: AutomationRun[],
  mode: RetryHistoryMode = "attempts",
): AutomationRun[] {
  if (mode === "attempts") return runs;
  const groups = new Map<string, AutomationRun>();
  for (const run of runs) {
    const key = run.retry_group_id ? `retry:${run.retry_group_id}` : `legacy:${run.id}`;
    const current = groups.get(key);
    if (!current || (run.attempt_number ?? 1) > (current.attempt_number ?? 1)) {
      groups.set(key, run);
    }
  }
  return [...groups.values()].sort((left, right) =>
    right.created_at.localeCompare(left.created_at),
  );
}
