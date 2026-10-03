---
id: "02-activity-reporter"
title: "Browser activity reporter"
status: pending
wave: 2
depends_on:
  - "01-activity-ledger"
plan: "plan.md"
requirements:
  - REQ-TASKS-USER-ACTIVITY-001
  - REQ-TASKS-USER-ACTIVITY-003
acceptance_criteria:
  - AC-TASKS-USER-ACTIVITY-001.1
  - AC-TASKS-USER-ACTIVITY-001.2
  - AC-TASKS-USER-ACTIVITY-001.3
  - AC-TASKS-USER-ACTIVITY-001.4
  - AC-TASKS-USER-ACTIVITY-001.5
  - AC-TASKS-USER-ACTIVITY-003.1
system_design:
  - ../../specs/tasks/system-design/session-user-activity.md
---

# Task 02: Browser activity reporter

## Summary

Add `use-session-user-activity`, a hook that sends `session.user_activity.beat`
every 15 s while the page is visible, focused, and interacted with in the last
60 s, for the focused session surface only. Mount it where the focused session
is known on desktop and mobile task pages.

## In scope

- Hook with passive `pointerdown`, `keydown`, `wheel`, `touchstart` listeners
  that store only a timestamp.
- Stop on blur, hidden, idle, session switch, unmount, flag off, a beat
  acknowledgement with `enabled: false`, or while a backend reload is required
  (`lib/platform/backend-reload-coordinator.ts`).
- A `SESSION_USER_ACTIVITY_BEAT_ACTION` constant next to
  `SESSION_SUBSCRIBE_ACTION` in `apps/web/lib/ws/client.ts`, and a send method on
  the WebSocket client.
- Mounting in the desktop dockview session surface and the mobile session layout.
- The hook adds no user-facing copy.

## Out of scope

- Any visible indicator or setting UI.
- Server behavior (task 01).
- An end-to-end read of the recorded time, which no core surface exposes; the
  fixture plugin test in task 07 proves it.

## Acceptance

- Fake-timer tests show beats only under all three engagement conditions and
  none after 60 s idle, on blur, hidden, or session switch, including a touch
  interaction path.
- Payload is exactly `{ session_id }`; with the flag off the hook sends nothing;
  an `enabled: false` acknowledgement and a reload-required state stop beating.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web exec vitest run hooks/domains/session/use-session-user-activity.test.ts lib/ws/client.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run lint)
```

## Files likely touched

- `apps/web/hooks/domains/session/use-session-user-activity.ts` and test
- `apps/web/lib/ws/client.ts` (action constant, send method) and `client.test.ts`
- Desktop session surface and `apps/web/components/task/mobile/session-mobile-layout.tsx`

## Dependencies

Task 01.

## Risks

- Identifying the focused session surface in multi-session dockview layouts;
  reuse the existing active-panel state rather than a new store. The existing
  `session.focus` action signals a mounted surface for git snapshots, not user
  engagement, so it must not be reused as a beat.

## Parallelism

`sequential`

## Inputs

- Design section: Browser reporter.
- Existing `visibilitychange` hooks, e.g. `apps/web/app/stats/stats-data.tsx`.

## Results

Pending.
