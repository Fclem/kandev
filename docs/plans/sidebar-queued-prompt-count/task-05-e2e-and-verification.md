---
id: "05-e2e-and-verification"
title: "E2E coverage and full verification"
status: done
wave: 4
depends_on: ["03-attach-count-to-task-payloads", "04-sidebar-badge-ui"]
plan: "plan.md"
spec: "../../specs/ui/sidebar-queued-prompt-count.md"
---

# Task 05: E2E Coverage and Full Verification

## Acceptance

- Desktop Playwright spec (`apps/web/e2e/tests/task/sidebar-queued-count.spec.ts`):
  - Create a task, seed an IDLE session, queue 3 prompts via the existing
    `apiClient.queueMessage` helper, open the workspace, and assert the task
    row in `SidebarTasksPage` shows the badge (`sidebar-task-queued-count`)
    with text `3`.
  - Clear the queue (add an `apiClient.clearQueue(sessionId)` helper backed by
    `message.queue.cancel` if none exists) and assert the badge disappears
    without a reload (live path through the status-summary broadcast).
  - Assert a task with an empty queue never shows the badge.
  - Teardown restores queue state so parallel specs cannot inherit counts.
- Mobile flow: same enqueue assertion driven through the mobile sidebar (per
  `mobile-parity`), confirming no hover dependency and no horizontal overflow.
- Backend Go tests, frontend unit tests, typecheck, lint, and both i18n gates
  all GREEN (commands below).

## TDD Sequence

1. Write the desktop spec (badge visible → live clear → hidden) and run it
   RED against the pre-feature build expectations if feasible, else after the
   unit work with a deliberately failing selector first.
2. Implement any missing E2E helper (`clearQueue`) and finish the spec.
3. Run the specs GREEN; run the full verification set.

## Verification

```bash
cd apps/web
pnpm e2e -- tests/task/sidebar-queued-count.spec.ts
# mobile variant per repo convention (e.g. mobile-sidebar-queued-count.spec.ts)

cd apps/backend
make test lint build

cd apps
pnpm --filter @kandev/web run typecheck
pnpm --filter @kandev/web run lint
pnpm --filter @kandev/web run i18n:check
pnpm --filter @kandev/web run i18n:ratchet
```

## Files Likely Touched

- `apps/web/e2e/tests/task/sidebar-queued-count.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-queued-count.spec.ts` (or a shared
  helper reusing the `SidebarTasksPage` page object)
- `apps/web/e2e/helpers/api-client.ts` (clearQueue helper if absent)

## Dependencies

Tasks 03 and 04 deliver the payload and the badge; this task proves them end
to end on desktop and mobile.

## Output Contract

Report the E2E run results (badge visible, live clear, hidden at 0), the full
verification command results, and changed files. Update this task and
`plan.md` status in the same implementation conversation.
