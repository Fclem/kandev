---
id: "03-settings-editor"
title: "Settings editor"
status: done
wave: 2
depends_on:
  - "02-message-metadata-setting"
plan: "plan.md"
requirements:
  - REQ-COSTS-CONVERSATION-USAGE-005
acceptance_criteria:
  - AC-COSTS-CONVERSATION-USAGE-005.2
  - AC-COSTS-CONVERSATION-USAGE-005.7
system_design:
  - ../../specs/costs/system-design/conversation-usage.md
---

# Task 03: Settings editor

## Summary

Add a Message metadata checkbox group to the task behavior settings so users choose which items the hover line shows. Add its copy to all seven locale catalogs.

## In scope

- `MessageMetadataSettings` component on the Conversation tab of `task-behavior-settings.tsx`, using `useSettingsSaveContributor` like `archive-confirmation-settings.tsx`, plus a settings discovery target.
- Labels and description in `settings` catalogs for `en`, `ja`, `ko`, `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`, with the Traditional Chinese pair generated and the pseudo locale regenerated.
- The user-facing setting and metric-window documentation is added in task 05, when the complete display behavior ships.

## Out of scope

- Rendering the line or window (tasks 04 and 05).

## ASCII UI preview

UI-01 settings excerpt from the [full preview](plan.md#ascii-ui-preview), `AC-COSTS-CONVERSATION-USAGE-005.7`.

```text
Message metadata
  [x] Model and session options   [x] Timestamp   [x] Duration
  [ ] TTFT        [ ] Tokens per second          [ ] Tokens   [ ] Cost
```

## Acceptance

1. The group shows the saved selection and saves a change. A failed save leaves the stored selection unchanged, keeps the draft dirty, and shows an error.
2. Checkbox targets are at least 44px on touch layouts with no horizontal overflow.
3. `pnpm run i18n:check` passes with the new keys in every catalog.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/settings/message-metadata-settings.test.tsx components/settings/task-behavior-settings.test.tsx components/settings/task-behavior-tabs.test.ts lib/settings-discovery/catalog.test.ts)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo && pnpm run i18n:check)
(cd apps/web && pnpm run typecheck && pnpm run lint)
```

## Files likely touched

- `apps/web/components/settings/message-metadata-settings.tsx` and test (new)
- `apps/web/components/settings/task-behavior-settings.tsx`, `task-behavior-tabs.ts` (`TASK_BEHAVIOR_TARGET_TABS` and `CONTRIBUTOR_TABS`, keyed by the contributor id passed to `useSettingsSaveContributor`) and `task-behavior-tabs.test.ts`
- `apps/web/lib/settings-discovery/catalog/preferences.ts` (`GENERAL_SETTINGS_TARGETS` entry and descriptor) and `catalog.test.ts`
- `apps/web/src/locales/*/settings.json`

## Dependencies

Task 02.

## Risks

- i18n checks reject untranslated duplicates and em dashes; declare verbatim terms such as TTFT in `_verbatim.json` only where the correct translation is the English word.

## Parallelism

`sequential`

## Inputs

- Design section "Presentation".
- `archive-confirmation-settings.tsx` and its test.

## Results

Results: Added the Conversation-tab checkbox group, save contributor, discovery target, touch-sized wrapping controls, and localized labels. The 21 targeted settings/discovery tests, Traditional Chinese and pseudo generation, `i18n:check`, web typecheck, and lint passed.
