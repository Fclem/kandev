---
id: "01-floating-store-state"
title: "Floating-group store, detach registry, materializer, and persistence"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/panel-pin-float.md"
---

# Task 01: Floating-group store, detach registry, materializer, and persistence

## Acceptance

- `useDockviewStore` exposes `floatingGroups: Record<groupLogicalId, FloatingGroupState>` (**and `EnvFloatingState.groups` is logical-keyed too; native groupId is a validated hint/value only**) (**groupId + groupLogicalId**, **columnId + columnLogicalId**, columnIndex, columnKind, **columnRole**, columnPinned, **columnWidth, columnMinWidth, columnMaxWidth**, treePath as a **tagged `{kind: "tree", path} | {kind: "flat", index}`**, edge, orientation, size, panels as full `FloatingPanelDef[]`, activePanelId, display, order) — **these fields are authoritative for identity/placement; role is NOT authoritative here: the validated v4 LayoutState / normalized-live registry is the SOLE writable role authority, and `columnRole`/`rootColumns` role entries in the blob are DENORMALIZED CACHES (registry precedence; cache-corruption test proves role and rightPanelsVisible cannot change from a corrupted cache)** (never native/traversal ids, never membership-inferred role; acceptance tests fail if native ids or membership inference are used) — plus the `identities`/`rootColumns` (keyed by `columnLogicalId`) sidecar, the v4 normalized-layout registry, and actions `floatGroup(groupId)`, `dockGroup(groupId)`, `setFloatingDisplay(groupId, display)`, `setFloatingActivePanel(groupId, panelId)`.
- **Placement classifier:** a pure function over the live `LayoutState` (root column id/index/pinned metadata) plus an **explicit nullable center identity** (`centerColumnId, isCenterKnown` — `findCenterGroupId` fabricates ids when no center exists, so an unknown center classifies as the documented custom fallback, never by promoting an arbitrary side column) classifies each group as side/vertical (non-center root column, edge by index relative to center) or center/horizontal (center column, bottom edge). It MUST NOT use `isCenterCandidateGroupId` as the decision; plan/preview/vscode/compact root columns classify as side/vertical on the right edge. Covered by tests for all five presets, a nested custom layout, and no-center/unknown-center cases.
- **Materializer:** re-dock/restore recreate groups by cloning the live layout, inserting the saved root column at `columnIndex` with pinned/width metadata when absent, inserting the group **atomically into both `column.tree` and `column.groups`** (one mutation helper; `serializeColumn` prefers `tree` while `serializePanels` iterates only `groups`, so a tree-only insert loses panel definitions), and applying through the existing serializer/`applyLayoutAndSet` machinery. `fallbackGroupPosition` is the explicit existing-group fallback only. Docking the last group in an emptied side column recreates the side column, never a center fallback; missing-column, no-center, and tree+flat round-trip cases are tested.
- **Detach registry:** canonical nested type `Map<envId, Map<panelId, token>>`
  (composite `(panelId, envId)` bookkeeping — same panel id in two envs
  holds two records; same-ID live coexistence is impossible since only one
  env is live). `setupPortalCleanup`'s `onDidRemovePanel` decision order: (1) registered composite key with the current token → consume the registration and skip `release`, terminal park/stop, vscode stop, and `handleMaximizeExitOnLastClose`, regardless of `isRestoringLayout`; (2) unregistered while `isRestoringLayout` → return (today's behavior); (3) unregistered otherwise → full cleanup (a user closing a floated tab mid-transaction is an ordinary close). Registrations are armed per expected removal (immediately around each synchronous remove/`fromJSON`, consumed once, drained on operation end). `panelPortalManager.reconcile`/`releaseByEnv` take an env-qualified exclusion predicate over `(panelId, entryEnvId, token)` (the live `PortalEntry` carries `envId`); `saveOutgoingEnv` passes the OUTGOING env. A stale token never clears a newer registration; the env registry clears on settle (success, failure, unmount).
- **Layout persistence is versioned:** the env layout key bumps to **v4**
  with a versioned envelope `{ version: 4, dockview, layout }` (native
  dockview JSON + normalized `LayoutState` carrying `logicalId`/`role`);
  **explicit v3-read/v4-write key constants** (v3 read until v4 written; v3
  deleted only after the validated v4 apply; the envelope `version` field is
  the idempotence marker); - **Dockview API SPIKE (blocking, against pinned `dockview-core ^4.13.1`):**
  document the exact `sv.setConstraints` + `resizeView` same-frame
  sequence used by mid-drag rollback, prove captured widths survive
  narrow/min-max conflict cases, and record the result in the task
  notes before the rollback implementation is accepted.
- **restore applies the SINGLE-`fromJSON`
  BY-CONSTRUCTION contract** — ONE pure native-JSON planning transform builds
  the final native JSON (all fixups/session/role folded in) + the fresh
  normalized native-ID registry BEFORE the single `api.fromJSON`, post-call
  work is observational/rebinding only (never a second fromJSON), and the
  live state is canonical-captured and asserted semantically/byte-equivalent
  to the planned after BEFORE portal adoption or commit (mismatch rolls back
  and replans); `migrateEnvLayoutV3(raw, envId)`
  assigns UUIDs once, persists v4 only after a validated apply, keeps a v3
  reader fallback, and retries a failed apply without minting new UUIDs;
  **the maximize slot uses the same v4 normalized schema** (`MAXIMIZE_V3_READ_PREFIX`/`MAXIMIZE_V4_WRITE_PREFIX`; v3 blobs read on upgrade, only `preMaximizeLayout` migrated to normalized v4, native `maximizedDockviewJson` retained untouched, v3 deleted only after a validated apply + post-exit pre-max restore, with retry/idempotence and malformed/partial-migration behavior defined; the pre-max state is never applied to the live overlay); **migration UUIDs derive from a documented stable semantic identity** (canonical panel-id sets + role, with collision/ambiguity rejection) so a crash before the v4 write cannot mint different ids on retry (crash-before-v4-write and repeated-retry tests); **the legacy
  `dockview-layout-v3` localStorage write is removed** (the v4 env slot is
  the sole layout surface; **the migration checklist enumerates EVERY
  current legacy consumer**: `dockview-layout-setup.ts` (production global
  writer, removed), `local-storage.ts` (v3 prefixes), `e2e/helpers/
  dockview-persistence.ts`, `pane-resize-right.spec.ts`,
  `plan-panel-indicator.spec.ts`, `saved-layout-session-isolation.spec.ts`,
  and `settings/layout-profiles.spec.ts`, plus `dockview-layout-restore.test.ts:388` (obsolete v2 test seed — migrated or removed before the gate), plus
  `sessionless-task-switch.spec.ts` (which WRITES `kandev.dockview.env-layout.<envId>`
  as a test fixture) and `plan-panel-indicator.spec.ts` (reads the legacy
  global localStorage key) — each migrated to the v4 helper or explicitly
  classified as a **test-fixture seed (read-only legacy reads and
  fixture-classified seeds are legal; production code NEVER writes v3/
  localStorage keys)**, and the manifest validator fails on any unclassified
  literal);
  `apps/web/e2e/helpers/dockview-persistence.ts` prefixes + all layout
  consumers are updated together; old-v3-restore, v4 round-trip,
  tree/flat-equality, maximize-slot migration, and e2e-helper-compatibility
  tests. **Placement identity:** `FloatingGroupState.columnLogicalId`
  (and `groupLogicalId` on defs) is the persisted placement identity; the
  native `columnId`/`groupId` are non-authoritative hints.
- **Transactions: one coordinator + digest journal + phase model:** a single coordinator (`floating-transaction.ts`) owns the per-env operation/generation and phase state (`begin → advance → settle`); while busy, **every public layout-mutation boundary** (float/dock/reset/toggle, the add-panel resolver, `buildDefaultLayout`, preset/custom apply, maximize/exit, programmatic actions) **and restore/recovery paths** (`recoverFloatingJournalOnce`, `restoreFloatingAfterLayout` enter the same coordinator — a restore completion during a busy phase is skipped with a retained pending-floating-restore marker, never executed from a stale snapshot) **reject/skip** non-destructively, and all three pin surfaces (grid header, floating header, collapsed bar) render disabled via `isFloatingTransactionBusy(envId)`; re-entrancy tests cover mutation, portals-adopted, rollback, and stale-cleanup phases, per surface and per programmatic action/restore entry. A per-env **operation journal** (`kandev.dockview.env-floating-journal.<envId>`) is written **before** mutation holding `{envId, transactionId, phase, before/after digests, raw before/after strings}`. **Digest protocol:** values are serialized once; digests are SHA-256 (`version: 1`) over the exact raw bytes written to sessionStorage (never a re-stringified parse), with an explicit absent sentinel; journal writes are status-returning and read-back verified, and the write/verify ordering is journal → blob → layout → phase → read-back verify → clear, with a specified phase-write-throw-after-mutation recovery. **`recoverFloatingJournalOnce(api, envId)`** uses digest comparison (four partial-write orderings + **no-op equality = settled**), with a recovery cache keyed by `(envId, transactionId, api instance)` — env-isolated, never one global generation. **Size budgets:** per-env cap (96 KB) and a **global floating allocation budget** (384 KB) enforced through a **validated owned-key index** (built on load/recovery, updated on budget-changing writes — no full prefix scan per toggle); quota races after a passing preflight fail closed via the journal rollback (the backstop). Recovery runs the phase model `mutating → restoring → portals-adopted → persist-recovered → settled`; only the portals-adopted phase persists the journaled layout through **status-returning APIs**; token cleanup is generation-guarded at settled. Journal-free restore is idempotent with **per-panel salvage** and **`allocateUniqueGroupId`** (saved id reused only when absent/owned by the exact group; otherwise `group-floating-<n>` from a dedicated namespace reserved against `api.groups` + same-operation allocations, attempt cap 64 — exhaustion fails non-destructively; saved→live mapping re-derived from saved ids against live ids on every reload). A store-owned transaction token gates every persistence entry point through one `canPersistLayout()`; the unload path is **one idempotent transaction-aware handler** writing the journaled pre-transaction layout. **Single storage policy: fail closed** on every blob-write failure — rollback to pinned; no ephemeral floating mode.
- **Recovery decision matrix + verified writes + journal integrity:**
  `recoverFloatingJournalOnce` uses the phase-aware matrix (both-before →
  verify the **before** pair and clear; partial → apply/verify the **after**
  pair; both-after/equal → settled; the journal clears only after the
  **selected target** is verified, never "always after"); digests are a
  tagged absent/present union over exact raw storage bytes; every write is
  `writeVerified` (set → read back exact bytes → compare; any mismatch is a
  failed write entering rollback; a failed journal write aborts before
  **`isEnvFloatingJournal(journal, envId)` validates version/env/phase/
  transaction/digest/raw shape and recomputes SHA-256 from each raw snapshot
  before any target is selected; an invalid/mismatched journal is
  quarantined through a **verified, idempotent protocol with ONE
  deterministic key `(envId, raw digest)`** (`...-journal.<envId>.corrupt-<digest>`:
  copy → read-back verify → remove original → verify absence; an existing
  deterministic key is verified and the flow proceeds directly to original
  removal — a second copy is never allocated for the same original, so
  crash-after-copy cannot accumulate `.corrupt-<n>` copies across restarts;
  a failed copy/removal keeps the original and is never cached as recovered;
  quarantine keys count toward budget and are removed by bounded
  per-env/task corrupt-key cleanup in `cleanupTaskStorage`; crash-after-copy,
  repeated-restart, and cleanup tests) and treated as unreadable (journal-free
  fallback with the caller's envId). **Skipped restores
  drain via coordinator-owned `drainPendingRestore` (internal token +
  recursion guard, async settle keeping busy through portal adoption;
  float-while-maximized uses `restoreForFloat` the same way)** — recheck
  envId, generation, api instance, marker; clear only after a successful
  settle restore or invalidation; a new begin consumes the retained marker.
  **`EnvFloatingState.rootColumns`** carries root-column metadata (incl.
  `columnRole`) as a DENORMALIZED CACHE inside the blob — the validated v4
  LayoutState / normalized-live registry is the sole writable role
  authority; the materializer resolves roles registry-first and a cache
  copy can never change materialized role or `rightPanelsVisible` (a
  cache-corruption test proves it) — with **coalesced
  persistence** (in-memory updates during layout applies; blob/journal
  written only at settled boundaries when bytes changed; no floating groups
  + unchanged sidecar ⇒ no write). **One sole pair writer:**
  `persistSettledPair(envId, before, after, token)` — a coordinator-owned,
  status-returning pair writer with transaction/generation ownership, exact
  raw before/after snapshots, and a lock/queue/reject policy; every writer
  (debounce timer/unload flush, `persistEnvLayoutNow`, `saveOutgoingEnv`,
  preset/custom/reset apply, float/dock) routes through it and the current
  independent `setEnvLayout`/`persistEnvLayoutNow` writers are replaced;
  race-ordering tests cover scheduled debounce × float begin × individual
  writes × unload in both directions. The coordinator exports ONLY the
  `FloatingTransactionFacade` contract (begin/advance/settle/isBusy/
  persistSettledPair); `drainPendingRestore`/`restoreForFloat` are absent
  from exports (source-boundary test enforces non-facade imports fail).
  **Stable identity:** capture assigns
  persisted UUIDs to every logical group and root column (panel/position
  identity is a validated fallback only); duplicate/unknown identities are
  rejected; sibling/first-panel-change tests prove saved ids survive.
  **Task deletion invalidates the env generation before
  cleanup**, so a late settle can never rewrite deleted keys; the budget
  index validates every indexed key against stored raw length/digest before
  each decision (one bounded rebuild on mismatch).
- **Persistence:** versioned `EnvFloatingState` blob under `kandev.dockview.env-floating.<envId>` with a type guard (mirroring `isEnvMaximizeState`) that validates each panel definition independently (JSON-safe values, params ≤ 64 KB serialized, depth-bounded recursive validation), enforces finite nonnegative numeric bounds (`order`/`nextOrder` ≤ 10 000, `size` ≤ 100 000, `columnIndex` ≤ 64, `columnWidth`/`columnMinWidth`/`columnMaxWidth` ≤ 100 000 with min ≤ width ≤ max when both present), **normalizes `nextOrder` on load to max(raw nextOrder, max(accepted group orders) + 1)** — the persisted high-water counter is preserved even when the highest-order group was removed (no accepted groups never resets to 1); exceeding the cap makes allocation fail non-destructively — rejects duplicate panel/group ids deterministically (first wins), defaults to `{}`; `nextOrder` monotonic counter with a documented **exhaustion policy** (at the cap, a new float fails non-destructively and the group stays pinned — no clamping or reuse); saved on env switch (`saveOutgoingEnv`), restored after layout restore, cleared by reset and `cleanupTaskStorage`.
- Width/height sync (`syncPinnedWidthsFromApi`, `enforcePinnedTargets`) does not re-insert or corrupt pinned widths while a group is floating.

## Verification

```bash
cd apps/web && pnpm vitest run lib/state/dockview-floating-store.test.ts lib/layout/panel-portal-manager.test.ts lib/local-storage.test.ts lib/state/dockview-store.test.ts lib/state/dockview-pinned-enforce.test.ts
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/lib/state/dockview-store.ts` (state, actions, detach registry, transaction token + begin/end, env-switch save/restore, reset-as-merge, maximize interplay, width-sync guard)
- `apps/web/lib/state/floating-transaction.ts` (new: transaction coordinator — per-env operation/generation, begin/advance/settle, busy-rejection, journal write/recovery, phase model)
- `apps/web/lib/state/dockview-floating.ts` (new: placement classifier, materializer, `restoreFloatingAfterLayout`, `recoverFloatingJournalOnce`, journal helpers, `EnvFloatingState` type guard, order allocation, size-budget preflight)
- `apps/web/lib/local-storage.ts` (`DOCKVIEW_ENV_FLOATING_PREFIX`, get/set/remove, `cleanupTaskStorage`)
- `apps/web/components/task/dockview-layout-setup.ts` (detach-registry checks in `setupPortalCleanup`; `canPersistLayout` gate in `setupLayoutPersistence` incl. `beforeunload`)
- `apps/web/lib/layout/panel-portal-manager.ts` (reconcile/releaseByEnv exclusion-set param)
- `apps/web/lib/state/layout-manager/types.ts` + `serializer.ts` (root-column geometry capture/update: pinned/width/min/max retention through apply/restore; `captureRootColumnGeometry`/`applyRootColumnGeometry`)
- `apps/web/package.json` (locked direct hash dependency, e.g. `@noble/hashes/sha256`, + static dependency check; benchmark/threshold acceptance)
- `apps/web/lib/state/dockview-floating-store.test.ts` (new), `apps/web/lib/layout/panel-portal-manager.test.ts` (new), existing store/pinned-enforce/local-storage tests

## Inputs

- Spec: Data model, Placement capture, State machine (transactions), Materializer, Non-destructive removal contract, Failure modes, Persistence guarantees.
- Existing patterns: `buildMaximizeActions` (`dockview-store.ts:934-1000`), `saveOutgoingEnv` (`:779-831`), `isEnvMaximizeState` (`lib/local-storage.ts:380-431`), `fallbackGroupPosition` (`dockview-layout-builders.ts:272`), `findCenterGroupId` (`layout-manager/applier.ts`), `setupPortalCleanup` (`dockview-layout-setup.ts:488-502`), `toggleRightPanels` (`dockview-store.ts:523-567`).

## Dependencies and Risks

- None before this task.
- Risk: detach-registry leak — a registered id never removed (aborted transaction) must still be cleared on settle or the next real close of that tab skips cleanup; test the abort path.
- Risk: materializer must go through the existing serializer/`applyLayoutAndSet`, or dockview's tree invariants silently break.
- Risk: `handleMaximizeExitOnLastClose` runs inside `onDidRemovePanel`; floating the last grid group must not trigger maximize exit.

## Output Contract

Report behavior implemented, files changed, targeted tests run, blockers, residual risks, and update this task plus `plan.md` to done.
