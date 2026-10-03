---
id: "02-message-metadata-setting"
title: "Message metadata fields setting"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COSTS-CONVERSATION-USAGE-005
acceptance_criteria:
  - AC-COSTS-CONVERSATION-USAGE-005.2
  - AC-COSTS-CONVERSATION-USAGE-005.3
system_design:
  - ../../specs/costs/system-design/conversation-usage.md
---

# Task 02: Message metadata fields setting

## Summary

Add the per-user `message_metadata_fields` setting from storage to the web store. Absent means the default selection, an explicit empty list means none, and other clients update live.

## In scope

- Model, SQLite JSON payload, DTO, controller, service validation and apply, settings event, boot projection, and settings catalog descriptor with regenerated discovery contracts.
- Web wire type, camelCase state `messageMetadataFields`, default, mapper, and `lib/message-metadata-fields.ts`.
- Live update through the existing user-settings event handler.

## Out of scope

- Settings editor UI and locales (task 03).
- Rendering any metadata.

## Acceptance

1. A new user and a stored blob without the key read `session_config`, `timestamp`, `duration`; an explicit empty list round-trips as empty.
2. An update with an unknown or duplicate value is rejected with an error naming `message_metadata_fields` and leaves the stored value unchanged. Stored unknown values are dropped on read.
3. The boot payload and a settings event deliver the value, and the web store reflects a change made by another client without reload.

## Verification

```bash
(cd apps/backend && go test ./internal/user/... ./internal/backendapp/ ./internal/settingscatalog/... -count=1)
(cd apps/backend && go run ./cmd/settings-catalog --check)
(node --test scripts/settings-contract-ci.test.mjs)
(cd apps/web && pnpm exec vitest run lib/ssr/user-settings.test.ts lib/message-metadata-fields.test.ts)
(cd apps/web && pnpm run typecheck)
```

## Files likely touched

- `apps/backend/internal/user/{models,store,service,dto,controller}/`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/settingscatalog/defaults.go`, `apps/web/lib/settings-discovery/contract.generated.json`, `apps/web/lib/settings-discovery/profile-contract.generated.json` (regenerate with `cd apps/backend && go run ./cmd/settings-catalog`)
- `apps/web/lib/types/http-user-settings.ts`, `lib/state/slices/settings/types.ts`, `lib/ssr/user-settings.ts`
- `apps/web/lib/message-metadata-fields.ts` and `message-metadata-fields.test.ts` (new)

## Dependencies

None.

## Risks

- A nil-versus-empty `*[]string` can be lost in JSON decoding; cover both in the DTO and store tests.
- The boot mapper test and settings catalog completeness tests can fail on a missing key.

## Parallelism

`parallel-safe` with task 01.

## Inputs

- Design section "Setting contract".
- The `quick_chat_tab_order_by_workspace` and `workflow_ids_with_auto_hide_empty_steps` implementations.

## Results

Pending.
