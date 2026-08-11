---
id: "06-e2e"
title: "E2E: prevent auto-start on open"
status: pending
wave: 2
parallelism: sequential
depends_on: ["05-open-time-gates"]
plan: "plan.md"
spec: "../../specs/prevent-agent-autostart-on-open/spec.md"
---

# Task 06: E2E — prevent auto-start on open

## Acceptance

- **Final-step case:** with the setting on, opening a task whose current
  workflow step is the final step (and has `auto_start_agent` in its on-enter
  actions) shows the Start agent button
  (`[data-testid="task-description-start-button"]`) and does not start an
  agent on its own. Clicking the button starts the agent.
- **Setting-off control:** with the setting off, the same task auto-starts on
  open (no start button; agent reaches a running/ready state).
- **Resume case:** with the setting on, a task whose session needs resume
  (recovered-idle shape) does not issue an automatic `session.launch`
  resume; the manual affordance is present.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
```

```bash
(cd apps/web && KANDEV_E2E_MOCK=true pnpm e2e:raw -- --project=chromium settings/prevent-auto-start-on-open.spec.ts)
```

## Files Likely Touched

- `apps/web/e2e/tests/settings/prevent-auto-start-on-open.spec.ts` (new)
- `apps/web/e2e/helpers/api-client.ts` (only if a workflow-with-autostart-on-final-step fixture helper is needed)

## Dependencies

Task 05 (the gating behavior under test), Task 03 (the setting is settable via
`updateUserSettings`).

## Inputs

- Spec scenarios 2, 4, 5.
- Existing patterns: `apps/web/e2e/tests/settings/startup-page.spec.ts`
  (settings via `apiClient.updateUserSettings` / UI), `api-client.ts`
  `createTask` with `workflow_id` + `workflow_step_id`, and the session
  recovery helpers in `e2e/tests/session/session-recovery.spec.ts`.
- Note: built-in final steps (office-default, kanban) do NOT carry
  `auto_start_agent`; the fixture must create a custom workflow through the API
  whose last step has `on_enter: [{ type: "auto_start_agent" }]`, or reuse an
  existing fixture workflow that already behaves that way.

## Output Contract

The spec's two user-visible gates are pinned end to end with the mock agent.
Cleanup: any fixture-only workflow/task created by the spec is removed in
teardown (mirror the try/finally pattern used by sibling specs).
