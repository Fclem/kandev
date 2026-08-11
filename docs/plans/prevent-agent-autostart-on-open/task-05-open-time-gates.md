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

- `useEnsureTaskSession` reads the setting and the kanban steps; when the
  setting is on and the task's `workflowStepId` is the final step (max
  `position` in `state.kanban.steps`), it calls
  `ensureTaskSession(taskId, { autoStart: false })`. All other cases call
  `ensureTaskSession(taskId)` exactly as today. Missing step id or empty steps
  → treated as "not final" (no gate).
- `useSessionResumption`'s automatic check no longer auto-resumes when the
  setting is on: with `status.needs_resume && status.is_resumable` it skips
  `resumeWithSilentFallback` and settles on `"idle"`. The manual
  `resumeSession()` action and the `is_agent_running` / `needs_workspace_restore`
  branches are unchanged.
- Both kanban surfaces (task page and preview) inherit the gates; no caller
  signature changes beyond the hook's widened input type.

## Verification

```bash
(cd apps/web && pnpm run typecheck)
```

```bash
(cd apps/web && pnpm vitest run hooks/domains/session/use-ensure-task-session.test.ts hooks/domains/session/use-session-resumption.test.ts)
```

## Files Likely Touched

- `apps/web/hooks/domains/session/use-ensure-task-session.ts` (+ its test; add a final-step helper, exported for tests)
- `apps/web/hooks/domains/session/use-session-resumption.ts` (+ its test; thread `preventAutoStart` into `checkAndResume` via `useSessionResetAndCheck`)
- `apps/web/components/task/task-page-content.tsx` (verify the kanban task object already carries `workflowStepId`; adjust the `EnsureTaskInput` usage if the hook's type requires it)
- `apps/web/components/kanban-with-preview.tsx` (same verification)

## Dependencies

Task 03 (store field + `ensureTaskSession` `autoStart` opt).

## Inputs

- Spec scenarios 1, 2, 3, 7, 8 and the "State machine" table.
- `useEnsureTaskSession` current flow: fires once per `(taskId, retryToken)` when
  the task has no sessions; latch must keep working with the widened input.
- `checkAndResume` branch order at `use-session-resumption.ts:284-336`;
  `state.kanban.steps` position semantics (sorted, `position` ascending).

## Output Contract

Opening a task honors the setting on both kanban surfaces:
final-step + no-session → prepare-only ensure (CREATED session, Start agent
button rendered by `TaskDescriptionStartButton`); post-restart idle session →
no auto-resume. Unit tests pin each branch, including the non-gated controls.
