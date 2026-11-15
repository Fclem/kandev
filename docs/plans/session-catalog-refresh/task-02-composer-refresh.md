---
id: "02-composer-refresh"
title: "Refresh session catalogs from the composer"
status: pending
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
(cd apps/web && pnpm exec vitest run hooks/domains/settings/use-profile-capability-discovery.test.tsx lib/ws/handlers/session-catalog-refresh.test.ts lib/state/slices/session-runtime/session-model-hydration.test.ts components/task/model-selector-catalog-refresh.test.tsx components/task/session-catalog-refresh-notice.test.tsx hooks/domains/session/use-session-restart.test.ts components/task/chat/chat-input-area-restart.test.tsx)
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

Pending. No product verification or browser capture in the design turn.
