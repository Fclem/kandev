---
id: "08-docs-and-promotion"
title: "Documentation and promotion"
status: done
wave: 9
depends_on:
  - "10-provider-limit-metrics"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-001
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-002
acceptance_criteria:
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.2
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.7
system_design:
  - ../../specs/agents/system-design/provider-limit-recovery.md
---

# Task 08: Documentation and Promotion

## Summary

Publish the user-facing behavior and the new metrics, then promote the specs
after every task passes.

## In scope

- `/docs-maintainer` update of the public agent profile and recovery pages
  under `docs/public/**`, covering both switches, waits, cancellation, and
  manual override.
- An `AGENTS.md` Observability entry for the `provider_limit_*` counters.
- Promotion of the requirement to `active` and the design to `current`. Set
  the plan to `implemented` with recorded results.

## Out of scope

- Behavior changes.

## Acceptance

1. Public docs describe both switches, the seven-day bound, account versus
   model limits, and how to cancel a wait.
2. `AGENTS.md` lists the counters and their closed label sets.
3. Specification and plan statuses match the delivered implementation.

## Verification

Run this complete block from the repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/provider-limit-recovery
```

## Files likely touched

- `docs/public/**` (agent profile and recovery pages)
- `AGENTS.md`
- `docs/specs/agents/{requirements,system-design}/provider-limit-recovery.md`
- `docs/plans/provider-limit-recovery/*.md`

## Dependencies

Task 07.

## Risks

None.

## Parallelism

`sequential`

## Inputs

- Delivered behavior from Tasks 01-07.

## Results

Updated `agents-and-profiles.md` with the per-profile switches, binding/model
limit scope, seven-day trusted-reset limit, fallback eligibility, wait limit,
and manual override. Updated `sessions-and-review.md` with task-session wait
cancellation and prompt behavior. Added all three counters, closed labels, and
identifier exclusions to the `AGENTS.md` Observability section.

Promoted the requirement to `active`, the system design to `current`, and the
plan to `implemented`. All work orders are checked complete.

Validation passed:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```
