---
created: 2026-10-10
status: done
requirements:
  - REQ-AGENTS-RUNTIME-NOTIFY-004
system_design:
  - ../../specs/agents/system-design/omp-automatic-updates.md
legacy_specs: []
---

# Implementation Plan: Periodic OMP automatic updates

## Overview

Deliver install-wide opt-in OMP automatic updates by invoking its trusted updater on the existing background cadence, independently of stable-release discovery. Implement durable admission, outcomes and notifications first, then expose safe consent and accurate results on desktop and phone. User-confirmed choices: in-place updates with a no-rollback warning; periodically check the configured channel rather than waiting for a newer stable release. Pause policy (bounded retry, pause on unknown installation state) is a design decision recorded in the ADR, not user-confirmed.

Both work orders are complete in the authorized primary session and ship together with the reviewed design package in one explicit-path Conventional Commit.

## Scope

### In scope

- Trusted built-in OMP capability, persisted opt-in, and checks at most once per fifteen-minute interval, gated on the previous attempt's start time.
- Changed, no-change, updater-failure, validation-failure and interrupted outcomes; bounded-retry and immediate pause rules; saved off/on recovery with a derived `auto_update_paused` status field.
- Existing maintenance exclusion, consent/recipe/availability guard at an admission boundary, bounded jobs and shutdown.
- Existing notification consumers, including the local provider payload whitelist and no-change suppression.
- Localized consent, warning and outcomes; desktop/mobile E2E; public how-to update.

### Out of scope

- Channel switching, exact-version targets, rollback, staged OMP installation or guessed installers.
- Changing manual OMP behavior (beyond the explicit rule that it leaves consent unchanged) or Codex pinned automation.
- Modifying vendor auto-update settings, remote installations, launch commands, session recovery or active processes.
- New scheduler, runtime flag, telemetry subsystem, Kandev tasks/sessions or implementation subagents.

## Technical approach

### Trusted capability and durable policy

Extend `agents.RuntimeUpdateCapability` with an explicit trusted automatic harness recipe declared only by built-in OMP metadata. Keep OMP owner external and management manual; do not gain authority from its release source. Keep the existing `omp-acp` runtime identity and the `AutoUpdateStore` key. Add optional zero-default policy fields `last_attempt_started_at`, `consecutive_failures`, `paused` and `last_change`; extend `UpdateOutcome` with mode, operation, observed version and bounded output excerpt. OMP outcomes leave target empty. A saved false-to-true transition clears pause and failures and never clears the cadence timestamp. Pinned records and semantics are unchanged.

### Scheduling, admission and outcomes

Branch `RunRuntimeUpdatePass` before pinned stable-update gating, after interrupted-attempt settlement. Eligible when consented, enabled, available, not paused, no active maintenance, and at least 14 minutes since `last_attempt_started_at`, where both times are pass start times and a negative difference is eligible. Admission is synchronous in the pass: availability via the cheap single-agent `IsInstalled` before any lock, a non-blocking semaphore slot, `runtimeAutoUpdateMu` as the outermost lock (consent, recipe and enablement only), then one `s.mu` critical section that claims maintenance, persists the running marker through a `beforeStart`-style callback and registers the job; only then `agent.update.started` and the worker. Any refusal persists nothing and creates no job, and no claimant can see a held claim without a registered job. Updater and probe contexts derive from the background pass context so shutdown and restore quiesce cancel them promptly; the probe keeps the host manager's own 60-second cap and a cancelled context classifies the attempt as interrupted even though the host probe swallows cancellation. `runDirectCommand` gets an opt-in tree-cancel mode for the automatic path: output through Cmd-owned `io.Writer`s (a `WaitDelay` does not release `StdoutPipe` scanners), own process group with SIGTERM then SIGKILL on POSIX, parent kill plus `WaitDelay` on Windows, in build-tagged files; only a detached descendant can survive. A typed automatic result (cause, versions, excerpt) is recorded on the job and passed to a new `onAutomaticFinished` hook, which `complete()` calls instead of `onFinished` for self-update jobs (and `retainAutomaticOutcome` skips them), persists, and publishes the terminal status with the paused flag and `last_change`. Outcomes use the explicit mapping table in the design (not `job.Status`): changed, previous-unknown repair, no change, updater error or deadline (retry, pause at three), validation failure including an `auth_required` probe (pause), shutdown or process loss (interrupted, counts as a failure). The excerpt keeps newlines and tabs, strips escapes and other controls, and is capped at the final 2 KiB. Terminal persistence failure leaves the running marker, which the next pass settles as interrupted.

### Existing outcome consumers

Update `publishRuntimeStatus`, `RuntimeUpdateNotice`, notification service payload and messages, `notifications/providers/local.go` (fixed payload key list), frontend notification types and `useUpdateAvailableToast`. Notify only for changed results or pausing failures; suppress no-change and non-pausing failures. Accept versionless self-update notices. Deduplicate by attempt ID. No pin/rollback/default guidance for OMP.

### Settings and public guidance

Extend `RuntimePolicyRow`: show the switch for supported capability or saved consent, with its own `aria-describedby` paragraph carrying the cadence and no-rollback warning; keep the shared managed paragraph scoped to pinned rows. Paused copy keys on `auto_update_paused`. Reuse `useRuntimeAutoUpdatePolicy` and Save/Discard. Localize every shipped catalog; run `pnpm run i18n:zh-hant`. Update `docs/public/agents-and-profiles.md` at implementation time using the current manual metadata behavior, and the root README only if its managed-only summary becomes inaccurate.

| Runtime shape | Identity / mechanism | Behavior | Evidence / fallback |
| --- | --- | --- | --- |
| OMP host ACP | `omp-acp`, trusted `omp update` + `omp acp` | Opt-in periodic in-place configured-channel checks | New OMP controller tests, smoke, settings E2E; unknown stable metadata does not gate |
| Codex/other managed ACP | Exact npm candidate identity | Existing staged stable automation unchanged | Existing `agent_autoupdate_test.go` and pinned regressions |
| Native external agent without declared automatic recipe | External/manual | Manual guidance only | Existing runtime coverage; automation request rejected |
| OMP unavailable/disabled | Retained saved consent | No command; opt-out reachable | Availability/enablement and Settings tests |
| OMP remote/container | Executor-owned installation | No update of that installation | Host-only recipe |

## ASCII UI preview

Entry point: Settings > Agents > expanded Agent runtime updates. Illustrative English copy; final copy is localized. Structural requirements: visible opt-in, warning beside the switch, truthful retained result, existing manual action and Save/Discard. Spacing is not a pixel specification.

### UI-01: Desktop OMP runtime row

```text
Oh My Pi   Current: 1.2.0   Stable latest: 1.1.0
Source: @oh-my-pi/pi-coding-agent   External installation
Automatic updates [off]   [Manage update] [Vendor guidance]
Checks the configured channel about every 15 minutes.
Updates change the installation in place. Kandev cannot roll back.
Last change: 1.1.0 to 1.2.0.
Last check: OMP reported no version change. Version: 1.2.0.
  > Updater output (collapsed, plain text)
                                      [Discard] [Save]
```

### UI-02: Phone OMP runtime row

```text
Oh My Pi
Current: 1.2.0
Stable latest: 1.1.0 (reference)
External installation
Automatic updates             [off]
Checks the configured channel about
every 15 minutes. Updates are in
place. Kandev cannot roll back.
[Manage update]
[Vendor guidance]
Last change: 1.1.0 to 1.2.0.
Last check: OMP reported no version
change. Version: 1.2.0.
  > Updater output
[Discard]                    [Save]
```

Both views reuse the page scroll owner and existing Save/Discard positioning. Phone switch, link and disclosure keep at least 44px hit targets; desktop keeps the surrounding compact density. No new overlay. "Manage update" is an OMP-specific localized label linking to the installed-agent card, where OMP's manual update control lives; it replaces the generic non-managed help text, which would contradict the switch.

Changed states: running says checking the configured channel, not preparing a target. A changed result shows previous and observed versions and stays as its own line until a later change replaces it. A repair result (previous version unknown) shows the validated version without claiming a change. An updater error or interruption says the check did not complete and will retry. A paused result (`auto_update_paused`) says checks are paused, the installation may have changed, Kandev cannot roll back, and to turn off and save, then on and save, to resume; it disappears once recovery is saved. A credential-less install pauses after its first check because an `auth_required` probe is a validation failure; the operator signs in and re-enables. The collapsed output excerpt appears for no-change and failed results. A check that never reached admission shows nothing, and automatic checks never appear in the installed-agent card unless a manual request adopted the running job, in which case it shows neutral copy instead of "updated successfully". Unavailable or disabled OMP starts no command, shows the existing limitation, and keeps opt-out. Users without permission cannot edit. A save error retains the saved baseline and the editable draft.

## Tests

Executed backend and focused frontend coverage. Named outcome cases are subtests of `TestOMPAutomaticTypedCauseReachesHook`; related cadence, recovery and admission assertions share the boundary test that exercises them rather than duplicating the same path. Browser and final delivery evidence is recorded below.

| AC | Files and named cases |
| --- | --- |
| 004.1 | `managedruntime/autoupdate_test.go`: legacy record reads, new field defaults, restart persistence; `settings/handlers/runtime_update_policy_test.go`: OMP enable, disable, permission, unsupported recipe; `registry/runtime_update_coverage_test.go`: exactly `omp-acp` carries the automatic harness recipe and no other registration does |
| 004.2, .3 | `controller/omp_autoupdate_cadence_test.go`: `TestOMPAutomaticChannelChecksIgnoreStableStatus`, `TestOMPAutomaticCadenceUsesStartTimeAndSurvivesRestart` (including fourteen-minute restart spacing), `TestOMPAutomaticBusyPassDoesNotConsumeInterval`, `TestOMPAutomaticClockStepBackIsEligible`; `omp_autoupdate_admission_test.go`: `TestOMPAutomaticReenableKeepsCadenceTimestamp`. Shared helpers live in `omp_autoupdate_fixtures_test.go`; each topical test file stays under the 800 effective-line limit. |
| 004.4, .5 | `omp_autoupdate_outcomes_test.go`: `TestOMPAutomaticTypedCauseReachesHook` retains the named changed, repair, no-change, missing-version, validation-failure and authentication-required cases; `TestOMPAutomaticLastChangeSurvivesNoChange`, `TestOMPAutomaticExcerptKeepsNewlinesStripsEscapesAndCapsAtRuneBoundary`. Existing manual unchanged-version regression stays in `agent_update_harness_test.go`. |
| 004.6, .7 | `omp_autoupdate_outcomes_test.go`: `TestOMPAutomaticUpdaterErrorRetriesThenPausesAtThree` and the typed outcome table. `omp_autoupdate_shutdown_test.go`: `TestOMPAutomaticUpdaterDeadlineCountsAsFailure`, `TestOMPAutomaticDeadlineAtUpdaterEdgeIsNotInterrupted`, `TestOMPAutomaticSlowUpdaterStillLeavesProbeItsFullCap`, `TestOMPAutomaticShutdownDuringProbeIsInterruptedNotPaused`, `TestOMPAutomaticCancelledAttemptPersistsInterruptedThroughFinishPath`, `TestOMPAutomaticStaleThirdFailurePausesBeforeAdmission`, `TestOMPAutomaticFailedTerminalSaveRemainsInterruptedEvidence`. `TestOMPAutomaticReenableKeepsCadenceTimestamp` also verifies saved recovery clears pause/failures without resetting cadence. |
| 004.2, .8, .9 | `omp_autoupdate_admission_test.go`: `TestOMPAutomaticGuardRejectionPersistsNothing`, `TestOMPAutomaticAvailabilityCannotBlockWithdrawal`, `TestOMPAutomaticAndManualNeverOverlap`, `TestOMPManualFinishNeverCallsAutomaticHook`; busy/cadence tests above and existing pinned identity/session regressions remain green. |
| 004.10 | `controller/runtime_update_notifications_test.go`, `notifications/service/runtime_updates_test.go`, `notifications/providers/local_test.go`: changed and pausing notices only (no notice for repair, no change or non-pausing failure), versionless pausing failure, attempt deduplication, `update_mode` reaches the WebSocket payload, no pin recovery text |
| 004.11 | `agent-runtime-policies.test.tsx`, `agent-runtime-update-control.test.tsx`, `runtime-version-summary.test.tsx`, `lib/agent-runtime-update.test.ts`, `lib/ws/handlers/agents.test.ts`, `settings-slice.test.ts`, `use-agent-runtime-updates.test.tsx`, `use-profile-model-capabilities.test.tsx`, `use-runtime-auto-update-policy.test.ts`, `use-update-available-toast.test.ts`: OMP-specific description target, manage-update link and replaced manual help, `last_change` line, collapsed excerpt, repair state, `auto_update_paused` copy and clearing, the ws handler dropping automatic self-update events unless the job is adopted or already held while still calling `refreshRuntimeUpdateStatuses` and bumping the activation counter for a succeeded update or repair (a finished manual result survives an automatic check; an adopted job whose finish arrives before its GET resolves still applies, because the 409 body's `active_job_id` is recorded in the `Record` before the GET and cleared when the held job is replaced), an open profile editor refreshing models after an automatic changed update, an adopted `up_to_date` job neither labelling the card "Up to date" nor disabling the manual Update action (`resolveRuntimeOperation` ignores its operation), neutral copy for an adopted automatic job (never "updated successfully" or "already up to date"), outcome states without target, draft/discard/save-failure, permission, unavailable opt-out |
| 004.12 | Mobile E2E below |
| 004.6, .7, .8 (tree-cancel seam, admission) | `controller/agent_update_tree_test.go`: `TestRunDirectCommandTreeCancelKillsGroupAndReleasesPipes`, `TestRunDirectCommandTreeCancelTreatsWaitDelayAfterZeroExitAsSuccess`. `omp_autoupdate_admission_test.go`: `TestOMPAutomaticRequiresTreeUpdater`, `TestOMPAutomaticFullSemaphoreDefersWithoutJob`, `TestOMPAutomaticRefusedAdmissionCreatesNoJobAndBroadcastsNothing`, `TestOMPAutomaticClaimNeverVisibleWithoutRegisteredJob`, `TestOMPAutomaticAvailabilityCannotBlockWithdrawal`. Authentication-required is a named case in the typed outcome table. |
| 004.10 (dispatch) | `TestOMPManualFinishNeverCallsAutomaticHook` (a manual OMP job, `self_update` but not `Automatic`, keeps the `onFinished` path); `TestOMPAutomaticChangedNoticeReplaysAfterLaterNoChangeCheck` (the notice is built from `last_change`'s attempt ID after the Outcome is overwritten) |

Use deterministic time and deferred subprocess/probe gates, not fifteen-minute sleeps. Tests assert observable execution, publication, admission and results, not source text, translated wording copies or wiring echoes. Existing literal-copy tests affected by new copy are removed, not re-pinned. Inspect LSP references before changing exported symbols, and search map-key consumers such as the local provider payload by text.

## E2E tests

Add OMP cases beside existing runtime-awareness flows in `agent-runtime-notifications.spec.ts`, `mobile-agent-runtime-notifications.spec.ts` and shared helpers. Projects `chromium` and `mobile-chrome`: expand the runtime section, inspect the warning, toggle/save/reload, discard, save failure, unavailable saved opt-out, changed/no-change/failure/paused results with the last-change line and collapsed excerpt, the manage-update link, and saved off/on recovery (AC 004.1, .4, .5, .7, .11, .12). Phone uses tap and measured 44px and overflow assertions. No new project or device override.

Command execution uses backend deterministic tests and a disposable real-controller smoke, not route-intercepted responses alone. Record fixture and real-provider evidence separately; never update a user's real OMP installation.

## Work orders

1. [Task 01: Durable periodic OMP execution](task-01-periodic-execution.md), wave 1, no dependencies.
2. [Task 02: Consent, outcomes, E2E and delivery](task-02-settings-and-delivery.md), wave 2, depends on Task 01.

Sequential: shared outcome contracts, status fields and the notification payload make these slices dependent. Both work orders ship in one pull request and one release: after Task 01 alone the consent endpoint would accept OMP while the warning UI does not exist, so Task 01 is not releasable by itself. No delegation is authorized.

## Verification results

Design investigation: `go test -tags fts5 ./internal/agent/settings/controller -run 'TestHarnessUpdate' -count=1` from `apps/backend` passed (existing SQLite C const warnings). This proves existing manual harness tests, not new automation behavior or a real OMP update.

Task 01 execution evidence: the seven-package backend acceptance command passed using `CGO_ENABLED=1 go test -trimpath -tags fts5` and task-owned cache/scratch paths; Windows `GOOS=windows CGO_ENABLED=0 go vet ./internal/agent/settings/controller` passed. Focused RED/GREEN exposed cadence, pause, typed-result, availability and notification failures before the corresponding production changes. Full commands and fixture isolation are recorded in [Task 01 Results](task-01-periodic-execution.md#results).

Disposable actual-controller smoke passed changed, no-change, opted-out and fixture `auth_required` cases. Actual credential-less OMP `18.8.7` ACP initialization and session creation succeeded under isolated HOME/XDG directories and a cleared environment, with zero advertised models and exit 0. No real `omp update` or prompt ran. The conditional authentication-policy change is not triggered; immediate pause on an actual `auth_required` validation result remains. The disposable controller scaffold was removed.

Task 01 final acceptance re-ran after the maintenance-claim visibility RED/GREEN fix: seven backend packages PASS, Windows vet PASS, and the race command PASS. The required race selector matched no provider tests, so the final selector additionally included `LocalProviderForwards`; all four packages then exercised tests and passed. The controller smoke additionally observed catalogue `1.1.0` with one publication for changed, and unchanged `1.0.0` with zero publications for no-change and failed validation.

Task 02 executed the requested `make fmt` then `make typecheck test lint`, after focused TDD and separately run desktop/mobile managed E2E. The final successful receipt is recorded below and in the work order; earlier failed runs and their causal fixture/environment corrections remain documented.

Task 02 focused frontend acceptance passed: `mise exec -- pnpm exec vitest run` with the eleven files listed in its work order, **11 files / 135 tests**. `mise exec -- pnpm run i18n:zh-hant` and `mise exec -- pnpm run i18n:pseudo` both passed. The final formatted full-check receipt appears below.

Browser development: initial full desktop **22 passed / 3 failed**, followed by focused summary/OMP **1 passed / 2 failed**. Corrections to the fixture's PATCH method, Save/Reset locators, and the summary's readiness-aligned status snapshot made focused desktop OMP **2/2 PASS** and summary **1/1 PASS**. Full phone notification/update command **11/11 PASS**. Commands, failure evidence and separation from vendor execution are recorded in [Task 02 Results](task-02-settings-and-delivery.md#results).

Inspected successful phone warning/no-change/pause captures against UI-02. Direct owned-browser desktop smoke at 1280x900 confirmed warning, ownership, reference vs observed versions, separate change history, neutral result, native output marker, literal output and Reset retaining consent. It allowed no runtime mutation; temporary browser/service/HOME/database were removed. `mise exec -- pnpm run i18n:check` PASS. OMP requirement/design promoted to active/current with the managed-scope exclusions, coverage and public documentation reconciled.

Requested final sequence: `env GOCACHE=/home/clem/kandev-omp-cache/go-build TMPDIR=/var/tmp/kandev-omp-scratch CGO_ENABLED=1 mise exec -- make fmt` **PASS**. Its changed-file inventory was restricted to work-package source and an owned ignored capture manifest. New topical Go tests have at most 333 raw lines, proving the required 800-effective-line limit; the tree-cancel tests remain in `agent_update_tree_test.go`.

Initial `make typecheck test lint` **FAIL** after passing typecheck: eight backend packages reported inherited application-environment conflicts, temporary-path errors and process-fixture readiness failures; frontend tests/lint were not reached. Isolated-environment remediation passed all eight affected packages with `TMPDIR=GOTMPDIR=/tmp` and unchanged assertions. Full checks are repeated, not replaced by this focused selection.

Safety deviation during that first broad run: an existing unsupported-install test inherited real Kandev service metadata and launched a dummy `v1.0.1` self-update helper. Its journal showed npm `ETARGET` and helper exit 1, not a successful installation; no OMP updater ran. Only its identified dummy intent and npm log were removed. The test fixture is now hermetic through the existing home/environment options; its original assertion passed under ambient service metadata. Subsequent verification clears application environment and uses a fresh HOME. Exact evidence, commands and mitigation are in [Task 02 Results](task-02-settings-and-delivery.md#results).

Repeated isolated `make fmt` **PASS**. The isolated full check then **FAIL** in three existing backend tests with `disk quota exceeded` on `/tmp`; frontend tests/lint were not reached. Moving both temporary roots to owned short disk-backed `/var/tmp/o184.SC9S` passed the affected bounded-history/expiration tests and filesystem/socket cases in four packages without changing assertions. Its full rerun passed typecheck/backend tests, then failed seven frontend tests in five files; lint was not reached.

Frontend verification corrections use the HTTP fixture's existing loopback override and assert Git failure evidence/lock preservation rather than version-specific prose. The supported two-worker budget kept original performance/timeouts and passed the copy-detector/ESLint cases. The remaining failing source-text Sonner wiring test and dead traversal were removed under test-value policy; its five observable behavior tests passed. Exact selections and counts are in Task 02 Results. Formatting and full checks are repeated after these fixture-only corrections.

The repeated format passed. The two-worker full check passed typecheck/backend tests but failed eleven frontend tests in four files, including timeout-triggered UI cascades; lint was not reached. The supported single-worker budget then passed all four affected files (**47 tests**) with unchanged code/assertions/timeouts. The required full check is rerunning under that budget.

The serial full check passed backend/typecheck, **2,807 frontend files / 25,199 tests (4 skipped)** and **5 CLI tests**, then failed script tests on missing `unzip`. A genuine unprivileged package extraction supplied it; the script gate next exposed missing Cargo. Installing Rust `1.92.0` and resolving its proxy environment made the real desktop signature suite pass, including verification and wrong-key rejection. Repository tool/lock manifests were not changed; exact setup/failures are recorded in Task 02 Results. Full gate completion remains pending.

The script rerun next exposed absent desktop Vite assets. Building `pnpm --filter @kandev/desktop build:vite` passed, and the actual rendered startup smoke passed (**1 test**) with existing installed browser binaries. No desktop source or script assertion changed. The full required check is rerunning after preparing the genuine tools/assets.

The next full check passed typecheck/backend tests but failed one frontend ESLint timeout (**2,806 passed files / 25,198 passed tests; 1 failed, 4 skipped**). An actual Vitest configuration probe disproved the earlier serial assumption: the root budget was one, but all three extended projects resolved eight workers. A disposable assertion failed `[8,8,8]` before a three-line config fix and passed `[1,1,1]` after explicitly preserving the resolved budget in each project. Existing budget/selection/ESLint behavior tests then passed (**3 files / 12 tests**) without weaker assertions/timeouts. Formatting and full checks are repeated after that correction.


With the actual one-worker project budgets, the complete typecheck/test chain passed: backend, **2,807 frontend files / 25,199 tests (4 skipped)**, **5 CLI tests** and all scripts, including real signature verification and **28 desktop launch-smoke tests**. Backend lint then found one duplicate notification mode literal. A private constant fixed that finding; focused notification tests and lint passed (**0 issues**). Formatting and the full required chain are repeated before final acceptance.

The next full run passed all typechecks/tests and backend lint (**0 issues**) but web lint rejected nine component/test structure warnings. Private helpers/components, shared keys and smaller test groups fixed the warnings without changing assertions or UI behavior. Focused lint passed with **0 warnings**, followed by **11 affected frontend files / 135 passing tests**. Canonical formatting and the complete check chain are repeated after the refactors.

The following chain passed formatting/typecheck/backend tests but encountered two unchanged frontend Git-fixture timeouts (**2,805 files / 25,197 tests passed; 2 failed; 4 skipped**). Both failed files passed unchanged in isolation (**21 tests**), and full web lint passed with zero warnings. The final rebuilt desktop selection passed **23/25**, including both OMP flows; boundary-flow timeout and subsequent backend-readiness failures remain recorded in Task 02 rather than claimed as acceptance.

Further E2E diagnostics found disk-backed readiness/whole-flow delays and an incidental inventory assertion: the fixed-window summary contained six identities before a later status snapshot knew OMP. Browser/Git fixtures now use private tmpfs storage, with large Go temporaries remaining disk-backed. The summary fixture captures status before releasing the gated startup pass and verifies complete retained eligibility plus validity/distinctness of all delivered members, payload-based counts, navigation and deduplication, instead of equating a historical batch with a later changing inventory. The previously failing real-backend phone summary passed **1/1** without changed deadlines; complete selections are repeated. Full failure counts and the reverted mock-provider experiment are recorded in Task 02.

Complete rebuilt E2E selections then passed **desktop 25/25 and phone 11/11**, one worker, no retries or changed deadlines. Passing captures were inspected against UI-01/UI-02, including external ownership, stable reference, opt-in risk, retained last-change/no-change distinction and saved pause recovery. The final full chain follows `make fmt` with `make typecheck test lint 'PNPM=env TMPDIR=/tmp/o184.zenf pnpm'`: the existing package-manager override changes only small Node/browser/Git temporary storage, leaving large Go/backend temporaries disk-backed and every suite/assertion enabled.

The one-worker final chain passed formatting/typecheck/backend tests but exposed one unrelated coordinator fixture race (**2,806 frontend files / 25,198 tests passed; 1 failed; 4 skipped**). Awaiting the existing React `act` initialization boundary in that fixture retained all assertions; its complete file passed **15/15**, and focused ESLint passed. No coordinator production code changed. The complete chain was repeated with private Node temporaries and the supported two-worker-per-project budget; all test identities, assertions and deadlines remained enabled.

Final acceptance: the isolated sequential `make fmt` and `make typecheck test lint 'PNPM=env TMPDIR=/tmp/o184.zenf VITEST_MAX_WORKERS=2 pnpm'` chain **PASS**. Backend, CLI and repository-script tests passed; web **2,807 files / 25,199 tests PASS, 4 existing skips**. Backend lint **0 issues**; web, harness, specification and architecture lint passed. Final affected frontend **11 files / 135 tests PASS** and `pnpm run i18n:check` **PASS**, including all locales and pseudo synchronization. Latest rebuilt desktop E2E **25/25 PASS** and phone E2E **11/11 PASS**; passing captures were inspected against UI-01/UI-02. Complete commands, earlier failures, safety incident and fixture corrections are retained in the work-order Results.

Final metadata/documentation checks **PASS**: `python3 scripts/list-docs.py validate` (370 decisions / 1,515 specifications), `python3 scripts/lint-spec-files.test.py` (36/36), `python3 scripts/lint-spec-files.py --all`, `node --test scripts/validate-public-docs.test.mjs` (63/63), `node scripts/validate-public-docs.mjs` (47 pages), and `git diff --check`. Owned capture/test-result outputs were removed after inspection.

The first normal commit attempt failed before hooks because the separate user-cache filesystem was full. Commit-cache storage is relocated to task-owned `/var/tmp/o184-home` via `PRE_COMMIT_HOME` and `XDG_CACHE_HOME`; hooks remain enabled and no shared cache is removed.

## Risks

- In-place update can leave the executable changed despite retained old capabilities; warning and manual recovery must not imply rollback.
- Every eligible interval invokes the vendor updater; channels cannot be inferred from stable npm metadata. An updater that declines an installation but exits zero looks like no change; the output excerpt and "reported no version change" copy mitigate this.
- Cadence compares stored pass start times against a 14-minute threshold, so it depends on the existing fifteen-minute ticker; changing that interval changes the real cadence.
- Admission ordering (`runtimeUpdatePassMu`, then `runtimeAutoUpdateMu`, then `s.mu` and the maintenance mutex) is concurrency-sensitive, as is keeping claim, marker and registration in one critical section. Shutdown cancels an admitted updater mid-install; the next `omp update` is its repair, so interruption counts as a failure rather than pausing at once. On Windows only the updater process is cancelled, so package-manager descendants can survive; this is accepted and stated rather than claimed away.
- Existing OMP historical work-order/public documentation claims about stable gating disagree with current source and design. Preserve current manual behavior; do not reintroduce those claims.
- Target-version consumers (notice, service, local provider, toast) need mode-aware handling; an empty target must not drop a pausing failure notice.
