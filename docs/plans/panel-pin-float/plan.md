# Plan: Floating (unpinned) workbench panels

Spec: [`docs/specs/ui/panel-pin-float.md`](../../specs/ui/panel-pin-float.md)

## Summary

Add a per-group pin toggle to the dockview workbench group headers (left of the
maximize control, message-queue pin icons). Unpinning floats the group over
the workbench; it collapses to an edge title bar when unfocused and re-docks
on pin click. State persists per task environment in sessionStorage, mirroring
the existing env layout / maximize persistence. **Revision 21 incorporates the
round-20 adversarial review** (this package has been adversarially reviewed
every round): a single coordinator-owned tombstone TTL (30 s) with no
token-carrying-event claim, `normalizeLayoutIdentities` on every restore
route, explicit grid-vs-floating active-state authority with atomic handoff,
a unified fail-closed untrusted-journal policy (persisted repair record,
read-only salvage, AlertDialog clear), canonical UTF-8 byte encoding with
Unicode golden vectors, separation of the one-time synchronous migration hash
from async ordinary journal hashing (precomputed digests, direct declared
dependency, benchmark threshold), a machine-readable consumer manifest with a
legacy-key validator, and the corrected task-03 dependency graph.

## Architecture

Panel content already lives outside dockview: `PanelPortalHost` renders every
registered panel into persistent portal elements owned by `panelPortalManager`
(`apps/web/lib/layout/panel-portal-manager.ts`); dockview wrappers adopt/release
those elements via `usePortalSlot`. **However, normal panel removal destroys
the portal**: `setupPortalCleanup`'s `onDidRemovePanel` handler
(`apps/web/components/task/dockview-layout-setup.ts:488-502`) calls
`panelPortalManager.release(panel.id)`, parks/stops terminals, stops vscode,
and runs `handleMaximizeExitOnLastClose`. Float therefore needs the explicit
non-destructive detach registry (below), not bare `api.removePanel`.

Maximize is store-driven (`maximizeGroup`/`exitMaximizedLayout` in
`apps/web/lib/state/dockview-store.ts:934-1000`), persisting a
`preMaximizeLayout` LayoutState per env. Floating reuses the same store +
per-env sessionStorage pattern.

### Key invariants

1. **Live env layout always.** The persisted env layout reflects the live
   grid (floated groups absent), unchanged from today. The floating blob
   (`kandev.dockview.env-floating.<envId>`, versioned + type-guarded like
   `isEnvMaximizeState`) carries complete `FloatingPanelDef`s (id, component,
   title, tabComponent, params) + placement metadata (columnId, columnIndex,
   columnKind, columnPinned, treePath, edge, orientation, size, order,
   display), so floated groups can be **materialized** after any reload/env
   switch/layout rebuild and re-floated.
2. **Detach registry, not a global id set.** Registrations are keyed by
   composite `(panelId, envId)` records `{ token }` — a same panel id in the
   old env's blob vs the new env's live grid is distinct (blob-level
   bookkeeping; same-ID live coexistence is impossible because only one env
   is live and the portal manager holds one entry per live panel).
   `setupPortalCleanup`'s `onDidRemovePanel` decision order: (1) registered
   `(panelId, envId)` with the current token → consume the registration and
   skip close side effects, regardless of `isRestoringLayout`; (2)
   unregistered while `isRestoringLayout` → return (existing behavior); (3)
   unregistered otherwise → full cleanup (a user closing a floated tab
   mid-transaction is an ordinary close). Registrations are armed per
   expected removal (immediately around each synchronous remove/`fromJSON`,
   consumed once, drained on operation end). `panelPortalManager.reconcile`/
   `releaseByEnv` take an env-qualified exclusion predicate over
   `(panelId, entryEnvId, token)`; `saveOutgoingEnv` passes the OUTGOING env.
   Stale tokens never clear newer registrations; the env's registry clears on
   transaction settle (success/failure/unmount).
3. **Placement classifier + materializer over LayoutState.** Dockview exposes
   no left/right/top/bottom group location, and `isCenterCandidateGroupId`
   misclassifies plan/preview/custom columns (it returns true for every group
   except three constants — `layout-manager/applier.ts:37-39`). Classification
   is a pure function over the live `LayoutState` (root column id/index/
   pinned metadata) plus an **explicit, nullable center identity**
   (`centerColumnId, isCenterKnown`) — `findCenterGroupId` fabricates ids when
   no center exists (`applier.ts:45-55`), so an unknown center classifies as
   the documented custom fallback, never by promoting an arbitrary side
   column. Plan/preview/vscode/compact root columns classify as side/vertical
   on their edge. Re-dock/restore materialize missing columns/groups by
   cloning the live layout, inserting the saved column at `columnIndex` with
   metadata, inserting the group **atomically into both `column.tree` and
   `column.groups`** (one mutation helper — `serializeColumn` prefers `tree`
   while `serializePanels` iterates only `groups`,
   `layout-manager/serializer.ts`), and applying through the existing
   serializer/`applyLayoutAndSet` machinery. `fallbackGroupPosition`
   (`dockview-layout-builders.ts:272`) is the explicit existing-group fallback
   only, never the column-creation mechanism.
4. **Transactions: one coordinator + digest journal + phase model.** A single
   coordinator (`floating-transaction.ts`) owns the per-env
   operation/generation and phase state via explicit `begin → advance →
   settle`; while busy, every public layout-mutation boundary (float/dock/
   reset/toggle, the add-panel resolver, `buildDefaultLayout`, preset/custom
   apply, maximize/exit, programmatic actions) **and restore/recovery paths**
   reject/skip with a non-destructive result, and all three pin surfaces
   render disabled via `isFloatingTransactionBusy(envId)`. A per-env
   **operation journal** (`kandev.dockview.env-floating-journal.<envId>`) is
   written **before** mutation holding `{envId, transactionId, phase, tagged
   before/after digests, raw strings}`; digests are SHA-256 over the exact
   raw storage bytes with a **tagged absent/present union** (absence is never
   conflated with a stored marker value); every write is **`writeVerified`**
   (set → read back exact bytes → compare; any mismatch is a failed write
   **Recovery is a phase-aware decision matrix**:
   both-before → verify the before pair and clear; partial → apply/verify the
   after pair; both-after/equal → settled — the journal clears only after the
   **selected target** is verified, never "always after". **Journal
   integrity:** `isEnvFloatingJournal(journal, envId)` validates version/env/
   phase/transaction/digest/raw shape and **recomputes SHA-256 from each raw
   snapshot** before any target is selected; an invalid or mismatched journal
   is quarantined via an idempotent deterministic key `(envId, raw digest)`
   (copy → verify → remove original → verify absence; never a second copy per
   original; bounded corrupt-key cleanup) and treated as unreadable (journal-free
   fallback with the caller's envId). Recovery cache
   keyed by `(envId, transactionId, api instance)`; `recoverFloatingJournalOnce`
   runs before every restore entry. **Settle drain is the same async settle
   operation, ordered before busy clears** (no interleaving `begin` window;
   the drain is a coordinator-owned internal operation authorized by an
   opaque Symbol capability with a `try/finally` that always token-guards the
   transition to settled or failed-settled, so busy can never stick); a new
   `begin` consumes any retained marker
   first; the marker clears only after a successful settle restore or
   **Root-column metadata lives INSIDE the blob**
   (`EnvFloatingState.rootColumns`, incl. `role`), rebuilt after every layout
   apply and reload, invalidated on preset/reset/env switch, covered by the
   journal, budget, and cleanup — no third storage key; the blob also carries
   a durable `identities` map (group/column UUIDs) that survives an empty
   floating state so refloat after dock never mints a new identity. **The
   persisted LAYOUT schema is versioned (v4 envelope)** — `{version: 4,
   dockview, layout}` so `logicalId`/`role` survive native dockview
   serialization; `migrateEnvLayoutV3(raw, envId)` assigns UUIDs and roles
   once (versioned marker; right = pinned column with files/changes, tie →
   leftmost, none → no pinned-right role) and persists v4 only after a
   validated apply, with a v3 reader fallback and e2e-helper prefix updates.
   **A synchronous role bootstrap** normalizes live `LayoutColumn.role` and
   rebuilds the in-memory sidecar BEFORE any `hasLivePinnedRightColumn` call.
   **One sole pair writer** (`persistSettledPair`) routes every persistence
   path (debounce/unload, `persistEnvLayoutNow`, `saveOutgoingEnv`,
   preset/custom/reset, float/dock) with a lock/queue/reject policy. **Size
   budgets:** per-env cap
   (96 KB) + **global allocation budget** (384 KB) enforced via a validated
   owned-key index whose entries are checked against stored raw
   length/digest + key set before every decision (one bounded prefix-scan
   rebuild on mismatch; sessionStorage has no same-document event); quota
   races after preflight fail closed via journal rollback (the backstop).
   Recovery runs the phase model `mutating →
   restoring → portals-adopted → persist-recovered → settled`; only
   portals-adopted persists the journaled layout (status-returning APIs);
   token cleanup is generation-guarded at settled. Journal-free fallback:
   per-panel salvage + `allocateUniqueGroupId` + **tagged tree/flat
   placement with an exact shape-change mapping** (flat→tree = DFS
   leaf-order index clamp; tree→flat = leaf-order index clamp). **Task
   deletion invalidates the env generation before cleanup** so a late settle
   can never rewrite deleted keys. One transaction-aware unload handler;
   **single storage policy: fail closed** — rollback to pinned; no ephemeral
   floating mode.
5. **Owned-region focus with refcounted, same-frame-leased pending collapse.**
   One module-level coordinator (pattern: `hooks/use-panel-search.ts`) with
   owned regions = floating window subtree + any Radix layer opened from
   within it (`useFloatingOwnedLayer`; idempotent unregister on dismiss AND
   React cleanup; **mandatory inventory of layer owners in every
   floating-capable panel** — chat, plan, terminal, files, changes, diff,
   plugins, editors). Collapse on: window-capture pointerdown outside all
   owned regions (**pending while the window's owned-layer refcount is above
   zero; application deferred past the microtask and held by a same-frame
   lease that re-checks window generation, pending generation, refcount, and
   new-registration — same-turn and same-frame Radix replacements never
   collapse; longer task gaps may, and that boundary is documented**);
   `focusout` to outside (relatedTarget after a microtask); Escape on the
   **bubble** phase honoring `event.defaultPrevented`. Only the
   focused/last-interacted expanded window collapses.
6. **Reset is an id-aware docking merge, session-aware, with a canonical
   session fallback.** The reset layout owns group/column placement; the
   floating definition owns the panel payload and saved tab order; the active
   panel is merged explicitly — scoped to valid definitions (`session:*` defs
   validated against the active session set; stale dropped; reset chat
   placeholder retained when none remain; **a valid active session with no
   center column lands in the first column's first group with the active
   session set, so the auto-session hook never double-inserts**). Reset-default
   panels are reused by id (a floated ordinary terminal keeps its real
   terminal id); floating state clears only after the merged grid is
   committed; groups do not re-float.
7. **Restore call sites (exhaustive).** `recoverFloatingJournalOnce(api,
   envId)` runs before every restore entry, then `restoreFloatingAfterLayout`
   (idempotent) runs before `isRestoringLayout` clears at: initial mount
   `restoreEnvLayout` (all three branches: saved env layout /
   `tryRestoreMaximizeOnly` / default+route-intent — `dockview-layout-restore.ts`),
   env-switch fast and slow paths (`dockview-env-switch.ts`), **maximize
   restore — selected sequence: defer-until-exit** (journal first, floating
   session entries reconciled with `floatingSessionWinner` written before the
   auto-session hook, the two-column overlay never mutated, a per-env
   pending-floating-restore marker set, materialization only after
   `exitMaximizedLayout`'s rAF settles), preset/custom apply,
   `toggleRightPanels` (`dockview-store.ts:523-567`), and reset/default
   build. One focused test per call site.
8. **Identity coordination (session + right panels).** Session replacement is
   one coordinator over grid + floating entries; the winner is written to a
   store-owned, **memory-only** `floatingSessionWinner: { sessionId, envId,
   generation } | null` atomically with replacement and consumed one-shot via
   atomic **compare-and-clear** `consumeFloatingSessionWinner(...)` from
   `shouldSkipPanelEnsure` (skips only the winner id; unrelated siblings
   ensured as today; stale winners cleared on generation/env transition and
   terminal paths, never clearing a newer generation). Replacement also
   **normalizes placement in a post-apply hook** — after the synchronous
   layout/session replacement AND incoming-session insertion, resolving the
   root column by direct live group membership + index (never
   `fromDockviewApi`'s panel-derived ids, never `findCenterGroupId`'s
   fabricated fallback); `isCenterKnown=false` keeps the custom fallback.
   `rightPanelsVisible` is exactly **`hasLivePinnedRightColumn`** — pinned
   canonical right-column presence, never any side column (plan/preview/
   vscode unpinned side columns return false, matching today's preset
   assignments); derived everywhere, never persisted. The toggle show path is
   floating-aware (**every** floating id excluded), removes empty groups,
   drops the right column when no pinned-right panels remain (legal
   serialized tree). The enforcement gate, toggle, and layout-setup detection
   share this one predicate. No panel id ever exists in both surfaces.

## Files

### Likely touched

- `apps/web/components/task/dockview-group-actions.tsx` — `PinButton` +
  placement in `GroupSplitCloseActionsView` (left of `MaximizeButton`).
- `apps/web/components/task/dockview-header-actions.tsx` — wire pin state +
  `floatGroup`/`dockGroup` into the shared `GroupSplitCloseActions`.
- `apps/web/components/task/dockview-floating-panel.tsx` (new) — floating
  window + collapsed edge bar overlay, rendered inside `DockviewDesktopLayout`
  root; adopts `panelPortalManager` elements; tablist/tab semantics; stacking
  by `(order, groupId)`.
- `apps/web/components/task/dockview-floating-coordinator.ts` (new) —
  module-level owned-region focus/outside-pointer/Escape ownership +
  `useFloatingOwnedLayer`.
- `apps/web/components/task/dockview-desktop-layout.tsx` — mount the floating
  overlay; wire `restoreFloatingAfterLayout` at ready.
- `apps/web/lib/state/dockview-store.ts` — `floatingGroups`, detach registry,
  transaction actions (`floatGroup`, `dockGroup`, `setFloatingDisplay`,
  `setFloatingActivePanel`), env-switch save/restore, reset-as-docking,
  maximize interplay.
- `apps/web/lib/state/dockview-floating.ts` (new) — placement classifier,
  materializer, `restoreFloatingAfterLayout`, journal helpers, versioned
  `EnvFloatingState` type guard (JSON-safe, 64 KB/def params bound), cleanup.
- `apps/web/components/task/dockview-layout-setup.ts` — detach-registry
  checks in `setupPortalCleanup`; restore completion points.
- `apps/web/lib/state/dockview-env-switch.ts` — session replacement returns an
  old→new mapping applied to floating entries (winner rule); restore
  invocation on fast and slow paths.
- `apps/web/lib/state/dockview-layout-builders.ts` — `focusOrAddPanel` becomes
  `focusOrAddFloatingOrGridPanel`.
- `apps/web/lib/state/dockview-panel-actions.ts`,
  `dockview-terminal-panel-actions.ts` — route every single-instance/add
  action through the resolver.
- `apps/web/lib/layout/panel-portal-manager.ts` — `reconcile`/`releaseByEnv`
  accept an exclusion set.
- `apps/web/lib/local-storage.ts` — `DOCKVIEW_ENV_FLOATING_PREFIX`, get/set/
  remove helpers + guard; `cleanupTaskStorage` removes the key.
- Locales: `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn,zh-hk,zh-tw}/task.json`
  — `pinPanel` / `unpinPanel` keys.

### Tests

- `apps/web/lib/state/dockview-floating-store.test.ts` (new) — float/dock
  transitions, placement classifier (default/compact/plan/preview/vscode +
  nested custom + plan-preset side classification + no-center/unknown-center
  custom fallback + **post-apply placement normalization**), materializer
  (missing column, no-center, tree-path insert, tree+flat round-trip with an
  existing nested tree, **`allocateUniqueGroupId` collision cases — saved
  column and live conflicting column**), maximize→float ordering +
  **defer-until-exit maximize-restore ordering (reload + task switch +
  simultaneous maximize + stale floating chat; overlay never mutated)**,
  last-group-in-column re-dock, transaction journal recovery (**digest-based
  matrix: float/dock/restore × both crash directions × all four partial-write
  orderings incl. throw-after-mutation**, throw-on-remove,
  throw-on-materialize, blob-write failure → fail-closed rollback to pinned,
  **recovery-cache isolation (env A then env B, new api instance re-checks)**,
  **`recoverFloatingJournalOnce` precedes every restore branch incl.
  maximize-only and route-intent**, per-env + global size-budget preflight
  (multi-env combined, unrelated sessionStorage usage), timer scheduled
  before transaction start, unload during transaction writes journaled layout
  exactly once, **busy-rejection across all mutator boundaries and all three
  pin surfaces for every phase**), per-panel divergence salvage (one
  conflicting + one non-conflicting tab; nothing disappears), display/active
  setters, persistence round-trip + type-guard rejection (undefined/cyclic/
  oversized params, negative/oversized numerics, duplicate ids, order
  exhaustion at 9 999/10 000/next, stale `nextOrder` normalization preserving
  the high-water mark, root-column geometry bounds), env save/restore,
  reset-as-merge (payload-wins collisions incl. floated ordinary terminal;
  stale session defs dropped; chat placeholder retained;
  valid-session-without-center-column fallback), stacking allocation.
- `apps/web/components/task/dockview-floating-panel.test.tsx` (new) — expanded
  window, owned-region collapse (outside pointer, focusout, portaled Radix
  layer open = owned), pointerdown deferral (outside click while a menu is
  open stays expanded until dismissal; pending collapse cancelled on
  owned-region click; layer closed via unmount/navigation decrements; two
  layers closing in both orders; **close-then-open replacement same-turn and
  same-frame; cross-frame boundary documented**; successor window inherits no
  stale pending collapse), Escape rules (defaultPrevented wins; nested
  AlertDialog/DropdownMenu), vertical/horizontal bar, tablist semantics +
  roving tabindex + Arrow/Home/End, title-click expand+activate, pin re-dock,
  stacking/offsets, empty-group removal, reactive title update on plugin
  re-registration while detached, layer-inventory primitives (one real test
  per Radix family incl. a plugin-panel layer).
- `apps/web/components/task/dockview-group-actions.test.tsx` — pin button
  placement/aria/tooltip/click.
- `apps/web/lib/layout/panel-portal-manager.test.ts` — detach-vs-close:
  registered ids survive removePanel + reconcile (synchronous and delayed
  `fromJSON` removals); unregistered close releases; same id across envs;
  exclusion-set semantics.
- `apps/web/lib/state/dockview-env-switch.test.ts` — floating session
  replacement (single stale, multi-stale winner rule, delayed replacement,
  active-tab rewrite, winner-floats-only suppresses grid insertion — no id in
  both surfaces, winner-in-grid drops floating copy), fast + slow path
  restore; **through the real desktop hook**: `useAutoSessionTab` skips
  ensure for a floating winner (tested via `dockview-desktop-layout`-level
  integration, not only direct `replaceStaleSessionPanels` calls).
- `apps/web/lib/state/dockview-panel-actions.test.ts` — duplicate prevention
  for plan/terminal/preview/review/plugin/session actions with floated panels.
- `apps/web/lib/state/dockview-pinned-enforce.test.ts` — right target not
  applied when all right groups float (one and both floated, toggle-right-
  panels hide→show while floating — floating ids excluded from the re-added
  column — and container resize).
- `apps/web/lib/local-storage.test.ts` — floating key helpers + guard +
  `cleanupTaskStorage` + fail-closed rollback on write failure.
- `apps/web/e2e/tests/task/panel-pin.spec.ts` (new, desktop) — full matrix:
  float/collapse/expand/dock, reload recreation, plan-preset orientation,
  maximize→float, task switch with floated chat, terminal liveness, keyboard
  collapse, portaled-menu collapse suppression, two groups on one edge,
  reset-merge, toggle-right-panels with floated right groups, storage-write
  failure (group stays pinned).

## Dependencies

- Store + persistence + detach registry + materializer (`task-01`) before UI
  (`task-02`, `task-03`).
- `task-03` (floating window + coordinator) before integration (`task-04`).
- E2E last (`task-05`).

## Verification

Per task: targeted unit runs (`cd apps/web && pnpm vitest run <file>`),
`pnpm run i18n:check` / `pnpm run i18n:zh-hant` after locale edits, then the
full gate: `make fmt` → `make typecheck` → `make test` → `make lint`. E2E:
`cd apps/web && pnpm e2e:raw tests/task/panel-pin.spec.ts` (mock profile,
`KANDEV_E2E_MOCK=true`).

## Risks

- **Tombstone TTL (30 s):** single coordinator-owned contract; idle apps
  never accumulate; post-TTL delayed removals are real closes (documented).
- **Active-state authority:** grid-vs-floating generation with atomic handoff;
  stale grid events never win while detached.
- **Untrusted-journal policy:** ONE fail-closed path (invalid/mismatched/
  unexpected all quarantine + repair record + read-only salvage); clear via
  AlertDialog removes quarantine + both keys.
- **Identity precondition on every restore route** (once per restored state).
- **UTF-8 canonical encoding** for digests and budgets.
- **SHA separation:** migration hash synchronous one-time; ordinary commits
  async with precomputed digests; direct dependency + benchmark.
- **Consumer manifest:** machine-readable + legacy-key validator in the
  frontend gate.
- **task-03 depends_on fixed** (includes task-02).
- **Reconciliation diff contract:** native owns payloads, normalized owns
  identity/placement/role, session panels excluded; one live instance per
  panel id; duplicate-prevention tests.
- **Maximize route:** portal-only materialization above the overlay (never
  the pre-max layout); route dispatcher with exactly one fromJSON per
  selected route.
- **Winner readiness barrier:** hook defers ensure while replacement pending;
  atomic move-back if the hook ran first.
- **Normalized-live-layout registry:** keyed by native ids, merged into every
  capture, fail-closed on unmapped objects.
- **Bootstrap callsite table + bypass test.**
- **Tree+flat reset merging** via the shared pure helper.
- **Domain-tagged canonicalization** (kind + role + sorted ids, SHA-256→UUID).
- **`isSerializedDockviewShape`** central guard.
- **CI anchor** (frontend-tests.yml job + exact command).
- **Restore route contracts:** regular v4 = fromJSON once → session
  replacement → normalized reconciliation (session panels excluded); maximize
  = native overlay only, pre-max never applied while maximized; call-order
  tests for every route.
- **Maximize migration:** V3_READ/V4_WRITE prefixes, pre-max-only migration,
  native retained, delete-after-validated-apply.
- **Legacy-localStorage E2E consumers:** both specs named and migrated.
- **One placement key space:** sidecar keyed by `columnLogicalId`;
  `groupLogicalId` on the group state; semantic stable keys for generated
  preset groups.
- **Unload phase table:** mutating/unverified ⇒ BEFORE + aborted journal;
  only verified-AFTER ⇒ AFTER + committed; failures retain the journal.
- **Bootstrap choke point:** `applyLayoutAndBootstrap` wrapper on every apply
  path + defensive bootstrap in enforcement + static callsite test.
- **Envelope discriminator:** own version===4 + dockview + layout.columns +
  identity/role fields; collision fixtures.
- **Migration UUID determinism:** semantic derivation, never traversal;
  crash-before-v4-write + retry tests.
- **vite-env.d.ts declaration** for the hook constant.
- **Old-v3 terminal-only right columns remain no-pinned-right** (normalized
  limitation; custom v4 layouts get explicit roles).
- **AST gate deliverables** (script + fixtures + package script + CI).
- **Layout v4 envelope:** native dockview JSON cannot carry `logicalId`/`role`;
  the versioned envelope + one-time migration + v3 fallback + e2e-helper
  prefix updates must land together or saved layouts break.
- **Sole pair writer:** every persistence path through `persistSettledPair`
  with lock/queue/reject; two writers computing different pairs is the
  failure mode.
- **Migration idempotence:** one-time marker; later restores consume stored
  roles, never re-infer.
- **Role bootstrap ordering:** normalize → rebuild sidecar → derive
  visibility → schedule persistence; first mount with empty blob must not see
  a phantom pinned-right gap.
- **Facade boundary:** only `floatingTransactionFacade` is exported;
  source-boundary test fails on non-facade imports.
- **Nested registry type:** `Map<envId, Map<panelId, token>>` canonical in
  spec/plan/task-01 (no panel-id-keyed contradiction).
- **Custom-layout envelopes:** versioned metadata for `SavedLayoutConfig`.
- **Settle-drain reentrancy:** `drainPendingRestore`/`restoreForFloat` are
  coordinator-owned with internal tokens and recursion guards, or the drain
  is re-rejected by its own busy gate.
- **Quarantine atomicity:** verified copy → verified absence → cache; a
  failed quarantine is never cached as recovered; quarantine keys count
  toward budget and cleanup.
- **Sidecar write amplification:** sidecar updates in memory; blob/journal
  writes only at settled boundaries when bytes changed — ordinary applies
  must never amplify into journal writes.
- **Capability stability:** portal-instance generation, useRef-stable,
  rotated on reacquire, absent on mobile.
- **Stable identity:** logical group/root-column ids are mandatory persisted
  UUIDs — never derived from membership/traversal; duplicate/unknown
  rejection.
- **columnRole:** the persisted role field (not inference) drives
  `hasLivePinnedRightColumn`.
- **Filtering total identity:** unique group ids assigned before filtering;
  undefined/duplicate ids fail closed.
- **Env-qualified exclusions:** predicate over (panelId, entryEnvId, token),
  never a plain id set.
- **Journal integrity:** `isEnvFloatingJournal` + digest recomputation are
  mandatory before any target selection; a corrupt journal must quarantine,
  not loop.
- **Capability transport:** the render-bound capability must reach the plugin
  (props + two-arg method + WeakMap), or ownership validation is impossible.
- **Sidecar-in-blob:** rootColumns inside `EnvFloatingState`, rebuilt on
  every layout apply/reload, invalidated on preset/reset/env switch — never a
  third key.
- **TreePath shape-change mapping:** exact DFS leaf-order clamp rules with
  asserted expected leaves.
- **Identity-preserving filtering:** one traversal, group-id-set equality
  asserted after filtering.
- **Budget-index validation:** entries checked against stored raw
  length/digest before every decision; one bounded rebuild on mismatch.
- **Digest-based recovery:** schema validity can never decide partial-write
  state; per-key digests + the phase marker + all four partial-write
  orderings are the only correct basis.
- **Group-id collisions:** salvage/dock must allocate unique group ids
  (`allocateUniqueGroupId`) or a stale id can overwrite/merge a live group.
- **`hasLivePinnedRightColumn`:** never count unpinned preset side columns;
  plan/preview/vscode must return false and never be hidden as right-panel
  toggles.
- **Busy coverage:** every public layout-mutation boundary and all three pin
  surfaces must observe `isFloatingTransactionBusy`, or a programmatic
  mutation mid-transaction makes the journal snapshot stale.
- **Recovery cache isolation:** keyed by `(envId, transactionId, api)`; a
  global generation would skip env B's journal after env A.
- **Global budget:** per-env caps alone can exhaust the shared quota;
  prefix-scan the owned storage before mutation.
- **Normalization timing:** the post-apply hook must run after session
  insertion with a real-center resolution (membership + index), never
  panel-derived ids or the fabricated fallback.
- **Maximize deferral:** the overlay is the live grid while maximized; only
  the defer-until-exit sequence is implemented.
- **Bidirectional journal consistency:** the operation journal, its recovery
  gate (`recoverFloatingJournalOnce` before EVERY restore entry), and the
  phases are the whole crash story; the journal must be written before
  mutation and cleared only after the chosen pair is persisted and verified.
- **Re-entrancy:** one transaction coordinator with busy-rejection (pin
  disabled); a second transaction mid-phase must be impossible.
- **Single unload handler:** one idempotent transaction-aware handler (never
  a second listener); a duplicated flush can overwrite the journaled layout
  with the mutated grid.
- **Same-frame lease:** pending collapse must survive same-turn and same-frame
  Radix replacements; the cross-frame boundary must be documented, not
  silently claimed.
- **Winner lifecycle:** compare-and-clear consumption, memory-only field,
  stale-winner cleanup on every terminal path without clearing a newer
  generation; written before the auto-session hook in BOTH env-switch and
  maximize-restore branches.
- **Reset session fallback:** valid sessions without a center column must land
  in the first column's first group, or the auto-session hook double-inserts.
- **Portal-safe rollback:** journal recovery must run as a restore-gated,
  exclusion-set transaction or rollback itself releases portals.
- **Empty-right legality:** excluding floating ids must also drop empty groups
  and the empty right column, or the serializer emits an illegal branch that
  corrupts the next restore.
- **Owned-layer inventory:** the tracked callsite table and the concrete host
  API must be delivered (task-03), or a plugin layer collapses the window.
- **Registry consume-once:** consuming a registration for a `fromJSON` removal
  during restore must not break `reconcile`'s post-restore behavior or a float
  whose removal never fires; settle-clear must still run.
- **Persistence guard completeness:** the transaction token must gate
  `persistNow`, the debounce callback, the event handler, AND `beforeunload`,
  and cancel/hold an already-scheduled timer — one missed entry point
  re-exposes the partial-layout persistence the round-4 review found.
- **Session identity:** a floating winner must be visible to
  `shouldSkipPanelEnsure` before the hook's ensure effect, or the incoming
  session id lands in both surfaces; the toggle-show path must not re-add
  floating right ids.
- **Detach registry leaks:** if a registered id is never removed from the grid
  (transaction aborted before removal), the registration must still be cleared
  on settle or the next real close of that tab skips cleanup. Consume-once +
  settle-clear are both required; test the abort path.
- **Materializer divergence:** the delta-LayoutState insert must update both
  `column.tree` and `column.groups` atomically and go through the existing
  serializer/`applyLayoutAndSet`, or dockview's tree invariants silently break
  and `serializePanels` drops the group's definitions.
- **Enforcement gate:** missing the live-pinned-right-column check lets
  `enforcePinnedTargets` resize the center to the right target when all right
  groups float; the gate and its tests are mandatory.
- **Restore ordering:** one missed completion point (e.g. `toggleRightPanels`
  or the initial maximize-only branch) re-exposes the reload blocker; the
  call-site table in the spec is the checklist and each entry has a test.
- **Maximize interplay:** float of the maximized group must derive placement
  from `preMaximizeLayout` and sequence removals so the trailing restore rAF
  does not reassert the overlay.
