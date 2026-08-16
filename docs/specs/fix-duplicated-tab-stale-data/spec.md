---
status: draft
created: 2026-08-16
owner: Kandev
---

# Reload Kandev when a tab is restored from a frozen browser snapshot

## Why

Duplicating a Kandev tab in Chrome restores a frozen snapshot of the page
(back/forward-style restore, not a fresh load; Chromium reports the duplicated
tab's navigation type as `back_forward`). The restored page keeps the JS heap
and DOM as they were when the snapshot was taken, and no boot payload is
re-fetched.

Kandev's data-freshness model assumes fresh loads: the SPA shell and all data
fetches are `Cache-Control: no-store`, so any real navigation re-reads current
backend state. A restored page bypasses that model entirely. The existing
foreground-refresh hooks (`useForegroundRefresh`) are best-effort, cover only
subsets of surfaces, and do not distinguish restores from normal focus events,
so a frozen restore can show stale data (for example, a task archived after
the snapshot still appears in Active tasks) until the user manually refreshes.

Chrome has been rolling out back/forward cache (bfcache) admission for
`Cache-Control: no-store` pages since 2024, so restores of this page are
becoming more common. The platform guidance for this exact situation is to
handle `pageshow` with `event.persisted === true` and refresh or reload the
page.

## What

- When the Kandev page is restored from a frozen browser snapshot, the app
  performs a full reload so state is re-fetched from the backend.
- A restore is detected on the `pageshow` event when `event.persisted ===
  true`, or when the current document's navigation type is `back_forward`
  (covers state-clone restores where `persisted` may be false).
- Normal page loads are unaffected: a fresh navigation, a manual refresh, and
  in-app SPA routing never trigger the reload.

## API surface

No backend, network, or public contract change. Observable behavior: after
Chrome's Duplicate tab (or a back/forward restore) the page reloads once
instead of showing frozen state. This is the same effect as the user's manual
refresh, automated.

## Failure modes

- Restore signals unavailable (no `PageTransitionEvent.persisted` and no
  Navigation Timing API): the handler degrades to a no-op and stale data can
  still appear until a manual refresh. Chrome and Firefox deliver both
  signals; Safari delivers `persisted` on its page-cache restores.
- Reload loop: a reload produces a fresh document whose navigation type is
  `reload` and whose `pageshow.persisted` is `false`, so no recursive reload.
- Open WebSocket at freeze time can make the page ineligible for bfcache on
  back/forward navigations in some Chrome versions; such navigations reload
  normally and are correct without the handler. Genuine restores (including
  Chrome's Duplicate tab) still deliver the signals the handler watches.

## Persistence guarantees

None new. Server state remains the source of truth; the reload re-reads it.
The fix writes no client storage.

## Scenarios

- **GIVEN** a Kandev tab showing a task in Active tasks, **WHEN** the task is
  archived and the user duplicates the tab in Chrome, **THEN** the duplicated
  tab reloads and the task is not shown as active.
- **GIVEN** a loaded Kandev page, **WHEN** the browser restores it from
  bfcache (back/forward navigation), **THEN** the page reloads with fresh
  data.
- **GIVEN** a Kandev page loading normally, **WHEN** it loads, **THEN** no
  reload is triggered.
- **GIVEN** a Kandev page, **WHEN** the user refreshes it manually, **THEN**
  no additional reload is triggered.

## Out of scope

- Incremental state reconciliation after restore. The reload is the
  reconciliation; WS reconnect/resubscribe continues to cover session-level
  data independently.
- Reloading on tab freeze/resume (`resume` event). Backgrounded-tab resume is
  the normal tab-switch flow already handled by the existing
  foreground-refresh hooks.
- Changing HTTP caching headers. The shell and data fetches already use
  `no-store`; bfcache is not the HTTP cache and cannot be disabled with
  headers.
