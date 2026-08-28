---
status: draft
system: tasks
created: 2026-08-19
owners:
  - kandev
---

# Additional Session Workspace Reuse Requirements

## Overview

Additional sessions must use the task's existing workspace. They must not
materialize a second worktree or change the files that the first session owns.

## Requirements

### REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001: Additional Session Workspace Reuse

**Intent:** Let additional sessions attach to a validated task workspace while
preserving independent session runtime state.

#### Acceptance criteria

- **AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.1:** An additional session
  shall attach only to a ready canonical environment with a complete,
  validated repository inventory.
- **AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.2:** Attach-only preparation
  shall not create, recreate, clone, fetch, pull, checkout, reset, or otherwise
  modify the shared workspace.
- **AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.3:** Each attached session
  shall receive independent execution identity and runtime state.
- **AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.4:** Unsafe reuse shall
  return a typed recoverable reset error; unsupported reuse shall return a typed
  non-retryable `choose_executor` error. Neither shall create a session or
  replacement workspace.
- **AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.5:** The materializing
  session, Files surface, Changes surface, terminals, and later attached or
  restored sessions shall resolve the same canonical task-environment workspace
  and repository worktree paths before and after a backend restart.
- **AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.6:** A successful worktree
  launch shall persist its complete physical repository tuple before the
  environment is reusable. Session launch or recovery shall not replace a
  concrete worktree path with the source repository checkout or an
  inventory-only row.

- **AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.7:** Attaching an
  additional session shall not change the canonical workspace's Git HEAD,
  branch, index, tracked files, untracked files, or repository registration.
- **AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.8:** A canonical
  persistence failure or process crash shall leave no permanently ready partial
  environment or runnable session. Recovery shall complete or roll back the
  interrupted attempt before new launches, and a stale attempt shall not
  overwrite or fail a newer successful attempt.
- **AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.9:** A failed resume shall
  restore the prior session and credential state. A failed prepare-only launch
  shall preserve its prior non-running session state.
- **AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.10:** Failure compensation
  shall durably remove resources created by the failed attempt, restore
  resources that the attempt recreated, and leave reused resources unchanged,
  including mixed multi-repository workspaces and backend restart during the
  attempt.
- **AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.11:** Reuse errors shall
  expose stable `reason`, `recoverable`, `retry_after_ms`, and `action` details
  without revealing filesystem paths, branches, credentials, tokens, or
  executor secrets.
- **AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.12:** An explicit source
  mutation may claim an idle ready environment and complete while concurrent
  reuse receives the typed preparing result. Derived projection repair shall
  remain physically read-only and bind only after its fenced update succeeds.

## Out of scope

- Preventing concurrent agents from editing the same file.
- A trusted filesystem read-only agent mode.
- Automatic workspace repair, reset, branch switching, or replacement during
  session spawn.
