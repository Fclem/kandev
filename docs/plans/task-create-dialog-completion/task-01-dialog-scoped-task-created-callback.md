---
id: "01-dialog-scoped-task-created-callback"
title: "Add the dialog-scoped task-created callback"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-VOICE-EXTRACTION-HOST-001
acceptance_criteria:
  - AC-PLUGINS-VOICE-EXTRACTION-HOST-001.9
system_design:
  - ../../specs/plugins/system-design/task-create-dialog-completion.md
---

# Task 01: Add the dialog-scoped task-created callback

## Summary

Provide a task-created callback to create-mode task-create plugin contributions. Keep notification tied to the owning dialog's successful create result and pass the created task identity.

## In scope

- The dialog-owned handler registry and scoped context.
- The public SDK and composer-slot callback types, with capability injection only for create-mode task-create contributions.
- Isolated sync/async handler failure logging and callback cleanup on unmount.
- Regression coverage for successful create, failure, dialog isolation, and non-create surfaces.

## Out of scope

- Global task lifecycle event changes.
- Edit and new-session completion callbacks.
- Plugin-specific tag persistence or task update behavior.

## Acceptance

1. The callback receives the exact task identity created by its owning dialog, once after successful creation.
2. Failed or canceled creation and unrelated task creation do not notify the callback.
3. Unmounting a contribution unregisters its callback; edit and new-session slots do not receive the create-only capability.
4. The public SDK types synchronous and asynchronous handlers; each failure is logged without blocking other handlers or changing task creation.

## Review remediation results

The acceptance criteria now include the public SDK/slot contract, async handler
failure isolation, and edit-mode capability exclusion. Verification results:

- Targeted task-create dialog tests — passed (40 tests).
- Web typecheck — passed.
- ESLint on changed web TypeScript files, including the plugin registry adapter — passed.
- SDK typecheck, including the typed composer-slot consumer fixture — passed.
- SDK runtime tests — passed (2 tests).
- Prettier checks for changed TypeScript files — passed.
- Specification validation and lint — passed.
- Public documentation validation — passed (62 tests; 47 published pages).
- `git diff --check` — passed.
