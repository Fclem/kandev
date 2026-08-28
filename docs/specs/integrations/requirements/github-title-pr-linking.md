---
status: draft
system: integrations
created: 2026-08-25
owners:
  - kandev
---

# GitHub Title PR Linking Requirements

## Overview

Kandev tasks often retain pull-request references in their titles when work is
created or handed off outside Kandev. The integration system owns resolving
those references against workspace-authorized GitHub repositories and turning
unambiguous matches into the same durable task-to-PR associations created by
the explicit **Link > GitHub Pull Request** flow.

## Terminology

- **Title PR reference:** A positive decimal GitHub pull-request number written
  as `#<number>` in a task title.
- **Unambiguous match:** Exactly one GitHub repository linked to the task
  contains a pull request with the referenced number, with every other linked
  GitHub repository producing a definitive no-match result.
- **Eligible trigger:** A task-title modification, task unarchive, or task
  session start or restore.

## Requirements

### REQ-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001: Unambiguous title reference linking

**Intent:** Preserve pull-request context already present in task titles without
requiring users to repeat the manual linking flow or risking a link to the wrong
repository.

**User story:** As a Kandev user, I want PR numbers in a task title to become
linked pull requests when GitHub can identify them uniquely, so that review and
CI state appears on the task automatically.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.1:** When an eligible trigger
  occurs, Kandev shall inspect every distinct valid title PR reference and try
  to resolve each reference independently across all GitHub repositories linked
  to the task.
- **AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.2:** When exactly one linked
  GitHub repository contains a referenced PR number and all other linked GitHub
  repositories definitively do not, Kandev shall create the same durable,
  idempotent task-to-PR association used by explicit GitHub PR linking.
- **AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.3:** When no linked GitHub
  repository contains a referenced number, more than one contains it, no
  GitHub repository is linked, or any repository result is indeterminate,
  Kandev shall leave that reference unlinked.
- **AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.4:** When a title contains
  multiple distinct PR references, Kandev shall link every reference that is
  independently unambiguous, including multiple PRs in one repository or PRs
  spread across different linked repositories.
- **AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.5:** A title-linking failure or
  ambiguous result shall not fail, delay completion of, or roll back the title
  modification, task unarchive, or session start or restore that triggered it.
  A later eligible trigger may retry unresolved references.
- **AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.6:** A successful automatic link
  shall be observable through the existing GitHub PR status, review, unlink,
  automation, and persistence surfaces without a separate title-link state.

## Out of scope

- Resolving full GitHub URLs or repository-qualified references from task
  titles.
- Linking GitHub issues, GitLab merge requests, or plugin-owned change requests.
- Scanning every existing task at startup or on a periodic schedule.
- Guessing from repository order, the primary repository, branch names, or
  partial provider failures.
- Changing the explicit **Link > GitHub Pull Request** input semantics.
