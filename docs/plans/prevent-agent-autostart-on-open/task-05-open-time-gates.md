---
id: "05-open-time-gates"
title: "Open-time gates in ensure and resume hooks"
status: pending
wave: 1
parallelism: sequential
depends_on: ["03-frontend-settings-plumbing"]
plan: "plan.md"
spec: "../../specs/prevent-agent-autostart-on-open/spec.md"
---

# Task 05: Open-time gates in ensure and resume hooks

## Acceptance

- `useEnsureTaskSession` accepts `{ id, workflowStepId, workflowId }` and reads
  `state.userSettings.preventAutoStartAgentOnOpen`. When the setting is on and
  the task's step is the final step (max `position`) of the task's OWN
  workflow, it calls `ensureTaskSession(taskId, { autoStart: false })`; all
  other cases call `ensureTaskSession(taskId)` exactly as today.
- The step list is resolved workflow-aware: the active workflow's steps
  (`state.kanban.steps` when `task.workflowId` matches
  `state.kanban.workflowId`) or the multi-workflow snapshot's steps
  (`state.kanbanMulti.snapshots[task.workflowId]?.steps`). Missing workflow id
  or step list → treated as "not final" (no gate).
- Callers pass normalized input: `task-page-content.tsx` maps the HTTP task's
  snake_case fields (`workflow_step_id`, `workflow_id`); `kanban-with-preview.tsx`
  `useSelectedTask` includes `workflowId` in its returned subset so
  cross-workflow preview tasks resolve their own steps.
- `useSessionResumption`'s automatic check no longer auto-resumes when the
  setting is on: with `status.needs_resume && status.is_resumable` it skips
  `resumeWithSilentFallback`, settles on `"idle"`, and records the skip in the
  store (a `resumeSkippedSessionIds` set on the tasks slice via a new
  `setResumeSkipped(sessionId, boolean)` action). The manual `resumeSession()`
  action and the `is_agent_running` / `needs_workspace_restore` branches are
  unchanged; the skip flag clears on manual resume or when the agent runs.
- The Start agent button (`TaskDescriptionStartButton` in
  `message-renderer.tsx`) renders for `sessionState === "CREATED"` AND for
  resume-skipped (recovered-idle) sessions; for the recovered-idle case it
  dispatches the resume request builder instead of `buildStartCreatedRequest`.

## Verification

```bash
(cd apps/web && pnpm run typecheck)
```

```bash
(cd apps/web && pnpm vitest run hooks/domains/session/use-ensure-task-session.test.ts hooks/domains/session/use-session-resumption.test.ts components/task/chat/message-renderer.test.tsx)
```

## Files Likely Touched

- `apps/web/hooks/domains/session/use-ensure-task-session.ts` (+ its test; add a `isFinalWorkflowStep(workflowStepId, steps)` helper, exported for tests)
- `apps/web/hooks/domains/session/use-session-resumption.ts` (+ its test; thread `preventAutoStart` into `checkAndResume` via `useSessionResetAndCheck`)
- `apps/web/lib/state/slices/kanban/types.ts` + `kanban-slice.ts` (the slice owning `tasks.activeSessionId`): add `resumeSkippedSessionIds` state + a `setResumeSkipped` action
- `apps/web/components/task/chat/message-renderer.tsx` (+ test): Start button visibility for resume-skipped sessions and resume intent dispatch
- `apps/web/components/task/task-page-content.tsx`: pass `{ id, workflowStepId: task?.workflow_step_id, workflowId: task?.workflow_id }`
- `apps/web/components/kanban-with-preview.tsx` (+ test): `useSelectedTask` returns `workflowId`

## Dependencies

Task 03 (store field + `ensureTaskSession` `autoStart` opt), Task 04 (Start
button copy covers both cases).

## Inputs

- Spec scenarios 1, 2, 3, 5, 7, 8 and the "State machine" table.
- `useEnsureTaskSession` current flow: fires once per `(taskId, retryToken)` when
  the task has no sessions; the latch must keep working with the widened input.
- `checkAndResume` branch order at `use-session-resumption.ts:284-336`;
  `state.kanban.steps` position semantics (sorted, `position` ascending) and
  `state.kanbanMulti.snapshots` (keyed by workflow id, each with its own
  `steps`); `TaskDescriptionStartButton` condition at
  `message-renderer.tsx:135-149` (currently `sessionState === "CREATED"` only).

## Output Contract

Opening a task honors the setting on both kanban surfaces:
final-step + no-session → prepare-only ensure (CREATED session, Start agent
button rendered); post-restart idle session → no auto-resume, skip recorded,
Start agent button rendered with a resume action. Unit tests pin each branch,
including the non-gated controls, the snake→camel caller normalization, and
the cross-workflow preview resolution.
