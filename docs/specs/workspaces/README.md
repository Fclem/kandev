---
status: draft
system: workspaces
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# Workspace system

## Purpose

The workspace system owns workspace lifecycle, repository catalog identity, Git
worktree creation/removal mechanics, branches, workspace secrets, and
workspace-scoped execution context.

## Ownership

This system owns workspace creation/deletion, repository attachment and sets,
local repository records, branch templates, secrets, workspace Git state, and
the filesystem/Git operations used when the task system materializes or cleans
task-owned worktrees.

## Exclusions

- Task environments, session binding, and task-scoped worktree lifetime belong
  to the [task system](../tasks/README.md).
- Provider credentials belong to the [integration system](../integrations/README.md).
- Workspace settings presentation belongs to the [UI system](../ui/README.md).

## Specification map

### Requirements



- [Create a Local Repository During Task Creation](requirements/create-local-repository.md)
- [Kanban workspace creation](requirements/creation.md)
- [Workspace Deletion](requirements/deletion.md)
- [Improve Kandev](requirements/improve-kandev.md)
- [Local Workspace Repositories](requirements/local-repositories.md)
- [Repository and Workspace Secrets](requirements/repository-secrets.md)
- [Repository Sets](requirements/repository-sets.md)
- [Copy and Move Secrets Between Scopes](requirements/secret-scope-transfer.md)
- [Workspace Base-Branch Propagation](requirements/workspace-base-branch-propagation.md)
- [Worktree Branch Templates](requirements/worktree-branch-templates.md)

### System design



- [Improve Kandev](system-design/improve-kandev.md)
- [Copy and Move Secrets Between Scopes](system-design/secret-scope-transfer.md)

## Migration record

Migration remains in progress while legacy source detail is extracted from the
canonical requirement and system-design documents above.

## Related systems

- [Tasks](../tasks/README.md): consumes workspace repositories and worktrees.
- [Integrations](../integrations/README.md): supplies remote repository identity.
