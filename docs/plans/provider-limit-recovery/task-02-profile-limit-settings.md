---
id: "02-profile-limit-settings"
title: "Per-profile limit recovery settings"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-001
acceptance_criteria:
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.1
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.2
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.3
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.4
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.5
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.6
system_design:
  - ../../specs/agents/system-design/provider-limit-recovery.md
---

# Task 02: Per-Profile Limit Recovery Settings

## Summary

Persist and expose `limit_fallback` and `resume_after_reset` on concrete
profiles. Add the shared `LimitPolicyFor` eligibility function. Render both
switches in the two Fallback settings surfaces on desktop and phone.

## In scope

- `models.AgentProfile` fields and additive SQLite and Postgres migrations.
- DTO, profile contract discovery, pointer partial updates, duplication, and
  export and import.
- Rejection of both fields for dynamic profiles.
- `AgentProfileInfo` fields through `profile_resolver.go`, and `LimitPolicyFor`.
- Web form state, payloads, the dirty and reconciliation helpers, switches in
  `cli-profile-fallback-fields.tsx` and `profile-model-fields.tsx`, and the
  `ModelFallbackSettingsShell` summary clause.
- `settings:*` copy in all seven locales; run `pnpm run i18n:zh-hant` for the
  Traditional Chinese locales.
- Playwright desktop and phone specs from the plan.

## Out of scope

- Any runtime behavior that reads the policy (Tasks 04-07).
- Profile-row limit pills (Task 03).

## Acceptance

1. New and upgraded profiles read both values as false. Save, reload,
   duplicate, and export/import round-trip them. An omitted field in an
   update keeps the saved value.
2. `LimitPolicyFor` returns a fallback only in the explicit fallback state with
   `limit_fallback` on. Resume stays available for strict profiles.
3. The limit-fallback switch is disabled, and keeps its value, under the same
   predicate as the explicit fallback controls. The summary and dirty state
   reflect both switches on desktop and phone.

## ASCII UI preview

`UI-01: Fallback settings` from the [plan](plan.md#ascii-ui-preview)
(AC 001.2, AC 001.3, AC 001.5):

```text
| Use fallback model when limited (i)                  [ on  ] |
|   Continue on the fallback model when a provider limit stops |
|   the primary model.                                         |
| Resume after reset (i)                               [ on  ] |
|   Resume stopped work automatically after a known reset      |
|   (up to 7 days).                                            |
```

## Verification

Run this complete block from the repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/agent/settings/... ./internal/agent/runtime/lifecycle -run 'LimitRecovery|LimitPolicy|Fallback' -count=1)
(cd apps/web && pnpm exec vitest run components/agent/cli-profile-fallback-fields.test.tsx components/settings/model-fallback-settings-shell.test.tsx components/settings/agent-profile-dirty.test.ts components/agent/cli-profile-editor.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-limit-recovery-settings.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-agent-limit-recovery-settings.spec.ts)
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/settings/{models/models.go,dto/dto.go,dto/profile_contract.go,store/sqlite.go,handlers/profile_handlers.go}`
- `apps/backend/internal/agent/runtime/lifecycle/profile_resolver.go`
- `apps/web/components/agent/{cli-profile-editor.tsx,cli-profile-fallback-fields.tsx}`
- `apps/web/components/settings/{profile-model-fields.tsx,model-fallback-settings-shell.tsx,agent-profile-dirty.ts,agent-profile-page-state.ts,agent-profile-reconciliation.ts}`
- `apps/web/app/settings/agents/[agentId]/agent-save-helpers.ts`
- `apps/web/locales/*/settings.json`

## Dependencies

None.

## Risks

- Postgres schema coverage (`postgres_schema_test.go`) must include the
  columns.
- The profile export format must stay backward compatible: missing keys read
  as false.

## Parallelism

`parallel-safe` with Task 01.

## Inputs

- System design: Profile policy.
- Existing `require_exact_model` implementation, which is the closest
  precedent.

## Results

Pending.
