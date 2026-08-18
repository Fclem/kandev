# Floating-panels result matrix (revision 50, committed)

The single machine-readable operation × reason × action matrix required by
`docs/specs/ui/panel-pin-float.md` (Result algebra) and
`docs/plans/panel-pin-float/task-04-integration-lifecycle.md`.

## Canonical schema (ONE vocabulary, disjoint categories, every row complete)

**`ResultStatus`** (the outcome): `applied | pruned | recovered-with-drops |
skipped | rejected | suppressed | terminal`.

**`OperationReason`** (why; EVERY row carries EXACTLY ONE — automatic
internal transitions use `automatic`):

```
busy | lease-held | quota-full | settle-timeout | invalid-definition |
stale-session | stale-identity | recovery-pending | journal-unavailable |
quarantine | repair-active | recovered-with-drops | apply-failed |
portal-failed | plugin-contract-failure | stale-capability | intent-cancelled
| automatic
```

- `stale-session` is the reason for status `pruned` (silent outcome; locale
  key `task:floatingError.pruned` for the repair catalog only).
- `recovered-with-drops` is BOTH a status and a reason (salvage outcome).
- `automatic` = internal success/no-reason transitions: NO locale key, NO
  user action.
- `stale-identity` = silent skip: debug-only key, no user action.
- `intent-cancelled` = env-switch cancellation: debug-only telemetry label,
  no surfaced key, NO retry UI.
- Locale keys: `task:floatingError.<localeSuffix>` with camelCase suffix for
  every surfaced reason: busy, leaseHeld, quotaFull, settleTimeout,
  invalidDefinition, journalUnavailable, quarantine, repairActive,
  recoveryPending, recoveredWithDrops, applyFailed, portalFailed,
  pluginContractFailure, **staleCapability**, pruned. `automatic`,
  `stale-session`, `stale-identity`, `intent-cancelled` are EXCLUDED.
  Plus `task:floatingRetry`, `task:floatingCancel`. Enumeration, row count,
  and locale list are GENERATED from this file by the locale gate.

**Invariant: every operation state maps to EXACTLY ONE (status, reason,
action) row — every row has a reason (never `—`).** A compile-time/test
assertion iterates the CLOSED operation-state union (below) and fails if
any state is covered by zero or two rows. Row count is the PARSED number of
table entries (41); the generator never trusts headings.

## Matrix (41 rows; count parsed from the table, never a heading)

| # | Operation / source state | Status | Reason | Suppression scope | User action | Terminal? | Cleared by |
|---|---|---|---|---|---|---|---|
| 1 | pin / float / dock / toggle / add-panel / preset / maximize while a transaction is mid-phase | rejected | busy | current env (controls disabled) | disabled control / toast | no | transaction settle |
| 2 | any public mutator while the GLOBAL api lease is held by ANOTHER env | rejected | lease-held | all envs (mutators rejected) | disabled / toast | no | global lease release |
| 3 | float/dock/persist write fails with quota | rejected | quota-full | current env | retry (free space) | no | retry after space freed |
| 4 | env-switch settle deadline expiry | terminal | settle-timeout | current env | retry / cancel | yes | user retry or cancel |
| 5 | invalid persisted definition rejected by the closed allow-list | recovered-with-drops | invalid-definition | current env | repair UI shows dropped list | no | result recorded + surviving state persisted |
| 6 | journal READ error (typed read `unavailable`) | suppressed | journal-unavailable | ALL envs (fail-closed, no materialization) | retry (never blind) | no (transient) | verified journal read succeeds |
| 7 | present invalid/mismatched/unexpected journal | terminal | quarantine | ALL envs (full suppression) | repair-clear / export (NOT retry) | yes | repair-clear completes (durable `done`) |
| 8 | durable repair record active / clear in progress | terminal | repair-active | ALL envs (full suppression) | repair-clear progress UI | yes | clear-journal terminal `done` |
| 9 | restore/persist deferred while a pending restore or unload-drain is active | suppressed | recovery-pending | current env (deferred, controls disabled) | none (automatic) | no | drain settles |
| 10 | stale/deleted-session pruning at resolve (revalidation failed) | pruned | stale-session | entry only | none (silent by contract) | yes (final outcome, NOT `terminal` status) | — |
| 11 | salvage/materialization drops (invalid def, orphaned) | recovered-with-drops | recovered-with-drops | current env | repair UI shows dropped [{id, reason}] | no | result recorded + surviving state persisted |
| 12 | absent/stale enforcement token | skipped | stale-identity | current env (no-op cleanup outcome) | none | yes (final outcome, NOT `terminal` status) | — |
| 13 | recovery both-before row (pre-mutation crash), restore ok | applied | automatic | current env | none (automatic restore) | no | recovery settle |
| 14 | recovery blob-after/layout-before or layout-after/blob-before, apply ok | applied | automatic | current env | none (automatic) | no | recovery settle |
| 15 | recovery both-after/both-equal, no-op precondition holds (no native mutation invoked) | applied | automatic | current env | none (equality verified, journal cleared) | no | recovery settle |
| 16 | recovery both-equal BUT fromJSON/apply WAS invoked — native snapshot rebuild succeeds | applied | automatic | current env | none (native rebuild + verify) | no | rebuild settle |
| 17 | recovery both-equal BUT fromJSON/apply WAS invoked — native rebuild FAILS | terminal | quarantine | ALL envs | repair-clear / export | yes | repair-clear |
| 18 | unload: hash-pending before digest-ready | applied | automatic | current env | write cached BEFORE pair + `aborted` journal | no | next recovery |
| 19 | unload: afterPairVerified | applied | automatic | current env | write AFTER pair, verify, clear journal | no | next recovery |
| 20 | maximize t1 (overlay) apply fails after partial mutation — rollback ok | rejected | apply-failed | current env | typed rejected + rollback (grid restored) | no | rollback settle |
| 21 | maximize t2 (exit) equivalence retry ok | applied | automatic | current env | retry re-plans + re-applies (attempt 2) | no | retry settle |
| 22 | maximize t2 (exit) persistent failure | terminal | quarantine | ALL envs | repair-clear / export | yes | repair-clear |
| 23 | nested rebuild/rollback failure (partial native mutation) | terminal | quarantine | ALL envs | repair-clear / export | yes | repair-clear |
| 24 | reset: forward apply ok, pair write fails, rollback ok | rejected | quota-full | current env | rollback applied + typed rejected | no | rollback settle |
| 25 | reset: rollback also fails (budget exhausted) | terminal | quarantine | ALL envs | repair-clear / export | yes | repair-clear |
| 26 | custom/RADIX layer duplicate-open rejection (per-open handshake) | rejected | busy | layer only | requestClose + host close signal | no | layer closes (real open=false ack) |
| 27 | outer `settle` called while a nested transaction is active | rejected | busy | current env | nested settle first | no | nested settle |
| 28 | portal adoption/lease failure — PREFLIGHT (nativeMutationStarted false + snapshot unchanged) | rejected | portal-failed | current env | RETRY ONLY (no durable repair state exists — repair-clear impossible unless a repair record was explicitly created) | no | retry |
| 29 | portal adoption/lease failure — POST-partial adoption (native mutation invoked) | terminal | portal-failed | ALL envs | quarantine + repair-clear / export (fail-closed: lease invalidated, fresh validated rebuild) | yes | repair-clear |
| 30 | plugin layer noncompliance (requestClose ignored / ack timeout) | terminal | plugin-contract-failure | layer only | revoke the OPEN's capability + typed failure UI | yes | open capability revoked (next open receives a FRESH per-open capability) |
| 31 | env-switch retry at settle succeeds | applied | automatic | current env | none (deferred switch applied) | no | settle |
| 32 | env-switch retry at settle still busy (second failure) | terminal | settle-timeout | current env | retry / cancel (one-shot terminal) | yes | user retry or cancel |
| 33 | quarantine copy or clear fails (copy/remove error, retry retained) | suppressed | repair-active | ALL envs | retry clear (never cached recovered) | no | verified copy/absence or repair-clear |
| 34 | journal-free divergence recovery on VERIFIED ABSENT journal, restore ok | applied | automatic | current env | none (automatic) | no | restore settle |
| 35 | journal-free divergence salvage with drops | recovered-with-drops | recovered-with-drops | current env | repair UI shows dropped list | no | result recorded + state persisted |
| 36 | NORMAL SUCCESS transitions (successful float/dock/pin/toggle/preset apply) | applied | automatic | current env | none (SILENT for user-initiated; returned to awaited/programmatic callers) | no | operation completed |
| 37 | reset forward native apply/fromJSON PARTIAL failure (before pair persistence), rollback ok | rejected | apply-failed | current env | typed rejected + rollback (grid restored) | no | rollback settle |
| 38 | maximize t1 ROLLBACK failure | terminal | quarantine | ALL envs | repair-clear / export | yes | repair-clear |
| 39 | recovery selected-target write/verify failure (journal retained, retry) | suppressed | repair-active | ALL envs | retry (never cached recovered) | no | verified target write/absence |
| 40 | env-switch intent cancelled (user cancel / unmount / target-deleted) | skipped | intent-cancelled | current env | none | yes (final outcome, NOT `terminal` status) | — |
| 41 | plugin uses a REVOKED cached capability (stale props) | rejected | stale-capability | layer only | synchronous no-side-effect rejection — re-read via requestCapability (may return pending-reissue) + retry with the fresh generation (NO second revocation, NO second timer) | no | requestCapability re-read (fresh generation) + retry |

## CLOSED operation-state union (for the exactly-once assertion)

**Scope rule: NORMAL SUCCESS transitions that are NOT LayoutMutationResult
values are admitted explicitly** — pinned→floating-expanded, floating
collapse/expand, and NORMAL plugin open/close are state transitions, NOT
results: they have no row by definition (the union's every-member maps
once; these members are declared result-free and the assertion verifies
that exclusion).

**Discriminator domains are SEPARATE (admission vs lifecycle):**

- `PluginLayerAdmission = first-open-admitted | duplicate-open-rejected |
  revoked-capability-at-admission` — the ADMISSION-CHECK RESULT is the
  observable host-side transition: first-open-admitted → NON-RESULT
  (excluded); duplicate-open-rejected → row 26, and its subsequent real
  open=false acknowledgement is `rejected-duplicate-close-ack`
  (NON-RESULT, distinct from lifecycle close-ack);
  revoked-capability-at-admission (a revoked/stale capability used at
  ANY admission, before OR after it ever admitted an open) → row 41
  (pre-admission reachability is EXPLICIT).
- `PluginLayerLifecycle = open-owned | close-ack | ack-timeout` — edges:
  open-owned (after a successful admission) → close-ack is NON-RESULT
  (close-ack IS the handshake's actual open=false callback); open-owned →
  ack-timeout ONLY when the plugin ignores requestClose (row 30); ack-
  timeout is NOT reachable from first-open-admitted directly.
- `FloatTransition = result-bearing-commit | result-free-visual-state` —
  the commit returns `applied` (row 36); the visual expand/collapse is a
  NON-result.
- Host-only generator fixture: two roots + an intentionally uncontrolled
  plugin; the generator iterates the discriminator values (edges above)
  and rejects a state covered by zero or two rows.

1. Public mutators: pin, float, dock, reset, toggle, add-panel, preset,
   maximize t1, maximize t2 exit, env-switch, plugin layer open.
2. Recovery matrix rows: both-before (13), blob-after/layout-before (14),
   layout-after/blob-before (14), both-after/equal-noop (15),
   both-equal-after-invoke success (16) / failure (17), unexpected (7),
   journal-read-unavailable (6), journal VERIFIED absent → divergence
   (34, 35), quarantine-copy/clear failure (33).
3. Unload phases: hash-pending pre-digest (18), digest-ready (18),
   afterPairVerified (19), normal completion (19).
4. Maximize: t1 apply failure (20), t1 rollback failure (38), t2 retry
   (21), t2 persistent (22).
5. Nested rebuild: each nested phase failure → 23; nested-active outer
   settle → 27.
6. Reset rollback: rollback ok (24), rollback exhausted (25), forward
   apply partial failure (37).
7. Suppression states: journal-unavailable (6), quarantine (7),
   repair-active (8), recovery-pending (9).
8. Env-switch branches: deferred-then-retry-success (31), retry-busy
   second failure (32), user-cancel/unmount/target-deleted (40).
9. Portal: preflight failure (28), post-partial-adoption failure (29).
10. Plugin layers (discriminated per the domains above): duplicate-open-
    rejected → 26; ack-timeout → 30; stale-capability → 41;
    first-open-admitted and close-ack → NON-RESULT (excluded).
11. Prune/salvage/allow-list: invalid-definition (5), stale-session (10),
    salvage drops (11, 35), stale-identity skip (12).
12. Normal success (result-free, excluded): pinned→floating-expanded,
    floating collapse/expand, normal plugin open/close, close-ack —
    declared non-results; the assertion checks they are NOT covered by
    any row; successful float/dock/pin ARE result-bearing (row 36).

## Cross-artifact coverage assertion

- The spec's reason enum (`docs/specs/ui/panel-pin-float.md`, Result
  algebra) MUST list exactly the OperationReason values above, INCLUDING
  `stale-capability` (generated from this file).
- TERMINAL semantics: `terminal` STATUS = durable/quarantine-class outcome
  requiring explicit clearing (repair-clear, user retry/cancel, or
  capability revocation). FINAL OUTCOMES (`pruned`, `skipped`,
  `intent-cancelled`) are terminal-in-effect but NOT `terminal` status.
  Generated assertion: `status == "terminal"` ⇒ `Terminal? == yes`
  (one-way implication — `pruned`/`skipped` rows legitimately have
  `Terminal? == yes` without `terminal` status; plan/task acceptance
  MUST use this one-way form — a two-way IFF would reject rows 10/12/40). No UI path may attempt
  materialization or portal adoption while rows 6-8 are active.
- The exhaustive switch over `LayoutMutationResult` (task-04) is compiled
  with `exhaustive` checks and the exactly-once assertion over the CLOSED
  union above (incl. discriminator edges); adding a state without a row
  fails CI.
- Row count (41 = parsed table entries) and the locale-key list are
  GENERATED from this file; a validator asserts the generated count against
  EVERY cross-artifact declaration (spec/plan/task prose never hand-copies
  a count).
- Locale keys must exist for every surfaced reason in `task:floatingError.*`
  (i18n gate; pseudo-locale completeness check); `automatic` /
  `stale-session` / `stale-identity` / `intent-cancelled` are excluded;
  `staleCapability` IS included.
