# Floating-panels result matrix (revision 40, committed)

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
portal-failed | plugin-contract-failure | automatic
```

- `stale-session` is the reason for status `pruned` (silent outcome; locale
  key `task:floatingError.pruned` for the repair catalog only).
- `recovered-with-drops` is BOTH a status and a reason (salvage outcome).
- `automatic` = internal success/no-reason transitions (recovery restores,
  unload writes, normal settle): NO locale key, NO user action.
- `stale-identity` = silent skip: debug-only key, no user action.
- `apply-failed` = a native apply/fromJSON failure after partial mutation
  (distinct from quota/busy); `portal-failed` = portal adoption/lease
  failure; `plugin-contract-failure` = plugin layer noncompliance
  (requestClose ignored / ack timeout).
- Locale keys: `task:floatingError.<localeSuffix>` where localeSuffix is the
  camelCase of the reason (busy, leaseHeld, quotaFull, settleTimeout,
  invalidDefinition, journalUnavailable, quarantine, repairActive,
  recoveryPending, recoveredWithDrops, applyFailed, portalFailed,
  pluginContractFailure, pruned) plus `task:floatingRetry`,
  `task:floatingCancel`. `automatic`, `stale-session`, `stale-identity`
  have NO surfaced locale key. Locale enumeration and row count are
  GENERATED from this file by the locale gate.

**Invariant: every operation state maps to EXACTLY ONE (status, reason,
action) row — every row has a reason (never `—`).** A compile-time/test
assertion iterates the CLOSED operation-state union (below) and fails if
any state is covered by zero or two rows.

## Matrix (27 rows; count generated from this file)

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
| 10 | stale/deleted-session pruning at resolve (revalidation failed) | pruned | stale-session | entry only | none (silent by contract) | yes | — |
| 11 | salvage/materialization drops (invalid def, orphaned) | recovered-with-drops | recovered-with-drops | current env | repair UI shows dropped [{id, reason}] | no | result recorded + surviving state persisted |
| 12 | absent/stale enforcement token | skipped | stale-identity | current env (no-op cleanup outcome) | none | yes | — |
| 13 | recovery both-before row (pre-mutation crash), restore ok | applied | automatic | current env | none (automatic restore) | no | recovery settle |
| 14 | recovery blob-after/layout-before or layout-after/blob-before, apply ok | applied | automatic | current env | none (automatic) | no | recovery settle |
| 15 | recovery both-after/both-equal, no-op precondition holds (no native mutation invoked) | applied | automatic | current env | none (equality verified, journal cleared) | no | recovery settle |
| 16a | recovery both-equal BUT fromJSON/apply WAS invoked — native snapshot rebuild succeeds | applied | automatic | current env | none (native rebuild + verify) | no | rebuild settle |
| 16b | recovery both-equal BUT fromJSON/apply WAS invoked — native rebuild FAILS | terminal | quarantine | ALL envs | repair-clear / export | yes | repair-clear |
| 17 | unload: hash-pending before digest-ready | applied | automatic | current env | write cached BEFORE pair + `aborted` journal | no | next recovery |
| 18 | unload: afterPairVerified | applied | automatic | current env | write AFTER pair, verify, clear journal | no | next recovery |
| 19 | maximize t1 (overlay) apply fails after partial mutation — rollback ok | rejected | apply-failed | current env | typed rejected + rollback (grid restored) | no | rollback settle |
| 20a | maximize t2 (exit) equivalence retry ok | applied | automatic | current env | retry re-plans + re-applies (attempt 2) | no | retry settle |
| 20b | maximize t2 (exit) persistent failure | terminal | quarantine | ALL envs | repair-clear / export | yes | repair-clear |
| 21 | nested rebuild/rollback failure (partial native mutation) | terminal | quarantine | ALL envs | repair-clear / export | yes | repair-clear |
| 22 | reset: forward apply ok, pair write fails, rollback ok | rejected | quota-full | current env | rollback applied + typed rejected | no | rollback settle |
| 23 | reset: rollback also fails (budget exhausted) | terminal | quarantine | ALL envs | repair-clear / export | yes | repair-clear |
| 24 | custom/RADIX layer second-open rejection (per-open handshake) | rejected | busy | layer only | requestClose + host close signal | no | layer closes (real open=false ack) |
| 25 | outer `settle` called while a nested transaction is active | rejected | busy | current env | nested settle first | no | nested settle |
| 26 | portal adoption/lease failure (portal-failed) | terminal | portal-failed | current env | retry / repair UI | no | retry or repair-clear |
| 27 | plugin layer noncompliance (requestClose ignored / ack timeout) | terminal | plugin-contract-failure | layer only | revoke capability + typed failure UI | yes | capability revoked + layer unregistered |
| 28 | env-switch retry at settle succeeds | applied | automatic | current env | none (deferred switch applied) | no | settle |
| 29 | env-switch retry at settle still busy (second failure) | terminal | settle-timeout | current env | retry / cancel (one-shot terminal) | yes | user retry or cancel |
| 30 | env-switch intent cancelled (user cancel / unmount / target-deleted) | skipped | stale-identity | current env | none | yes | — |
| 31 | quarantine copy or clear fails (copy/remove error, retry retained) | suppressed | repair-active | ALL envs | retry clear (never cached recovered) | no | verified copy/absence or repair-clear |
| 32 | journal-free divergence recovery on VERIFIED ABSENT journal, restore ok | applied | automatic | current env | none (automatic) | no | restore settle |
| 33 | journal-free divergence salvage with drops | recovered-with-drops | recovered-with-drops | current env | repair UI shows dropped list | no | result recorded + state persisted |

## CLOSED operation-state union (for the exactly-once assertion)

Every state the coordinator, recovery, unload, suppression, maximize,
nested, reset, env-switch, portal, and plugin machines can reach — each maps
to exactly one row above:

1. Public mutators: pin, float, dock, reset, toggle, add-panel, preset,
   maximize t1, maximize t2 exit, env-switch, plugin layer open.
2. Recovery matrix rows: both-before (13), blob-after/layout-before (14),
   layout-after/blob-before (14), both-after/equal-noop (15),
   both-equal-after-invoke success (16a) / failure (16b), unexpected (7),
   journal-read-unavailable (6), journal VERIFIED absent → divergence
   (32, 33), quarantine-copy/clear failure (31).
3. Unload phases: hash-pending pre-digest (17), digest-ready (17),
   afterPairVerified (18), normal completion (18).
4. Maximize: t1 apply failure (19), t2 retry (20a), t2 persistent (20b).
5. Nested rebuild: each nested phase failure → 21; nested-active outer
   settle → 25.
6. Reset rollback: rollback ok (22), rollback exhausted (23).
7. Suppression states: journal-unavailable (6), quarantine (7),
   repair-active (8), recovery-pending (9).
8. Env-switch branches: deferred-then-retry-success (28), retry-busy
   second failure (29), user-cancel/unmount/target-deleted (30).
9. Portal: adoption/lease failure (26).
10. Plugin layers: second-open rejection (24), noncompliance ack-timeout
    (27), normal open/close (no row — admitted, not a result).
11. Prune/salvage/allow-list: invalid-definition (5), stale-session (10),
    salvage drops (11, 33), stale-identity skip (12).

## Cross-artifact coverage assertion

- The spec's reason enum (`docs/specs/ui/panel-pin-float.md`, Result
  algebra) MUST list exactly the OperationReason values above (generated).
- TERMINAL semantics: `terminal` = the current operation's outcome is final
  (no auto-retry); clearing mechanism is per-row (quarantine/repair-active
  clear ONLY via verified journal/repair-clear; settle-timeout via user
  retry/cancel; plugin-contract-failure via capability revocation). No UI
  path may attempt materialization or portal adoption while rows 6-8 are
  active.
- The exhaustive switch over `LayoutMutationResult` (task-04) is compiled
  with `exhaustive` checks and the exactly-once assertion over the closed
  union above; adding a state without a row fails CI.
- Row count (33) and the locale-key list are GENERATED from this file; the
  spec/plan "24 rows" prose is replaced by "rows generated from
  result-matrix.md".
- Locale keys must exist for every surfaced reason in `task:floatingError.*`
  (i18n gate; pseudo-locale completeness check); `automatic`/
  `stale-session`/`stale-identity` are excluded from the key generation.
