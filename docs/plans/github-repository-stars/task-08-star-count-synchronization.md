---
id: "08-star-count-synchronization"
title: "Synchronize repository star counts after confirmed mutations"
status: done
wave: 8
depends_on:
  - 02-plugin-settings-controls
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.13
system_design:
  - ../../specs/integrations/system-design/github-repository-star-actors.md
---
# Task 08: Synchronize repository star counts after confirmed mutations

## Summary

Make the catalog's displayed star count respond to successful star/unstar actions. Reuse the existing plugin-stars store, mutation fences, URL canonicalizer, and localized number formatter; no backend or auth changes.

## In scope

- Seed one canonical-repository count projection from catalog data and render it in `MarketplaceEntryRow` instead of reading `entry.stars` directly. Preserve unknown values and share the projection across duplicate repository rows.
- Apply a count delta atomically with an accepted, confirmed mutation transition. Separate status and mutation commits, consume mutation epochs, and reject stale or duplicate results before adjusting counts.
- Keep aggregate counts independent of actor connection resets; retain in-memory state only, discarded on full reload.
- Add focused store, hook, and row regression coverage. Extend the existing browser star flow to assert count transitions and unchanged counts on failure. Update the repository-stars section of `docs/public/plugins-marketplace.md` to describe the session-local count adjustment and catalog baseline.

## Out of scope

- Live GitHub count fetching, polling, external-user activity reconciliation, persistent count storage, sorting changes, auth changes, and new user-facing strings.

## Acceptance

1. A known count of 12 remains 12 while pending, becomes 13 after confirmed star, and returns to 12 after confirmed unstar. Every row for the same canonical repository changes together, including when the action originates from another plugin surface. Unknown remains unknown; zero never becomes negative.
2. Hydration, failed mutations, late responses from obsolete actor scopes, and duplicate mutation responses do not adjust counts. Repeated render or actor switch does not re-seed or reset the projection.
3. Browser evidence proves the successful and failed action paths without page reload; desktop and phone retain existing target sizes and wrapping.

## ASCII UI preview

UI-03, desktop: `★ 12 [☆ Star]` -> pending unchanged -> `★ 13 [★ Unstar]`.
Phone: same count transition, existing wrapping and 44px action target preserved.
See [plan UI-03](plan.md#ui-03-confirmed-star-count-update).

## Verification

```bash
(cd apps/web && pnpm exec vitest run lib/state/slices/plugins hooks/domains/plugins/use-plugin-repository-stars.test.tsx components/settings/plugins/marketplace-entry-row.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:raw --project=chromium tests/settings/plugin-repository-stars.spec.ts)
(cd apps/web && pnpm e2e:raw --project=mobile-chrome tests/settings/mobile-plugin-repository-stars.spec.ts)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `apps/web/lib/state/slices/plugins/{types,plugins-slice,plugins-slice.test,star-count-actions}.ts`.
- `apps/web/hooks/domains/plugins/use-plugin-repository-stars.ts`, `.test.tsx`, and `use-plugin-repository-star-count.ts`.
- `apps/web/components/settings/plugins/marketplace-entry-row.tsx` and `.test.tsx`.
- `apps/web/e2e/tests/settings/plugin-repository-stars.spec.ts`, `mobile-plugin-repository-stars.spec.ts`, and their existing helpers.
- `docs/public/plugins-marketplace.md`.
- `apps/web/src/locales/{ja,ko}/plugins.json` (complete the existing star-action translations).

## Dependencies

Task 02's existing confirmed-mutation cache. Independent of pending auth enhancements and deployment-secret work.

## Risks

Catalog counts can be stale and concurrent external GitHub changes are not represented. This work adjusts the visible baseline for this session's confirmed transitions only; it does not claim a live GitHub total.

## Parallelism

`sequential`

## Inputs

- AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.13 and the count synchronization design section.
- Existing epoch-fenced store, hook, row, and E2E patterns.

## Results

Implemented `AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.13`. Confirmed mutations now update a shared canonical-repository count in the same transaction as the boolean state. Mutation epochs are consumed, failed/stale/duplicate results cannot adjust counts, and actor switches invalidate pending results while preserving the aggregate projection. Unknown counts remain unknown and decrements stop at zero.

Red: the marketplace row regression failed because both rows still showed 3 after a confirmed star, rather than 4. Green: both rows change to 4 after confirmation and return to 3 after unstar. Store coverage also proves pending/failure, no-op/duplicate completion, actor replacement, unknown count, re-seeding, and zero-floor behavior.

Verification:

- Targeted work-order Vitest command: 3 files, 40 tests passed. Final broader run, `pnpm exec vitest run lib/state/slices/plugins hooks/domains/plugins components/settings/plugins lib/plugins/plugin-star-connection.test.ts`: 25 files, 204 tests passed.
- `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck`: passed.
- ESLint on the changed implementation and regression files: passed with no warnings after extracting connection hydration from the star hook.
- Managed chromium and mobile-chrome runs: 14 and 2 tests passed. After the final locale updates, `make build-web` and both exact raw E2E commands above passed again: 14 desktop and 2 phone tests.
- `pnpm run i18n:check`: passed after adding the three existing star-action translations missing in Japanese and Korean; existing unused-key warnings remain informational.
- `node --test scripts/validate-public-docs.test.mjs`: 64 tests passed. Public-docs, catalog, and spec-lint validators passed.
- Interactive browser smoke on the updated LAN demo: observed 12 -> 13 -> 12 using browser-only catalog/provider fixtures, with no real GitHub writes. Phone rendering retained a 44px toggle and no horizontal overflow. Browser tab closed; the user-requested demo remains at `http://192.168.10.103:53823/settings/plugins`.

Public docs updated: `docs/public/plugins-marketplace.md` describes session-local adjustment against the catalog baseline. Limitation: the demo host currently cannot reach the official GitHub Pages catalog; interactive screenshots used a temporary browser fixture, not persisted catalog data. Browser suites use isolated mock-provider backends.
