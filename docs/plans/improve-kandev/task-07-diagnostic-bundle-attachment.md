---
id: "07-diagnostic-bundle-attachment"
title: "Align Improve Kandev diagnostic bundle attachment"
status: completed
wave: 5
depends_on: ["03-frontend-dialog-and-mobile", "04-e2e-coverage", "06-workspace-target-choice-e2e"]
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-IMPROVE-KANDEV-001
acceptance_criteria:
  - AC-WORKSPACES-IMPROVE-KANDEV-001.6
system_design:
  - ../../specs/workspaces/system-design/improve-kandev.md
  - ../../specs/platform/system-design/diagnostic-logging-02.md
  - ../../specs/platform/system-design/diagnostic-logging-01.md
---

# Task 07: Align Improve Kandev diagnostic bundle attachment

## Summary

Bring the browser's Include recent logs flow into line with AC `.6` and the shared diagnostic-bundle contract. Request the complete standard source set, wait for a ready or partial job, lease its ZIP into the bootstrap-owned task directory, and prove that the leased path reaches the submitted task description. ACP capture remains outside this flow.

## In scope

- Update `buildImproveKandevDescription` to request exactly `backend`, `frontend`, and `runtime`; do not request `acp` or provide `session_ids`.
- Preserve the current ready/partial lifecycle: use a ready initial response directly; poll collecting/building jobs until ready or partial; lease the terminal archive using the bootstrap `bundle_dir` and the completed job ID.
- Ensure the generated task-description text accurately describes the included standard sources and appends the path returned by the lease endpoint, not a locally inferred path.
- Add deterministic helper tests for a ready job and a polled partial job. Assert
  the exact source set, absence of ACP/session selection, lease arguments, and
  that the returned leased path appears in the final description. Also cover
  failed and expired terminal jobs and lease rejection, asserting no path is
  appended.
- Add controlled browser E2E scenarios titled `leased diagnostic bundle path
  reaches task description` and `diagnostic attachment failure does not block
  task creation`. Intercept bundle creation/status/lease and task creation;
  verify the partial-success description path and task submission without a
  bundle path for failed/expired collection or lease rejection.
- Rename `includeRecentBackendAndBrowserLogs` to the generic
  `includeRecentLogs` key; update all six shipped locale catalogs and the
  pseudo-locale, and remove the obsolete key. Keep the label consistent with
  the AC `.6` wording and the expanded standard source set.
- Update the phone E2E with scenario title `phone task creation includes recent
  logs`; assert the localized **Include recent logs** toggle in the shared
  task-create form.
- Preserve best-effort semantics: bundle collection or lease failure must not
  block task creation or append a missing/unleased path.

## Out of scope

- Changes to diagnostic bundle backend collection, source authorization, ZIP contents, or lease ownership validation.
- ACP capture. ACP requires the separate human download, session authorization, and disclosure contract.
- Changes to report-kind toggle defaults or workspace bootstrap behavior.

## Acceptance

- With log capture enabled, the browser requests exactly `backend`, `frontend`,
  and `runtime`; the request excludes `acp` and `session_ids`.
- Ready and partial jobs both proceed to lease. A collecting/building job is
  polled until partial/ready, and the lease call uses the completed job ID and
  bootstrap `bundle_dir`.
- The final description submitted to task creation includes the exact path
  returned by lease. No path is added before a successful lease.
- Failed or expired collection, and lease failure, preserve task submission and
  leave the description free of an unleased bundle path.
- Log capture disabled continues to skip diagnostic job creation and lease.
- The shared task-create toggle uses the localized **Include recent logs**
  label in all shipped locales, without retaining the obsolete backend/browser
  key.

## UI Preview

This content-only label change maps to AC `.6` in the existing shared task-create
form. The nearest mobile exemplar is `TaskCreateDialog`; the rendered mobile
check is in `mobile-improve-kandev.spec.ts`.

### UI-03: Desktop task creation, log-label excerpt

```text
| [x] Include recent logs |
```

### UI-05: Phone task creation, same shared label

```text
| [x] Include recent logs |
```


## Verification

Both Playwright commands use the managed runner and rebuild the backend,
web, and plugin artifacts; either can run independently after source changes.

```bash
(cd apps/web && pnpm exec vitest run components/improve-kandev-dialog-helpers.test.ts)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/improve-kandev.spec.ts -- --grep "leased diagnostic bundle path reaches task description|diagnostic attachment failure does not block task creation")
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/mobile-improve-kandev.spec.ts -- --grep "includes recent logs")
```

## Files likely touched

- `apps/web/components/improve-kandev-dialog-helpers.ts`
- `apps/web/components/improve-kandev-dialog-helpers.test.ts`
- `apps/web/e2e/tests/improve-kandev.spec.ts`
- `apps/web/e2e/tests/mobile-improve-kandev.spec.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,pseudo}/common.json`
- `apps/web/components/improve-kandev-dialog-create.tsx`

## Dependencies

Tasks 03, 04, and 06. Task 06 edits the same Improve Kandev browser E2E file, so this work order follows it.

## Risks

- The standard source list is distinct from ACP: adding runtime must not accidentally invoke the ACP session picker or attach session identifiers.
- A partial job is a successful archive outcome; treating it as failure would silently drop the requested diagnostic context.
- The backend-returned lease path is authoritative; using the bootstrap's predicted `bundle_file` could point at a file that was never copied.

## Parallelism

`sequential`

## Inputs

- `docs/specs/workspaces/requirements/improve-kandev.md`, AC `.6`.
- `docs/specs/workspaces/system-design/improve-kandev.md`, diagnostic attachment and ACP exclusion.
- `docs/specs/platform/system-design/diagnostic-logging-02.md`, supported sources and job lifecycle.
- Existing `buildImproveKandevDescription` helper and Improve Kandev E2E fixtures.
- `docs/i18n.md`

## Results

`buildImproveKandevDescription` now requests only backend, frontend, and
runtime logs; it waits for ready/partial, leases with the bootstrap directory
and terminal job ID, and appends the lease-returned path. The shared task-create
submit wiring now forwards its description transformer, so Improve Kandev
actually attaches the context before task creation. Collection and lease
failures remain best-effort. The renamed localized **Include recent logs** key
is present in every shipped locale and pseudo catalog; the obsolete key is
removed.

Helper tests passed (ready, partial polling, failed, expired, and rejected
lease). Desktop E2E passed for partial archive path submission and expired
collection without blocking task creation. Mobile E2E passed for the localized
toggle. `i18n:check`, `i18n:ratchet`, frontend typecheck, and targeted ESLint
passed.
