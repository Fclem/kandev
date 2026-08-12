---
spec: docs/specs/ui/remove-hide-disabled-nav-options.md
created: 2026-08-12
status: building
---

# Implementation Plan: Remove Hide-Disabled Left-Panel-Nav Options

## Overview

Remove the two obsolete settings — "Hide disabled integrations from left
panel navigation" (`/settings/integrations`) and "Hide disabled agent
profiles from left panel navigation" (`/settings/agents`) — together with
every consumer of their filtering: the settings-page rows, the
`useNavAvailability` enabled-gate, the settings-menu branch filters, the
two domain hooks and the shared `useLocalStorageBoolean` primitive they
were built on, their i18n keys, their unit and E2E tests, and the spec
sections describing the removed behavior.

Order: remove the UI rows and nav-filter consumers first (tasks 01–03),
then delete the now-orphaned hooks and primitive (task 04), then i18n
keys, E2E specs, and docs (tasks 05–07). The settings-menu tree
branches and per-integration enable toggles stay; they simply stop
filtering.

---

## Frontend

### Settings-page rows

- `apps/web/components/integrations/integrations-index-page.tsx` —
  delete `HideDisabledIntegrationsSetting` and its render site; drop the
  now-unused `useHideDisabledIntegrationsInNav`,
  `useDraftedIntegrationEnabled`, `Switch`, and `Label` imports.
- `apps/web/app/settings/agents/page.tsx` — delete the
  `<HideDisabledAgentProfilesSetting />` render and its import; keep the
  header `<Separator />` (the header/content divider matches the
  integrations page).
- Delete `apps/web/app/settings/agents/hide-disabled-agent-profiles-setting.tsx`
  (+ its `.test.tsx`).

### Nav filtering

- `apps/web/hooks/use-nav-availability.ts` — visibility becomes
  configuration-only: drop the five `use*Enabled` hook reads and the
  `useHideDisabledIntegrationsInNav` read; each map key resolves to its
  `configured` boolean directly. Update the module doc comment.
- `apps/web/components/app-sidebar/sections/settings/use-settings-menu-branches.ts`
  — delete `useVisibleIntegrationSlugs` and the
  `useHideDisabledAgentProfilesInNav` read; `buildWorkspacesBranch` and
  `buildAgentsBranch` are called without filter args. Drop the enabled /
  hide-disabled / `IntegrationSlug` / `WORKSPACE_INTEGRATIONS` imports.
- `apps/web/components/app-sidebar/sections/settings/settings-menu-branches.ts`
  — drop the `visibleSlugs` param from `integrationNodes`, the
  `visibleIntegrationSlugs` param from `buildWorkspacesBranch`, and the
  `hideDisabled` param from `buildAgentsBranch`, plus their filter
  logic. The `IntegrationSlug` type stays (it types the node
  `integrationSlug` field consumed by `integration-enabled.tsx` badges).

### Dead code

- Delete `apps/web/hooks/domains/integrations/use-hide-disabled-integrations-in-nav.ts`
  (+ `.test.ts`),
  `apps/web/hooks/domains/settings/use-hide-disabled-agent-profiles-in-nav.ts`
  (+ `.test.ts`), and the now-unused shared primitive
  `apps/web/hooks/use-local-storage-boolean.ts` (+ `.test.ts`).
- `apps/web/hooks/local-storage-mock.test-helpers.ts` — update the
  doc comment (the shared helper stays; it still serves the per-integration
  enabled-hook tests).

### i18n

- Remove `hideDisabledAgentProfilesFromNav`,
  `hideDisabledAgentProfilesFromNavDescription`,
  `hideDisabledIntegrationsFromNav`,
  `hideDisabledIntegrationsFromNavDescription` from
  `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn}/settings.json`.

### E2E

- Delete `apps/web/e2e/tests/integrations/hide-disabled-integrations-nav.spec.ts`
  and `apps/web/e2e/tests/settings/hide-disabled-agent-profiles-nav.spec.ts`.
  No replacement E2E: the removal's contract (configured ⇒ visible) is
  covered by the updated unit tests and the existing sidebar/settings
  navigation specs.

---

## Tests

- **What:** the integrations index page renders exactly six switches (one
  per integration) and no `#hide-disabled-integrations-in-nav` element.
  **File:** `apps/web/app/settings/integrations/page.test.tsx`. **How:**
  update the existing render test (7 → 6 switches), delete the
  hide-disabled draft test and the `kandev:integrations:hideDisabledInNav:v1`
  `beforeEach` cleanup.
- **What:** `useNavAvailability` returns `configured` for each key,
  ignoring the enabled toggles entirely.
  **File:** `apps/web/hooks/use-nav-availability.test.ts`. **How:** remove
  the hide-disabled and enabled-hook mocks and the `decoupling enabled
  from nav visibility` `describe.each`; keep configured/not-configured
  assertions (workspace scoping tests unchanged).
- **What:** the settings-menu branches list every integration and every
  profile unconditionally.
  **Files:**
  `apps/web/components/app-sidebar/sections/settings/settings-menu-branches.test.ts`
  (drop the visible-set cases, keep the lists-everything default),
  `use-settings-menu-branches.test.ts` (drop the hide-disabled/enabled
  mocks and the visibility describe; keep an all-integrations-listed case
  and the flat-mode-no-branches case),
  `settings-tree-render.test.tsx` (drop `HIDE_DISABLED_AGENT_KEY`, its
  `beforeEach`/`afterEach` localStorage lines, and the two hide-setting
  tests; keep the disabled-badge test),
  `apps/web/components/integrations/integrations-menu.test.ts` (drop the
  `use-hide-disabled-integrations-in-nav` mock and the decoupling
  comment).
- **What:** the two domain hooks, the shared primitive, and their tests
  no longer exist; nothing imports them.
  **File:** deleted files. **How:** deletion; `grep` for
  `useHideDisabled|hideDisabledInNav|useLocalStorageBoolean` returns no
  hits in `apps/web` (outside `local-storage-mock.test-helpers.ts`'s
  updated comment).
- **What:** the four i18n keys are absent from every locale.
  **File:** the four `settings.json` files. **How:**
  `cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet`.
- **What:** both E2E specs are gone and nothing references them.
  **File:** deleted specs. **How:** deletion + `grep`.

---

## E2E Tests

No new E2E tests. The two specs that exercised the removed feature are
deleted (task 06). The surviving contract ("configured integrations
always show in nav; disabled profiles always show in the Settings tree")
is asserted by the updated unit tests in task 03 and by the existing
sidebar/settings navigation E2E specs, which are unchanged.

---

## Verification Results

- Task 01: `pnpm --filter @kandev/web test -- app/settings/integrations/page.test.tsx` — red 1 failed (7 switches), green 4 passed.
- Task 03: 5-file targeted suite — 78 passed (after rewriting the stale `buildAgentsBranch` hide-filter test). Final touched-suite run (6 files incl. page.test.tsx): 82 passed.
- Task 04: `grep` for hook/primitive names in `apps/web` → no matches.
- Task 05: `pnpm run i18n:check` ✓, `pnpm run i18n:ratchet` ✓ (0 added).
- Task 06: `grep` for spec names in `apps/web` → no matches.
- Task 07: `git diff --check` clean.
- Gate: `make fmt` ✓ · `make typecheck` ✓ (exit 0) · `make lint` ✓ (exit 0) · `make test`: backend/web have pre-existing environmental failures (process-capture tests, Docker-gateway `http-git-server.test.ts` — reproduces identically at pristine HEAD `723c14001`; zero backend diff) · `make test-cli` ✓ (0 failures) · `test-scripts` blocked by missing `unzip` binary (no package-manager access).

---

## Implementation Waves And Parallel Candidates

Wave 1 (parallel candidates — disjoint files; user authorization required):
- [x] [task-01-remove-integrations-setting](task-01-remove-integrations-setting.md)
- [x] [task-02-remove-agent-profiles-setting](task-02-remove-agent-profiles-setting.md)
- [x] [task-03-remove-nav-filtering](task-03-remove-nav-filtering.md)
- [x] [task-05-remove-i18n-keys](task-05-remove-i18n-keys.md)
- [x] [task-06-remove-e2e-specs](task-06-remove-e2e-specs.md)
- [x] [task-07-update-docs](task-07-update-docs.md)

Wave 2 (must land after 01–03 — deletes the hooks the earlier tasks
stop importing):
- [x] [task-04-delete-orphaned-hooks](task-04-delete-orphaned-hooks.md)

## Open Questions

(none)
