# Floating-panels owned-key budget table (revision 50, committed)

The SINGLE source of truth for floating-panels storage accounting. Consumed
by the budget validator (`scripts/validate-floating-budget.mjs`) and its
tests; the spec/plan prose references THIS table and contains no duplicate
budget decisions. Rows are machine-parsed (strict markdown table).

Per-env cap: 96 KB default. Global cap: 384 KB default (all envs). Recovery
allowance: reserved ATOMICALLY across environments before any mutation
(proportional: 2 × exact raw journal bytes + repair record + clear journal +
cleanup record + fixed overhead per env).

| Key | Per-env cap inclusion | Global cap inclusion | Raw-byte accounting | Counts when |
|---|---|---|---|---|
| `env-floating.<enc>` | yes | yes | exact stored string UTF-8 | always (owned) |
| `env-floating-journal.<enc>` | yes | yes | exact raw journal string (incl. raw snapshots) | always (owned) |
| `.corrupt-<digest>` (quarantine) | yes (inside recovery allowance) | yes (inside recovery allowance) | exact stored string | always (owned) |
| `env-repair.<enc>` | yes (recovery allowance) | yes (recovery allowance) | exact stored string | always (owned) |
| `env-repair-clear.<enc>` | yes (recovery allowance) | yes (recovery allowance) | exact stored string | always (owned) |
| `env-floating-cleanup.<enc>` | yes (recovery allowance) | yes (recovery allowance) | exact stored string | always (owned) |
| `env-layout-v4.<enc>` | CONDITIONAL | CONDITIONAL | exact stored string | ONLY when the env has ANY floating state (blob or journal present); a ZERO-FLOATING env's layout bytes NEVER count against the floating cap — unrelated layout persistence can never be blocked by the floating allocation |
| `env-maximize-v4.<enc>` | CONDITIONAL | CONDITIONAL | exact stored string | ONLY when the env has floating state OR a pending maximize envelope for a floating group; a pure grid maximize with no floating involvement never counts |
| `env-layout-v3.<enc>` (read/fallback) | yes (transient, migration window only) | yes (transient) | exact stored string | ONLY while the v3→v4 migration is incomplete (retained until validated v4 apply); counted so a failed migration cannot hide legacy bytes |
| `env-maximize-v3.<enc>` (read/fallback) | yes (transient) | yes (transient) | exact stored string | ONLY while migration is incomplete |

## Rules (validator-enforced)

1. Preflight (float/dock/persist) computes the post-write totals under the
   table above; a single interpretation is used BEFORE and AFTER mutation.
2. The recovery allowance is reserved before any normal write and can never
   be consumed by normal (non-recovery) bytes.
3. Conditional keys (`layout-v4`/`maximize-v4`) are included ONLY per the
   "Counts when" column — a zero-floating env is never blocked.
4. Transient v3 keys are included only during the migration window; after a
   validated v4 apply they are deleted (cleanup matrix) and excluded.
5. Every owned key (including `env-floating-cleanup`) is byte-accounted with
   its exact stored string; no estimates.
6. The validator derives expected totals from this table and fails on any
   divergence between preflight and post-mutation accounting.
