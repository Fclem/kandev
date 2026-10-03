---
id: "02-profile-order-ui"
title: "Profile reorder and sort UI"
status: done
wave: 2
depends_on:
  - "01-profile-order-backend"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROFILE-LIST-ORDERING-001
  - REQ-AGENTS-PROFILE-LIST-ORDERING-002
  - REQ-AGENTS-PROFILE-LIST-ORDERING-003
acceptance_criteria:
  - AC-AGENTS-PROFILE-LIST-ORDERING-001.1
  - AC-AGENTS-PROFILE-LIST-ORDERING-001.2
  - AC-AGENTS-PROFILE-LIST-ORDERING-001.3
  - AC-AGENTS-PROFILE-LIST-ORDERING-001.4
  - AC-AGENTS-PROFILE-LIST-ORDERING-001.5
  - AC-AGENTS-PROFILE-LIST-ORDERING-001.6
  - AC-AGENTS-PROFILE-LIST-ORDERING-001.7
  - AC-AGENTS-PROFILE-LIST-ORDERING-001.8
  - AC-AGENTS-PROFILE-LIST-ORDERING-002.1
  - AC-AGENTS-PROFILE-LIST-ORDERING-002.2
  - AC-AGENTS-PROFILE-LIST-ORDERING-002.3
  - AC-AGENTS-PROFILE-LIST-ORDERING-002.4
  - AC-AGENTS-PROFILE-LIST-ORDERING-002.5
  - AC-AGENTS-PROFILE-LIST-ORDERING-002.6
  - AC-AGENTS-PROFILE-LIST-ORDERING-002.7
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.1
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.2
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.3
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.5
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.7
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.11
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.13
system_design:
  - ../../specs/agents/system-design/profile-list-ordering.md
---

# Task 02: Profile reorder and sort UI

## Summary

Add drag handles and sortable rows to each installed agent's profile list on
Settings > Agents, the "Sort by name" action, a per-agent save queue that keeps
the latest optimistic intent through an earlier save failure, rolls back only
when the latest intent fails, prepending of new profiles, and cross-client sync.
Cover the flow with unit and Playwright tests.

## In scope

- `agent-profile-order.ts` helpers, `reorderAgentProfilesAction`, slice state
  `ProfileOrderSync` with `acceptServerOrder`, `reconcileAgentOrders` (in the
  slice setters and the hydrator), membership-epoch guarded
  `applyAgentListSnapshot`, and `setAgentProfileOrder`, `profile-order-queue.ts`,
  the `use-profile-order` accessor, and WS handler.
- Capture and enforce the client profile mutation epoch for every fetched
  `GET /agents` snapshot, including direct browser reads outside
  `AgentListResourceScope`; preserve the existing stable-epoch route hydration
  and rehydration guard.
- Sortable `AgentProfilesSubList`, sort button in `InstalledAgentsHeader`.
- Locale strings in all seven languages (including pseudo) and a short
  public-docs section.
- Desktop and mobile E2E specs, and updates to the two layout specs.

## Out of scope

- Backend changes (Task 01). Agent-card order, Dynamic agent profiles, Office list.

## Acceptance

- Dragging by the handle (pointer, touch, keyboard) or running Sort by name saves
  the order, which survives reload and appears on a second open page and in the
  Settings navigation tree; a profile created or duplicated lists first within its
  own agent.
- Non-administrators see no handle or sort button; a drop onto another agent's
  row changes nothing. When a save fails with no newer queued intent, restore
  the saved order and show a toast.
- If a newer drag arrives while a save is in flight, keep showing and submit the
  latest drag. On `409`, accept the refetch only through the membership-epoch
  guard; reconcile new IDs first in refreshed server order, then surviving
  queued IDs in their queued order. A profile created after the queued drag
  stays first, and deleted IDs are omitted. Opening, duplicating, and deleting
  profiles still work.
- An agent-list snapshot, cached response, or event with an older order revision
  never replaces a newer known order or a pending drag (settings load, Settings
  route re-hydration, and task-page hydration included); `pnpm test`,
  `pnpm run i18n:check`, and `pnpm run i18n:ratchet` pass.
- After a profile create/delete event, a delayed pre-event agent-list response,
  including the refetch for a `409`, cannot replace membership even when
  `profile_order_revision` is unchanged. A created profile remains visible
  first and a deleted profile stays absent. The `409` path captures the client
  epoch, rejects a mismatched response atomically, obtains a fresh resource
  result, then reconciles and replays surviving queued intent against current
  membership.
- `app/settings/agents/page.agent-list-snapshot.test.tsx` and
  `app/settings/agents/[agentId]/profiles/[profileId]/use-agent-profile-settings.test.tsx`
  hold each direct list GET response, apply create and delete events before it
  resolves, then assert the pre-event response cannot replace membership in
  either mirrored list.

## ASCII UI preview

Views UI-01, UI-02, and UI-03 in [plan.md](plan.md#ascii-ui-preview). UI-01
excerpt:

```text
Installed agents       [Terminal] [Sort by name] [Rescan] [Add TUI agent]
| Claude
| [::] * Work profile      [sonnet] [No fallback]        [dup] [del]
| [::] * Personal          [opus]   [Next model]         [dup] [del]
```

Applies to `AC-AGENTS-PROFILE-LIST-ORDERING-001.1`, `001.2`, `001.7`, `002.1`.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/settings/agent-profile-order.test.ts lib/settings/profile-order-queue.test.ts lib/state/slices/settings/settings-slice.test.ts hooks/domains/settings/agent-list-resource.test.ts lib/state/hydration/hydrator.test.ts hooks/domains/settings/use-profile-duplicate.test.ts lib/ws/handlers/agents.test.ts "app/settings/agents/[agentId]/agent-save-helpers.test.ts" "app/settings/agents/[agentId]/agent-save-helpers-provider.test.ts" "app/settings/agents/[agentId]/profiles/[profileId]/use-agent-profile-settings.test.tsx" app/office/setup/agent-profile-setup-controls.test.tsx "app/settings/agents/page.agent-list-snapshot.test.tsx" "app/settings/agents/page.sort.test.tsx" components/settings/agents)
(cd apps/web && pnpm test)
(cd apps/web && pnpm run typecheck && pnpm run i18n:pseudo && pnpm run i18n:zh-hant && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps/web && pnpm e2e:run -- tests/settings/agent-profile-order.spec.ts tests/settings/agent-profile-layout.spec.ts)
(cd apps/web && pnpm e2e:run -- --project mobile-chrome tests/settings/mobile-agent-profile-order.spec.ts tests/settings/mobile-agent-profile-layout.spec.ts)
(cd apps/web && pnpm e2e:run -- --project auth tests/auth/agent-profile-order-member.spec.ts)
```

## Files likely touched

- `apps/web/lib/settings/agent-profile-order.ts` and `.test.ts`
- `apps/web/lib/settings/profile-order-queue.ts` and `.test.ts`
- `apps/web/lib/state/slices/settings/settings-slice.ts` and `types.ts`
- `apps/web/lib/state/slices/settings/settings-slice.test.ts`
- `apps/web/app/actions/agents.ts`
- `apps/web/hooks/domains/settings/use-profile-order.ts`
- `apps/web/hooks/domains/settings/use-profile-duplicate.ts` and `.test.ts`
- `apps/web/lib/ws/handlers/agents.ts` and `.test.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/app/settings/agents/[agentId]/agent-save-helpers.ts`, `agent-save-helpers.test.ts`, and `agent-save-helpers-provider.test.ts`
- `apps/web/app/office/setup/agent-profile-setup-controls.tsx` and `.test.tsx`
- `apps/web/lib/state/hydration/hydrator.ts` and `hydrator.test.ts`
- `apps/web/hooks/domains/settings/agent-list-resource.ts` and `.test.ts`
- `apps/web/app/settings/agents/[agentId]/profiles/[profileId]/use-agent-profile-settings.ts` and `.test.tsx`
- `apps/web/components/settings/agents/agent-profiles-section.tsx` and test
- `apps/web/app/settings/agents/page.tsx`, `page.sort.test.tsx`, and `page.agent-list-snapshot.test.tsx`
- `apps/web/src/locales/*/agents.json`
- `apps/web/e2e/tests/settings/agent-profile-order.spec.ts`
- `apps/web/e2e/tests/auth/agent-profile-order-member.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-profile-order.spec.ts`
- `apps/web/e2e/tests/settings/agent-profile-layout.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-profile-layout.spec.ts`
- `docs/public/agents-and-profiles.md`

## Dependencies

Task 01 (endpoint, event, stored order).

## Risks

- The overlay link in `ProfileRowCard` can swallow handle clicks; keep the
  handle in the `z-10` action layer.
- Optimistic order, the echoed WS event, and rollback must not flip-flop; the
  save queue owns that behavior.
- The e2e backend is worker-scoped: new specs must use dedicated profiles and
  restore order, or later specs that use `profiles[0]` see the change.
- Component size limits in `apps/web/AGENTS.md`: extract a sortable row
  component rather than growing `ProfileRowCard`.

## Parallelism

`sequential`

## Inputs

- System design sections Frontend, Store updates, Save coordination, Sort action,
  Failure and recovery.
- `components/task/sidebar-filter/automatic-color-rule-list.tsx` and
  `automatic-color-rule-card.tsx` for dnd-kit setup and the translated
  role description.
- `docs/i18n.md` for locale rules.

## Results

Implemented the sortable profile rows, sort-by-name action, revision-aware
cross-client reconciliation, profile-prepend flows, all locale updates, public
documentation, and desktop/mobile/auth E2E coverage.

Focused Vitest passed (16 files, 150 tests); typecheck, web lint, pseudo/zh-Hant
generation, i18n checks, and the new-code ratchet passed. Auth and mobile E2E
passed. Desktop profile ordering and layout E2E passed (4 tests), including
keyboard reorder after the test waits for dnd-kit's drag activation and resolved
target announcement. Full `pnpm test` reported 2,480 passing tests and 15
unrelated infrastructure/timeouts, including unavailable localhost:3000 and
Docker bridge services.
