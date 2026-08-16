---
id: "03-security-page-relative-last-seen"
title: "Security page option, relative column, tooltip, live update"
status: done
wave: 3
depends_on: ["02-frontend-settings-contract"]
plan: "plan.md"
spec: "../../specs/relative-last-seen/spec.md"
---

# Task 03: Security page option, relative column, tooltip, live update

## Acceptance

- The Active sessions card on `/settings/account/security` shows a labeled **Last seen** select
  (Absolute time / Relative time) above the table, with self-documenting copy and a discovery
  target. The card's `SettingsCard` keeps its existing `sessions` target (it accepts exactly one
  `discoveryTargetId`); the select's wrapper registers `ACCOUNT_SETTINGS_TARGETS.lastSeenDisplay`
  separately via `useSettingsTargetRegistration` from `settings-target-provider.tsx`, and a
  discovery-target test asserts both the sessions and the last-seen-display targets register and
  reveal. The trigger and options get a mobile-appropriate touch target (min-h-11 / 44px at
  phone widths via responsive classes on this instance; the shared Select defaults to 28px, below
  the /mobile-parity 44px active-dimension guidance).
- Default (`absolute`) renders `formatDateTime(last_seen_at)` exactly as before.
- Relative mode renders a locale-aware `formatRelativeTime(last_seen_at, now)` label inside a
  tooltip whose content is the absolute `formatDateTime`, and the label advances while the page
  stays open. The relative trigger is focusable (`tabIndex={0}`, semantic element) and exposes the
  absolute timestamp as its accessible name and native `title` fallback, so touch and assistive
  tech reach the exact moment without hover. An unparseable `last_seen_at` renders an empty cell
  with no tooltip (guards the `formatDateTime` `RangeError` on invalid dates).
- Changing the select keeps the optimistic value in local component state (never written into the
  store) and persists via a queued `createQueuedUserSettingsSyncWithResponse({ last_seen_display })`
  write with a latest-operation revision guard (the `use-app-status-bar-order.ts` pattern). The
  store's `userSettings.lastSeenDisplay` is the confirmed baseline, updated by exactly two
  unchanged ingestion paths: (1) a successful PATCH response mapped with
  `mapUserSettingsResponse(response, current)` and applied THROUGH `setUserSettings` (never a raw
  store write; `setUserSettings` discards older revisions, `settings-slice.ts:245-250`); (2) a
  newer `user.settings.updated` WS snapshot through the existing handler path
  (`mapUserSettingsData` inside `store.setState` with the handler's own revision gate,
  `users.ts:12-24`), which Task 02 requires to stay UNCHANGED. On failure, drop the optimistic
  override and fall back to the baseline (only when the failed write is still the latest) and show
  error copy.
- The optimistic override is gated on the pending write itself: while the select's write is in
  flight, store changes do NOT clear the override. The backend event is a FULL snapshot re-published
  by every user-settings write (any tab, any field), and full-snapshot HTTP responses are applied
  by callers that build from a fresh default state, so own echoes, unrelated same-tab snapshots
  (app-status-bar order, review top bar), and fresh-default response mappings all re-assert the
  pre-write baseline or the write's own confirmation and must not kill a newer queued selection.
  The override clears only when the LATEST operation settles: success applies the confirmed value,
  failure drops to the store baseline, both guarded by the latest-operation counter; after settle
  the cell always renders server truth, including a foreign `last_seen_display` change that landed
  while the write was pending. The WS handler is unchanged (own echoes applied normally), so the
  store converges even if the component unmounts mid-write.
- Component tests cover absolute default, relative label, tooltip content on hover, Tab focus
  opening the tooltip, accessible name/title carrying the absolute timestamp, live ticking (fake
  timers), select persistence, unparseable-input empty cell, the stale-failure no-revert guard, two
  consecutive failures settling on the server-confirmed baseline, a pending local write followed by
  a foreign `last_seen_display` WS event KEEPS the override while the write is in flight and
  renders the foreign value only after the latest operation settles (with the event arriving
  before the write settles and a delayed older PATCH response arriving after it, both asserted),
  a deferred PATCH response
  arriving after a newer WS event (the response is discarded by the `setUserSettings` revision
  guard, so the store keeps the newer value), a pending local write is NOT cleared by an unrelated
  full snapshot while in flight (own echo, same-tab app-status-bar/review write, or fresh-default
  `mapUserSettingsResponse` carrying the old `last_seen_display`; the override survives until its
  own operation settles), a foreign `last_seen_display` change landing while a write is pending
  renders after that write settles, a queued A-then-B selection keeps B's optimistic value visible
  while A settles (the store briefly confirms A but the latest-operation guard prevents A from
  clearing B; when B settles the override clears and the cell renders the confirmed value, or a
  newer foreign value that superseded it), and an unmount/remount schedule: a pending write
  started, the component unmounted, an own/newer WS snapshot delivered, the PATCH resolved
  (success) and rejected (failure), then remount — the store converges and the rendered value is
  correct in both paths with no stale-override or unhandled-rejection side effects. The component
  tests exercise the PATCH-response ingestion path (via `setUserSettings`) and the WS ingestion
  path (via the unchanged handler's `mapUserSettingsData` + revision gate) as separate mocked
  paths.

## Verification

```bash
cd apps/web && pnpm run typecheck
cd apps/web && pnpm vitest run components/settings/account
cd apps/web && pnpm run lint:i18n components/settings/account/security-settings.tsx
```

## Files likely touched

- `apps/web/components/settings/account/security-settings.tsx` (+ new `security-settings.test.tsx`)
- `apps/web/lib/settings-discovery/catalog/account.ts`
- `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn,zh-hk,zh-tw}/account.json`

## Dependencies

Task 02 (store field and payload type must exist).

## Inputs

- Spec "What", "Failure modes", "Scenarios"
- Existing precedent: `ChangesPanelLayoutCard` select anatomy, `useNow(30_000)` live labels,
  `formatRelativeTime`/`formatDateTime` from `@/lib/i18n/formats`, Radix tooltip usage rules in
  `apps/web/AGENTS.md`

## Output contract

Return a compact handoff capsule with acceptance status, exact test command/results, risk tags,
uncertainties, and set this task to `done`.
