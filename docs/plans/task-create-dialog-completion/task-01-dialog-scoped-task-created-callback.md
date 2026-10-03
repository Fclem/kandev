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
- Callback injection only for create-mode task-create contributions.
- Regression coverage for successful create, failure, dialog isolation, and non-create surfaces.

## Out of scope

- Global task lifecycle event changes.
- Edit and new-session completion callbacks.
- Plugin-specific tag persistence or task update behavior.

## Acceptance

1. The callback receives the exact task identity created by its owning dialog, once after successful creation.
2. Failed or canceled creation and unrelated task creation do not notify the callback.
3. Unmounting a contribution unregisters its callback; edit and new-session slots do not receive the create-only capability.
