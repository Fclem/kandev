---
id: "01-frontend-pin"
title: "Frontend pin control and open-state persistence"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/message-queue-pin.md"
---

# Task 01: Frontend pin control and open-state persistence

- **Acceptance:**
  1. `useQueuePinned(sessionId)` returns a per-session localStorage-backed
     boolean defaulting to `false`, following the `useLocalStorageBoolean`
     shape (storage key `kandev:queue:pinned:<session_id>:v1`, sync event
     `kandev:queue:pinned-changed`); `null` session degrades to `false` with
     no storage access.
  2. The expanded queue panel header shows a `queue-pin` toggle between
     **Clear all** and **X** with `aria-pressed`, localized title/aria-label,
     and the sibling touch sizing; clicking it toggles and persists the
     session pin.
  3. When pinned with queued entries, `QueueAffordance` mounts with the panel
     open (no chip); unpinned mounts stay collapsed; zero entries always hide
     the panel; session switches follow the target session's pin.
  4. All new copy goes through `t()` with keys added to en, pseudo, pt-pt,
     and zh-cn `chat.json`; `pnpm run i18n:check` and `i18n:ratchet` pass.
- **Verification** (one block rooted at `apps`):
  ```bash
  cd apps && pnpm install --frozen-lockfile
  cd apps/web && pnpm run typecheck && pnpm run lint
  cd apps && pnpm --filter @kandev/web test -- hooks/use-queue-pinned.test.ts components/task/chat/queued-ghost-list.test.tsx
  cd apps/web && pnpm run i18n:check
  ```
  Note: `useQueuePinned` is mocked in the component test via a localStorage
  mock (see `hooks/local-storage-mock.test-helpers`); the hook has its own
  unit test.
- **Files likely touched:**
  - `apps/web/hooks/use-queue-pinned.ts` (new) + `use-queue-pinned.test.ts` (new)
  - `apps/web/components/task/chat/queued-ghost-panel-header.tsx`
  - `apps/web/components/task/chat/queued-ghost-list.tsx`
  - `apps/web/components/task/chat/queued-ghost-list.test.tsx`
  - `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn}/chat.json`
- **Dependencies:** None.
- **Parallelism:** sequential.
- **Inputs:** spec `What` + `Data model` + `Failure modes` sections;
  plan Frontend + Tests sections. Patterns: `apps/web/hooks/use-local-storage-boolean.ts`
  (and its test), existing header buttons in `queued-ghost-panel-header.tsx`,
  render-phase state adjustment in `useQueuePanelOpenState`.
- **Output contract:** summary, files changed, exact commands + outcomes,
  task/plan status update. Confirm `IconPin`/`IconPinned` exist in the
  installed `@tabler/icons-react`; if not, use the closest existing
  filled/outline pair and note it.

## Results

- `cd apps && pnpm --filter @kandev/web test -- hooks/use-queue-pinned.test.ts components/task/chat/queued-ghost-list.test.tsx components/task/chat/queued-ghost-pin.test.tsx` → 3 files, 56 tests passed.
- `cd apps/web && pnpm run typecheck` → passed (`tsc --noEmit` clean).
- `cd apps/web && pnpm run lint` → clean (`eslint --max-warnings 0`, 0 warnings).
- `cd apps/web && pnpm run i18n:check` → passed (keys in sync, no em dashes, no inline plurals).
- `cd apps/web && pnpm run i18n:ratchet` → passed (0 added + 2 modified files clean).
- `IconPin`/`IconPinned` confirmed present in installed `@tabler/icons-react` (rendered SVG check).
- Files changed beyond the planned list: `useQueuePanelOpenState` extracted to
  `apps/web/components/task/chat/use-queue-panel-open-state.ts` (lint 600-line
  cap); pin tests live in a dedicated `queued-ghost-pin.test.tsx` (split-test
  convention, cf. `pr-ci-popover.automation.test.tsx`); `useQueuePinned` also
  exposes `toggle` (swallows persistence failures) to keep `QueueAffordance`
  under the 100-line function cap. Spec updated: a pinned panel opens when
  entries arrive asynchronously after mount (initial open state is `pinned`,
  not gated on entry count; the render path gates on `hasEntries`).
- Security/trust boundaries: `None`. External side effects: `None`.
