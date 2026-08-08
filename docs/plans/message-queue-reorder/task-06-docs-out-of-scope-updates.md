---
id: "06-docs-out-of-scope-updates"
title: "Docs: out-of-scope updates and index"
status: pending
wave: 6
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/message-queue-reorder.md"
---

# Task 06: Docs out-of-scope updates and index

## Acceptance

1. `docs/specs/ui/message-queue-management.md` no longer lists queue
   reordering as out of scope; it links to `message-queue-reorder.md` where
   behavior overlaps (positions, provenance, reserved rows).
2. `docs/specs/ui/message-queue-send-now.md` replaces its "Reordering queued
   messages before sending" out-of-scope line with a link to the new spec
   (bulk Send Now still dispatches in current FIFO order — now the reordered
   order).
3. `docs/specs/INDEX.md` lists the new spec under `ui/` with status `draft`.

## Verification

```bash
git diff --check
```

Manual review: links resolve, no other spec/ADR references queue reordering
as unavailable.

## Files likely touched

- `docs/specs/ui/message-queue-management.md`
- `docs/specs/ui/message-queue-send-now.md`
- `docs/specs/INDEX.md`

## Dependencies

None (may run any time after the spec lands; listed last so the shipped
specs are updated after the feature is proven).

## Parallelism

Sequential.

## Inputs

- Spec: `## What` (positions, provenance, reserved rows), `## Out of Scope`.
- `docs/specs/INDEX.md` conventions (`ui/` flat-file layout, status column).

## Output contract

Summary, files changed, blockers, risks; update task + plan statuses in the
same conversation.

## Results

Pending.
