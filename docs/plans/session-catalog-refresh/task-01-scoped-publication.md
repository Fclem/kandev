---
id: "01-scoped-publication"
title: "Publish scoped saved-profile catalog observations"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-CATALOG-001
  - REQ-AGENTS-SESSION-CATALOG-002
acceptance_criteria:
  - AC-AGENTS-SESSION-CATALOG-001.1
  - AC-AGENTS-SESSION-CATALOG-001.2
  - AC-AGENTS-SESSION-CATALOG-001.3
  - AC-AGENTS-SESSION-CATALOG-001.4
  - AC-AGENTS-SESSION-CATALOG-001.5
  - AC-AGENTS-SESSION-CATALOG-001.6
  - AC-AGENTS-SESSION-CATALOG-001.7
  - AC-AGENTS-SESSION-CATALOG-001.8
  - AC-AGENTS-SESSION-CATALOG-002.4
system_design:
  - ../../specs/agents/system-design/session-catalog-refresh.md
  - ../../specs/agents/system-design/session-catalog-task-discovery.md
---

# Task 01: Publish scoped saved-profile catalog observations

## Summary

Deliver the saved-profile refresh API, scoped candidate/notice persistence, and
session event/boot contract. The endpoint is independently exercisable before
editor integration; this order changes no rendered UI and never restarts a
runtime. Also implement accepted replacement-catalog settlement so the next
order consumes a complete state machine rather than adding persistence rules.

## In scope

- Keep the saved-only POST and editor discovery response. Successful generic
  probing gates publication but supplies no session candidates. Acquire separate
  fresh task-owned results under the linked task-discovery design; no draft or
  host-CLI-enriched result may supply them.
- Wire a narrow publication consumer through backendapp. Pin/recheck profile
  context, secret/runtime generation, and refresh currency. Cancellation stops
  remaining fan-out; preserve and report already committed results.
- Add bounded concrete-profile/live-execution session enumeration, including
  initialized IDLE sessions and excluding terminal/archived tasks. Authorize
  each owning task. Use actual concrete profile and executor context.
- Use the revised capability matrix: qualified native Local/Worktree fresh helpers,
  legacy local_pc only with proven context, remote/unknown/unsupported notice-only.
  Implement typed provider capability, not an arbitrary provider-name publisher
  switch. No generic installer/environment equality, new ACP conversations or
  warm-cache list reads as forced-fresh proof.
- Own the backend-only authenticated task-probe API and proxy denials, positive
  process/session-source epoch fences, sealed private launch snapshot, fresh
  helper lifecycle/pagination/cancellation and versioned provenance cutover.
  Remove feature-specific installer-digest checks, obsolete generic candidate
  publication and their misleading positive fixtures; preserve installer sharing.
- Add bounded typed `profile_catalog_refresh` metadata, semantic deduplication,
  monotonic session revision, transactional expected-execution checks, and
  post-commit session-scoped event delivery. Boot/detail returns the record.
- Add owner-correlated terminal discovery results through adapter/session
  response/stream, normalized wire, lifecycle, and orchestration. Report
  complete-empty and mode-only results even for strict-policy LoadSession, plus
  explicit unsupported/failed outcomes; nil transient state is not evidence.
  Update supported ACP/native producers, preserving model/mode surface support
  separately from list length. No readiness/ACK fallback to fabricated success.
  Include accepted launch-owner correlation in the existing launch response or
  correlated session projection for the frontend's observation deadline.
- Settle/clear only the matching notice from an accepted replacement-owner
  complete result, including empty; unsupported/failed ends checking with a
  revisioned reason and retained notice. Disable stale unsupported choices while
  preserving last-known labels. Preserve newer notices and reject stale provider
  ownership. Do not reset runtime configuration or baselines.
- Add deterministic service/handler/repository tests and a throwaway API smoke
  exercising the real route, repository, session event, and reload path.

## Out of scope

- Editor/composer integration, rendered notification, translations, or screenshots.
- Automatic restart, adapter cache injection, SetModel policy changes, freezing
  launch inputs or changing ordinary resume's current-input resolution, new table
  migration, generic credential projection or the superseded attestation protocol.
- Global inbox notifications, new workers, automatic retries, and metrics projects.

## Acceptance

1. A real saved-refresh request creates the expected host candidate or remote
   notice on authorized matching sessions, persists it through repository reopen,
   publishes only after commit, and leaves effective settings/history unchanged.
2. Draft/failed/foreign/stale requests and changed execution ownership cannot
   publish; duplicate catalogs are coalesced. Every listed executor class and
   concrete dynamic-routing identity has a deterministic behavior test.
3. Accepted replacement results settle exactly their eligible notice, including
   strict-policy complete-empty and mode-only/unsupported shapes with no ordinary
   model event. Failed/unsupported results are not fresh-catalog success; delayed
   old reports and newer notices remain protected. Partial persistence failure
   and cancellation preserve committed sessions and honest outcomes.

## Verification

Run from repository root. Load `/tdd` before changing tests or production code.
Confirm RED by an expected candidate/preservation/ownership assertion, not a
missing test symbol. New test names below are proposed targets.

Before any command that writes, confirm the cache and temporary paths exist and
are cache-backed; set the same environment for all test commands:

```bash
findmnt -no SOURCE --target "$HOME/.cache"
findmnt -no SOURCE --target "$HOME/.cache/tmp"
df -h /mnt/clem-cache
export TMPDIR="$HOME/.cache/tmp"
export GOTMPDIR="$HOME/.cache/tmp"
export GOCACHE="$HOME/.cache/go-build"
export GOMODCACHE="$HOME/.cache/go-mod"
(cd apps/backend && go test -tags fts5 ./internal/agent/settings/controller ./internal/agent/settings/handlers -run 'TestSavedProfileCatalogRefresh|TestSavedRefreshDoesNotPublishDraftOrFailedProbe' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run 'TestSessionCatalogRefresh' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/task/repository/sqlite -run 'TestSessionCatalogRefresh' -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/orchestrator -run 'TestSessionCatalogRefreshOrderingAndSettlement' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/adapter/transport/codexappserver ./internal/agent/runtime/lifecycle -run 'TestSessionCatalogDiscoveryResult' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agentctl/server/adapter/transport/codexappserver -run 'TestSessionCatalogTaskDiscovery' -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/agentctl/server/process ./internal/orchestrator -run 'TestSessionCatalogTaskDiscovery|TestSessionCatalogRefreshOrderingAndSettlement' -count=1)
```

Every `findmnt` result above MUST start with `/dev/sdd`; absence or another device
is a failed prerequisite, not permission to continue. Put the new smoke process's
data, logs, and scratch under that same verified cache drive. PostgreSQL
conditional-write coverage must run against a disposable database using the
repository's `KANDEV_TEST_POSTGRES_DSN` convention; record an unavailable database
as a verification blocker, not a passing skipped test.

After GREEN, run the complete changed package tests once:

```bash
(cd apps/backend && go test -tags fts5 ./internal/agent/settings/controller ./internal/agent/settings/handlers ./internal/orchestrator ./internal/task/repository/sqlite ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/adapter/transport/codexappserver ./internal/agent/runtime/lifecycle -count=1)
```

Smoke procedure: launch a disposable backend with mock-agent on cache-backed
storage, seed same-profile host sessions plus foreign/other-profile sessions,
start a turn and retain an input-waiting sibling, and POST the new saved-refresh
route. Observe exact candidates/notices, an unchanged active execution/turn,
unchanged effective model/mode/options, committed event and GET/reopen hydration.
Change the mock discovery source and repeat. Interleave owner replacement and a
newer refresh before allowing a deferred provider report; observe that the newer
notice survives. Do not patch a frontend store to manufacture the result. Remove
throwaway smoke files after recording outcomes.

Run the plan's specification and coverage preflights after documentation changes.
Record the exact runtime command used and observations in Results. No permanent
smoke-only harness or source-text assertion should remain.

## Files likely touched

Existing boundaries:

- `apps/backend/internal/agent/settings/controller/profile_discovery.go`
- `apps/backend/internal/agent/settings/controller/controller.go`
- `apps/backend/internal/agent/settings/handlers/profile_discovery_handlers.go`
- `apps/backend/internal/agent/settings/dto/dto.go`
- `apps/backend/internal/agent/hostutility/profile_probe.go`
- `apps/backend/internal/backendapp/` composition and API route registration
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/agent/runtime/lifecycle/event_types.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go` and `types.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`
- Native Codex adapter and normalized agentctl session response/stream contracts
- Session DTO/boot projection and gateway session-event subscription boundaries
- Process manager final-command/ownership capture; authenticated native client;
  instance API and every browser credential-injecting proxy boundary.

Proposed focused files:

- `apps/backend/internal/agent/settings/controller/session_catalog_refresh.go`
- `apps/backend/internal/agent/settings/controller/session_catalog_refresh_test.go`
- `apps/backend/internal/agent/settings/handlers/session_catalog_refresh_test.go`
- `apps/backend/internal/orchestrator/session_catalog_refresh.go`
- `apps/backend/internal/orchestrator/session_catalog_refresh_test.go`
- `apps/backend/internal/task/repository/sqlite/session_catalog_refresh.go`
- `apps/backend/internal/task/repository/sqlite/session_catalog_refresh_test.go`
- Adapter/native/runtime `session_catalog_discovery_test.go` suites defining
  `TestSessionCatalogDiscoveryResult` against real handshake/finalization paths

Locate current interface/DTO owners before editing; do not grow the already-large
streaming or repository files with the entire feature. Use focused helpers and
the existing ownership/authorization seams. The listed new files are intentional
implementation targets, not claims that they exist at the planning checkpoint.

## Dependencies

None. Existing profile discovery, normalized catalog types, settings-event owner
checks, and session metadata facilities are present. Do not depend on unfinished
Fusion qualification in the adjacent selector repair plan.

## Risks

- Saved-profile permission does not imply access to all consumers of a profile.
- Host equality needs effective launch evidence, not just `local`/`worktree`,
  legacy `local_pc`, or profile ID; empty locality is not proof.
- The old active-session query omits IDLE; do not silently omit waiting runtimes.
- Process-local probe revisions do not order durable per-session observations.
- A JSON-only CAS without execution-owner protection can write onto a successor.
- Partial batch publication and cancellation cannot truthfully report all-session
  success or roll back another session's newly accepted state.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/session-catalog-refresh.md), 001
  and 002.4.
- [Design](../../specs/agents/system-design/session-catalog-refresh.md), Saved
  refresh operation through Persistence and ordering; Explicit restart settlement.
- Existing `profile_discovery_test.go`, `event_handlers_streaming_test.go`, and
  metadata CAS/repository tests as behavioral and dialect patterns.
- Scoped backend guidance and the plan's compatibility matrix.

## Results

### Revised acquisition implementation (2026-11-21)

A later explicit user request authorized implementation of the revised package.
The sections after this one are historical; they do not qualify this boundary.

Generic candidate eligibility is removed. The saved POST remains the editor
response and success gate; `PublishProfileCatalog(ctx, profileID, agentName,
commit)` receives no probe choices or launch context. Installer-digest and
environment-revision comparisons were deleted from candidate admission; the
installer-sharing digest itself is unchanged.

- **Sealed launch and helper.** `process.Manager` seals the successful command
  (`cmd.Path`, argv, `cmd.Environ()`, `cmd.Dir`) per generation and runs a fresh
  helper from it in its own process group. Behavioral RED was the stubbed
  endpoint failing every case. GREEN covers exact live/helper environment, argv
  and cwd (including an empty value and a task credential); only
  `initialize`/`initialized`/paginated `model/list` frames; unchanged live
  advertisement; explicit `[]`; null, absent, malformed, repeated-cursor, page and
  choice bounds; unsupported version; zero/foreign/stale owners and missing
  launch evidence; same-process reload and replacement during the helper;
  cancellation reaping the helper and its grandchild only; one-helper admission.
- **Capability.** The table qualifies only the built-in unwrapped managed
  `@openai/codex@0.169.0 app-server` launch (models only) and the mock-agent
  fixture. ACP, flags, wrappers, custom commands and other versions are
  unsupported.
- **API and proxies.** The instance route requires authenticated control and the
  exact `X-Instance-ID`, decodes a bounded owner-only request and returns
  `no-store` replies. A mutation that returned the plain transport made the
  browser-proxy test RED; `ProxyTransport` now refuses traversal, case,
  encoded and trailing variants. The client binds the reply to its request,
  instance, manager, generation and handshake epoch.
- **Lifecycle and publication.** The client records handshake epochs from
  New/Load/Resume/Reset replies; the manager ID comes from the authenticated
  delivery descriptor. A host launch records an in-memory saved-source digest of
  resolved profile environment, flags, prefix, plugins, provider binding and
  runtime selection; edited sources are not current. The publisher authorizes
  before probing, admits two sessions, bounds fan-out to 60 seconds and probes to
  15 seconds, then rechecks under the existing CAS. Records carry
  `source: task_probe_v1` and the per-surface `probe`; unsupported/failed probes
  become `task_discovery_unsupported`/`task_discovery_failed` notices.
- **Legacy cutover.** Startup retirement converts generic-origin host candidates
  to a newer-revision `context_unverified` notice without touching provider
  choices or settings; SQLite and PostgreSQL contracts pass.

Actual native proof used `@openai/codex@0.169.0` through real `kandev`,
`agentctl` and the saved POST, with isolated cache-backed HOME, a local fake
Responses provider and project catalog files. Local and Worktree sessions kept
live catalog A after their files changed; their helpers returned distinct
task catalogs while the editor returned generic A. Executions, rollout count and
provider-request count were unchanged by the helpers. Consented Stop/Resume of
each kept the same native thread, advertised that task's B catalog, cleared the
notice and left the sibling unchanged. A throwaway harness recorded the
production helper's frames to the real binary as `initialize`, `initialized`,
`model/list`. Evidence: `/home/clem/.cache/session-catalog-5237/native/`.

Final-source package runs used a sanitized environment (no inherited live-service
`KANDEV_*`, private cache-backed HOME) with PostgreSQL enabled. `process`,
`api`, `runtime/agentctl`, `lifecycle`, `orchestrator`, settings
`controller`/`handlers`, `cmd/mock-agent` and `sessioncatalog` passed complete.
`TestPostgresBootInitializesRepositories` failed only because the local DSN had
no password, so the test's rebuilt key/value DSN lost `dbname`; it passed with a
password-bearing DSN. `TestDeliveryRetentionCompleteTerminalOutcomes` hit its
context deadline while E2E and root gates ran concurrently and passed alone. All
other `internal/task/repository/sqlite` and `internal/backendapp` tests passed.
Changed-package `golangci-lint` reported 0 issues; earlier race runs of the
refresh and discovery tests passed.

Root gates: `make fmt` passed, then one sanitized `make -k typecheck test lint`
run passed typecheck, every backend package, CLI and script tests, backend
`golangci-lint` and harness/specification/architecture lint. Its web failures were
two load timeouts in unrelated files that pass alone, plus one Docker-spec
complexity warning since fixed; Task 02 records the details. Docs validators and
`git diff --check` are recorded in the plan. This work order is complete.

### Revised acquisition design checkpoint

New architecture is draft for review, not implemented/qualified. AC001.7-001.8
extend task-owned provenance and non-conversational discovery. Prove actual
native live catalog A, changed task config B, fresh helper B without live cache
mutation, then consented resume B preserving original native history. Inspect
protocol frames: no thread/new/load/resume or prompt from helper. Exercise auth,
proxy, owner/source/binding changes, positive generations, legacy provenance
clearing, malformed/null/paginated results, partial cancellation and two tasks
with distinct catalogs. Unknown ACP/custom capabilities remain explicit; mock
mode coverage is not actual-provider qualification. Existing results below are
historical and do not close this checkpoint. Await later explicit implementation.

Implementation remains in progress. Prior runtime/browser and PostgreSQL
full-package evidence is recorded below; final post-refactor gates remain
incomplete. This work order is not verified or complete, and no implementation
commit exists.

- Focused RED/GREEN evidence covers saved-only raw-probe publication before CLI
  enrichment, preserved saved selection, superseded refresh rejection, runtime
  currency invalidation, execution/prior-record/source CAS, and compatible
  local/worktree/legacy-host versus notice-only executor classification.
- `TestSessionCatalogRefreshOrderingAndSettlement` passed all four terminal
  cases in the focused run: complete-empty, mode-only, unsupported, and failed.
  It also exercised stale attempts, absent and malformed evidence, post-commit
  delivery, duplicate reports, preserved effective settings/draft, and rejection
  of a report after a newer notice. The same command failed later because the
  initial accepted-launch test fixture did not advance readiness; that fixture
  has been corrected, not counted as product evidence.
- The lifecycle discovery suite passed explicit terminal transport, restore
  failure, no-report preservation, and displaced-owner refusal before native
  initialization. SQLite source reservation first reproduced inherited
  `provider_restored` policy, then passed with the actual startup policy.
- Further behavioral RED exposed stopped/failed/completed/prepared executions
  and terminal tasks being admitted, then exposed the same omissions in the
  transactional write after enumeration was corrected. Accepted Resume owner
  correlation and ID-only structured model catalogs also reached expected RED.
  Those lifecycle exclusions, accepted-owner correlation, and grouped-catalog
  fixes passed their focused SQLite/orchestrator regressions.
- Final source freshness reached behavioral RED/GREEN: the runtime reads the
  owned adapter advertisement after applicable settings settlement, rather than
  publishing the initial handshake's stale model list. Focused lifecycle proof
  also rejects stopped-process compatibility.
- Full frozen pnpm installation succeeded offline using Node 24.19.0 in a
  cache-backed workspace mirror. Shared package source exports link to the
  inherited checkout; checkout dependency links resolve to the cache drive.
  Missing Git dependency archives were reproduced from their exact locked
  commits and content-verified; manifests and lockfiles were not substituted.
- Focused real-SQLite multi-connection and orchestrator race checks passed before
  the final independent provider-revision extension. The final complete scoped/
  race commands, real PostgreSQL verification, broad formatting/typecheck/test/
  lint, documentation validation, and complete browser/real-Docker proof remain
  outstanding. A disposable PostgreSQL DSN and concrete Docker-equipped
  executor access have been requested from the parent; neither gate is waived.
- Further real-SQLite RED/GREEN reproduced publication while wrapper changes,
  enabled toggles, and deletion were still pending persistence. Bounded active
  saved-mutation fencing now invalidates old flights and rejects publication
  during those writes. Focused controller/handler checks passed (0.199s/0.081s).
- Provider availability now has an independently owned projection inside the
  existing provider snapshot. Behavioral RED/GREEN demonstrated that unsupported
  evidence survives reload even without an eligible host notice, while a notice
  published after replacement began remains unchanged. Focused orchestrator,
  SQLite, and gateway checks passed (2.333s/0.718s/0.418s). A later-genuine-
  advertisement regression exposed sticky unsupported state; its fix is under
  focused verification, not yet runtime proof.
- Later genuine provider advertisements reached GREEN without changing effective
  intent or baselines (orchestrator 0.658s, ACP 0.275s).
- Built real `kandev`, `agentctl`, and `mock-agent` binaries into
  `/home/clem/.cache/kandev-session-catalog-runtime/bin` and exercised the new
  saved POST against an initialized Local session in an isolated SQLite/home.
  Ordinary `session.stop` and `session.launch` with `intent=resume` preserved the
  session and native conversation. The Resume ACK owner exactly matched the
  committed complete/tombstone owner, including its source generation.
- Earlier runtime inspection observed notice-only for extra workspace GitLab
  inputs and observed `host_candidates` with matching disposable profile/
  workspace credentials. Those observations do not prove full launch-context
  equivalence. The current feature predicate incorrectly reuses installer
  identity: it omits inherited/final child-process context and compares an
  installer projection against a different saved-probe projection. This is a
  feature defect, not only a fixture mismatch. Disabling mock GitLab is not a
  remedy; installer sharing semantics must remain unchanged.
- Shared SQLite/PostgreSQL concurrent-CAS coverage exercises competing initial
  publications and terminal settlement versus a newer refresh. The SQLite and
  orchestrator focused race run passed (2.300s/3.038s). Independent provider
  revision monotonicity across source rotations subsequently reached GREEN
  (orchestrator 0.770s, SQLite 1.059s); a final race rerun remains required.
  PostgreSQL is an unexecuted mandatory gate without a disposable DSN.
- The earlier-projection current-input regression passed in lifecycle (0.074s).
  It does not qualify final child-environment equivalence. This work order
  remains in progress until that correction and all required verification pass.
- Further verification is paused at 91% cache-drive usage. Safe older-cache
  reclamation was attempted and the required Hermes alert was sent. Additional
  cache headroom and the equipped executor are prerequisites, not waivers.

### Launch-environment correction continuation

The reviewed design's actual-context boundary remains authoritative. No
production correction or new behavioral RED has run while cache capacity is
blocked. Existing compatibility/current-input tests cover the earlier projection,
not the complete final child environment.

Capture direction and unapproved evidence proposal:

- Compare the actual successfully launched host-probe and agent commands, not
  maps reconstructed from profile settings. ACP probe capture belongs after
  `sanitizeEnvForAgent`, protocol/runtime preparation and wrapper resolution;
  Codex probe already exposes `launchEnv` from `startForProbe`. Native capture
  belongs at the final `process.Manager` command, after instance Git isolation,
  adapter environment, `StripEnv`, control-credential stripping, managed runtime,
  wrapper and plugin preparation.
- Use `exec.Cmd.Environ()` as the effective-environment producer, preserving Go's
  inheritance and duplicate-key semantics. Also compare actual executable,
  final argv and cwd context. The earlier suggestion to derive this protocol
  from `hostutility.profileKey` is superseded: the
  [unapproved transport proposal](../../specs/agents/system-design/session-catalog-launch-context-evidence.md)
  uses an independent fresh batch seed and explicitly reviewed key domains,
  control-purpose grants and owner binding. No new transport/key contract is
  approved. No raw environment or derived material may enter browser metadata.
- Include actual key presence and exact empty/nonempty values, HOME, PATH,
  indexed Git configuration, credential and wrapper/provider-consumed variables.
  Do not drop empty values or exclude credential names. Native configuration
  strips inherited managed Git authorization and npm variables differently from
  probe construction; a real difference must remain notice-only, not be repaired
  by copying runtime authority into the saved-profile probe.
- Preserve source binding/update evidence separately from resolved-value
  equality, using the existing authorized definition/secret readers. Secret
  metadata exposes `UpdatedAt`, not a numeric version. Missing, ambiguous or
  changed binding/update evidence cannot prove compatibility. Do not implement
  a second environment precedence resolver.
- Bind evidence to the actual process generation and runtime/source owner.
  Missing/legacy evidence, a replaced process, changed launch inputs, or a
  mismatched prepared wrapper/runtime context stays notice-only. Remove the
  feature's installer-digest dependency, not the installer feature itself.

The parent confirmed that no additional generated-variable exclusion is
approved, including for direct providers. Existing builders already remove
control credentials, ACP debug settings and agent-declared `StripEnv` keys
before spawn; their actual absence needs no synthetic normalization. Do not
blanket-exclude `KANDEV_*`, drop empty/unset distinctions, or ignore resolved
wrapper context. Unequal or unproven contexts stay notice-only. This decision
is settled; do not wait for another answer to the same exclusion question.

That safety default does not fulfill AC-AGENTS-SESSION-CATALOG-001.1 if every
ordinary host path is ineligible. The
[normal-host feasibility assessment](../../specs/agents/system-design/session-catalog-refresh.md#unresolved-normal-host-feasibility)
traces built-in Claude and native Codex through their actual probe/live paths:
ordinary utility/task cwd differs, generated/model environment can differ, and
executable/argv equality must be measured rather than inferred. This is
source-derived evidence, not a new runtime capture. Do not manufacture a positive
fixture by projecting runtime secrets or identities into the saved probe.

Required behavioral coverage before accepting the correction: inherited
nonempty value versus explicit empty clear; genuine absence versus empty;
explicit nonempty credentials; existing profile precedence and actual overrides;
changed HOME/PATH or resolved wrapper context; same proven actual context; stale
process/binding/update evidence; and unknown/legacy evidence. Build the positive
runtime fixture from measured equivalent contexts. These checks and the full
mandatory PostgreSQL/Docker/broad verification remain unexecuted.

### Read-only feasibility and security review checkpoint

The main design now records the concrete normal-provider mismatch and leaves
AC-AGENTS-SESSION-CATALOG-001.1 unresolved. A separate draft security proposal
specifies authorized callers/instance owners, purpose grants, challenge/replay
lifetime, independent HKDF domains, process/source/binding/ambient invalidation,
and final executable/argv/cwd/environment inputs. Its auth, key and expiry
policies are not approved or implemented. Review acknowledged the feasibility
blocker as unresolved and declined approval of the transport. Do not implement
or further elaborate that protocol as the next step: attestation cannot make
different launch contexts equivalent.

Await the user-owned choice between a genuine provider-specific catalog-
dependency contract and a reviewed architecture that discovers candidates in an
authorized actual task context. Universal notice-only is a separate explicit
scope reduction, not an implementation fallback satisfying AC001.1.

No production code, test, dependency installation or service launch changed in
this read-only phase. Cache capacity, real PostgreSQL and Docker remain blocked;
earlier native-identity/restart/terminal evidence is retained independently of the
superseded eligibility proof.

### Resumed implementation and native PostgreSQL (2026-11-17)

Cache headroom has recovered.
Unchanged-contract implementation and verification have resumed. Eligibility
correction still awaits the design decision above; no attestation transport or
generated-variable exclusion has been implemented.

Behavioral RED/GREEN now also covers a current owner's terminal availability
and captured-notice settlement arriving after that owner has failed. Publication
still excludes failed sessions. Startup reservation retains its original state
guards, while terminal persistence accepts a failed current owner only behind
the same execution/profile/source and prior-record fences. A replaced owner is
still rejected. The shared SQLite/PostgreSQL contract exercise includes this
boundary. Focused SQLite and lifecycle checks passed (0.564s and 0.269s).

Native disposable PostgreSQL 18.6 was provisioned without sudo or system
installation: official Ubuntu server/client/libpq archives were downloaded and
extracted beneath `/home/clem/.cache/session-catalog-pg-fddb34e4`. The server
uses private peer authentication at the cache-backed Unix socket directory
`/home/clem/.cache/tmp/pg5237-fddb34e4`, port 55437, with no TCP listener.
An actual SQL connection confirmed its cache-backed data directory and
`shared_memory_type=mmap` plus `dynamic_shared_memory_type=mmap`.

`TestSessionCatalogRefreshConditionalWritePostgres` passed against that real
server (2.222s), including the shared failed-owner exercise. The subsequent
configured-PostgreSQL command also passed both scoped packages without skips:

```sh
go test -v -p=2 -tags fts5 ./internal/task/repository/sqlite ./internal/orchestrator -run 'TestSessionCatalogRefresh' -count=1
```

The real PostgreSQL concurrent-CAS case passed (1.31s), and the conditional-write
case with its eleven lifecycle exclusions passed again (1.57s). SQLite
concurrent CAS passed as well (0.29s). Complete controller, handler, orchestrator,
ACP, native Codex and lifecycle packages subsequently passed. The required
ordering/settlement race check passed (2.024s).

The full repository package with PostgreSQL enabled exceeded the global
45-minute deadline near the worktree-ownership cases. The then-current test,
`TestCutoverPostgres_OrphanedSessionWorktreeUsesFlatOwner`, had run for two
seconds; the server had no remaining client backends after the process exited.
This is not evidence of a stuck catalog test. The complete, untrimmed package
subsequently passed (2646.706s) with verbose output and a two-hour deadline:

```sh
go test -v -p=2 -timeout=2h -tags fts5 ./internal/task/repository/sqlite -count=1
```

The saved log is
`/home/clem/.cache/tmp/session-catalog-runlogs-20261117/kandev-run.backend-test.Qh2oLbPK.log`.
The feature's PostgreSQL concurrent-CAS and conditional-write cases ran and
passed again (1.18s/1.38s), including all eleven lifecycle-exclusion subcases.
The package retains twelve unrelated opt-in/dialect-specific skips; those are
not claimed as exercised coverage.

The expanded real desktop/phone smoke now also observes a running same-profile
session, an input-waiting same-profile sibling, and another concrete profile.
Saved refresh preserves all executions and runtime selections; the other
profile's record is unchanged. The later explicit ordinary restart affects
only its captured target and retains typed uncertain-delivery recovery. This
does not qualify the defective launch-equivalence predicate above.

Final broad checks, real Docker executor proof and the unresolved eligibility
correction remain mandatory. No verified implementation or commit exists.

A new local rootless-runtime feasibility probe, `unshare --user --map-root-user
-- true`, failed while writing `/proc/self/uid_map` with `Operation not permitted`.
It made no persistent changes. No Docker runtime was provisioned or tested;
this probe is not executor proof. Real engine access or an equipped executor
remains necessary.
The local rootless helpers `rootlesskit`, `newuidmap`, `newgidmap`,
`slirp4netns` and `fuse-overlayfs` are also absent.

### Current validation and cache checkpoint (2026-11-17)

Owned lint findings led to phase extraction in validation, publication and
transactional metadata writes, without changing the owner/CAS/size/revision
fences. The focused catalog suite passed all seven tested packages with real
PostgreSQL enabled. The subsequent full rerun was cancelled when cache usage
reached 90%; it is not a completed final gate.

A new regression reproduced false completion when one surface reported support
without presence and another reported presence without support. No surface had
an observed supported result. `ValidateDiscovery` now requires the two facts
on the same surface, matching the existing browser completion predicate.
The regression ran RED then GREEN. A temporary executable decoded actual JSON
frames and observed cross-surface rejection plus supported empty mode-only
acceptance; its scaffold was removed.

All active verification writers were stopped at the cache threshold and the
owned PostgreSQL server shut down cleanly. No current managed Go-cache entry was
old enough to remove. An audit found 2,789 unopened/unmapped pre-today generated
Go intermediate files; inode/device/link-count/time rechecks preceded oldest-first
removal of 4,158,107,648 allocated bytes. Cache use returned to 87%. Today's
artifacts, active unrelated Kandev binaries, source trees and database files
were preserved. PostgreSQL resumed on its private socket with mmap shared
memory and bounded 128 MB WAL. Current validator-consumer checks include real
PostgreSQL; complete post-refactor and Docker/eligibility gates remain open.

The equivalent paired-surface expression passes its regression
(`kandev-run.catalog-guard-style.fcclzr6F.log`, 0.004s). Fresh required
formatting passed after the frontend refactors (231.26s).

Current-source verification on 2026-11-18 produced passing results for all seven
required backend packages. Controller, orchestrator, ACP, native Codex, lifecycle
and the complete PostgreSQL-enabled repository passed in
`kandev-run.backend-final-current.qu3pIuEq.log`; the repository took 2397.394s.
Its PostgreSQL concurrent-CAS and conditional-write tests ran and passed,
including all eleven exclusion cases. That aggregate invocation exited
nonzero because three settings-handler update fixtures timed out while their
synchronous discovery reached installed personal CLIs. Isolating those fixtures'
HOME/PATH produced focused GREEN and a complete handler-package PASS (0.710s):
`kandev-run.handlers-final-current.OXI9VZ6d.log`. No update production behavior
or timeout was weakened.

The required ordering/settlement race check passed (2.025s):
`kandev-run.race-final-current.PlTD0ZKT.log`. Full Go lint reported zero issues
in `kandev-run.current-backend-lint.4o4hiThu.log`; scoped lint passed after
the subsequent fixture-only correction.

Full-app typecheck completed. The full web suite was cancelled at 91% cache
usage after a separate Linear lifetime test failed; it is not a pass. Owned
verification writers and PostgreSQL stopped cleanly. Oldest-first recovery
then removed 84,971 unopened, single-link, pre-2026-11-17 cache artifacts after
device/inode/link/time rechecks, totalling 1,018,114,048 allocated bytes. Today's
and yesterday's artifacts, hardlinked dependencies, source trees and databases
were protected. Cache use returned to 90%, with bounded checks resumed.

The remaining backend root-suite failures were also checked with inherited
service settings and PostgreSQL opt-in removed. Backend application (301.372s),
Office routines (2.148s), secrets (1.285s) and Sentry (0.975s) passed. The initial
runner attempt had omitted the retained environment shell arguments and failed
module setup; restoring the verified cache paths and offline proxy corrected
that setup. The unmodified Git 2.53.0 worktree strict-lock failure remains a
separate failing root gate.

Final harness, architecture, specification, public-document and locale checks
passed. Current coverage declarations accept all 168 known changed paths; this
set includes environment/output links and is not a staging list.

Fresh managed browser builds require additional policy-safe cache headroom.
At 90% use, less than 0.5 GB remains before the limit; a 1 GB build budget is not
available. The required Hermes capacity alert was sent successfully to Clem.
No current/recent cache data was deleted to force the build.

Neither the actual launch-context defect nor the Docker-access prerequisite
changed. Full root tests, fresh browser qualification and corrected eligibility
remain required; this is not a completed work order or verified commit.

### Continued verification after capacity recovery (2026-11-18)

Cache capacity recovered to 30% use. After an interruption removed the first
desktop job's retained result, no pass was inferred. Fresh full-build
qualification subsequently passed all 22 desktop and 19 phone cases, with
current captures inspected. These passes do not qualify launch eligibility.

The complete worktree package now passes (231.273s). Only error-wording and
internal attempt-count assertions were removed; strict cases still require no
created worktree and `errors.Is(err, ErrGitCommandFailed)`. No production Git
behavior, retry policy or fallback changed.

Executor discovery still exposes only Local/Worktree runnable profiles. Native
Docker has neither a binary nor a socket; real Docker proof remains unavailable.
No ordinary positive eligibility path is qualified by the current predicate.

### Provider dependency research checkpoint (2026-11-19)

The later direct user choice in the parent is **Research provider-specific
contracts**, recorded around 10:53 in parent plan version
`72ce5036-8480-442f-9421-7ccc02bad8ab`. It supersedes the child's task-context
answer at 10:32:54.307. No task-context rewrite, generated-key exclusion,
attestation transport or universal notice-only reduction is approved. The
implementation owner retains the five documents; no parent rewrite is awaited.

The exact-version model-and-mode dependency matrix and measured results are
recorded in the cache-backed research artifact
`/home/clem/.cache/tmp/session-catalog-provider-research-fddb34e4/provider-catalog-dependency-matrix.md`.
Its primary-source citations are versioned npm archives and fixed upstream
commits, not provider cache-key or installer-identity assumptions.

- Current managed defaults are Claude ACP 0.107.0 (SDK 0.3.323), Codex ACP 2.6.0
  (native dependency range `^0.166.0`), native Codex 0.169.0, Gemini 0.68.0,
  OpenCode 1.19.5/2.3.0, Copilot 1.0.111, Pi ACP 0.0.34 and Muse ACP 0.11.0
  (SDK 1.4.12). These pins do not establish actual process/transitive versions.
- Native Codex's managed default, Kandev's 0.154.0 wire schema and the installed
  host CLI 0.153.4 are separate evidence. The experiment used the official
  0.169.0 binary and upstream commit
  `d32063f96c2dff1dc96af839b3952abb6acfeda4`.
- With isolated HOME/CODEX_HOME, no credentials and only initialize/model-list
  traffic, a retained 0.169.0 process kept `research-first-model` after its
  configured catalog file changed. A new process with identical
  executable/argv/cwd/environment returned `research-second-model`. No thread,
  prompt, login or model assignment was sent. All processes stopped and were
  awaited. This is a freshness counterexample, not eligibility qualification.
- [Native source](https://github.com/openai/codex/blob/d32063f96c2dff1dc96af839b3952abb6acfeda4/codex-rs/app-server/src/request_processors/catalog_processor.rs#L304-L360)
  uses `OnlineIfUncached`; startup configuration is retained until invalidation.
  Its task-owned RPC remains an alternative research seam, not an approved
  architecture or a guaranteed fresh model-and-mode read.
- [OpenCode 1.19.5](https://github.com/anomalyco/opencode/blob/810c0eba1a7348036f8bdf61e6632d0c36c6876c/packages/opencode/src/provider/provider.ts#L649-L716)
  consumes `GITLAB_TOKEN` for provider activation and working-directory model
  discovery. Credential-name exclusion and empty/unset collapse are unsafe.
  [V1](https://github.com/anomalyco/opencode/blob/810c0eba1a7348036f8bdf61e6632d0c36c6876c/packages/opencode/src/acp/service.ts#L735-L795)
  and [v2](https://github.com/anomalyco/opencode/blob/a6b15d33daf6ab7d35569f6d711fdb630b9f04e7/packages/cli/src/acp/catalog.ts#L129-L168)
  scope models and mode agents to the directory; v2 includes location/plugin
  activation and update generations.
- Exact published Claude and Gemini sources also consume cwd settings,
  provider/auth/account inputs and mode policy. Pi modes are native thinking
  levels, not permission modes. Underlying unpinned/unknown runtimes and
  Copilot's unestablished dependency closure remain explicitly unknown.
- The unmodified Muse 0.11.0 mode factory, with actual ACP SDK 1.8.0 and
  compatible frozen-cache Zod 4.4.3, produced different choices for exec/SDK,
  root and explicit YOLO opt-in contexts. This proves those mode inputs matter;
  it is not an authenticated session or model-equivalence test.

Research has not established any irrelevant-context closure or an ordinary
positive generic-utility-to-task path. AC001.1 remains open. A further
provider-specific contract or separately approved discovery design is required
before a compatibility correction can be qualified; none was fabricated.

Native missing/null model-array handling now has actual protocol RED/GREEN:
missing required data fails before terminal completion, while explicit `[]`
remains valid complete-empty. Native/factory/API packages and a standalone
protocol smoke passed; the smoke scaffold was removed. Scoped lint was deferred
after an existing exclusive golangci lock, not bypassed.

The original exact aggregate failed on SQLite's cumulative 10-minute budget,
not an assertion. The untrimmed SQLite suite subsequently passed in 1008.424s
with a 2-hour budget. Preserve the original failed gate. Its retained follow-on
chain continues full web checks with observed restart/selector/onboarding
failures; no cancelled or unobserved result is called a pass.

### Cache gate and cancelled aggregate continuation (2026-11-19)

A pre-TDD capacity check reached 91%. The owned `bg_11` aggregate writer was
cancelled before terminal-owner production/test edits began. Its complete
SQLite pass (1008.424s) remains observed; subsequent full web execution was
cancelled with 11 restart, 1 onboarding-refresh and 10 selector failures plus
localhost:3000 connection refusals. No final web verdict or later
CLI/scripts/lint pass is inferred.

Older secondary build caches were empty. Conservative scans of both compiled
Go caches found no unlinked regular entries whose access/modification evidence
was safely old; current-day/open/hardlinked/source/database data was preserved.
No data was deleted. The mandatory Hermes capacity alert was sent successfully.
Production/TDD writes remain blocked pending safe headroom.

Native regression qualification is precise: the new actual protocol RED/GREEN
case exercised an absent required data array; explicit `[]` was also exercised
successfully. The nil guard also handles JSON null by implementation, but a
null-specific protocol case has not been separately qualified and remains
required. Do not treat that unobserved variant or deferred scoped lint as done.

The process-owner getter defect remains unfixed. Existing client process
generation can bind the read, but zero is a legacy wildcard and cannot prove
ownership. Same-process reload also needs a separate handshake source fence;
ACP settings revisions change on setters and are not a substitute for source
identity. No attestation or task-candidate transport was introduced.

### Independent regression fixes after cache expansion (2026-11-19)

The cache drive expanded to 189 GB and measured 58%, clearing the storage gate.
Provider research is closed. The intervening task-discovery replacement design is
draft and explicitly requires review plus a later implementation request before
new runtime APIs/helpers. The independent fixes below do not implement it.

Process replacement with the same native ID reproduced stale terminal delivery
in both replacement-adapter and retained-pointer cases. Both ran RED, then
GREEN after the existing observation RPC named its process generation and
validated manager ownership before and after the getter. Zero process
generation cannot use the legacy wildcard as proof. The client also checks that
its generation did not change while awaiting the response.

A separate same-process/same-native-ID reload regression ran RED, then GREEN
with adapter-owned handshake source fences around the getter. ACP now has a
handshake counter distinct from settings revisions; native uses its existing
handshake counter. This fence detects changes during observation; it does not
establish host/task launch equivalence or introduce an authorized fresh-read API.

Focused catalog tests passed in API/ACP/native/lifecycle
(0.091s/0.055s/0.040s/0.116s). Explicit-null protocol coverage also passed
alongside absent and complete-empty cases (0.036s); null covered the existing
nil guard, not a newly fabricated pre-fix RED.

An authenticated runtime smoke used the actual API server, HTTP start,
WebSocket client, process manager and disposable ACP child. It accepted an
owned complete-empty catalog, advanced source on same-ID reload, rejected a
legacy-zero process owner, and rejected the retired client after a replacement
was initialized with the same native ID while accepting the new client's
catalog. Every child stopped cleanly; smoke source files were removed. The
fixture was not a real-provider eligibility or Docker qualification.

Root formatting completed (295.58s). `bg_28` then completed: 13 full affected
Go packages passed, adapter/e2e had no test files, four terminal-discovery race
packages passed, and root typecheck passed. The full web run failed eight tests
in four files; CLI/scripts/lint were not reached. Complete output: `artifact://890`.
The subsequent focused four-file test remediation passed 63 tests (160.21s).
Root formatting reran successfully (241.95s). `bg_31` then passed current root
typecheck, all 3373 web files (30818 tests passed/four skipped), and CLI five
tests. Scripts stopped at the updater-signature verifier because the offline
Cargo cache lacked an exact locked crate; lint was not reached in that chain.
Locked Linux Cargo fetch then succeeded into verified cache storage; exact
`make test-scripts` passed (363.76s). Initial root lint reported ST1005 on the new
sentinel's initial capital. Correcting that letter preserved behavior; the native
transport package passed (0.106s), and exact `make lint` exited zero (812.31s),
including backend zero reported issues and web/harness/spec/architecture gates.
Its generated-file-filter warning naming an older external checkout remains in
the retained log; this is not a warning-free claim.
The original default-timeout SQLite aggregate failure remains unwaived.
AC001.1, revised-package implementation approval, real Docker, final aggregate
qualification and verified commit remain open; no review-ready signal.
