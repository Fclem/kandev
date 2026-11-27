---
id: "02-composer-refresh"
title: "Refresh session catalogs from the composer"
status: in_progress
wave: 2
depends_on:
  - "01-scoped-publication"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-CATALOG-001
  - REQ-AGENTS-SESSION-CATALOG-002
  - REQ-AGENTS-SESSION-CATALOG-003
acceptance_criteria:
  - AC-AGENTS-SESSION-CATALOG-001.1
  - AC-AGENTS-SESSION-CATALOG-001.2
  - AC-AGENTS-SESSION-CATALOG-001.3
  - AC-AGENTS-SESSION-CATALOG-001.4
  - AC-AGENTS-SESSION-CATALOG-001.5
  - AC-AGENTS-SESSION-CATALOG-001.6
  - AC-AGENTS-SESSION-CATALOG-001.7
  - AC-AGENTS-SESSION-CATALOG-001.8
  - AC-AGENTS-SESSION-CATALOG-002.1
  - AC-AGENTS-SESSION-CATALOG-002.2
  - AC-AGENTS-SESSION-CATALOG-002.3
  - AC-AGENTS-SESSION-CATALOG-002.4
  - AC-AGENTS-SESSION-CATALOG-002.5
  - AC-AGENTS-SESSION-CATALOG-003.1
  - AC-AGENTS-SESSION-CATALOG-003.2
  - AC-AGENTS-SESSION-CATALOG-003.3
system_design:
  - ../../specs/agents/system-design/session-catalog-refresh.md
  - ../../specs/agents/system-design/session-catalog-task-discovery.md
---

# Task 02: Refresh session catalogs from the composer

## Summary

Connect saved-profile Refresh to the publication endpoint and expose candidate
choices plus explicit session rediscovery in task, preview, and Quick Chat
composers. Finish the flow with real desktop, phone, and isolated-executor browser
evidence, translations, and guidance. Reuse existing restart rather than
introducing a second runtime lifecycle.

## In scope

- Route only explicit saved-baseline refreshes to Task 01's endpoint. Preserve
  pure draft/new-profile/automatic discovery and all existing save/draft guards.
- Add typed API/event/boot contracts, revision-aware store hydration, and the
  dependency-neutral candidate view model. An ordinary provider snapshot must
  not erase a pending host observation; a clearing tombstone must survive stale
  boot/HTTP data.
- Accept candidate choices only with current `task_probe_v1` provenance and owner.
  Legacy generic candidates become a revisioned notice without resetting provider
  selections. Task probe failure/unsupported is not a provider terminal result,
  never starts the post-Resume deadline and cannot settle its notice.
- Integrate model and mode candidates without importing host defaults or dependent
  option state. Preserve normalized grouping, current labels, and exact identity.
  Advertised choices use normal handlers; host-only activation opens confirmation
  with no SetModel/SetConfigOption request.
- Expose the localized composer notice even when the model trigger is hidden.
  Capture task/session/revision, reuse existing confirmation and `useSessionRestart`,
  disclose current profile environment/flags/wrapper resolution, and retain
  pending/recovery distinctions until an owner-correlated terminal result.
  Bound missing-result checking to 60 seconds after Resume ACK in one shared
  store-scoped deadline; timeout retains a retryable notice without runtime action.
- Add all translations, Traditional Chinese and pseudo generation, accessibility,
  desktop density, touch targets, responsive state/focus guards, and concise user
  guidance in `docs/public/agents-and-profiles.md` (reference page). Update the
  root README only if its future-session-only summary becomes inaccurate; do
  not change screenshot catalog entries without an actual replacement asset.
- Add focused component/state tests and desktop/phone/Docker E2E flows using the
  real saved refresh and runtime paths. Record actual screenshots and inspect
  the changed surface against the previews.
- Verify ordinary resume uses current saved launch inputs, including edits made
  while confirmation is open. Preserve saved model/mode/config-option overrides
  under the existing policy; do not promise or implement frozen env/flags/wrapper.
  An account/wrapper-related restore failure must preserve native conversation
  identity and expose recovery rather than create a replacement conversation.

## Out of scope

- Adapter cache injection, unadvertised RPCs, automatic restart/retry, remote host
  catalog import, runtime package updates, or changing resume-settings policy.
- A new conversation, context reset, uncertain prompt replay, global notification
  integration, or completing the separate Fusion qualification work.

## Acceptance

1. Explicit saved-profile Refresh reaches an already-open compatible host picker,
   while dirty drafts do not publish and remote sessions keep executor choices.
   New models and modes are visible as candidates without changing current work;
   existing advertised choices remain normally selectable.
2. The notice/candidate flow requires explicit restart consent, targets the same
   captured session/executor, retains conversation and input, rejects duplicate
   restarts/submissions while pending, and distinguishes terminal complete-empty,
   unsupported, failure, and missing-report outcomes. Checking ends by 60 seconds
   after Resume ACK if no terminal result arrives. Cancel, changed profile launch
   inputs, failed requests, and blocked recovery retain honest state and recovery.
3. Desktop, phone, preview, and Quick Chat share state semantics. Phone controls,
   containment, focus, loading/failure announcements, reload/reconnect ordering,
   translations, public guidance, and the complete planned E2E flows pass.

## ASCII UI preview

Use [the full plan previews](plan.md#ascii-ui-preview). These excerpts retain the
same labels and apply to 002.1-002.5 and 003.1-003.3.

### UI-5237-01: Desktop composer and candidates

```text
[Current model v] [Profile models refreshed *]               [Send]
  Current model                                         selected
  Newly discovered model                Requires session refresh >
```

### UI-5237-02: Desktop confirmation

```text
Refresh session
Restart this agent to check models and modes.
This can interrupt its current turn. Conversation and draft retained.
Current profile environment, flags, and wrapper apply on resume.
                                      [Cancel] [Refresh session]
```

### UI-5237-03: Phone composer and confirmation

```text
[Current model v]          [Send]
Profile models refreshed
[Refresh session]

+-----------------------------------+
| Refresh session                   |
| Restart to check models/modes.    |
| This can interrupt this turn.     |
| Conversation and draft retained.  |
| Current profile inputs apply.     |
| [Cancel]       [Refresh session]  |
+-----------------------------------+
```

### UI-5237-04: Status

```text
Restarting session...           (editable draft; Send blocked)
Checking session models...     (request ACK is not completion)
No additional choices reported by this executor.
Session refresh failed.        [existing recovery action]
No choices reported by this executor.     (terminal complete-empty)
Model discovery unsupported.             [existing recovery]
Could not confirm choices.               [Refresh session]
```

Required: current label, notice outside the trigger's hydration gate, explicit
interruption consent, one scoped status, phone wrapping without hover, and
44px phone/coarse-pointer actions. Reuse the existing confirmation scroll owner
and safe-area clearance. No new composer scroller. Labels are illustrative;
all product copy is localized.

## Verification

Run from repository root; load `/tdd`, `/e2e`, and `/mobile-parity`. Follow the
Task 01 cache-drive prerequisite and export the same TMPDIR/GOTMPDIR/Go-cache
environment. Confirm `/dev/sdd` backing before pnpm caches, generated assets,
Vite output, backend binaries, mock runtime data, Playwright output, or browser
profiles can be written. Use existing cache-drive binds or supported runner output
configuration; do not build into an unbound iSCSI checkout. In a fresh worktree,
install once before the first pnpm command:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Use behavioral RED cases for saved-only publication, advertised-versus-candidate
selection, mode preservation, revision ordering, and explicit consent. Do not
accept a missing-selector failure as behavioral RED. Proposed test files:
Cover the no-report deadline with a controlled clock at just before and at 60
seconds after Resume ACK, a matching late result, a newer notice, navigation,
reload, and multiple composer mounts. Test terminal-result-before-ACK as well as
ACK-before-result: already-settled state must not become a new checking spinner.
Suppress the actual terminal result in the browser fixture; never substitute a
delayed ACK as the missing-result scenario.

```bash
(cd apps/web && pnpm exec vitest run \
  hooks/domains/settings/use-profile-capability-discovery.test.tsx \
  lib/ws/handlers/session-catalog-refresh.test.ts \
  lib/state/slices/session-runtime/session-model-hydration.test.ts \
  lib/state/slices/session-runtime/session-model-catalog-hydration.test.ts \
  components/model-config-selector-catalog.test.tsx \
  components/task/model-selector-catalog-refresh.test.tsx \
  components/task/model-selector-catalog-modes-routing.test.tsx \
  components/task/session-catalog-refresh-notice.test.tsx \
  hooks/domains/session/use-session-restart.test.ts \
  hooks/domains/session/use-session-catalog-restart.test.ts \
  components/task/chat/chat-input-area-restart.test.tsx \
  components/task/chat/chat-input-area.test.tsx)
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator -run 'TestSessionCatalogRefreshUsesCurrentLaunchSettings|TestSessionCatalogRefresh' -count=1)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/session/session-catalog-refresh.spec.ts tests/session/session-tab-restart.spec.ts tests/settings/profile-capability-discovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-catalog-refresh.spec.ts tests/session/mobile-session-tab-restart.spec.ts tests/settings/mobile-profile-capability-discovery.spec.ts)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers tests/docker/session-catalog-refresh.spec.ts)
```

Managed E2E rebuilds the production assets; do not use `--no-build` after changes.
Check discovery includes every named new spec. Do not overlap suites or override
the managed worker budget. If a new pure helper suite is added, run it too and
list its actual command in Results. No mock-only provider event may stand in for
the real endpoint-to-session path. Docker absence is a runtime-proof blocker,
not justification to claim remote execution was verified.

Capture and inspect actual UI-5237-01 through UI-5237-04 at desktop fine pointer,
phone touch, and 767px fine-pointer phone. Exercise the two-tab refresh while a
turn is running and observe that no stop/launch/model-mutation request occurs
until explicit consent. Verify a real new candidate/mode and later provider
selection after rediscovery, not merely that a notice exists. Include unchanged
catalog and Resume failure. Confirm persisted session/native conversation identity,
history, unsent draft/attachments, and exact executor before/after; preserve
normal uncertain-delivery recovery instead of forcing its completion.
For revised acquisition, additionally prove two authorized task contexts receive
distinct task-owned catalogs while settings may show a third generic catalog.
Real native fresh-helper acquisition must run through the endpoint; mock frames
alone do not qualify the native positive path. Unsupported ACP/remote sessions
keep their own choices and truthful notice. Repeat legacy metadata/reload and
stale-owner tests against versioned provenance. Existing screenshots/runs are
historical until this new source contract is exercised.

The user requested these final implementation commands, in this order, after
the focused checks and with cache-backed generated/build outputs:

```bash
make fmt
make typecheck test lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check -- docs/specs docs/plans/session-catalog-refresh docs/public README.md
```

Run the plan's documentation-coverage preflight on every actual changed path and
all referenced artifacts; record the results. Keep work orders pending/in progress
until all specified evidence exists. Do not repeatedly run broad checks mid-edit.

When the implemented behavior matches the pair and both work orders pass,
record every command/result, promote requirements to `active`, design to
`current`, plan to `implemented`, and both work orders to `done`. Leave unresolved
provider/runtime evidence explicit rather than marking it complete. Commit only
in the later authorized implementation/delivery turn, using explicit paths and
the `/commit` skill; the planning package remains unstaged and uncommitted.

## Files likely touched

Existing boundaries:

- `apps/web/hooks/domains/settings/use-profile-capability-discovery.ts`
- `apps/web/hooks/domains/settings/use-profile-capability-discovery.test.tsx`
- `apps/web/lib/api/domains/profile-capability-api.ts`
- `apps/web/lib/types/http-agents.ts` and generated backend wire types
- `apps/web/lib/state/slices/session-runtime/` types, actions, hydration
- `apps/web/lib/ws/handlers/` event registration and scoped session handling
- `apps/web/components/task/model-selector.tsx`
- `apps/web/components/task/model-selector-change-handlers.ts`
- `apps/web/hooks/domains/session/use-session-restart.ts` and its test
- Existing mode-control and task/preview/Quick Chat composer integration points
- `apps/web/components/task/chat/chat-input-area-restart.test.tsx`
- `apps/web/src/locales/` task/agents catalogs
- `apps/backend/cmd/mock-agent/` deterministic discovery fixture support if needed
- `apps/web/e2e/helpers/profile-capability-discovery.ts`
- `docs/public/agents-and-profiles.md`, `README.md` when required

Proposed feature files:

- `apps/web/lib/ws/handlers/session-catalog-refresh.ts` and `.test.ts`
- `apps/web/hooks/domains/session/use-session-catalog-refresh.ts`
- `apps/web/components/task/session-catalog-refresh-notice.tsx` and `.test.tsx`
- `apps/web/components/task/model-selector-catalog-refresh.test.tsx`
- `apps/web/e2e/tests/session/session-catalog-refresh.spec.ts`
- `apps/web/e2e/tests/session/mobile-session-catalog-refresh.spec.ts`
- `apps/web/e2e/tests/docker/session-catalog-refresh.spec.ts`
- Shared E2E helpers beside the new specs when reused

Discover current concrete mode/composer callers before editing. Existing
`ConfigChatRestartConfirmation` is a pattern, not a domain component to import
into Task UI. Keep logic in shared session hooks and dependency-neutral helpers.

## Dependencies

Task 01: the saved-refresh endpoint, persisted bounded observation, scoped event,
boot shape, revision tombstone, and replacement-owner settlement are complete.
Task 01's terminal result contract also covers complete-empty/mode-only and
unsupported/failed outcomes. The frontend owns bounded no-report presentation,
not a new runtime discovery timeout or automatic recovery policy.

## Risks

- Showing a host candidate as an ordinary applied option recreates the
  adapter-cache bug; do not bypass provider validation to make a test pass.
- Hiding the notice behind `configHydrated` removes recovery for empty catalogs.
- A request acknowledgment is not a fresh provider catalog or proof of startup.
- Mode/option controls, multiple composer mounts, and stale HTTP boot can each
  clobber the wrong session unless all read the same scoped revision owner.
- A running turn may require explicit durable recovery after confirmed restart;
  conversation preservation does not authorize silent prompt replay.
- Dirty launch settings must never be saved or published implicitly.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/session-catalog-refresh.md), all
  criteria, and [design](../../specs/agents/system-design/session-catalog-refresh.md),
  Projection through Verification.
- [Plan](plan.md), compatibility matrix, all UI previews and E2E scenarios.
- Existing profile capability discovery, session restart, mobile selector,
  confirmation, state hydration, and session-scoped event patterns.
- Web/backend guidance and scoped confirmation guidance before using those
  primitives. Root cache/output constraints apply to every verification process.

## Results

### Revised acquisition implementation (2026-11-21)

The paragraphs after this section are historical generic-candidate evidence and
do not qualify task-owned provenance.

- **Provenance gate.** `sessionCatalogRecordSchema` accepts host candidates only
  with `source: "task_probe_v1"` and a complete `probe`. A missing or legacy
  source becomes a `context_unverified` rediscovery notice; an unsupported probe
  surface drops its list. Because every HTTP, boot and WebSocket parse uses the
  schema, no consumer can reinterpret legacy candidates. Behavioral RED: the
  existing projection kept the legacy record and stripped the new provenance;
  GREEN after the transform. Six affected frontend suites passed 44 tests,
  with fixtures migrated to the task-owned shape.
- **Mock fixture.** The mock agent answers a read-only `initialize` extension
  from its own catalog file and records a `catalog_probe` trace event. It opens
  no session (RED: invalid result before wiring; GREEN after).
- **Browser fixture.** The desktop catalog scenario no longer aligns GitLab
  credentials between the settings probe and the task, because candidates no
  longer depend on that equality. It asserts `source: task_probe_v1` and that
  helper processes emitted only `initialize`/`catalog_probe`. The Docker spec
  additionally asserts that no task probe provenance reaches an isolated
  executor.
- **Guidance.** The public profile guide describes per-session fresh readers,
  the qualified native capability, distinct task catalogs, notice-only cases and
  the 60-second fan-out bound. `i18n:check` and the new-code ratchet passed.
- **Browser collections.** After rebuilding the backend, fixtures, plugin
  package and `build:e2e` assets on cache-backed outputs, the final-source
  desktop collection (catalog, restart and profile discovery specs, project
  `chromium`) passed 22 tests in 7.9 minutes, and the phone collection (project
  `mobile-chrome`) passed 19 tests in 5.1 minutes. Inspected captures show the
  task-owned candidate marked **Requires session refresh**, the provider-only
  group, retained draft, and the phone confirmation with 44px actions.
- **Native proof.** The actual Codex 0.169.0 Local/Worktree experiment in Task 01
  ran through the real saved endpoint and native helper; two tasks received
  distinct task-owned catalogs while the editor reported a third.
- **Docker.** Still unavailable: no `docker`/`podman` binary or socket, and the
  install's Local Docker executor has no profile and points to a missing
  socket. `tests/docker/session-catalog-refresh.spec.ts` now also asserts that no
  task-probe provenance reaches the container, but it has not run. This remains
  the open external gate for this work order.
- **Root gates.** `make fmt` passed; its only rewrite outside this change
  (`apps/packages/plugin-sdk/sdk-release.json`) was restored to committed bytes.
  `make -k typecheck test lint` then ran once in a sanitized environment with
  `GOFLAGS='-p=2 -timeout=2h'`: typecheck, every backend package, CLI and
  script tests, backend `golangci-lint` (0 issues), harness, specification and
  architecture lint passed. Web tests passed 31,437 with 2 load timeouts in
  unrelated files (`scripts/lib/migrated-system-query-owner-wiring.test.ts`,
  `storage-maintenance-settings.test.tsx`); both files passed alone (66 tests).
  Oxlint flagged complexity 16 in the Docker spec after its new assertions; the
  record checks moved into `expectExecutorScopedNotice` and `make lint-web` then
  reported 0 warnings.

In progress with two open blockers: the real Docker run above, and the complete
web gate. The full root run recorded 2 web failures; the isolated 66/66 pass is
diagnosis, not a passing complete web or root gate.

Full-web diagnosis (2026-11-21). The retained root log shows both failures are
deadline-bound: the first cold ESLint config load in
`migrated-system-query-owner-wiring` took 19.6s against 15s, and the storage
test took 5.6s against 5s while neighbouring cases took 1.7-4.7s. One complete
retry then ran `make test-web` as the only gate from this task, with
`VITEST_MAX_WORKERS=4` (default 8), the default Node heap, unchanged selection
and unchanged deadlines. It finished in 1135s with 31,436 passed, 4 skipped and
3 failed. The original two passed. The new failures were also deadline-bound: two
cold ESLint loads (`tanstack-query-eslint-config`, `task-link-wiring`) over 15s,
and `ssh-failure-diagnostics`, whose fake-Docker read has a 1s limit and left no
output. Host load, sampled every 30s and including this run plus other agents'
services, averaged 11.85 (1-minute, peak 18.53) on 40 CPUs. The complete web gate
is still not passed. The remaining
cause is deadline-bound tests on a shared host with varying load. No assertion,
deadline or selection was changed.

Serialized retry (requested configuration): `make test-web` as this task's only
heavy job with `VITEST_MAX_WORKERS=1 RAYON_NUM_THREADS=1 TOKIO_WORKER_THREADS=1
GOMAXPROCS=1`, default Node heap, unchanged selection and deadlines. Result rc=2
in 1125s: 31,436 passed, 4 skipped, 3 failed. Two cold ESLint loads
(`eslint-i18n-config`, `migrated-system-query-owner-wiring`) exceeded 15s, and
`goal-section.test.tsx:135` saw the Set goal button still disabled immediately
after a name change. Sampled 1-minute load averaged 10.91 (peak 13.52). Wall time
matched the 4-worker run, so the cap did not measurably reduce suite contention.
Retries stop here.

The `goal-section.test.tsx:135` failure is a real pre-existing product bug, not
host load: a deterministic jsdom probe using React's real scheduler showed that
a name typed after the first-data commit but before the goal editor's passive
adoption effect is overwritten by `EMPTY_GOAL_FORM`. It has not been reproduced
in an actual browser. It is outside this change and is tracked by
[#5425](https://github.com/kdlbs/kandev/issues/5425) in a separate task; no
coordinator code or fixture was changed here. The two ESLint cold-load failures
each finished in 7-8s when run alone with `--testTimeout=120000`. That is
diagnosis only. The complete web gate remains unpassed: the #5425 failure plus
the two ESLint cold-load timeouts under load. Real Docker also remains open.

### Rebased verification (2026-11-22, HEAD after upstream `cef5eb5e61`)

The parent rebased this branch onto upstream `cef5eb5e61`, which includes the
#5425 goal-editor fix (#5430) and the picker configuration-feedback work (#5411).

- **Merge review.** The mode and model selectors run the `requiresRefresh`
  guards before `feedback.begin()`. The configuration-feedback and catalog-refresh
  slices both remain.
- **Browser collections on fresh builds.** The desktop catalog, restart,
  profile-discovery, model-selector and configuration-feedback specs passed 36
  and failed 1. The phone equivalents (including mode mismatch) passed 25. The
  failure was real: the upstream `session-configuration-feedback.spec.ts` first
  launch of a `prepare_session` task failed with `catalog discovery startup owner
  changed`. Temporary diagnostics, since removed, showed that the executor row was
  still `prepared`, and the starting and terminal catalog owner queries admitted
  only `starting`, `running` and `ready`. A repository regression
  (`prepared-execution`) failed on SQLite before the fix. Commit `e1b4a8926a`
  admits `prepared` for startup and terminal ownership only; live publication
  still requires `running` or `ready`. The SQLite and PostgreSQL catalog
  contracts passed afterwards. On rebuilt binaries the desktop feedback spec
  passed 3/3 and the phone spec 2/2. The other 33 desktop cases ran on the
  pre-fix binary; the fix changes only these two SQL predicates.
- **`make fmt`** made no changes.
- **`make -k typecheck test lint`**, run once with PostgreSQL enabled, failed.
  The causes, exactly:
  - Typecheck ran out of memory at the default Node heap. A rerun with the
    5 GiB heap used by the earlier passing root run passed.
  - Backend: every package passed except four.
    - `office/routines` (3 PostgreSQL tests: `relation "tasks" does not exist`).
    - `secrets` (6 PostgreSQL transfer tests).
    - `sentry` (`TestPostgresStoreSchemaReplay`).
    - `orchestrator` (`TestStepCompletionOutcome_SupersededConsumerRetryThroughHandler`).
    The office, secrets and sentry packages are untouched by this branch. Their
    failures reproduce on a pristine `cef5eb5e61` extraction, including on a
    freshly created database. The orchestrator case passed 3/3 alone on both
    trees.
  - Web: 31,894 passed, 4 skipped and 11 failed, all on time limits. Eight are
    in `linear-watch-dialog-lifetime`, two in `sentry-watch-dialog-lifetime`,
    and one is the `migrated-system-query-owner-wiring` ESLint cold load.
    `goal-section.test.tsx` passed 17/17. Run alone, the three files passed
    91/91; that is diagnosis only.
  - CLI and script tests, backend `golangci-lint` (0 issues), and harness,
    specification and architecture lint passed.
  - Web lint failed: `model-selector-composed-config.test.tsx` reached 602
    lines. Commit `56bb7d7238` adds the repository's file-level test
    `max-lines` exemption, and `make lint-web` then reported 0 warnings.
- **Docs.** `list-docs.py validate` (461 decisions, 2091 specifications),
  `lint-spec-files` tests and `--all`, public docs (63 tests, 56 pages),
  `git diff --check` and pr-docs coverage (`covered`) passed.

The complete backend and web gates remain unpassed. Backend has pre-existing
upstream PostgreSQL failures plus one orchestrator case that failed in the
aggregate and passed alone. Web has time-limit failures under shared-host load.

### Real Docker run (2026-11-25, HEAD `647510bc71`)

The containers project ran `tests/docker/session-catalog-refresh.spec.ts` against
a real Docker 29.1.3 daemon (data root on the cache drive) and passed 1/1 in
2.8 minutes with no retries. It built the repository's unchanged
`kandev-agent:e2e` image. APT inside that build reached Debian through the host
APT cache, via a Docker client-config `httpProxy`; neither the repository nor
the test changed. The run verified the following:

- The saved refresh left an `executor_scoped` notice with no candidates and no
  task-probe `source` or `probe`.
- The execution was unchanged.
- The host-only model was absent.
- Consented refresh stopped and resumed the session in the same environment and
  executor profile, loaded the original native session ID, created no new
  session and did not replay the prompt.
- The draft, runtime configuration and messages were retained.
- The provider then advertised `Profile env host-only-new`, and selecting it
  updated the session.

The executor-notice and rediscovered captures were inspected. Earlier attempts
failed in image setup only, because registry and Debian egress was blocked; no
product step ran in those attempts. The real Docker gate is now passed. The
complete backend and web aggregate gates above remain unpassed.

### Remaining aggregate failures: root causes (2026-11-26)

None of the remaining aggregate failures is caused by the catalog change. Each
is pre-existing, and none was edited on this branch except the timeout revert at
the end.

1. **Fixture DSN-format mismatch (`secrets`, 6 PostgreSQL tests).**
   `openPGWithConns` appends ` options=-csearch_path=<schema>`, which works only
   for key=value DSNs. The local URL DSN swallowed it, so the tests shared one
   schema and hit duplicate keys. They pass with a key=value DSN, the form CI
   uses.
2. **PostgreSQL 18 `23001` product gap (`sentry`
   `TestPostgresStoreSchemaReplay`).** `db.IsForeignKeyViolation` matches only
   SQLSTATE `23503`. PostgreSQL 18.6 reports `23001` for `ON DELETE RESTRICT`
   (reproduced in psql), so `DeleteInstance` returns a raw error instead of
   `ErrInstanceInUse`. CI pins PostgreSQL 16.
3. **Missing referenced table in the `office/routines` fixture (3 PostgreSQL
   tests).** The fixture creates only the settings and office schemas, but
   office tables reference `tasks(id)`. SQLite tolerates the missing table;
   PostgreSQL rejects it. CI never runs this package with a PostgreSQL DSN.
4. **Orchestrator event-count race
   (`TestStepCompletionOutcome_SupersededConsumerRetryThroughHandler`).** It
   failed 2 of 300 runs under CPU contention. In every case the extra
   `task_session.state_changed` came from the asynchronous
   `launchProcessOnEnter` waiting event after the move to step2, and the
   assertion counts all subjects.
5. **Slow tests under load (Linear/Sentry `*-watch-dialog-lifetime`).** Run
   alone, cases take 2.1–3.9 seconds against a 5-second limit. Commit
   `5b0b9d62e0` had raised three cases to `10_000` milliseconds; this branch now
   reverts them to upstream bytes. One run of both files afterwards (1-minute
   load about 2–4): Sentry passed 20/20; Linear failed 1 of 28. The failure was
   `create scope switch preserves portable fields and clears captured scoped
   selections` at 5621 ms, one of the reverted cases.
6. **ESLint cold load (ESLint wiring tests).** The first configuration
   resolution takes 4.0–7.3 seconds alone and over 19.6 seconds under aggregate
   contention, against a 15-second limit.

### Rebased re-verification (2026-11-26, upstream `f675a554a5`)

The branch was rebased onto upstream `f675a554a5` (56 new commits). Upstream
split the AGENTS trees into `STORE-STRUCTURE.md` and `API-GUIDANCE.md`; the
catalog lines moved there. The mode selector keeps upstream's Pi thinking-level
handling, now in `mode-selector-state.ts`, alongside the catalog projection.

One sequential run on fresh builds:

- **Browser and Docker specs.** Desktop (catalog, restart, profile discovery,
  model selector, picker feedback) passed 37/37. Phone (equivalents plus mode
  mismatch) passed 25/25. The real Docker catalog spec passed 1/1.
- **`make fmt`** made no changes.
- **`make -k typecheck test lint`** ran once with the 5 GiB Node heap and
  PostgreSQL through a key=value DSN (as CI uses), and returned rc=2.
  - Typecheck, CLI and script tests, backend `golangci-lint` (0 issues), web
    lint (0 warnings), and harness, specification and architecture lint passed.
  - Backend: every package passed except `office/routines` (item 3: three
    PostgreSQL fixture failures) and `sentry` (item 2: PostgreSQL 18 `23001`).
    `secrets` (item 1) and the orchestrator test (item 4) passed.
  - Web: 32,082 passed, 4 skipped and 3 failed, all on time limits. The Linear
    `create scope switch …` case took 5104 ms (item 5). The ESLint cold loads
    took 18.7 s (`tanstack-query-eslint-config`) and 27.5 s (`task-link-wiring`)
    (item 6). `goal-section` passed.

Every remaining failure is one of the pre-existing, non-catalog causes listed
above. The complete aggregate gate remains unpassed.

### Rebased re-verification (2026-11-27, upstream `6388b86453`)

The branch was rebased onto upstream `6388b86453` (23 further commits). Upstream
added shared `streams.ConfigOptionsSource*` constants, so the duplicate local
constant was removed and catalog enrichment now uses the shared keys. Build,
vet, gofmt and the ACP adapter package passed.

One sequential run on fresh builds (sampled 1-minute load peaked at 45.9):

- **Browser and Docker specs.** Desktop passed 38/38, phone passed 26/26 and the
  real Docker spec passed 1/1.
- **`make fmt`** again reformatted only `apps/packages/plugin-sdk/sdk-release.json`,
  which was restored to committed bytes.
- **`make -k typecheck test lint`** returned rc=2.
  - Typecheck, CLI and script tests, backend `golangci-lint` (0 issues), web
    lint (0 warnings), and harness, specification and architecture lint passed.
  - Backend: every package passed except `office/routines` (item 3) and
    `sentry` (item 2).
  - Web: 32,623 passed, 4 skipped and 8 failed, all load-bound.
    - Item 6: four ESLint cold loads took 16.3–22.8 s.
    - Item 5: two Linear cases timed out at 5506 ms and 5868 ms. A third Linear
      case did not find its dialog within the default query wait (3920 ms
      total).
    - Item 5: one Sentry case timed out.
    - `storage-maintenance-settings` timed out at 6172 ms.

No catalog test failed. The complete aggregate gate remains unpassed for the
recorded non-catalog causes.

### Historical generic-candidate evidence

In progress. Root formatting/typecheck, earlier scoped backend/PostgreSQL/race
results and complete desktop/phone collections passed. The full scripts gate
now passes after native Rust and cache-backed desktop-build prerequisites were
restored. Current final backend/lint and full-web gates remain incomplete after
the cache-threshold interruption. Actual launch-equivalence eligibility still
needs a design decision and correction; real Docker proof remains unavailable.
No complete verified implementation or commit is claimed.

- Explicit saved-profile refresh is separate from automatic/draft probing,
  including a mode-only dirty draft. An automatic runtime activation was found
  using the saved publication path; its behavioral RED was corrected to use
  the pure probe. The combined discovery/capabilities run passed 26 tests.
- Session-scoped WebSocket projections validate bounded input and preserve
  independent monotonic host-notice and provider-availability revisions.
  The handler suite passed four tests, including stale tombstones, malformed
  input, and foreign-task rejection.
- Ordinary captured-target Stop/Resume starts one store-owned 60-second
  observation after the accepted Resume ACK. Controlled-clock tests cover
  result-before-ACK, timeout, late result, newer notice, multiple consumers,
  unmount/navigation, and reload without recreating checking.
- Host-only models, modes, grouped leaves, and composed bindings open refresh
  consent before model/config/resolve or lifecycle requests. Provider-only
  choices retain their normal handlers. Explicit empty, mode-only, unsupported,
  and failed results retire stale selectable controls without changing intent.
- Phone model and mode pickers reuse their existing confirmation host, preserving
  one modal and the origin choice step on Cancel. Desktop uses the anchored
  confirmation. The shared notice remains outside model hydration, toolbar
  visibility, and composer disclosure gates, including Quick Chat's composer.
- Real-store hydration passed four tests: stale full-session data cannot replace
  a live tombstone; malformed host metadata does not discard valid provider data.
- A fine-pointer desktop runtime smoke exercised the real saved POST, visible
  notice, Cancel, and confirmed ordinary Stop/Resume. SQL retained native
  `mock-session-801539-1` at generation 1 while execution identity changed.
  Runtime model/mode/config intent and the unsent draft remained unchanged.
- That smoke exposed fresh provider choices being disabled while the full HTTP
  snapshot still carried the previous execution/source. Behavioral RED cases
  now cover both metadata and live-execution ordering. The hook reuses
  `sessionAgentctl` identity and newer owner-correlated provider evidence; a
  previous execution's provider remains rejected. The latest combined
  picker/notice/restart run passed 33 tests. The real picker then displayed all
  four current executor choices enabled without a page reload.
- Desktop screenshots were captured and inspected under
  `/home/clem/.cache/kandev-session-catalog-runtime/`, including
  `desktop-native-refresh-consent.png` and
  `desktop-refreshed-provider-choices.png`. Phone visual proof remains pending.
- A process-captured JSON advertisement fixture progressed from missing-model
  RED to GREEN. Existing initialized agents retain their catalog; fresh probe
  and native LoadSession use the new file, and ordinary provider validation
  admits only advertised selections.
- `TestSessionCatalogRefreshUsesCurrentLaunchSettings` passed through ordinary
  `Manager.StopAgent` and `Launch`, exercising current saved launch inputs rather
  than frozen historical inputs. It uses the earlier installer-based context
  projection and is not proof of final child-environment equivalence. Task 01's
  correction continuation records that newly identified defect and required
  replacement evidence. Its strict-environment setup failure was not feature RED.
- Managed desktop runs built real backend, production web assets, and packaged
  fixtures. The candidate fixture supplies matching disposable saved credentials
  and an explicit Local executor profile. The latest run passed saved
  publication and unchanged-intent assertions, then failed an immediate grouped
  choice read; its web-first wait has not been exercised. Those earlier
  publication assertions do not qualify the defective installer-based
  compatibility predicate. Temporary diagnostics were removed.
- Added desktop/phone browser cases for actual withheld terminal delivery, the
  shared post-ACK deadline boundary, late current results, newer notices and
  reload. Added the 767px fine-pointer boundary, edits while consent is open,
  and real native-restore rejection/recovery scenarios. Phone coverage also
  checks touch-only dismissal, modal focus containment, retained attachments and
  on-screen 44px actions. These new scenarios have not yet passed a browser run.
- All seven required locales and pseudo were generated. `i18n:check` passed,
  with 622 unused-key warnings and no blocking key/copy errors. Host notice
  wording describes refreshed observations, including unchanged catalogs.
- Public agent/profile guidance, public transport contracts, and the owning
  design now describe saved-only publication and independent provider evidence.
  Read-only catalog validation passed (451 decisions, 2010 specifications), and
  all-file specification lint passed. Public-document tests/validation and
  final documentation coverage remain unexecuted.

Remaining: run the complete desktop/phone collections with adjacent
restart/settings suites, and complete scoped/race/broad checks and
documentation coverage.
Real native PostgreSQL is now available and its scoped
conditional-write/concurrent-CAS checks pass. Real Docker executor access is
still unavailable; the parent has been asked for the equipped executor.
The unresolved eligibility correction and every remaining gate stay mandatory.

Verification previously paused at 91% cache-drive usage after safe reclamation
and a successful Hermes alert. Cache headroom is now restored, dependencies and
managed artifacts remain available, and unchanged-contract browser work has
resumed. Root/iSCSI/tmpfs fallback remains forbidden.

The resumed desktop run reproduced missing grouped choices after a full
web-first assertion window. Pre-teardown evidence showed complete source-owned
provider availability in persisted metadata, but only an initial minimal model
snapshot in the mounted client. The initial empty attempt ID is omitted by the
Go snapshot serializer; the frontend now normalizes that omission without
accepting missing execution/generation evidence. Independently revisioned catalog
availability now hydrates before ordinary stale-session timestamp rejection,
without replacing current session state, effective settings or draft metadata.

Both consumer regressions ran RED before their production corrections. The
focused reconciler/picker suites then passed all 28 tests. A fresh `build:e2e`
and the managed desktop saved-refresh flow passed (one test, 38.6 seconds).
The candidate, consent and completed-flow screenshots were inspected: grouped
host candidates remain refresh-only, provider-only choices remain available,
consent discloses current launch inputs, and conversation/draft/attachment intent
survives ordinary Stop/Resume. Temporary capture code was removed after proof.
Animation waits now pass Locators, and restore-error handling no longer swallows
unrelated timeouts. The remaining desktop cases initially stopped in the shared
test helper because it treated Fetch Response.status as a function; that helper
is corrected. Native current-input consent, real restore failure with explicit
recovery, and mode-only discovery have now passed their desktop browser cases;
the corresponding screenshots were inspected. Stored draft content survives
the ordinary failed-session composer gate and returns editable after recovery.
Subsequent desktop and phone deadline proof is recorded below; the complete
planned browser collections remain open.
These results do not qualify the defective launch-equivalence predicate.

Three additional consumer regressions ran RED for concrete routed profiles and
stale consent after concrete-profile replacement. Model/mode candidate projection,
provider ownership and confirmation now use the established
`execution_profile_id || agent_profile_id` precedence. The picker, reconciler
and shared session-consumer suites passed all 41 tests after correction.

Actual native empty/unsupported discovery exposed settings rejection being
misclassified as discovery failure. Lifecycle RED/GREEN now preserves the owned
explicit terminal report without bypassing mode/model validation. The real
unsupported browser case passed after rebuilding the guarded backend artifacts.
The empty case stopped at an execution-ID mismatch in its failure assertion;
its test now correlates the source with the persisted discovery-owner marker,
while successful ACKs still require matching response and session identities.
That empty-case path subsequently passed on desktop and phone. Mode-only and
unsupported paths also passed on phone, including an explicit unsupported-status
assertion with the latest notice owner guards.

Deadline diagnostics identified another genuine terminal-evidence carrier:
`session.state_changed.session_metadata`. The loss fixture now withholds those
catalog fields as well as dedicated events and HTTP metadata, leaving the
ordinary Resume ACK and unrelated lifecycle/settings data unchanged. The
observed result-before-ACK settlement was valid product behavior, not a timer
failure. Temporary loss diagnostics were removed after matching late-delivery
proof passed.

Disposable native PostgreSQL 18.6 is running on cache-backed storage. Scoped
repository/orchestrator checks, including its real conditional-write and
concurrent-CAS contracts, passed without PostgreSQL skips. Full changed-package,
race and broad pipelines remain outstanding; Docker and launch-equivalence
feasibility are not waived.

The latest user instruction also requests that, only after complete verified
implementation, the parent run `@50-luna-review-fix` with Sol: at most 50 rounds
or three consecutive clean rounds, with findings addressed between rounds.
The parent has been notified of that future handoff. No review worker, commit
or completion signal has been started.

### Further consumer and browser proof (2026-11-17)

Three additional notice regressions ran RED: unsupported/failed presentation
survived replacement ownership, and another concrete profile's notice remained
visible. The notice now derives terminal presentation only from its current
source owner and hides an explicitly mismatched concrete profile. All five
notice component tests passed after correction, without pinning English copy.

Managed desktop proof now also includes explicit empty discovery, preview,
Quick Chat, matching late delivery after the exact post-ACK deadline, and the
767px fine-pointer/newer-notice boundary. Their screenshots were inspected.
Quick Chat's observed environment lacks proven concrete host context; its
refresh remains notice-only rather than inventing host eligibility. Its own
ordinary Stop/Resume retains native identity, history, configuration and draft.

The first full coarse-pointer phone collection passed eight of ten cases:
both deadline/late-result paths, current inputs, restore failure/recovery,
empty, mode-only, unsupported and Quick Chat. The basic candidate/consent case
then passed after replacing a nonexistent drawer-overlay selector with the
shipped mode sheet's touch Close action. Candidate Cancel retains the owning
picker and focus; its final real provider model/mode selections pass.

The remaining interrupted-turn fixture previously matched a separate
`session_focus` restore-inspection response before the ordinary Resume
completed. Its waiter now excludes automatic activation sources. An actual
desktop run observed the typed `unknown_prompt_outcome` conflict, retained
recovery and exactly two original prompts, without replay. Generic cancellation
did not reliably retain that block on phone. The fixture now reuses the
established cancel-hold and native-restore-rejection controls, and refreshes
through a real second Settings tab while the accepted turn is running. This
deterministic case passed on phone (25.5s) and desktop (24.4s), with inspected
screenshots, a retained real uncertainty block, existing recovery actions and
exactly two original native prompts. No replay or forced recovery completion
occurred. The assertions use the shipped recovery surface rather than requiring
the legacy banner on phone. Temporary launch diagnostics were removed.

No result here qualifies the defective launch-equivalence predicate. No final
formatter/broad pipeline, real Docker proof, complete verified implementation,
commit or Sol review handoff is claimed.

All fourteen affected frontend consumer/store suites subsequently passed,
130 tests, with the configured single-worker focused command:

```sh
pnpm exec vitest run --maxWorkers=1 components/task/model-selector-catalog-refresh.test.tsx components/task/session-catalog-refresh-notice.test.tsx lib/ws/handlers/session-catalog-refresh.test.ts components/task/chat/chat-input-area-restart.test.tsx components/task/chat/chat-input-area.test.tsx components/task/model-selector-composed-config.test.tsx components/task/model-selector-consecutive-switch.test.tsx components/task/model-selector-stale-config.test.tsx hooks/domains/session/session-state-reconciler.test.ts hooks/domains/session/use-session-restart.test.ts hooks/domains/session/use-session.test.ts hooks/domains/settings/use-profile-capability-discovery.test.tsx hooks/domains/settings/use-profile-model-capabilities.test.tsx lib/state/slices/session-runtime/session-model-hydration.test.ts
```

The scoped architecture guides now identify the shared catalog ownership types
and session-runtime observation state. Harness validation passed 19 tests and
all 205 harness files; both guides remain at 300 lines.

The first complete backend package run passed settings controller/handlers,
orchestrator, ACP and native Codex. The repository package exceeded Go's default
ten-minute package deadline while creating another isolated PostgreSQL schema;
the currently running test had been active for one second, and its stack was in
migrations. Lifecycle retained-workspace probes returned process 404. A direct
`sh -lc` smoke established that the inherited personal login profile fails
before any command by sourcing a missing `/tmp/cargo/env`. No personal profile
or forbidden temporary file was changed. A private cache-backed disposable HOME
passes the actual login-shell smoke. Under that isolation, the targeted retained-
workspace regressions passed (25.117s), followed by the complete lifecycle package
(413.839s). No retained-workspace production code or tests changed. The complete
PostgreSQL repository package first reached the global 45-minute deadline near
the final worktree-ownership tests, with its then-current test only two seconds
old. Its untrimmed, verbose two-hour-deadline run then passed (2646.706s).

The required independent ordering/settlement race command passed (2.024s), with
the disposable PostgreSQL DSN configured:

```sh
go test -race -p=2 -tags fts5 ./internal/orchestrator -run 'TestSessionCatalogRefreshOrderingAndSettlement' -count=1
```

The exact Traditional Chinese generation, pseudo-locale generation and
`i18n:check` sequence also passed. Required locale keys, placeholders, plurals,
module-scope translation, punctuation and non-JSX copy checks were clean. The
existing unused-key report emitted 622 non-blocking warnings. Backend, helper,
fixture-package and web production assets were rebuilt before further browser
proof.

### Additional runtime and scope proof (2026-11-17)

The desktop saved-refresh case now proves that a repeated identical refresh
leaves the complete pending record and execution unchanged. A real unchanged
native catalog is also covered on desktop (16.1s) and phone (17.5s): valid
baseline models remain selectable, host-only candidates retire, and no new
model is fabricated. All eleven phone cases have passed across focused runs;
this is not yet a passing combined collection with the adjacent suites.

The running-turn flow now launches two actual siblings: one using the same
saved profile and one using another concrete profile. Before consent, the
running target and input-waiting sibling receive candidates under the current
predicate without execution, mode, model or configuration changes. The other
profile's record and runtime state stay unchanged. Only after consent opens
does the fixture enable the established native restore rejection, proving
ordinary Resume resolves those current flags. Phone (31.7s) and desktop (32.4s)
then retain normal uncertain-delivery recovery, saved conversation/input and
exactly the two original prompts. The phone driver uses its shipped Sessions
sheet rather than nonexistent desktop tabs. Runtime comparisons use full
session GETs, not the lightweight list projection, which omits metadata.
Eligibility remains explicitly unqualified.

Inspected phone captures include the 320px candidate list and picker-owned
confirmation, unchanged catalog, empty/mode-only/unsupported outcomes, current-
input consent, native restore rejection, unknown-prompt recovery, late/newer
notice and Quick Chat. The complete collections subsequently passed sequentially:
22 desktop tests and 19 phone tests, including every named adjacent suite.

Additional complete backend package checks passed for mock-agent, hostutility,
runtime/agentctl, server/api, stream types, gateway, executor and websocket.
The common sessioncatalog and event packages compile and have no direct test
files; their consumers supply behavioral coverage.

Documentation validation passed for 451 decisions, 2011 specifications,
36 specification-validator tests, 63 public-document tests and 55 published
pages. The coverage preflight accepted both work orders with no errors across
all 159 current tracked/untracked paths, including temporary cache bindings
that must not be staged. A focused browser command mistakenly used an
unsupported output environment name; its own artifacts were moved from the
checkout to verified cache storage, and that checkout output was removed.
Subsequent browser commands use explicit cache-backed `--output` paths.
The five compiler state outputs were resolved using TypeScript's own API and
bound to the cache drive before any final typecheck. Generated release metadata
was already individually cache-backed.

No final formatter/full pipeline, real Docker gate, completed implementation,
commit or Sol review-ready handoff is claimed.

### Combined browser collections (2026-11-17)

The first combined desktop run passed all thirteen catalog cases, all three
profile-discovery cases and five of six ordinary restart cases. The remaining
test asserted legacy recovery-banner wording while the captured UI was still
restarting; its waiter did not correlate the explicit Resume source. The normal
restart contract requires recovery after a failed Resume, not a permanent
uncertainty block after a successful native restore.

That test now uses its own profile with the existing native-restore rejection
and controlled cancel-hold, correlates the ordinary Resume, and asserts the
actual unknown-delivery block, available recovery action, retained history and
exact Stop/Resume target. Its implementation-specific wording assertion was
deleted rather than re-pinned. The focused case passed (20.0s), followed by both
complete collections:

- Desktop: 22 passed (7.0m), log
  `/home/clem/.cache/tmp/session-catalog-runlogs-20261117/kandev-run.e2e.IdLPrJ2Q.log`.
- Phone: 19 passed (6.0m), log
  `/home/clem/.cache/tmp/session-catalog-runlogs-20261117/kandev-run.e2e.fgDB709M.log`.

The requested `make fmt` passed (248.32s) before any broad typecheck/test/lint
invocation. Its unrelated SDK release-JSON formatting change was restored.
Cache use reached 86%; an audit of immutable Go cache entries checked access,
modification and change times, link counts, and open/mapped files. Oldest-first
reclamation removed 8 GiB of unused pre-today entries from the ordinary Go
cache, without touching today's files, the managed Go cache or live PostgreSQL
data. Cache use returned to 79%.

Passing these fixtures does not qualify actual launch equivalence. The parent's
design selection and real Docker access remain outstanding; no complete,
commit or Sol review-ready signal is issued.

The initial requested `make typecheck test lint` invocation stopped in classic
`tsc`: its live heap reached the default 4 GiB V8 ceiling and aborted with
`JavaScript heap out of memory`. No type diagnostic, tests or lint completed in
that invocation. Its saved log is
`/home/clem/.cache/tmp/session-catalog-runlogs-20261117/kandev-run.verify.8FI0sEhH.log`.
Measured host memory availability was about 6.4 GiB; the service cgroup has no
memory ceiling. Raising the heap to 5 GiB exposed real compiler errors: the
canonical `AppState` contract omitted the new catalog state/actions, and new
fixtures used unbranded profile/session IDs, omitted required session timestamps
or supplied Playwright's `exact` option to Testing Library.

The canonical state field and typed action composition now include the catalog
slice, following existing store conventions. Fixtures use the existing branded
ID constructors and valid required timestamps; Testing Library retains its
default exact string-name matching. No casts, weakened production types or
compiler substitution were added. The fourteen affected frontend suites passed
again (130 tests, 67.76s). The complete requested root targets are running with
the 5 GiB heap, serialized workspace execution, 2 GiB Go memory targets, two Go
build workers, a two-hour package deadline, a 16 GiB inherited address-space
ceiling and core dumps disabled. Tests and lint did not run in either prior
failed compiler invocation.

### Full root verification continuation (2026-11-17)

The corrected classic compiler passed for every app. The subsequent complete
backend invocation ran to completion but failed ten packages; its log is
`/home/clem/.cache/tmp/session-catalog-runlogs-20261117/kandev-run.verify.u0548tgE.log`.
Failures included this live service's inherited home, ports, trusted proxies,
feature toggles and service-install metadata affecting unit expectations.
Additional opt-in PostgreSQL failures included missing foundational tables,
duplicate fixture IDs and a Sentry typed-foreign-key assertion. These results
are not a passing full test gate.

Source inspection also found a legacy Cursor-auth fixture using normalized
parent slugs with a legacy child suffix. The cache temp path contains `.cache`,
so that synthetic mixed encoding is not a real legacy project slug. The fixture
now uses the existing legacy parent encoder; production credential aggregation
is unchanged. The first service-isolated retry was cancelled after that fixture
issue was identified; no passing result is claimed for the cancelled run.

Six affected failing packages are being checked with service-specific `KANDEV_*`
variables removed. The full remaining web/CLI/script/lint targets are running
independently with `make -k`, so a failure cannot suppress the other gates.
The baseline full-test environment does not enable every unrelated opt-in
PostgreSQL suite. Required feature PostgreSQL contracts and the complete
PostgreSQL-enabled repository package have already run and passed against the
real disposable server; that evidence is retained, not replaced by a skip.
No further production change or launch-eligibility qualification is claimed.

### Lint and verification-resource continuation (2026-11-17)

The remaining root targets passed all five CLI tests, all 205 harness files,
specification lint and architecture lint, but did not pass the complete gate.
The 16 GiB virtual-address ceiling prevented Wasm instantiation and the native
Oxlint allocator, despite available physical memory. Removing that ceiling
while retaining the Node heap/Go memory/worker budgets passed the actual notice
suite (five tests, 7.61s) and native toolchain probe checks. The full web suite
is running with that corrected resource configuration.

Backend lint reported eight complexity and twenty-four repeated-literal
findings in this implementation. Catalog validation, publication projection,
owner admission, provider availability and transactional metadata writes now
use small named helpers. Catalog outcomes/dispositions/policies use shared
catalog-owned constants; executor classification uses canonical local/worktree
kinds plus the explicit legacy alias. Transaction scope, ownership fences,
previous-record CAS, prior-revision increment and the metadata size limits
remain unchanged. No host-eligibility correction or new attestation was added.

The catalog-focused command passed the seven tested backend packages with real
PostgreSQL enabled; the common package compiled without direct tests. Repository
cases passed in 5.011s. A final launch-marker write extraction is included in the
subsequent complete seven-package run, followed by the required race and full
lint gates. These post-refactor gates are still running.

After memory correction, script tests progressed through the release-desktop
gate and stopped because `cargo` was absent from PATH. The existing real Rust
1.92 toolchain was located. Its locked/offline verifier build uses the existing
cache-backed Cargo home, a cache-backed target, one build worker and no
incremental/debug artifacts; no root-drive dependency/cache writes are allowed.
That gate is being exercised, not waived.

Five previously failing complete packages passed with live-service environment
removed: MCP configuration, settings handlers, configuration, launcher and
updates. The unmodified worktree strict-lock cases still fail with Git 2.53.0;
an isolated trace, also without inherited Git config, reports `fetching ref
refs/remotes/origin/main failed: reference already exists`. No production
worktree change or weakened assertion was introduced. Actual launch-context
eligibility and real Docker execution remain unresolved.

### Current completed checks and interrupted gates (2026-11-17)

The locked/offline real Rust verifier made the full release-desktop script pass
(215.26s). The next complete script invocation reached the startup browser case
and exposed its missing `dist` prerequisite. Desktop output is now cache-backed,
the existing Chromium executable was confirmed, and `build:vite` plus the
entire `make test-scripts` target passed (257.57s combined). Saved logs:

- `/home/clem/.cache/tmp/session-catalog-runlogs-20261117/kandev-run.desktop-script-prerequisite.n269IQKX.log`
- `/home/clem/.cache/tmp/session-catalog-runlogs-20261117/kandev-run.desktop-startup-prerequisite.m7bKoW6e.log`
- `/home/clem/.cache/tmp/session-catalog-runlogs-20261117/kandev-run.scripts-final-prerequisites.wutzSpUO.log`

The cache drive later reached 90%. Full web, full post-refactor backend and
focused consumer runs were cancelled, not counted as passes; the owned
PostgreSQL server stopped cleanly. Safe oldest-first removal of unopened
pre-today generated Go intermediates restored 87% use. Current managed-cache
entries and unrelated active Kandev binaries were not removed. PostgreSQL has
resumed on its private socket with a bounded WAL budget; checks now proceed
within the available headroom.

An owned terminal-validation regression also ran RED before correction:
`discovery-presence-red.I6qPWerA.log` reports acceptance of support and presence
from different surfaces despite no observed supported surface. Requiring the
pair on one surface matches the existing browser predicate. The regression
then passed (`discovery-presence-green-focused.F4v1VHie.log`), and an actual
temporary CLI decoded JSON and reported:

```text
cross-surface completion rejected
supported empty mode-only completion accepted
```

The CLI scaffold was removed. No new browser schema or copy was required.
Current consumer checks include the native PostgreSQL DSN. The earlier complete
backend/PostgreSQL/race evidence precedes these refactors and is retained as
historical proof, not substituted for the still-open final gates.

### Lint remediation and current-source checks (2026-11-17)

The latest completed full lint run reported one equivalent Go boolean rewrite
and owned TypeScript complexity, size, parameter-count and fixture findings:
`/home/clem/.cache/tmp/session-catalog-runlogs-20261117/kandev-run.lint-current.3Pq8I6cQ.log`.
The paired-surface guard was rewritten without changing its predicate; its
regression passed in `kandev-run.catalog-guard-style.fcclzr6F.log`.

Catalog owner matching, availability, notice presentation, composed-draft
ownership, saved-publication eligibility and restart target lookup now have
bounded helpers. The existing eight consumer suites passed 101 tests before
the test-module split:
`kandev-run.catalog-orchestration-refactor.My61pcEd.log`. Mode/routed-profile
and shared post-ACK observation cases then moved into independent test modules.
Consent assertions now inspect the exposed modal instead of Radix data-state
attributes; forwarding/mock-echo tests were removed rather than re-pinned.

Browser helpers use a typed context rather than long positional lists; all
13 affected callers migrated. Pointer activation reuses the existing strict
mouse/touch helper. Terminal evidence, delayed metadata stripping, deadline
advancement and late delivery retain their original runtime boundaries and
assertions. Fresh browser qualification remains pending.

The required `make fmt` passed after these changes (231.26s):
`kandev-run.format-after-remediation.SHU3ltoE.log`. The unrelated generated SDK
manifest formatting was restored to its unchanged committed bytes. The latest
full-app typecheck completed, but the full web suite was cancelled at the cache
threshold. Its partial log reports a separate Linear lifetime case failure and
connection refusals to 127.0.0.1:3000:
`kandev-run.current-types-web.rnEZW9fQ.log`. It is not a passing web gate.

On 2026-11-18 the last owned restart-callback complexity finding was addressed
by extracting the unchanged post-ACK observation phase. The captured notice,
accepted-owner parsing and store-owned deadline are unchanged. Scoped formatting
passed. With explicit Node 24.19.0, all twelve required frontend suites passed
106 tests (58.05s), including both split modules and accessible consent
assertions. Full-app typecheck and full web lint then passed after the final
extraction. The combined supported-runtime invocation completed in 154.74s.

All seven required backend packages now have current-source passing results;
the complete PostgreSQL-enabled repository passed in 2397.394s and the required
race test in 2.025s. Full Go lint reported zero issues. The handler fixture
correction and exact logs are recorded in Task 01. Four remaining backend
packages also pass after service-environment isolation. The separate unmodified
worktree strict-lock failure still prevents a passing full root test gate.

The Linear lifetime case was isolated from the full suite and live service
settings for diagnosis. It still reaches its existing 5000 ms deadline.
No timeout or assertion was changed; full web tests remain unqualified.
Final harness, architecture, specification, public-document and locale checks
passed, as did coverage validation for all 168 known changed paths.

Cache use reached 91% again; owned writers and PostgreSQL stopped cleanly.
Revalidated oldest-first removal of 1,018,114,048 allocated bytes of unopened,
single-link older tool-cache artifacts restored 90%. Current/recent files,
hardlinked dependencies, source and databases were preserved. Bounded checks
completed, but less than 0.5 GB of policy headroom cannot accommodate the 1 GB
fresh managed-browser-build budget. Hermes reported the capacity blocker to
Clem successfully.

Actual launch eligibility, fresh desktop/phone qualification and real Docker
proof remain open. No final commit or Sol review-ready handoff exists.

### Continued verification after capacity recovery (2026-11-18)

Capacity recovered to 30% use, clearing the earlier cache blocker. The first
interrupted desktop build supplied no inferred pass. Subsequent fresh full-build
collections passed all 22 desktop and 19 phone cases, and current consent,
terminal-empty, unavailable/late-notice and QuickChat captures were inspected.
These results do not qualify the defective launch-eligibility predicate.

Temporary Linear probes and keyboard experiments were removed. The original
input helper was restored. The final complex lifetime case uses the existing
same-file 10-second harness-budget pattern; serial qualification passed all 28
cases (105.53s). All interaction/focus/selection assertions remain; production's
shared 60-second deadline is unchanged. The earlier concurrent trial's eleven
timeouts were not treated as a qualified fix.

The complete worktree package passes after removing incidental error-text
assertions while retaining typed strict-failure/no-worktree assertions. No
production Git behavior changed. Real Docker proof and qualified launch
eligibility remain open.

### Research direction and current gate evidence (2026-11-19)

The later parent user choice is provider-specific contract research, not the
earlier child task-context choice. No parent contract rewrite is awaited.
Task 01 records the exact-version model-and-mode matrix and primary citations;
no provider exclusion, new transport or ordinary positive host path is
qualified. The existing candidate/live authority and restart/terminal UI proofs
remain independent evidence, not an AC001.1 waiver.

`make fmt` completed. The original exact typecheck/test/lint aggregate passed
typecheck and the other backend packages but failed SQLite's cumulative
10-minute timeout. The complete isolated SQLite suite then passed in 1008.424s
with a 2-hour budget. The same retained follow-on chain is running full web
checks and has reported restart/selector/onboarding failures; CLI/scripts/lint
must not be inferred passed if the chain stops there.

Both work orders remain in progress. No final verified implementation, commit,
Docker waiver or Sol review-ready handoff exists.

### Cancelled full-web gate and read-only diagnosis (2026-11-19)

Cache use reached 91% before further TDD. The owned aggregate was cancelled and
its output retained. SQLite's complete isolated pass remains valid; full web
has only partial observed failures, not a complete terminal verdict. No later
CLI/scripts/lint result is inferred. Hermes reported the capacity blocker
successfully; no current-day/open/hardlinked/source/database data was deleted.

Read-only source diagnosis found:

- The restart regression fixture omits required `sessionCatalogRefresh`, while
  `use-session-restart.ts` reads it before its transport try/catch.
- Selector fixtures omit that slice and `hydrateSessionCatalogRefresh`, both
  consumed by the catalog-aware hook. The existing full `createAppStore`
  fixture used by catalog restart tests is the reusable pattern; production
  optional chaining/no-ops would mask the fixture defect.
- The onboarding refresh case asserts a second pure-probe call after explicit
  saved refresh. The implementation correctly calls `refreshAgentProfileCatalog`;
  its test mock exports only `probeAgentProfile`. The obsolete forwarding-count
  assertion must not be re-pinned. Preserve consumer-visible catalog/draft,
  pending/replay, unconfirmed-mode and phone focus behavior instead.

These are source-grounded diagnoses, not exercised fixes or GREEN claims.
Production/test writes and requalification remain blocked by cache capacity.

### Observed web failure remediation (2026-11-19)

After cache expansion cleared the write gate, the restart fixture received the
required empty catalog slice. Selector tests now use the existing full
`createAppStore` API, including its real hydration action. Obsolete trigger-class,
subscription-detail and onboarding pure-probe forwarding-count assertions were
deleted rather than re-pinned. No production fallback/no-op was added to hide
missing test state.

The three previously failing suites passed all 49 remaining tests (11.69s).
Consumer-visible restart selection/pending/replay, dynamic hydration,
unconfirmed mode and phone choice/focus cases remain exercised. Full web and
remaining aggregate qualification are not inferred from this focused pass.

Before the following four-file remediation, documentation qualification passed:
451 decisions/2012 specifications,
36 specification-lint tests, 63 public-document tests and 55 published pages.
The documentation whitespace check passed after removing trailing empty lines.
Changed-path preflight returned `covered` for 174 actual paths (94 triggering),
with complete contents for all six referenced documents, including the intervening
draft task-discovery design. That is structural coverage, not approval of its new
API/helper architecture. The inventory includes cache/output links and is not an
explicit staging list.

### Completed full-web run and focused remediation (2026-11-19)

`bg_28` completed root typecheck, then full web in 7635.32s: 3369 files passed,
four failed; 30811 tests passed, eight failed, four skipped. Its `&&` chain did
not reach CLI/scripts/lint. `artifact://890` retains the complete result.

Three direct-host HTTP Git fixture cases now use the existing explicit loopback
gateway option. Actual asynchronous `git ls-remote` proves trusted-origin reads
through both backend/global and executor/environment rewrites; copying/config
format assertions were removed. The production helper/default Docker path is
unchanged, so this cannot substitute for real Docker qualification.

The incidental Git stderr-regex test was deleted; no-fetch lock ownership and
stale/live worktree behavior remain. Repeated full-parent GitLab/Sentry cases use
the existing scoped 10-second test-budget convention, with all draft/focus,
stale-settlement and pending-save assertions retained. No production deadline,
global test timeout or recovery guard changed.

All four remediated suites passed 63 tests (160.21s). Root formatting then passed
(241.95s). `bg_31` completed current root typecheck and full web successfully:
3373 files passed, 30818 tests passed/four skipped, 7189.17s. CLI five tests passed.
Scripts stopped at an offline Cargo dependency miss in the updater-signature
verifier; no script/lint aggregate pass is inferred.

The complete raw log is retained byte-for-byte at
`/home/clem/.cache/tmp/kandev-session-catalog-root-current-fddb34e4.log`
(SHA-256 `1840b3a5e9c1f5c98dc045597a5180598c6d1fa8e71fc34d43fd00985e09cd15`).
Exact locked Linux Cargo fetch succeeded in verified cache-backed storage,
without manifest substitutions. Exact `make test-scripts` then passed (363.76s).
Root lint initially failed ST1005 on a new native error's initial capital.
After that case-only correction, the full native transport package passed
(0.106s), and exact `make lint` passed (812.31s): backend zero reported issues,
web, 205 harness files, all specifications and architecture. Its older external
checkout generated-file-filter warning is retained, not suppressed.

Additional byte-preserved logs:
- `/home/clem/.cache/tmp/kandev-session-catalog-scripts-current-fddb34e4.log`,
  SHA-256 `6161ebf229d81a8918b3ce4ee8451174661630ac6fc50817ec321cad2924596c`.
- `/home/clem/.cache/tmp/kandev-session-catalog-lint-current-fddb34e4.log`,
  SHA-256 `ad009733e3939b144772309a4db73460a2f72a92d6daba94dc3319f1621cc4fa`.

Current six-document coverage preflight returned `covered` for 178 actual paths
(94 triggering), with zero errors; requested documentation whitespace also passed.
Cache/output links remain excluded from any eventual explicit staging list.
