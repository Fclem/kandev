---
status: current
system: plugins
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-EXTRACTION-001
created: 2026-09-23
owners:
  - kandev
---

# Prompt History Extraction System Design

## Purpose and boundaries

This design removes core's built-in Prompt history panel and the core-only code
that fed it, while preserving every generic Host contract a replacement plugin
uses. The plugin system owns the extraction outcome because the durable contract
after removal is "the review surface is a plugin task panel over published Host
APIs", not "core renders a panel".

Adjacent contracts this design uses but does not own:

- [Prompt History Plugin Host Prerequisites](../requirements/prompt-history-extraction-host.md)
  and its design own the browser conversation façade, task-panel registration,
  the navigation capability, `host.ui.PromptMentionText`, and favorite state.
- [Prompt History Panel System Design](../../ui/system-design/prompt-history-panel.md)
  owns the product behavior of the panel being removed.
- The [task system](../../tasks/README.md) owns message and turn storage,
  including the durable per-session prompt ordinal.
- [UI prompt alias rendering](../../ui/system-design/prompt-alias-rendering.md)
  owns the shared alias presentation that both the transcript and plugins use.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLUGINS-PROMPT-HISTORY-EXTRACTION-001` | [Components and responsibilities](#components-and-responsibilities), [Data and contracts](#data-and-contracts), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery), [Persistence](#persistence) |

## Components and responsibilities

### Removed from core

| Removed | Current responsibility | Replacement |
| --- | --- | --- |
| `apps/web/components/task/prompt-history-panel-content.tsx`, `prompt-history-panel-row.tsx`, `prompt-history-panel-host.tsx` and their tests | Renders rows, expansion, floating older-page indicator, passthrough empty state, and binds the arrow to transcript navigation | Plugin panel rendered from `host.conversation.*` and `host.ui.PromptMentionText` |
| `apps/web/hooks/domains/session/use-session-prompts.ts` | Loads the prompt-only first page from the session message API with `author_type=user`, readiness gating, and refresh generations | Plugin's own paging over `host.conversation.useSessionMessages` |
| `apps/web/hooks/use-lazy-load-prompts.ts` | Auto-loads older prompt pages into the prompt projection | Plugin's own `loadMore()` loop |
| `apps/web/lib/state/slices/session/prompt-message-actions.ts` and the session `messagePrompts` slice (its `types.ts` declaration, `session-slice.ts` initial state and `removeTaskSession` cleanup, `default-state.ts` `mergePromptHistoryState`, `app-state-types.ts` `messagePrompts` plus `replacePromptMessages`/`prependPromptMessages`/`setPromptMessagesLoading`/`setPromptMessagesLoadingMore`) | Separate user-message projection with its own cursor, metadata, generations, hydration merge, and live message-event fan-out | Plugin scope cache in `apps/web/lib/plugins/conversation-source-scope.ts`, which already reconciles independently of the store. The settings `PromptsState` re-exported by `lib/state/slices/index.ts` and `lib/state/store-reexports.ts` is the saved-prompt settings type and stays |
| `apps/web/lib/prompt-history.ts` prompt-entry half (`PromptHistoryEntry`, `buildPromptHistoryEntries`, agent-prompt detection, prompt ordering) and its prompt-entry tests | Derives ordered entries with durations and ordinals | Plugin derivation from the public message DTO (`promptIndex`, author, timestamps) plus `host.conversation.useSessionTurns` |
| `joinInFlightWhileLoading`, `stickToBottomWhileLoading`, and `lifecycleKey` in `apps/web/hooks/use-lazy-load-sentinel.ts`, plus their option tests | Sentinel re-arm, stick-to-bottom, and observer-lifecycle behavior set in production only by the removed panel | None; the transcript keeps the defaults (never join while loading, never stick without an explicit option, no explicit lifecycle key) |
| `useScrollPinnedToBottom`, `STICK_BOTTOM_TOLERANCE_PX`, and the `isPinned`/`refreshPinned` plumbing in the same hook, with the pin-listener case | The scroll-pin state that only `stickToBottomWhileLoading`'s stick write reads | None; the option goes, so nothing reads the pin |
| `minUserPromptsPerLoad` in `apps/web/hooks/use-lazy-load-messages.ts`, with its docblock, cases, and the helpers only it needs (`countUserPrompts` and the `loadedPrompts` accumulator and parameter) | A prompt-count accumulation target with no production caller left after the panel's removal | None; the transcript's callers pass only `minTextPartsPerLoad`, and the orphans would otherwise fail `eslint --max-warnings 0` |
| `PROMPT_HISTORY_PANEL_ID`, `REUSABLE_PANEL_IDS`/`KNOWN_PANEL_IDS`/`PANEL_REGISTRY` entries and index re-exports in `apps/web/lib/state/layout-manager/`, `addPromptHistoryPanel` in `dockview-extra-panel-actions.ts` and `dockview-store.ts` | Gives the built-in panel a saved-layout identity and an "add panel" action | Plugin task panels use the generic `plugin-panel` component and plugin panel ids |
| `apps/web/components/task/dockview-add-panel-items.tsx` Prompt history row and its tests | Desktop and Office "+" menu entry | Plugin task-panel entries in the same menu |
| `dockview-panel-content.tsx`, `dockview-shared.tsx`, `dockview-desktop-layout.tsx` `prompt-history` registrations and their tests | Maps the component name to the panel content and marks it renderable on desktop | Generic `plugin-panel` mapping |
| `apps/web/components/settings/layouts/layout-editor.tsx` `prompt-history` placeholder | Layout-profile editor placeholder | None |
| `apps/web/components/task/mobile/plugin-panel-picker.tsx` `showPromptHistory` and its option, `session-mobile-bottom-nav.tsx`, `session-mobile-layout.tsx`, `lib/state/slices/ui/types.ts` `MobileSessionPanel` member, plus their tests | Native-mobile `Panels` entry and full-height surface | Plugin panels already appear in the same picker through the mobile plugin-panel lifecycle |
| `task:promptHistory*`, `task:expandPrompt`, `task:collapsePrompt` copy in the 7 catalogs, plus their `src/locales/pt-pt/_verbatim.json` declarations | Panel title, row labels, empty state, expand controls | Plugin-owned copy; no core key keeps a panel-only string, and a declaration without a catalog key fails `pnpm run i18n:check` |
| `formatRelativeCompact` in `apps/web/lib/i18n/formats.ts` and its `common:mShort`, `common:hShort`, `common:dShort` keys in the 7 catalogs | Compact elapsed time for the panel rows, whose docblock named the removed duration affordance | None; the helper had no other caller and no test. The `common:*Short` keys go with it |
| `apps/web/e2e/tests/task/prompt-history-panel.spec.ts`, `prompt-history-auto-load.spec.ts`, `mobile-prompt-history-panel.spec.ts`, `apps/web/e2e/helpers/prompt-history-long-seed.ts` | Core panel end-to-end coverage and its seed helper | Existing plugin specs (`tests/plugins/prompt-history-plugin.spec.ts`, `mobile-prompt-history-plugin.spec.ts`, `mobile-plugin-task-panel.spec.ts`) |
| The retired-panel locators in `apps/web/e2e/tests/settings/layout-profiles.spec.ts` and `mobile-layout-profiles.spec.ts` | Layout-editor round-trip for the built-in panel | Rewritten against a surviving reusable panel, plus the retired-panel compatibility case |

`apps/web/components/task/chat/message-list-native-scroll.ts` and other files
that mention the panel in comments are edited for comments only.

### Retained in core

These are the contracts the replacement depends on. They keep their current
public shape and must not gain prompt-history-specific names or branches.

- Browser conversation façade: `apps/web/lib/plugins/conversation-host.tsx`,
  `conversation-source-scope.ts`, `conversation-reconciliation.ts`,
  `host-runtime-resources.ts`, and the Go handlers in
  `apps/backend/internal/plugins/conversation_handlers*.go`.
- Alias presentation: `host.ui.PromptMentionText` and the shared prompt-mention
  presentation module under `apps/web/components/task/chat/messages/`.
- Task-panel registration and navigation:
  `apps/web/components/task/plugin-task-panel.tsx`,
  `scrollTranscriptToMessage` in `lib/state/dockview-extra-panel-actions.ts`,
  the generic `plugin-panel` dockview component, and the mobile plugin-panel
  picker path.
- Read-only favorite state: `apps/web/lib/state/slices/message-favorites/`.
- Message contract: `prompt_index` in `models.Message` and `v1.Message`, the
  persisted per-session sequence read by
  `apps/backend/internal/task/repository/sqlite/message_prompt_index.go`, and
  the `author_type=user` message filter.
- Shared transcript machinery: `hooks/use-lazy-load-messages.ts` (including the
  visible-pagination stop at prompt `#1`), `hooks/use-lazy-load-sentinel.ts`
  without the panel-only option, and `message-list-native-scroll.ts`.
- Turn-duration helpers used by the transcript hover row:
  `messageTurnDurationSeconds`, `formatPromptDuration`, and
  `PromptDurationUnits`, relocated to `apps/web/lib/turn-duration.ts` and
  consumed by `apps/web/components/task/chat/messages/message-actions.tsx`.
  The panel-specific "earlier of turn completion and next prompt" bound is not
  part of that module.

### Changed at a shared boundary

#### Every stored payload that reaches the renderer

A stored layout can reach `api.fromJSON` through several routes, and only some
are filtered today. The removal is safe only when all of them are:

| Route | Payload | Filtered today |
| --- | --- | --- |
| On-ready restore (`tryRestoreEnvLayout`) | per-environment saved layout | yes, `sanitizeLayout` with `DESKTOP_VALID_COMPONENTS` |
| Environment switch, slow path (`performEnvSwitch` in `apps/web/lib/state/dockview-env-switch.ts`) | per-environment saved layout | **no**: `getHealthyEnvLayout` checks shape health only, then `restoreSerializedDockview` calls `api.fromJSON` |
| Apply a saved profile from the preset dropdown (`applyCustomLayout` → `restoreCustomLayout` in `apps/web/lib/state/dockview-store.ts`) | raw `SavedLayout.layout`, both the `columns` branch and the legacy serialized branch | **no**: `normalizeReusableSessionPanels` and `materializeReusableChatPanel` pass non-chat panels through |
| Build the default layout from a custom default profile (`performBuildDefault` and `userDefaultLayout` in `apps/web/lib/state/dockview-store.ts`, fed by `resolveEffectiveDefaultLayout` in `apps/web/lib/layout/layout-profiles.ts`) | the default profile's layout, normalized to a `LayoutState` | **no** today: `validateReusableLayout` normalizes session panels only and the retired id is still reusable, so the profile passes and reaches `applyLayoutAndSet`; yes after this package's profile normalization |
| Retained hidden-right-pane column (`kandevHiddenRightPane` metadata on the per-environment layout, re-inserted by `toggleRightPanels` → `restoreRightPane`) | the metadata's `column: LayoutColumn`, panel definitions included | **no**: `readHiddenRightPane` shape-validates only, both restore paths strip the metadata before sanitizing, and `filterColumn` dedupes by panel id rather than renderability |
| Maximize restore, on-ready (`applySavedMaximize`) | `maximizedDockviewJson` and `preMaximizeLayout` | **no** |
| Maximize restore, environment switch (`restoreMaximizeFromStorage`) | same blob | **no** |
| Layout editor preview | compatibility-normalized profile | yes (`getLayoutProfileCompatibility`) |

An unfiltered payload cannot be instantiated. `dockview-react` looks the
component up in its `components` map and builds the panel content part with
`undefined`; that part's constructor guards the component type and throws
synchronously while `api.fromJSON` drains its initialization queue, so the throw
lands inside the existing `fromJSON` rollback. The rollbacks therefore run:
`restoreSerializedDockview` rethrows; the environment-switch route then applies
that environment's own saved layout and falls back to the default build only
when none is healthy; and the maximize readers delete the blob through
`removeEnvMaximizeState`. The damage is losing the stored layout state, not an
uncaught render crash, and the self-heal deletes what it cannot repair.

#### Shared primitives

Two helpers in a neutral module
(`apps/web/lib/state/layout-manager/sanitize-serialized-layout.ts`) replace the
per-route reasoning:

- `sanitizeSerializedLayout(payload, validComponents)` — the existing
  `sanitizeLayout`, renamed and moved so the store and the env-switch module can
  call it without importing a component module that already imports the store.
  It moves with no re-export shim: `dockview-layout-restore.ts` stops exporting
  it, and `dockview-layout-restore.test.ts` re-points its import (its 15
  `sanitizeLayout` cases move with the function).
- `filterLayoutStateByComponents(state, validComponents)` — the `LayoutState`
  form: drops panels whose component is not renderable, drops a group or column
  left empty *by that drop* (a pre-existing empty group is left alone, because
  empty groups are live state other code preserves), repoints or clears a
  dangling `activePanel`, and preserves `rootOrientation`. `rewriteReusableChatPanels` models the null-propagation but
  returns `{ columns }` only, so it is the pattern, not the shape.

A second neutral module
(`apps/web/lib/state/layout-manager/renderable-components.ts`) owns the
renderable component names as a static export (`RENDERABLE_COMPONENT_NAMES` plus
an `isRenderableComponent()` predicate) rather than a runtime registry, and both
helpers above import it. It has to be static: `dockview-desktop-layout.tsx` is
loaded through `dynamic(() => import(...), { ssr: false })`, and the settings
route that validates saved profiles never imports it, so a registry populated at
that module's load would be empty there and the profile normalizer would behave
differently depending on whether the user had visited a task first.
  - `dockview-desktop-layout.tsx` builds its `components` map from the list
    (every entry is already `PortalSlot`), or at minimum derives
    `DESKTOP_VALID_COMPONENTS` from the same list, so the set and the real
    registrations cannot diverge; a test pins that equality.
  - Every consumer uses the predicate: the serialized sanitizer, the
    `LayoutState` filter, and the profile normalizer. The on-ready restore path
    keeps passing the set it already receives from its caller.

#### Call sites

- `apps/web/components/task/dockview-layout-restore.ts`: `applySavedMaximize`
  and `tryRestoreMaximizeOnly` take the caller's component set, filter the
  serialized payload with `sanitizeSerializedLayout`, filter `preMaximizeLayout`
  with `filterLayoutStateByComponents`, and skip the maximize overlay when the
  maximized group does not survive.
- `apps/web/lib/state/dockview-store.ts`: `restoreMaximizeFromStorage` does the
  same through the renderable-component predicate, and `restoreCustomLayout`
  filters the incoming `SavedLayout.layout` on both branches before applying it.
- `apps/web/lib/state/dockview-env-switch.ts`: the slow path sanitizes the
  healthy saved layout before `restoreSerializedDockview` and holds the sanitized
  payload in `saved` — the local is `const` today and becomes `let` — with no
  storage write, so the active-view replay (`restoreSavedActiveViews`) and the
  right-column width read (`savedRightColumnWidth`) see the layout that was
  actually applied.
  `replaceStaleSessionPanels` takes no payload and is unaffected.
- `apps/web/components/task/mobile/session-mobile-bottom-nav.tsx`: renders the
  `Panels` entry from the canvases and plugin panels that can actually populate
  its sheet, because the term that always kept it visible belonged to the
  removed panel.
- `apps/web/lib/state/dockview-right-pane.ts`: the retained hidden-right-pane
  column is pruned with `filterLayoutStateByComponents` inside `restoreRightPane`
  before insertion. That is the choke point, not an option: `readHiddenRightPane`
  has three readers — `restoreMaximizeFromStorage` (`dockview-store.ts`), the
  environment-switch handler (`dockview-store.ts`), and `setupReadyDockview` in
  `apps/web/components/task/dockview-desktop-layout.tsx`, which populates the
  store on an ordinary page load — and pruning only at read sites would leave the
  ordinary route re-inserting an unrenderable component. The next capture
  persists the pruned column. Without it, a user
  whose retired panel sat in the hidden column keeps `component:
  "prompt-history"` in that metadata, the pane still reports available and
  hidden, and clicking "show right panels" applies an unrenderable component:
  `applyLayout` throws, `applyLayoutAndSet` rolls back, the handler restores the
  hidden state, and the column's surviving panels stay unreachable.
- `apps/web/lib/layout/layout-profiles.ts`: normalizes a saved layout profile
  before validation by dropping panels whose **component is no longer
  renderable** — the same static predicate every other route uses, and one that
  is available on the settings route, not a reusable-id-set rule — then dropping
  a group or column left empty *by that drop* (pre-existing empty groups are left
  alone) and repointing or clearing a dangling `activePanel`, next to the
  existing session-panel normalization. The empty-group clause matters as much as the
  drop: `validateGroup` rejects a zero-panel group, so a group whose only panel
  was the retired one would otherwise keep the profile invalid and lose the
  whole customized default.
  Keeping the predicate component-based and load-independent is deliberate.
  `validateGroup`'s
  `unsupported-panel` check stays reachable and still governs ids that are not
  reusable but whose component is fine, such as the dynamic `pr-detail|<key>`
  and `mr-detail|<key>` panels a captured layout can carry; those profiles keep
  today's `legacy` classification and the existing case that asserts it stays
  valid evidence. Dropping every non-reusable id instead would make that check
  unreachable and silently change how those profiles behave. Without it, `validateReusableLayout` reports
  `unsupported-panel` for the retired id, `resolveEffectiveDefaultLayout` falls
  back to the built-in default, and a user's customized default layout silently
  stops applying. That normalization covers the *validation and default
  resolution* path; the apply path is covered by `restoreCustomLayout` above.
  Both are generic (the profile drop is driven by the renderable-component
  predicate, the apply-path filter by the same predicate), never a
  prompt-history branch.

## Data and contracts

- No backend, DTO, table, migration, or route change. The removal deletes a
  browser consumer only.
- `prompt_index` stays on `models.Message` and the public `v1.Message`, and the
  plugin conversation DTO keeps `promptIndex`, because ordinals, durations, and
  prompt-only pages are plugin-facing behavior. The initial-task-brief fallback
  and session prompt sequence are untouched.
- Retired browser identities: the fixed panel id and component name
  `prompt-history` and the `MobileSessionPanel` member `"prompt-history"`. They
  are removed from their registries and are not reused for a different panel.
  Plugin task panels use the `plugin:<pluginId>:<panelKey>` namespace and the
  generic `plugin-panel` component.
- The saved-layout entry shape is unchanged: a `LayoutPanel` keeps `id`,
  `component`, `title`, and optional `params`. The removal invalidates entries
  whose `component` is `prompt-history`; nothing rewrites stored layouts.
- The message-favorite store keeps its session-scoped shape and storage
  contract; the extracted panel was only one reader of it.

## Control flow

1. **Open the surface (desktop).** `AddPanelMenuItems` lists canonical panels
   plus registered plugin task panels. After removal the Prompt history row is
   gone; an installed replacement plugin contributes its own row and opens a
   `plugin-panel` dockview panel with a plugin panel id.
2. **Open the surface (phone).** The `Panels` bottom-navigation picker lists
   task canvases and plugin panels. After removal its only prompt-history path
   is the plugin registration; `showPromptHistory` and its dedicated button are
   deleted. The bottom-navigation `Panels` entry is rendered from
   `showPromptHistory || hasTaskCanvases || mobilePluginPanelsAvailable`, so
   dropping the first term means a task with no canvases and no enabled plugin
   panel no longer shows an entry that would open an empty sheet.
3. **Read prompt data.** The plugin calls the façade, which performs authorized
   reads and live reconciliation into its own scope cache. Core's store is no
   longer involved, so no store slice, generation counter, or live fan-out is
   needed for prompts.
4. **Navigate to a prompt.** The plugin's navigation capability calls
   `scrollTranscriptToMessage`, which activates the chat panel and queues the
   pending scroll target in the dockview store; the transcript consumes it
   exactly as before.
5. **Restore a saved layout.** `sanitizeSerializedLayout(layout, DESKTOP_VALID_COMPONENTS)`
   runs before `fromJSON`; with `prompt-history` no longer a known component or
   known panel id, the entry is dropped and the remaining panels restore
   unchanged. `filterEphemeral` and the canonical-title normalization apply the
   same rule when a layout is captured or saved.

## Failure and recovery

- A stale saved layout, layout profile, or per-environment layout that contains
  `prompt-history` restores without the panel. On the restore route the existing
  sanitize step drops the entry, logs the dropped ids through the
  `dockview:restore` debug logger, and applies the rest of the layout; the
  profile route drops it during normalization, which is not logged. Either way
  no error, notice, or empty panel is shown, and the user repeats no action.
- A saved default layout profile that references `prompt-history` keeps
  applying. Profile normalization drops the retired entry — and any group or
  column it leaves empty — instead of failing validation, so the remaining panels
  still load and the settings surface does not demote the profile to a legacy,
  ignored record.
- Browser-side maximized state is the least obvious of the unsanitized paths,
  and it is reachable: the blob stores a full `api.toJSON()` of the layout, so a single
  maximize action taken while the retired panel was open captures its identity,
  and the panel used the default tab, whose double-click maximizes. On restore,
  `applySavedMaximize` hands `maximizedDockviewJson` straight to `api.fromJSON`
  and keeps `preMaximizeLayout` raw. Removing the identity from the renderer's
  `components` map and from `KNOWN_PANEL_IDS` leaves nothing for
  `dockview-react` to instantiate, so the panel content part is built with an
  undefined component and its type guard throws synchronously inside
  `api.fromJSON`. The existing self-heal catches that throw and deletes the
  blob, so the repair is a discard: the maximum state is gone. On the
  environment-switch route `performEnvSwitch` then applies the environment's own
  saved layout and falls back to the default build only when none is healthy;
  on the maximize-only reader, which runs only when no usable per-environment
  layout exists, the built-in default layout is what remains if the overlay is
  skipped. That reader's only recoverable state is the filtered
  `preMaximizeLayout`, so the hardening below applies it instead of falling
  through (see below).
  The two structures need different primitives, because their shapes differ:
  `maximizedDockviewJson` is a serialized dockview payload (`panels` plus
  `grid.root`), so `sanitizeSerializedLayout` applies; `preMaximizeLayout` is a
  `LayoutState` (`columns[].groups[].panels` plus an optional tree), so it is
  filtered panel by panel against the same component set, with a group or column
  left empty *by that filter* dropped and a pre-existing empty group left alone
  (empty groups are live state that other code preserves), a dangling
  `activePanel` repointed or cleared, and the
  `rootOrientation` carried through — `rewriteReusableChatPanels` models the
  null-propagation but returns `{ columns }`, and dropping `rootOrientation`
  would flip a vertical root on every maximize restore.
  `filterEphemeral` is deliberately not used there: its set omits `canvas`,
  `review-detail`, `file-editor`, `diff-viewer`, `commit-detail`, and the legacy
  aliases, and it preserves empty groups by design, so running it over a live
  pre-maximize layout would drop panels the user still has.
  When the maximized group does not survive sanitization, the two readers skip
  the overlay differently, because only one of them has a saved layout to fall
  back on:
  - On the on-ready maximize-only reader (`tryRestoreMaximizeOnly`), which is
    reached only when there is no usable per-environment layout, skipping the
    overlay must not return false: that path ends in the built-in default layout
    and would discard the panels the blob's `preMaximizeLayout` holds. Apply the
    filtered `preMaximizeLayout` through the normal layout-apply path, leave
    `preMaximizeLayout` and `maximizedGroupId` null, and return true.
  - On the environment-switch reader (`restoreMaximizeFromStorage`), returning
    false is correct: `performEnvSwitch` then applies the environment's
    sanitized saved layout, which is normally the same pre-maximize layout,
    because `saveOutgoingEnv` stores it under that environment's key alongside
    the blob.
  A blob that still fails after sanitization keeps the existing self-heal
  deletion, and no saved layout, profile, or favorite is deleted.
- With no replacement plugin installed, the workbench has no Prompt history
  entry at all, so there is no loading, empty, error, or passthrough state to
  render. The panel's former empty and passthrough states become the plugin's
  responsibility.
- Deleting the core projection must not disturb transcript behavior. Session
  slice edits remove only prompt-specific state and actions; mixed-author
  transcript merging, message signatures, deletion handling, and
  `removeTaskSession` cleanup stay intact.
- Deleting the `lib/prompt-history.ts` entry builder must not disturb the
  transcript turn duration. The duration helpers move unchanged, and
  `message-actions.tsx` keeps its current rendering and copy.

## Persistence

- Server-side storage is unchanged: no SQLite or PostgreSQL migration, no
  route, table, or column change, and no startup work. Saved layouts and saved
  layout profiles, including the active default, are persisted user settings
  (`saved_layouts`, written through the settings API), so a retired panel inside
  one is normalized when the settings are read and applied rather than rewritten
  in place.
- Browser-local state: the per-environment layout and its profile identity
  (`kandev.dockview.env-layout-v3.<envId>`,
  `kandev.dockview.env-layout-profile-v1.<envId>`) and the per-environment
  maximized-state blob
  (`kandev.dockview.env-maximize-v3.<envId>` in `apps/web/lib/local-storage.ts`)
  may still reference `prompt-history`; they stay inert, and the retired entry
  is dropped when the record is used. The message-favorite session storage stays
  readable. No saved layout, profile, or favorite is deleted during the upgrade;
  only a maximized-state blob that cannot survive sanitization is discarded.
- Restart behavior is unchanged: layouts reload through the same sanitize path,
  and the replacement plugin, when installed, re-registers its panel on load.

## Security

Authorization is unchanged: the removal deletes a consumer of already-authorized
data. The façade continues to require an active plugin, the `api_read:messages`
capability, and task-session access before returning messages or turns, and the
transcript navigation capability continues to validate session and generation.
No new privileged surface is introduced, and no prompt-history-specific
authorization path is added.

## Observability

No new metrics or counters. The dockview restore path already reports dropped
invalid panel ids at debug level (`dockview:restore`), which is the visible
evidence that a stale panel entry was ignored. Plugin conversation telemetry and
gateway logs are unaffected.

## Related decisions

- [Browser plugin conversation facade](../../../decisions/2026-09-06-browser-plugin-conversation-facade.md).
- [Conversation source reconciliation](../../../decisions/2026-09-16-conversation-source-reconciliation.md).
