# Floating-panels result matrix (revision 37, committed)

The single machine-readable operation × reason × action matrix required by
`docs/specs/ui/panel-pin-float.md` (Result algebra) and
`docs/plans/panel-pin-float/task-04-integration-lifecycle.md`. One closed
`LayoutMutationResult` union:

```
type LayoutMutationResult =
  | { status: "applied" }
  | { status: "pruned"; reason: "stale-session"; dropped: { id: string; reason: string }[] }
  | { status: "recovered-with-drops"; dropped: { id: string; reason: string }[] }
  | { status: "skipped"; reason: "stale-identity" }
  | { status: "rejected"; reason: RejectedReason; retry: boolean }
  | { status: "suppressed"; reason: SuppressedReason; retry: boolean }
  | { status: "terminal"; reason: TerminalReason; action: "repair-clear" | "export" | "retry" | "cancel" }

RejectedReason = "busy" | "lease-held" | "quota-full" | "settle-timeout" | "invalid-definition"
SuppressedReason = "journal-unavailable" | "quarantine" | "repair-active" | "recovery-pending"
TerminalReason = "quarantine" | "repair-active" | "journal-unavailable"
```

## Matrix

Every row: operation / source state → reason → suppression scope → user
action → terminal? → cleared by. Locale keys: `task:floatingError.<reason>`,
`task:floatingRetry`, `task:floatingCancel`.

| # | Operation / source state | Reason | Suppression scope | User action | Terminal? | Cleared by |
|---|---|---|---|---|---|---|
| 1 | pin / float / dock / reset / toggle / add-panel / preset / maximize while a transaction is mid-phase | `busy` | current env (controls disabled) | disabled control / toast | no | transaction settle |
| 2 | any public mutator while the GLOBAL api lease is held by ANOTHER env | `lease-held` | all envs (mutators rejected) | disabled / toast | no | lease release |
| 3 | float/dock/persist write fails with quota | `quota-full` | current env | retry (free space) | no | retry after space freed |
| 4 | env-switch settle deadline expiry | `settle-timeout` | current env | retry / cancel | yes | user retry or cancel |
| 5 | invalid persisted definition rejected by the closed allow-list | `invalid-definition` | current env | recovered-with-drops result (see 15) | no | definition dropped + result recorded |
| 6 | journal READ error (typed read `unavailable`) | `journal-unavailable` | ALL envs (fail-closed, no materialization) | retry (never blind) | yes (until verified read) | verified journal read succeeds |
| 7 | present invalid/mismatched/unexpected journal | `quarantine` | ALL envs (full suppression) | repair-clear / export (NOT retry) | yes (until cleared) | repair-clear completes (durable `done`) |
| 8 | durable repair record active / clear in progress | `repair-active` | ALL envs (full suppression) | repair-clear progress UI | yes (until cleared) | clear-journal terminal `done` |
| 9 | restore/persist deferred while a pending restore or unload-drain is active | `recovery-pending` | current env (deferred, controls disabled) | none (automatic) | no | drain settles |
| 10 | stale/deleted-session pruning at resolve (revalidation failed) | `stale-session` (pruned) | entry only | none (silent by contract) | yes | — |
| 11 | salvage/materialization drops (invalid def, orphaned) | `recovered-with-drops` | current env | repair UI shows dropped [{id, reason}] | no | result recorded + surviving state persisted |
| 12 | absent/stale enforcement token | `stale-identity` (skipped) | current env (no-op cleanup outcome) | none | yes | — |
| 13 | recovery both-before row (pre-mutation crash) | — (internal) | — | restore before pair, verified, clear journal | no | recovery settle |
| 14 | recovery blob-after/layout-before or layout-after/blob-before | — (internal) | — | apply+verify after pair, clear journal | no | recovery settle |
| 15 | recovery both-after/both-equal with no-op precondition | — (internal) | — | verify equality, clear journal (no write) | no | recovery settle |
| 16 | recovery both-equal but fromJSON/apply WAS invoked | — (internal, guarded by nested rebuild) | — | native snapshot rebuild/rebind BEFORE clear; rebuild failure → row 7/8 | no | rebuild settle or repair |
| 17 | unload: digest-ready, pair unverified | — (internal) | — | write BEFORE pair + `aborted` journal | no | next recovery |
| 18 | unload: afterPairVerified | — (internal) | — | write AFTER pair, `mutating` journal, verify, clear | no | next recovery |
| 19 | maximize t1 (overlay) failure | `busy` / rollback | current env | typed rejected + rollback (grid unchanged) | no | rollback settle |
| 20 | maximize t2 (exit) failure incl. persistent equivalence mismatch | `terminal` (quarantine/repair-active) | ALL envs | repair-clear / export | yes | repair-clear |
| 21 | nested rebuild/rollback failure (partial native mutation) | `terminal` (quarantine/repair-active) | ALL envs | repair-clear / export | yes | repair-clear |
| 22 | reset: forward apply ok, pair write fails, rollback ok | — (internal) | — | rollback applied + typed rejected | no | rollback settle |
| 23 | reset: rollback also fails (budget exhausted) | `terminal` (quarantine/repair-active) | ALL envs | repair-clear / export | yes | repair-clear |
| 24 | custom/RADIX layer second-open rejection (per-open handshake) | `busy` (layer admission) | layer only | requestClose + host close signal | no | layer closes |

## Cross-artifact coverage assertion

- Every recovery-matrix row (13-16), unload-phase row (17-18), maximize
  transaction state (19-20), nested-rebuild state (21), and reset-rollback
  state (22-23) maps to EXACTLY ONE outcome and UI action above.
- Terminal states (6, 7, 8, 20, 21, 23) share ONE clearing mechanism:
  verified journal/repair-clear. No UI path may attempt materialization or
  portal adoption while rows 6-8 are active.
- The exhaustive switch over `LayoutMutationResult` (task-04) is compiled
  with `exhaustive` checks; adding a state without a row here fails CI.
- Locale keys must exist for every reason in `task:floatingError.*` (i18n
  gate; pseudo-locale completeness check).
