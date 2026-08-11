import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ensureTaskSession } from "@/lib/services/session-launch-service";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import { useAppStore } from "@/components/state-provider";

/** Minimal task shape consumed by useEnsureTaskSession. */
export type EnsureTaskInput = {
  id?: string | null;
  /** Snake_case workflow_step_id from the HTTP Task type. */
  workflowStepId?: string | null;
  /** Snake_case workflow_id from the HTTP Task type. */
  workflowId?: string | null;
} | null;

export type KanbanStepLike = {
  id: string;
  position: number;
};

/**
 * Reports whether the step is the terminal step of its workflow. Terminal is
 * the maximum under the `(position, id)` ordering (ties broken by max id):
 * step positions are caller-supplied and not uniqueness-validated, so equal
 * positions are representable and must not make both steps terminal.
 * Missing step id or an empty step list → not final (no gate).
 */
export function isFinalWorkflowStep(
  workflowStepId: string | null | undefined,
  steps: KanbanStepLike[] | undefined,
): boolean {
  if (!workflowStepId || !steps || steps.length === 0) return false;
  const current = steps.find((s) => s.id === workflowStepId);
  if (!current) return false;
  const terminal = steps.reduce<typeof current | null>((best, step) => {
    if (!best) return step;
    if (step.position > best.position) return step;
    if (step.position === best.position && step.id > best.id) return step;
    return best;
  }, null);
  return terminal?.id === current.id;
}

export type EnsureTaskSessionStatus = "idle" | "preparing" | "error";

export type UseEnsureTaskSessionResult = {
  status: EnsureTaskSessionStatus;
  error: Error | null;
  retry: () => void;
};

/**
 * Ensures the task has at least one session by delegating to the backend's
 * idempotent `session.ensure` endpoint. The backend resolves the agent profile
 * (task metadata → workflow step → workflow → workspace default) and chooses
 * prepare vs start based on the workflow step's auto_start_agent action — the
 * frontend stays thin and contract-free.
 *
 * Behavior:
 * - No-op while sessions are still loading.
 * - No-op when the task already has at least one session.
 * - No-op when `enabled === false` or `task.id` is missing.
 * - Idempotent per task id within a mount; switching tasks resets the latch.
 */
export function useEnsureTaskSession(
  task: EnsureTaskInput,
  opts?: { enabled?: boolean },
): UseEnsureTaskSessionResult {
  const enabled = opts?.enabled ?? true;
  const taskId = task?.id ?? null;
  const { sessions, isLoaded, loadSessions } = useTaskSessions(taskId);

  const [status, setStatus] = useState<EnsureTaskSessionStatus>("idle");
  const [error, setError] = useState<Error | null>(null);
  const [retryToken, setRetryToken] = useState(0);

  // Prevent-auto-start preference + workflow-aware step resolution. The
  // terminal-step gate applies to the task's OWN workflow: the active
  // workflow's steps (state.kanban.steps) when the task is in it, otherwise
  // the multi-workflow snapshot's steps for that workflow.
  const preventAutoStart = useAppStore((state) => state.userSettings.preventAutoStartAgentOnOpen);
  const kanbanWorkflowId = useAppStore((state) => state.kanban.workflowId);
  const kanbanSteps = useAppStore((state) => state.kanban.steps);
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const taskWorkflowId = task?.workflowId ?? null;

  const isFinalStep = useMemo(() => {
    if (!preventAutoStart || !task?.workflowStepId) return false;
    const snapshot = taskWorkflowId ? snapshots[taskWorkflowId] : undefined;
    let steps: KanbanStepLike[] | undefined;
    if (snapshot !== undefined) {
      steps = snapshot.steps;
    } else if (taskWorkflowId === null || taskWorkflowId === kanbanWorkflowId) {
      steps = kanbanSteps;
    } else {
      steps = [];
    }
    return isFinalWorkflowStep(task.workflowStepId, steps);
  }, [
    preventAutoStart,
    task?.workflowStepId,
    taskWorkflowId,
    kanbanWorkflowId,
    kanbanSteps,
    snapshots,
  ]);

  // Latch keyed by `${taskId}:${retryToken}` so a re-mount on the same task
  // doesn't refire, but switching tasks or calling retry() does.
  const launchedKeyRef = useRef<string | null>(null);
  const previousTaskIdRef = useRef<string | null>(taskId);

  /* eslint-disable react-hooks/set-state-in-effect -- task changes must clear stale ensure-session errors before early returns */
  useEffect(() => {
    if (previousTaskIdRef.current === taskId) return;
    previousTaskIdRef.current = taskId;
    launchedKeyRef.current = null;
    setStatus("idle");
    setError(null);
  }, [taskId]);
  /* eslint-enable react-hooks/set-state-in-effect */

  /* eslint-disable react-hooks/set-state-in-effect -- ensuring a session is a side effect; status mirrors that external work */
  useEffect(() => {
    if (!enabled || !taskId || !isLoaded) return;
    if (sessions.length > 0) return;
    const key = `${taskId}:${retryToken}`;
    if (launchedKeyRef.current === key) return;
    launchedKeyRef.current = key;

    // Cancel guard so a stale resolution can't overwrite a switched-away task.
    let cancelled = false;
    setStatus("preparing");
    setError(null);
    const ensurePromise = isFinalStep
      ? ensureTaskSession(taskId, { autoStart: false })
      : ensureTaskSession(taskId);
    ensurePromise
      .then(async () => {
        if (cancelled || launchedKeyRef.current !== key) return;
        // Force-reload: backend may have returned an existing_* source our initial list missed.
        await loadSessions(true);
        if (cancelled || launchedKeyRef.current !== key) return;
        setStatus("idle");
      })
      .catch((err: unknown) => {
        if (cancelled || launchedKeyRef.current !== key) return;
        setStatus("error");
        setError(err instanceof Error ? err : new Error(String(err)));
        launchedKeyRef.current = null;
      });

    return () => {
      cancelled = true;
    };
  }, [enabled, taskId, isLoaded, loadSessions, sessions.length, retryToken]);
  /* eslint-enable react-hooks/set-state-in-effect */

  const retry = useCallback(() => {
    setStatus("idle");
    setError(null);
    setRetryToken((n) => n + 1);
  }, []);

  return { status, error, retry };
}
