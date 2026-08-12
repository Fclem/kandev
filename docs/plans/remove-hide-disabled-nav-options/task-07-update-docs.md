---
id: "07-update-docs"
title: "Update superseded specs and index"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/remove-hide-disabled-nav-options.md"
---

# Task 07: Update superseded specs and index

Bring the two superseded feature specs and the specs index in line with
the removal. The new removal spec
(`docs/specs/ui/remove-hide-disabled-nav-options.md`) is already written.

- **Acceptance:**
  1. `docs/specs/integrations/enable-disable-toggle.md` no longer
     describes the "Hide disabled integrations from left panel
     navigation" setting (What bullets, Data-model row for
     `kandev:integrations:hideDisabledInNav:v1`, the
     `use-hide-disabled-integrations-in-nav.ts` API bullet, the
     hide-disabled Failure-mode paragraph, the nav-visibility Scenarios,
     and the Out-of-scope references), and carries a short note pointing
     at the removal spec. The per-integration toggle contract remains
     intact; status stays `shipped`.
  2. `docs/specs/agents/hide-disabled-profiles-nav.md` has
     `status: archived` in its frontmatter with a one-line note linking
     the removal spec.
  3. `docs/specs/INDEX.md` adds
     `[remove-hide-disabled-nav-options](ui/remove-hide-disabled-nav-options.md)`
     under the `ui/` umbrella; the `enable-disable-toggle` row keeps
     status `shipped`.
- **Verification:**
  ```bash
  git diff --check
  ```
- **Files likely touched:**
  - `docs/specs/integrations/enable-disable-toggle.md`
  - `docs/specs/agents/hide-disabled-profiles-nav.md`
  - `docs/specs/INDEX.md`
- **Dependencies:** None.
- **Parallelism:** parallel-safe (disjoint files).

## Change

1. Edit `enable-disable-toggle.md` per acceptance 1. Historical
   `docs/plans/integrations-enable-disable-toggle/**` transcripts are
   implementation records — leave them.
2. Archive `hide-disabled-profiles-nav.md` per acceptance 2.
3. Update `docs/specs/INDEX.md` per acceptance 3.

## Inputs

- Spec: the removal spec itself (the superseding record).
- Plan: Tests (docs task).

## Output contract

Summary, files changed, exact commands run and outcomes, blockers/risks,
task/plan status update in the same conversation.

## Results

- `docs/specs/integrations/enable-disable-toggle.md` — nav-visibility setting content removed (What/Data model/API/Failure modes/Scenarios/Out of scope), title narrowed to "Integration Enable/Disable Toggle", removal note links the new spec. Status stays `shipped`.
- `docs/specs/agents/hide-disabled-profiles-nav.md` — `status: archived` + removal note.
- `docs/specs/INDEX.md` — added `remove-hide-disabled-nav-options` under `ui/`.
- `git diff --check` — clean.
