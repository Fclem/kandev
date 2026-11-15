---
id: "01-scoped-publication"
title: "Publish scoped saved-profile catalog observations"
status: pending
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
  - AC-AGENTS-SESSION-CATALOG-002.4
system_design:
  - ../../specs/agents/system-design/session-catalog-refresh.md
---

# Task 01: Publish scoped saved-profile catalog observations

## Summary

Deliver the saved-profile refresh API, scoped candidate/notice persistence, and
session event/boot contract. The endpoint is independently exercisable before
editor integration; this order changes no rendered UI and never restarts a
runtime. Also implement accepted replacement-catalog settlement so the next
order consumes a complete state machine rather than adding persistence rules.

## In scope

- Add the saved-only POST from the design, sharing the existing internal probe.
  Preserve the existing model response plus a distinct sanitized publication
  outcome. Never publish host-CLI-enriched suggestions or draft results.
- Wire a narrow publication consumer through backendapp. Pin/recheck profile
  context, secret/runtime generation, and refresh currency. Cancellation stops
  remaining fan-out; preserve and report already committed results.
- Add bounded concrete-profile/live-execution session enumeration, including
  initialized IDLE sessions and excluding terminal/archived tasks. Authorize
  each owning task. Use actual concrete profile and executor context.
- Apply the full compatibility matrix from the plan: canonical `local` and
  `worktree` host choices with separate fixtures, legacy `local_pc` only as an
  alias; empty/unknown locality, isolated/remote, and unverified contexts get
  notices only. Unsupported saved-profile probes publish nothing. Do not create
  a provider-name allowlist.
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
  migration, or provider-specific probing.
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

Pending. No production or permanent test changes in the design turn.
