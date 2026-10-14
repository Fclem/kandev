---
id: "01-periodic-execution"
title: "Deliver durable periodic OMP execution"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-NOTIFY-004
acceptance_criteria:
  - AC-AGENTS-RUNTIME-NOTIFY-004.1
  - AC-AGENTS-RUNTIME-NOTIFY-004.2
  - AC-AGENTS-RUNTIME-NOTIFY-004.3
  - AC-AGENTS-RUNTIME-NOTIFY-004.4
  - AC-AGENTS-RUNTIME-NOTIFY-004.5
  - AC-AGENTS-RUNTIME-NOTIFY-004.6
  - AC-AGENTS-RUNTIME-NOTIFY-004.7
  - AC-AGENTS-RUNTIME-NOTIFY-004.8
  - AC-AGENTS-RUNTIME-NOTIFY-004.9
  - AC-AGENTS-RUNTIME-NOTIFY-004.10
system_design:
  - ../../specs/agents/system-design/omp-automatic-updates.md
---

# Task 01: Deliver durable periodic OMP execution

## Summary

Extend the existing consent/background/maintenance pipeline to execute trusted automatic OMP channel checks without stable-version gating. Deliver durable scheduling, truthful changed/no-change/failure results, and existing notification compatibility without changing manual OMP or pinned automation behavior.

## In scope

- Read the requirement, design, ADR and scoped backend guidance. Use `/tdd`; inspect exported symbol references through LSP before changing capability, policy, outcome or notice contracts.
- Extend trusted built-in OMP capability; preserve external ownership and existing identity. Unknown/custom/external recipes remain non-automatic.
- Reuse `AutoUpdateStore` with zero-default fields `last_attempt_started_at`, `consecutive_failures`, `paused`, `last_change`, and outcome fields mode/operation/observed version/output excerpt. Add backward-compatible JSON reads; no duplicate persistence subsystem. Add derived `auto_update_paused` and `last_change` to `AgentUpdateStatusDTO` (fill in `agent_update_status.go`). A saved false-to-true harness transition clears pause and failures and never clears `last_attempt_started_at`; an idempotent true save changes nothing.
- Branch `RunRuntimeUpdatePass` for opted-in OMP independently of metadata/check state, after interrupted-attempt settlement. Eligible when consented, enabled, available, not paused, no active maintenance, and at least 14 minutes after `last_attempt_started_at`; both times are pass start times and a negative difference counts as eligible. Busy maintenance writes nothing and does not consume the interval.
- Add automatic harness admission synchronously inside `RunRuntimeUpdatePass` (serialised by `runtimeUpdatePassMu`). Check availability through one helper, `c.agentAvailable(ctx, ag)`, before taking any lock, with explicit semantics shared with the status path (nil discovery is available, mock-mode synthesized availability is honoured, otherwise the single-agent `ag.IsInstalled(ctx)`), injectable in tests; the status path's `detectAgents` must not run under the consent mutex. Take a job-store semaphore slot without blocking (a full semaphore defers to the next pass and consumes nothing). Take `runtimeAutoUpdateMu` as the outermost lock and re-read under it the policy (consent, paused, `last_attempt_started_at`), trusted recipe and enablement, re-evaluating eligibility instead of trusting the earlier status snapshot, so a pause written by `settleInterruptedAutomaticUpdate` in the same pass (a stale marker settled as the third failure) admits nothing. Call a new store method that, in one `s.mu` critical section, claims the maintenance slot, runs a `beforeStart`-style callback persisting the running marker, and registers the job in `s.jobs` and `activeByAgt`, so no claimant ever sees a held claim without a registered job; any failure releases the slot and claim and persists nothing. Keep the lock order `runtimeUpdatePassMu`, `runtimeAutoUpdateMu`, `s.mu`, maintenance mutex. Release the consent mutex before the subprocess; only after the critical section broadcast `agent.update.started` and start the worker, which already holds the slot. The job store therefore never exposes an unadmitted automatic job through `Get`, `GetActive` or `ListAll`. Derive the updater and probe contexts from the background pass context (as pinned jobs do via `job.context`), not `context.Background()`, so shutdown and restore quiesce cancel an admitted update promptly; start the updater's hard timeout when the worker begins running it. The probe runs on a context derived from the pass context, not the updater's hard-timeout context, and keeps the host manager's 60-second cap. After `Probe` returns, classify `cancelled` only when `errors.Is(passCtx.Err(), context.Canceled)`, regardless of the returned error or `caps.Status` (the host probe swallows cancellation); a deadline on the hard-timeout child stays `updater_deadline`. Keep late terminal results scoped to their job ID.
- Give `runDirectCommand` (`agent_update.go`) an opt-in tree-cancel mode for the automatic path, exposed through an optional `RuntimeTreeUpdater.RunUpdateTree` interface on `hostRuntimeUpdater` backed by a new `directCommandExecutor` method; automatic support requires it, so a fake or updater without it makes automation unsupported rather than falling back. `cmd.WaitDelay` alone does not release `StdoutPipe`/`StderrPipe` scanners, so in this mode route output through line-splitting `io.Writer`s on `cmd.Stdout`/`cmd.Stderr` (Cmd-owned copy goroutines), start the command in its own process group on POSIX, have `cmd.Cancel` send SIGTERM to the group, arm a five-second named-constant timer that SIGKILLs the whole group (stop it when `Wait` returns), and set `cmd.WaitDelay` to the timer plus one second so the pipes close even if a descendant ignores both signals. Treat `exec.ErrWaitDelay` with an uncancelled context as success and continue to the probe. Windows kills the parent and relies on `WaitDelay`, so descendants can survive there; state that residual. Put the group handling in build-tagged files (`!windows` and `windows`). Manual updates keep the current behavior. On POSIX the accepted residual is a descendant that detached itself.
- Record a typed automatic result on `AgentUpdateJob` (cause `changed`, `repaired`, `no_change`, `updater_error`, `updater_deadline`, `validation_failed` or `cancelled`, plus previous and observed versions and the excerpt) and pass it to a new `onAutomaticFinished` hook wired in `controller.go` beside `onFinished`. In `complete()`, for `self_update` jobs call `onAutomaticFinished` instead of `onFinished`, before the active-entry delete, and make `retainAutomaticOutcome` return early for `self_update` jobs so it cannot overwrite the typed result with `string(job.Status)` or publish a second status. Do not infer the cause from `job.Status` or the error string. The hook applies the table's accounting, persists the policy and then publishes the terminal status carrying `auto_update_paused` and `last_change`, so a pausing-failure notice is not deferred to the next pass.
- Dispatch to `onAutomaticFinished` only when `Automatic && UpdateMode == self_update`; a manual OMP job (also `self_update`, not `Automatic`) keeps the existing `onFinished` path and must never reach the hook. Store the attempt ID in `last_change` and have `publishRuntimeStatus` publish the changed notice from `last_change` so replay survives later no-change checks.
- Map outcomes explicitly per the design table, not from `job.Status`: changed `update`, previous-unknown `repair` (no `last_change`), same version `up_to_date` with output excerpt, updater error or updater deadline (failed; retry, pause at three consecutive), validation failure, probe `auth_required`, probe timeout or versionless probe (failed; pause), shutdown cancel or running marker with no worker (interrupted; counts as a failure, no immediate pause). Empty/unusable observation is failure. Manual unchanged-version still fails. Keep the previous catalogue on failure. Persist `last_change` so no-change cannot erase it. Sanitize and cap the output excerpt when persisting: strip ANSI CSI and OSC sequences and C0 and C1 controls except `\n` and `\t`, normalise `\r\n` and `\r` to `\n`, keep at most the final 2 KiB at a rune boundary. If terminal persistence fails the running marker remains and the next pass settles it as interrupted. After admission the job DTO carries `automatic: true`, status `succeeded` with operation `update`, `repair` or `up_to_date`, or `failed` with the error.
- Manual OMP update keeps going through the same claim and leaves consent and pause unchanged. At the controller, a manual request while an automatic job is active returns `MaintenanceConflictError` (409) and runs no second updater.
- Extend `RuntimeUpdateNotice`, service payload and `publishRuntimeStatus` by update mode, and add `update_mode` to the fixed payload key list in `notifications/providers/local.go`. Notify only for changed results (observed version) or pausing failures (previous version as context; service must accept a versionless self-update notice). Suppress repair (validated version), no-change and non-pausing failures. Deduplicate by attempt ID. No pin rollback/default wording; pinned messages unchanged.
- Add deterministic tests in topical files, each under the 800 effective-line limit: `omp_autoupdate_cadence_test.go`, `omp_autoupdate_outcomes_test.go`, `omp_autoupdate_admission_test.go`, `omp_autoupdate_shutdown_test.go` and a shared `omp_autoupdate_fixtures_test.go`, plus existing policy/controller/service/provider suites for the plan matrix. Put the `TestRunDirectCommandTreeCancel*` cases in a new `agent_update_tree_test.go`, because `agent_update_test.go` is already near the limit. RED must prove actual forbidden/missing execution or incorrect retained result, not missing scaffolding. Use a clock seam/deferred runner, no interval sleeps.
- Extend `apps/backend/internal/agent/registry/runtime_update_coverage_test.go` to assert that exactly `omp-acp` carries the automatic harness recipe and no other registration does, managed or external; its current guard fails any non-managed registration with `cap.Managed != nil` but does not police the new mutation-authority path.
- Smoke through a disposable actual controller and trusted updater/probe fixture: one changed check, one no-change check, one opted-out pass, and one credential-less probe that returns `auth_required` (record OMP's actual result and apply the design's open policy check); observe commands, retained results and catalogue. Keep this temporary smoke outside permanent tests and remove it after evidence. Never mutate a real OMP installation/user database.

## Out of scope

Rendered Settings, frontend copy, Playwright and public documentation are Task 02. No new installer/channel/target, runtime flag, remote cache mutation, session restart, or npm pin path. Retry is limited to the design's normal-interval retry after an updater error.

## Acceptance

1. Durable saved consent starts a trusted OMP check on an eligible background pass regardless of stable status, respects the start-time cadence across restart (attempts at least fourteen minutes apart), and cannot overlap maintenance or cross a pre-start withdrawal; guard failure persists nothing and shutdown cancels an admitted update, including a forked package-manager child, promptly and records interrupted without pausing before the third.
2. Every outcome-table row retains truthful version evidence and pause semantics, with catalogue publication only after healthy validation, no invented target, `last_change` retained, and a saved off/on clearing pause. Existing pinned automation and manual harness tests remain green.
3. Existing notification delivery, including the local provider payload, suppresses no-change and non-pausing noise and conveys changed and pausing outcomes without pin recovery claims or dropping unknown-version failures; disposable execution smoke records actual invocation and withdrawal behavior.

## Verification

Run from repository root. Use RED/GREEN targeted `-run` cases during development, then the complete package commands below. Do not run build/lint/formatters mid-flight; final requested formatting/full gates belong to Task 02.

```bash
(cd apps/backend && CGO_ENABLED=1 go test -tags fts5 ./internal/agent/agents ./internal/agent/managedruntime ./internal/agent/registry ./internal/agent/settings/controller ./internal/agent/settings/handlers ./internal/notifications/service ./internal/notifications/providers)
(cd apps/backend && CGO_ENABLED=1 go test -race -tags fts5 ./internal/agent/settings/controller ./internal/agent/managedruntime ./internal/notifications/service ./internal/notifications/providers -run 'OMPAutomatic|OMPManual|HarnessUpdate|Automatic|RuntimeUpdate|TreeCancel' -count=1)
(cd apps/backend && GOOS=windows CGO_ENABLED=0 go vet ./internal/agent/settings/controller)
```

Record the disposable smoke command/output in Results, then remove its scaffold. Tests alone do not prove real subprocess integration.

## Files likely touched

- `apps/backend/internal/agent/agents/runtime_update_capability.go`, `omp_acp.go`, `runtime_update_notice.go`, and adjacent behavioral tests.
- `apps/backend/internal/agent/managedruntime/autoupdate.go`, `autoupdate_test.go`.
- `apps/backend/internal/agent/settings/controller/agent_autoupdate.go`, `agent_update.go` (`runDirectCommand` tree-cancel mode), new build-tagged `agent_update_proc_unix.go` and `agent_update_proc_windows.go`, `agent_update_harness.go`, `agent_update_job.go` (`complete()` and finish paths), `agent_update_status.go`, `controller.go` (hook wiring), `runtime_update_background.go`, `runtime_update_notifications.go`.
- Existing controller automation/harness/background tests, new `agent_update_tree_test.go` (forked-grandchild fixture and `TestRunDirectCommandTreeCancel*`), the new topical `omp_autoupdate_*_test.go` files listed above, and `apps/backend/internal/agent/registry/runtime_update_coverage_test.go`.
- `apps/backend/internal/agent/settings/dto/dto.go` and `handlers/runtime_update_policy_test.go`; policy handler only if existing permission/schema handling needs change.
- `apps/backend/internal/notifications/service/runtime_updates.go`, `runtime_updates_test.go`, `apps/backend/internal/notifications/providers/local.go`, `local_test.go`, and related existing notification contract consumers discovered through references and text search (the local provider uses a fixed payload key list).

## Dependencies

None. Package is not implementation authorization; wait for the later explicit user request. Task 01 is not releasable alone: its consent endpoint would accept OMP before Task 02's warning UI exists, so both work orders ship in one pull request.

## Risks

In-place mutation precedes validation; the old catalogue is not rollback. Persisted attempt/worker/claim ordering is concurrency-sensitive. Notification version currently implies a target and must become mode-aware. Real vendor execution evidence must remain distinct from deterministic fixture coverage.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/omp-periodic-updates.md), active REQ-AGENTS-RUNTIME-NOTIFY-004.
- [Design](../../specs/agents/system-design/omp-automatic-updates.md), all backend and notification sections.
- [Decision](../../decisions/2026-10-10-periodic-omp-self-updates.md).
- Existing `agent_autoupdate_test.go`, `agent_update_harness_test.go`, `runtime_update_background_test.go`, `runtime_updates_test.go` patterns.

## Results

Implemented trusted OMP automatic authority, durable consent/cadence/accounting, synchronous maintenance admission, bounded tree cancellation, typed outcomes, status projection and mode-aware notification delivery. OMP remains externally installed; pinned and manual paths remain separate.

### TDD evidence

- RED: enabling OMP was rejected; stable-unknown/equal/older/newer cases did not admit a check. GREEN: eligible consented cases each invoked one updater with no invented target.
- RED: restart admitted a check before fourteen minutes; re-enabling retained failure/pause state; stale third failure admitted a fourth check. GREEN: start-time spacing, saved off/on recovery and third-failure pause.
- RED: retained results lost operation/output, accepted unusable observations and omitted failure accounting. GREEN: typed changed/repair/no-change/failure matrix, bounded sanitized multiline excerpt and last-change retention.
- RED: changed notification was lost after a later check, versionless failure was dropped, and the local payload omitted mode. GREEN: change replay, suppression, versionless deduplicated failure and local `update_mode`.
- RED: stale availability admitted OMP after its executable disappeared. GREEN: single-agent availability guards admission and honours mock-mode synthesis.
- Tree-cancel fixture: cancellation releases inherited pipes and kills the POSIX group; a zero-exit updater with inherited pipes proceeds despite `exec.ErrWaitDelay`.
- RED: an install claimant observed the automatic maintenance claim before the running marker and job registration completed. GREEN: admission holds the maintenance mutex through persistence/registration; refused persistence leaks neither job nor claim.
- Deterministic boundary coverage passes for deadline-edge success/error classification, a slow updater preserving the probe's full allowance, shutdown cause reaching the typed finish hook once, terminal-save failure retaining the running marker, and refusal of an updater without tree cancellation.

### Executed checks

Go commands used task-owned `GOCACHE=/home/clem/kandev-omp-cache/go-build` and `TMPDIR=/var/tmp/kandev-omp-scratch`. The default managed cache was full; no shared cache or user installation was removed.

- `env GOCACHE=/home/clem/kandev-omp-cache/go-build TMPDIR=/var/tmp/kandev-omp-scratch CGO_ENABLED=1 go test -trimpath -tags fts5 ./internal/agent/agents ./internal/agent/managedruntime ./internal/agent/registry ./internal/agent/settings/controller ./internal/agent/settings/handlers ./internal/notifications/service ./internal/notifications/providers` (from `apps/backend`): PASS, all seven packages. First run exposed handler fixtures probing installed tools before their two-second deadline; isolated mock discovery and fixture `PATH` instead of weakening the deadline.
- `env GOCACHE=/home/clem/kandev-omp-cache/go-build TMPDIR=/var/tmp/kandev-omp-scratch GOOS=windows CGO_ENABLED=0 go vet ./internal/agent/settings/controller` (from `apps/backend`): PASS.
- `env GOCACHE=/home/clem/kandev-omp-cache/go-build TMPDIR=/var/tmp/kandev-omp-scratch CGO_ENABLED=1 go test -trimpath -race -tags fts5 ./internal/agent/settings/controller ./internal/agent/managedruntime ./internal/notifications/service ./internal/notifications/providers -run 'OMPAutomatic|OMPManual|HarnessUpdate|Automatic|RuntimeUpdate|TreeCancel' -count=1` (from `apps/backend`): PASS; provider package had no matching tests.
- Final expanded race command added `|LocalProviderForwards` to the same `-run` expression: PASS, all four packages exercised. The seven-package acceptance and Windows vet commands above also passed after the atomic-admission fix.

### Disposable real-controller smoke

`env GOCACHE=/home/clem/kandev-omp-cache/go-build TMPDIR=/var/tmp/kandev-omp-scratch CGO_ENABLED=1 go test -trimpath -tags fts5 ./internal/agent/settings/controller -run TestOMPDisposableControllerSmoke -count=1 -v` (from `apps/backend`): PASS, four scenarios. The actual controller used built-in OMP metadata and its trusted `omp update` recipe; a fixture redirected execution to disposable updater/probe executables through the real tree executor. No real updater ran. The temporary `omp_autoupdate_smoke_test.go` was removed after the run.

| Fixture scenario | Observed retained result |
| --- | --- |
| Changed | `succeeded`, operation `update`, observed `1.1.0`, last-change retained, failures 0, not paused; catalogue `1.1.0`, one publication |
| No change | `succeeded`, operation `up_to_date`, observed `1.0.0`, no last-change claim, failures 0, not paused; catalogue stays `1.0.0`, zero publications |
| Opted out | No job; fixture invocation marker absent; catalogue unchanged |
| Credential-less `auth_required` | `failed`, unknown observed version, failures 1, paused; catalogue stays `1.0.0`, zero publications |

All executed updater scenarios retained plain-text `fixture updater complete\n`; ANSI colour codes were removed.

### Actual OMP authentication policy checkpoint

- `env GOCACHE=/home/clem/kandev-omp-cache/go-build TMPDIR=/var/tmp/kandev-omp-scratch make -C apps/backend build-acpdbg`: PASS.
- `env -i HOME=/var/tmp/kandev-omp-scratch/credentialless-home XDG_CONFIG_HOME=/var/tmp/kandev-omp-scratch/credentialless-home/config XDG_DATA_HOME=/var/tmp/kandev-omp-scratch/credentialless-home/data XDG_CACHE_HOME=/var/tmp/kandev-omp-scratch/credentialless-home/cache PATH=/usr/bin:/bin:/home/clem/.local/bin TERM=dumb apps/backend/bin/acpdbg probe --exec '/home/clem/.local/bin/omp acp' --workdir /var/tmp/kandev-omp-scratch/credentialless-work --file /var/tmp/kandev-omp-scratch/credentialless-omp.jsonl --timeout 60s --stderr`: PASS. Actual installed OMP `18.8.7`, ACP protocol 1, `initialize` and `session/new` succeeded without inherited credentials; two modes, zero models, advertised local-credential authentication method, close exit 0.

The actual credential-less handshake did **not** return `auth_required`. The design's conditional change to retry-counted authentication failures therefore does not apply; immediate pause on fixture `auth_required` remains. This evidence proves credential-less handshake only, not a real vendor update or prompt. Captured frames and all probe-written data are isolated under the disposable scratch directory; the real installation and user home were not changed.
