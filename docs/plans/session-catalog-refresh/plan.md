---
created: 2026-11-10
status: draft
requirements:
  - REQ-AGENTS-SESSION-CATALOG-001
  - REQ-AGENTS-SESSION-CATALOG-002
  - REQ-AGENTS-SESSION-CATALOG-003
system_design:
  - ../../specs/agents/system-design/session-catalog-refresh.md
  - ../../specs/agents/system-design/session-catalog-task-discovery.md
legacy_specs: []
---

# Implementation Plan: Session catalog refresh

## Overview

Implement [issue 5237's selected comment](https://github.com/kdlbs/kandev/issues/5237#issuecomment-6773837684),
not an automatic harness restart. First deliver the scoped saved-profile refresh
and durable session projection API. Then connect the editor and composer to that
API and the existing explicit restart flow, proving desktop and phone outcomes.

Agents owns the [requirements](../../specs/agents/requirements/session-catalog-refresh.md)
and [design](../../specs/agents/system-design/session-catalog-refresh.md).
The primary session executes sequentially after a later explicit implementation
request. No delegation, production/permanent test change, or commit is authorized
by completing this design package.

## Evidence and decisions

The issue reports that killing the harness then resuming refreshes the picker.
The linked comment rejects automatic restart because work may be running and
asks to preserve remote executor discovery. No context-bundle paths were supplied.

Source confirms two independent caches: SQL/React selector snapshots and the
adapter's `availableModels`/`availableConfigOptions`. In
`acp.Adapter.setModelWithConn`, an unknown model fails validation before an RPC.
A database-only replacement cannot satisfy selection of a new model.

Decision: publish host choices as explicit candidates, not effective provider
state. Already advertised choices remain directly selectable. Host-only choices
require user-confirmed restart and fresh executor advertisement before selection.
Remote sessions receive only a scoped refresh notice. This deliberately preserves
provider validation rather than injecting host choices into adapter caches.

The profile probe itself remains pure, as required by
AC-AGENTS-PROFILE-DISCOVERY-001.5. Live effective state remains session-authoritative,
as required by AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-001.6. The new saved-profile
publication endpoint is separate from draft probing, and candidates never become
live option/default state. Existing requirements, their historical delivery
plans, and runtime updater behavior are intentionally unchanged.

The related [selector repair package](../model-catalog-selector-repair/plan.md)
contains unfinished provider qualification. Do not depend on its unimplemented
Fusion acquisition or claim that this refresh package completes it. Preserve
whatever bounded normalized catalog the current probe actually supplies.
Issue 4994's resume-settings change is explicitly excluded.

Ordinary resume already resolves the profile's current environment, flags, and
wrapper; existing session snapshots freeze neither those inputs nor secret
values. Preserve model/mode/config-option overrides under existing resume policy,
and disclose current launch-input resolution before confirmation. This package
does not add frozen launch capture. Changed-account/native-restore failures retain
the conversation and use existing recovery without a new-session fallback.

## Scope

### In scope

- Explicit successful saved-profile refresh publication to authorized matching
  task sessions, including current output and waiting sessions.
- Compatible actual host/worktree candidate display and isolated/remote notice-only
  behavior, with concrete execution-profile identity and context checks.
- Models and modes, grouped catalog fidelity, preserved effective settings,
  persistence, boot/reconnect projection, and late-result/owner guards.
- Composer notice, candidate distinction, confirmation, shared restart pending
  state, settled replacement catalog, and visible failure/recovery.
- Desktop/phone localization, accessibility, browser proof, and public guidance.

### Out of scope

- Automatic restarts, remote probes, adapter catalog injection, bypassing
  advertised-model validation, or changing provider-specific dialects.
- Saving a draft, changing a profile's default model, changing ordinary resume's
  launch-input resolution, freezing launch inputs, context reset, prompt replay,
  or runtime updates.
- New flags, global notifications, telemetry projects, native CLI catalogs as
  live capability proof, or a guarantee of model parity across executors.

## Technical approach

### Saved-profile operation and durable session observation

Keep the saved-profile POST/editor response and pure settings probe. Its success
gates a bounded authorized fan-out; each eligible host obtains its own fresh
task-context result under the [acquisition contract](../../specs/agents/system-design/session-catalog-task-discovery.md).
Wire the narrow consumer in backendapp; settings stays independent of runtime.
Never transfer generic utility choices. Return sanitized partial/unavailable
publication separately, without exposing foreign session counts or IDs.

Add a bounded query by actual concrete profile and live execution, including IDLE
rather than relying on `ListActiveTaskSessions`, which omits it. Authorize each
owning task. Serialize the new `profile_catalog_refresh` metadata record with
session settings, using transactional current-owner checks when that owner can
change outside the lock. Keep models/modes as candidates and never mutate
`acp_model_state` current values, runtime configuration, baselines, or turns.

Publish the new session-scoped event after commit. Persist monotonic revision and
explicit clearing; hydrate it through ordinary task detail and boot. Clear only
against accepted settled replacement ownership and the matching notice revision.

Task 01 also supplies explicit owner-correlated terminal discovery results through
the adapter/session wire and lifecycle path. Complete-empty, mode-only,
unsupported, and failed outcomes must settle without relying on a nonempty
`session_models` event. Task 02 bounds post-ACK observation to 60 seconds in the
shared session store; no report becomes unavailable with a retained notice, not
success or an automatic restart. A late matching current result can still settle.

### Editor and composer integration

Change `useProfileCapabilityDiscovery` so only an explicit saved-baseline refresh
uses the new operation. Dirty/new drafts and automatic discoveries keep their
pure existing endpoints. Build the candidate projection in dependency-neutral
state/helpers; extend session-runtime hydration and WS handlers without mixing
host defaults into `activeModel`, `sessionMode`, or dynamic config state.

Use `ModelSelector` and the existing mode control as projection consumers. Shared
exact values use current handlers. Host-only values remain clearly marked and
open the refresh confirmation without RPCs. Place the notice outside selector
hydration/visibility gates. Capture target and revision before confirmation.
Reuse `useSessionRestart`, `useSessionRestarting`, and the current composer
submission guards. Do not fork stop/resume logic.

Inspect current mode-control call sites before implementation; it is not enough
to refresh the model list while leaving modes stale. Preserve preview and Quick
Chat reuse through session-scoped hooks rather than adding page-specific effects.

### Compatibility matrix

| Provider/transport and actual executor | Identity/capability shape | Behavior and verification | Unsupported or unknown fallback |
| --- | --- | --- | --- |
| Qualified native Codex built-in unwrapped host app-server, actual 0.169.0 | Sealed current task command/context plus source/profile fences | Fresh helper initialize/model-list only; complete pagination, model surface supported; actual native Local and Worktree proof required | Unknown runtime/version/wrapper or unavailable context gets notice-only; native modes remain explicitly unsupported |
| ACP including grouped/mode-capable providers | No qualified generic fresh read-only task acquisition established by research | Preserve live provider choices and offer notice/explicit rediscovery; deterministic task-probe fixture covers model/mode UI independently | No invented RPC or new native conversation to manufacture eligibility; concrete future capabilities need same-contract proof |
| Custom/passthrough/wrapped or legacy host runtime | No qualified acquisition capability/launch evidence | Notice-only with exact current selections retained | No generic environment exclusions or installer equality |
| Any Docker/SSH/Remote Docker/Sprites/Kubernetes/plugin remote, or unknown locality | Actual executor identity | Notice-only; explicit restart stays on that executor; real Docker proof remains required | Never run automatic task probes or import host choices |
| Dynamic routing profile | Actual concrete execution profile and authorized owning task | Only actual matching concrete users enter acquisition/publication | No dynamic-parent propagation |
| Static/virtual/non-discoverable provider | Unsupported task acquisition | Retain provider choices and truthful unsupported/rediscovery notice | No fabricated dynamic support or automatic restart |

All registered profiles and executors take one of these explicit paths. That is
coverage of the scope, not a claim that every upstream provider supports dynamic
catalog discovery or conversation-preserving restart. Unit fixtures cover the
classification matrix; real remote browser proof covers Docker, not every cloud.

## ASCII UI preview

Hierarchy, control order, candidate distinction, consent, and status are required.
Labels and model names are illustrative and must be localized in implementation.
Reuse current picker grouping and responsive primitives; spacing is not a pixel
specification.

### UI-5237-01: Composer and host candidates, desktop

Entry: active task/preview/Quick Chat composer after saved-profile Refresh.

```text
+--------------------------------------------------------------+
| Message draft (retained)                                      |
+--------------------------------------------------------------+
| [Current model v] [Profile models refreshed *]          [Send] |
+--------------------------------------------------------------+

[Current model v] opens existing model settings:
  Filter models...
  Current model                                      selected
  Newly discovered model             Requires session refresh >
  Current session
    Provider-only model
  Mode: current mode (provider-confirmed)
```

Already advertised rows keep their ordinary selection behavior. A candidate
mode follows the same rule. The host's probe defaults never appear selected.
The compact notice is also shown when the model trigger cannot render.

### UI-5237-02: Explicit refresh confirmation, desktop

Entry: notice action or host-only candidate activation. Anchored confirmation.

```text
+---------------------------------------------------+
| Refresh session                                   |
| Restart this agent to check its models and modes.  |
| This can interrupt its current turn.               |
| Conversation and message draft are retained.       |
| Current profile environment, flags, wrapper apply.   |
|                         [Cancel] [Refresh session] |
+---------------------------------------------------+
```

Remote/isolated variant says "Profile models refreshed. Check this executor's
models and modes." It does not display host model names or promise new remote
models. No restart occurs before confirmation.

### UI-5237-03: Phone composer and confirmation

```text
+-----------------------------------+
| Message draft (retained)          |
|                                   |
+-----------------------------------+
| [Current model v]          [Send] |
| Profile models refreshed          |
| [Refresh session]                 |
+-----------------------------------+

Existing inset confirmation surface:
+-----------------------------------+
| Refresh session                   |
| Restart this agent to check its   |
| models and modes. This can        |
| interrupt its current turn.       |
| Conversation and draft retained.  |
| Current profile launch inputs apply. |
| [Cancel]       [Refresh session]  |
+-----------------------------------+
```

The notice wraps below model controls rather than compressing the editor or
requiring a hover bubble. Phone and coarse-pointer actions have at least 44px
hit targets. Existing confirmation owns safe-area padding and one internal
scroller; existing model picker owns its own navigation/scrolling. No document
horizontal overflow, desktop preference write, or new fixed-height panel.

### UI-5237-04: Pending, no-change, and failure states

```text
Restart requests pending:  Restarting session...       (Send disabled)
Awaiting provider report:  Checking session models...  (no success claim)
Settled, unchanged:        No additional choices reported by this executor.
Failure:                  Session refresh failed. [existing recovery action]
Empty result:             No choices reported by this executor.
Unsupported result:       Model discovery unsupported. [existing recovery]
No report after 60s:      Could not confirm choices. [Refresh session]
```

Retain notice on failed/uncertain restart, retain editable draft, and use a live
status region. Do not disable normal interaction indefinitely after the restart
requests settle; existing startup/recovery admission owns subsequent Send state.
Map all previews to 002.1-002.5 and 003.1-003.3.

## Tests

Names below are proposed implementation targets, not tests run during design.

| Acceptance criteria | Test location and proposed cases |
| --- | --- |
| 001.1, 001.3 | `settings/handlers/session_catalog_refresh_test.go`: `TestSavedProfileCatalogRefresh`; `settings/controller/session_catalog_refresh_test.go`: `TestSavedRefreshDoesNotPublishDraftOrFailedProbe` |
| 001.2, 001.4, 001.5 | `orchestrator/session_catalog_refresh_test.go`: `TestSessionCatalogRefreshEligibility`, `TestSessionCatalogRefreshPreservesEffectiveState`; separate canonical `local` and `worktree`, legacy `local_pc`, empty/unknown locality, IDLE, concrete routing, archived/foreign tasks, and remote classes |
| 001.6, 002.4 | `orchestrator/session_catalog_refresh_test.go`: `TestSessionCatalogRefreshOrderingAndSettlement`; `task/repository/sqlite/session_catalog_refresh_test.go`: `TestSessionCatalogRefreshConditionalWrite`; adapter/runtime `TestSessionCatalogDiscoveryResult` suites cover strict resume complete-empty, mode-only, unsupported, failed, stale owner, and absent-report shapes; SQLite and configured PostgreSQL |
| 001.1-001.6 | `hooks/domains/settings/use-profile-capability-discovery.test.tsx`; `lib/ws/handlers/session-catalog-refresh.test.ts`; `lib/state/slices/session-runtime/session-model-hydration.test.ts`: saved-only routing, candidate hydration, newer event versus stale HTTP, tombstone, provider event retention |
| 002.1, 001.2 | `components/task/model-selector-catalog-refresh.test.tsx`: candidate versus advertised selection, no unadvertised RPC, unchanged effective model/options, candidate modes, structured groups and removed current selection |
| 002.2-002.5, 003.1-003.3 | `hooks/domains/session/use-session-restart.test.ts`; new `components/task/session-catalog-refresh-notice.test.tsx`: captured target/revision, cancel, duplicate hooks, pending, terminal empty/unsupported, no-report 60s controlled-clock boundary, late result, failure and focus; runtime `TestSessionCatalogRefreshUsesCurrentLaunchSettings` covers changed env/flags/wrapper and retained native identity on restore failure |
| 001.7-001.8 | `process`/`api`/native adapter `TestSessionCatalogTaskDiscovery` suites: authenticated owner/source capture, fresh helper protocol, task-specific results, paging/cleanup/cancellation; legacy provenance rejection and real native warm-A/fresh-B/resume-B experiment |

The complete identifiers use the prefix `AC-AGENTS-SESSION-CATALOG-`.
Tests assert choices, preserved state, authority, and transitions, not source text,
forwarding calls alone, incidental wording, or merely larger list lengths.
For each changed behavior, first record the expected behavioral RED, then GREEN.

## E2E tests

- `e2e/tests/session/session-catalog-refresh.spec.ts`, project `chromium`:
  two browser tabs, saved profile Refresh to already-open host session, exact new
  candidate/mode, unchanged active execution/current turn, normal advertised
  selection, candidate confirmation, preserved history/draft, settled replacement
  selection, reload, dirty-profile non-publication, strict-policy complete-empty,
  mode-only/unsupported terminal results, and suppressed post-ACK results reaching
  unavailable without another restart. Include a saved launch-input change before
  confirmation and while confirmation is open, observing ordinary resume's actual
  inputs and preserved conversation on failure. Covers 001.1-001.6, 002.1-002.5,
  003.1, 003.3.
- `e2e/tests/session/mobile-session-catalog-refresh.spec.ts`, `mobile-chrome`:
  same outcome through touch, wrapped notice, confirmation/dismiss/focus,
  candidate selection after rediscovery, no overflow, 44px targets; also phone
  fine-pointer at 767px for the added notice. Covers 003.1-003.3 plus 002.1-002.5.
- `e2e/tests/docker/session-catalog-refresh.spec.ts`, `containers`:
  actual Docker session retains executor-only catalog after a host saved-profile
  refresh; notice-driven confirmed restart rechecks that same executor and keeps
  history. Covers 001.5 and 002.3-002.4. Requires Docker and
  `KANDEV_E2E_CONTAINERS=1`; do not substitute a patched `executor_type` field as
  evidence of real remote execution.

Reuse `profile-capability-discovery` fixtures and `session-tab-restart-helpers`
where they preserve the causal path. Extend mock-agent only with a deterministic
catalog source change shared by probe and replacement launch; do not inject a
client-only catalog and call it end-to-end. Observe requests before Refresh to
prove no automatic stop/launch/model mutation while the old turn continues.
Restore all fixture mutations and close the second tab even on failure.

## Work orders

- [ ] [Task 01: Publish scoped saved-profile catalog observations](task-01-scoped-publication.md)
- [ ] [Task 02: Refresh session catalogs from the composer](task-02-composer-refresh.md)

Task 02 depends on Task 01. The split follows the independently verifiable
publication/settlement API boundary, not a shared schema migration wave. Both
orders are sequential; they share contracts and are not parallel-safe.

### Revised acquisition checkpoint

The user selected fresh task-context design after provider research completed
without a qualified generic-transfer path. A later explicit user request then
authorized its implementation. Task 01 owns the replacement acquisition and
provenance cutover; Task 02 consumes only qualified task-owned candidates and
repeats endpoint/browser proof. Historical results below are not proof of this
boundary; each work order records the revised evidence first.

Review tradeoff: initial production-native model acquisition is explicit, while
ACP mode/model acquisition remains unsupported until qualified. This is not a
promise that arbitrary providers support a safe new catalog API. The mock
model/mode fixture is not production-provider qualification.

Add targeted commands after Task 01 RED/GREEN:

```bash
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agentctl/server/adapter/transport/codexappserver ./internal/orchestrator -run 'TestSessionCatalogTaskDiscovery|TestSessionCatalogRefresh' -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/agentctl/server/process ./internal/orchestrator -run 'TestSessionCatalogTaskDiscovery|TestSessionCatalogRefreshOrderingAndSettlement' -count=1)
```

Run complete changed packages as well as the existing native PostgreSQL,
desktop/phone/Docker and final root gates. Record versioned actual-provider
fresh-helper/no-conversation proof separately from mocked/UI evidence.

## Verification

Each work order contains exact commands. From repo root, validate artifacts with:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/session-catalog-refresh
```

Run `.github/scripts/pr-docs.cjs:validateCoverage` on actual tracked/untracked
paths and the complete requirement/design/plan/work-order contents; separately
include prospective production paths at design time to validate non-exempt
coverage. Confirm all work-order REQ/AC definitions, relative design paths, and
plan design membership. Do not treat documentation-only exemption as that proof.

Implementation bootstrap: install dependencies once from `apps/` in a fresh
worktree. Before tests/builds, check all temporary/cache/output destinations with
`findmnt`; only `/dev/sdd...` is allowed. Use `TMPDIR`/`GOTMPDIR=$HOME/.cache/tmp`,
Go caches on the cache drive, and cache-backed Vite/backend/E2E output directories.
Do not let a managed runner create build output under an iSCSI checkout without
an existing verified cache-drive bind. Stop to correct output storage before
running a forbidden write, not after.

## Verification results

Design validation on 2026-11-10:

- `python3 scripts/list-docs.py validate`: passed (447 decisions, 1970 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- Work-order traceability: both orders resolve all referenced IDs among the three
  requirements and fourteen acceptance criteria; design membership and plan
  membership passed. All 22 local Markdown links resolve.
- `.github/scripts/pr-docs.cjs:validateCoverage`: actual five-document change is
  `exempt`; prospective settings/orchestrator/selector production paths are
  `covered` by both work orders with no errors. Prospective paths were validation
  inputs only, not production changes.
- `git diff --check -- docs/specs docs/plans/session-catalog-refresh`: passed.
- Status shows only this new requirement, design, and three-file plan directory,
  all unstaged and uncommitted.

At the planning handoff, implementation was pending. No live provider, product
test, formatting/build, runtime mutation, or browser verification ran during that
planning turn.

Round 1 Sol adversarial review (2026-11-11) found three material design defects:
an unsupported frozen-launch promise, a legacy-only host predicate, and no
terminal empty/unsupported discovery path. The package now describes ordinary
resume's actual current-input behavior, canonical host identities, and explicit
terminal discovery plus bounded missing-report handling. Implementation was still
pending at that checkpoint. Subsequent review and revised-package checks are
recorded in the task plan; those design corrections are not product verification.

Implementation is in progress under explicit user authorization. Task 01's
backend and focused SQLite/runtime/race evidence exist, but its compatibility
predicate still needs correction: installer identity does not prove equality of
the actual final probe and agent contexts. No additional generated-key exclusion
is approved. The source-grounded normal-host feasibility assessment leaves
AC-AGENTS-SESSION-CATALOG-001.1 unresolved; the
[separate attestation proposal](../../specs/agents/system-design/session-catalog-launch-context-evidence.md)
is a security-review draft, not implementation authorization or a solution to
that feasibility blocker.
Review acknowledged the blocker, not the transport. Further transport/crypto
implementation is not the next step. The user-owned decision is a supported
provider-specific catalog-dependency contract or a separately reviewed actual-
task-context discovery architecture; universal notice-only needs explicit scope
reduction. Eligibility correction awaits that direction; unchanged-contract
implementation and verification resumed after cache headroom recovered.

Task 02's composer flow, focused tests, desktop/phone smoke, locales and guidance
are implemented within the unchanged contract; candidate-publication observations
do not qualify the defective eligibility predicate. Complete desktop and phone
collections, including adjacent restart/settings suites, passed. Their actual
runtime paths include current-input/restore/terminal/preview/Quick Chat/deadline,
duplicate refresh, unchanged native catalogs and running/sibling/other-profile
isolation, with inspected captures.

Native disposable PostgreSQL 18.6 is provisioned on cache-backed storage.
Its scoped conditional-write and concurrent-CAS contracts passed without skips;
the required ordering/settlement race check passed. All specified backend
packages and fourteen affected frontend suites (130 tests) passed. The complete
PostgreSQL-enabled repository package passed untrimmed (2646.706s) with an
extended global deadline; its feature-specific PostgreSQL cases ran rather than
skipping. Combined desktop and phone collections passed (22 and 19 tests).
Required locales and current specification/public-document validators passed.
Formatting and full root classic typecheck passed. The first complete backend
test run exposed inherited service settings, a mixed legacy-slug fixture and
unrelated opt-in PostgreSQL failures. Five packages passed after service-env
isolation and the fixture correction; unmodified worktree strict-lock cases
still fail with Git 2.53.0. Owned lint refactors passed focused catalog cases,
and a false cross-surface completion regression now passes with executable
wire-validation smoke. The full scripts gate passes after native Rust and
cache-backed desktop-build prerequisites were restored.

Current-source results now pass all seven required backend packages. Six passed
in the complete matrix, including the PostgreSQL-enabled repository (2397.394s);
three handler fixtures initially timed out when discovery reached personal
CLIs. Fixture HOME/PATH isolation made the complete handler package pass
(0.710s), without weakening production behavior or timeouts. The required race
check passed (2.025s). Full Go lint reported zero issues; scoped lint passed
after the fixture-only correction.

Fresh required formatting passed (231.26s). The twelve required frontend suites
pass 106 tests after helper extraction, module splits and accessible consent
assertions under Node 24.19.0. Full-app typecheck and full web lint passed after
the final extraction. The earlier full web suite was cancelled at 91% cache usage
after reporting a separate Linear lifetime test failure and local connection
refusals; it is not a passing gate. Revalidated older-cache recovery restored
90% use while protecting current/recent data, source and databases.

Four remaining backend packages pass with service-environment isolation.
The separate unmodified worktree strict-lock failure remains; an isolated
Linear lifetime case also still reaches its existing 5000 ms deadline. No
assertion or timeout was weakened. Final harness, architecture, specification,
public-document, locale and changed-path coverage checks pass.

Fresh desktop/phone qualification remains pending after browser-helper
refactoring. At 90% cache use, less than 0.5 GB of policy-safe headroom cannot
cover the 1 GB fresh-build budget. The required Hermes capacity alert was sent
successfully. Real Docker execution and corrected launch eligibility also remain
mandatory. Both work orders stay open; no final verified implementation, commit
or Sol review-ready handoff exists.

Execution resumed after capacity recovered to 30% use. Following an interrupted
desktop job with no inferred pass, fresh full-build collections passed 22
desktop and 19 phone cases, with current visual captures inspected. These
passes do not qualify launch eligibility. The original exact aggregate passed
typecheck and other backend packages but failed SQLite's cumulative 10-minute
timeout; the complete isolated SQLite suite later passed in 1008.424s with a
2-hour budget. The retained follow-on chain is running full web checks with
observed restart/selector/onboarding failures; no later targets are inferred
passed.

The later parent user choice (2026-11-19 around 10:53) is provider-specific
contract research, superseding the earlier child task-context answer. No
task-context rewrite, context exclusion or attestation transport is approved.
Task 01 records exact-version model-and-mode dependencies and measured native
catalog/mode-factory behavior. No ordinary positive generic-utility-to-task
path is qualified; AC001.1 and real Docker proof remain mandatory and open.
The five package documents are not frozen awaiting parent edits. Both work
orders remain in progress; no complete verified commit or review-ready signal.

Before terminal-owner TDD, cache use reached 91% again. The owned follow-on
aggregate was cancelled and its output retained: SQLite's complete 1008.424s
pass is preserved; full web has partial observed failures and no complete
terminal verdict. No later targets passed by inference. Older secondary caches
were empty and conservative compiled-cache scans found no safe old candidates;
no protected data was deleted. Hermes reported the capacity gate successfully.
Read-only source diagnosis identified incomplete restart/selector fixtures and
an obsolete onboarding forwarding-count assertion. No fixes/reruns are claimed.
Terminal process/source fences and explicit-null native protocol coverage remain
open alongside ordinary eligibility, Docker and final verification.

Cache expansion to 189 GB/58% cleared that storage gate. Independent
same-native-ID process replacement and same-process reload regressions now ran
RED/GREEN with manager/process and adapter-handshake source fences. Actual
authenticated HTTP/WebSocket/process smoke confirmed owned complete-empty,
same-ID reload and stale/unknown owner handling; disposable children stopped
and smoke sources were removed. Native explicit-null coverage passed without
claiming a new pre-fix RED. Corrected web fixtures and removal of obsolete
wiring assertions yielded 49 passing tests across the three failed suites.
These are independent fixes, not AC001.1 or Docker qualification. Subsequent
qualification passed 13 full Go packages (one additional package had no tests),
four terminal-discovery race packages and root typecheck. Full web completed
7635.32s with 30811 passing/eight failing/four skipped tests in 3373 files;
CLI/scripts/lint were not reached. Four-file test-only remediation then passed
63 tests. Current `bg_31` subsequently passed root typecheck, full web
(3373 files, 30818 tests passed/four skipped, 7189.17s) and CLI five tests.
Scripts stopped at an offline exact-Cargo-cache miss; locked fetch then restored
the prerequisites and exact `make test-scripts` passed (363.76s). A case-only
ST1005 correction was followed by native-package pass and exact `make lint` pass
(812.31s), with its external-checkout warning retained. No original SQLite timeout
waiver or complete aggregate pass is inferred. The revised task-discovery draft
still requires its review/later-implementation checkpoint and actual native
helper/provenance plus real Docker qualification.

Revised acquisition implementation (2026-11-21, explicit user authorization).
Generic candidate eligibility is removed; host candidates now require a fresh
task probe in the live owner's sealed launch context (`task_probe_v1`). Task 01
records the helper, capability, API, lifecycle, publication and legacy cutover
evidence, including the actual Codex 0.169.0 Local/Worktree experiment: live A
unchanged, distinct task catalogs, generic editor A, helper frames limited to
`initialize`/`initialized`/`model/list`, and consented resume of the same native
thread. Final-source package, PostgreSQL, race and changed-package lint runs
passed apart from two environment-only cases explained in Task 01. Desktop and
phone collections passed 22 and 19 tests. `make fmt` passed. One sanitized
`make -k typecheck test lint` passed every target except web tests (31,437
passed, 2 timeouts in unrelated files that pass alone) and one Docker-spec
complexity warning, fixed and rechecked with `make lint-web`. The complete web
gate is therefore not passed. `list-docs.py validate` (457 decisions, 2055
specifications), `lint-spec-files.py --all`, the harness lint, public-docs
validation (55 pages) and `git diff --check` passed. Task 01 is done. Task 02
stays in progress with two blockers: the complete web gate and real Docker (no
daemon is reachable here; its spec has not run). No complete verification or
review-ready state is claimed. A resource-bounded complete web retry (sole gate,
4 workers, unchanged deadlines) passed the original two cases but failed three
other deadline-bound cases under shared-host load. A serialized one-worker retry
then failed three cases: two ESLint cold-load timeouts and
`goal-section.test.tsx:135`, a real pre-existing lost-edit bug tracked by
[#5425](https://github.com/kdlbs/kandev/issues/5425) outside this change. The
complete web gate stays unpassed; Task 02 records the details.

After the 2026-11-22 rebase onto upstream `cef5eb5e61`, browser verification
found a real first-launch failure for prepared sessions, fixed in `e1b4a8926a`.
The root gate again failed: three backend packages with failures that reproduce
on pristine upstream, one orchestrator case that passed alone, and 11 web
time-limit failures under load. Typecheck
passed after rerunning at the 5 GiB heap; web lint passed after `56bb7d7238`;
goal-section now passes. Task 02 records exact results. On 2026-11-25 the real
Docker spec passed 1/1 against a real daemon. The backend and web aggregate
gates remain unpassed. After the 2026-11-26 rebase onto upstream `f675a554a5`,
desktop 37/37, phone 25/25 and Docker 1/1 passed. The root gate failed only on
pre-existing, non-catalog causes recorded in Task 02.

## Risks

- Host-probe compatibility cannot be inferred from profile ID alone. Older
  incomplete contexts must use notice-only rather than fabricated equivalence.
- Provider resume/load may return an unchanged catalog or reject restore. Keep
  native identity and normal recovery; never create a replacement conversation
  or force an unadvertised model to manufacture success.
- Catalog projection must not leak host-CLI suggestions, probe defaults, or
  model-dependent options into effective session configuration.
- Session owner replacement and a second profile refresh can race. Provider
  generation counters and process-local probe revisions are different domains.
- Global profile administration does not authorize cross-workspace publication.
- Docker is representative isolated-executor runtime evidence; SSH/cloud/plugin
  cases still need deterministic contract coverage and must not be reported as
  live-tested unless actually exercised.
