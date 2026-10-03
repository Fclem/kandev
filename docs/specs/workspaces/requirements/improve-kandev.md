---
status: active
system: workspaces
created: 2026-04-29
owners:
  - Carlos Florencio
---
# Improve Kandev Requirements

## Overview

Users who hit a bug or have a feature idea today have no in-app way to report it, and even when they do, the report sits as text someone else has to act on. Make filing an improvement a one-click action that produces a real, actionable task the user's own agent picks up immediately — turning every report into a contribution.

## Requirements

### REQ-WORKSPACES-IMPROVE-KANDEV-001: Improve Kandev

**Intent:** Users who hit a bug or have a feature idea today have no in-app way to report it, and even when they do, the report sits as text someone else has to act on. Make filing an improvement a one-click action that produces a real, actionable task the user's own agent picks up immediately — turning every report into a contribution.

#### Acceptance criteria

- **AC-WORKSPACES-IMPROVE-KANDEV-001.1:** The **Improve Kandev** action in the desktop app-sidebar footer opens a task-creation dialog that is pre-configured for the kandev codebase: repository locked to `https://github.com/kdlbs/kandev`, base branch `main`, workflow selected from the hidden Improve Kandev workflows, and description seeded with a starter template. On phones the same action is a 44px-or-larger row in the existing mobile home menu's **Utilities** section.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.2:** The dialog reuses the existing task-create UI, including prompt enhancement, image paste, and file attachments.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.3:** The dialog explains the flow up front: the agent will implement the change, the user will test it, then the agent opens a PR. Brief copy positions this as the user contributing to kandev's future.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.4:** The explanation includes a "Do not show this again" preference. Once selected, later uses of **Improve Kandev** skip the explanation and enter the pre-configured task-creation flow. GitHub-auth recovery takes precedence; if the dedicated workspace does not exist, the workspace-choice panel in AC `.8` appears before bootstrap. The preference is local to the current browser profile and can be cleared with other local UI state.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.5:** The task-creation dialog offers three report kinds: **Bug fix**, **Feature request**, and **Open issue**. Bug fixes and feature requests use the existing implementation workflow. Open issue uses a separate hidden, one-step workflow and visibly explains that the agent only publishes a GitHub issue; it does not implement the change or open a pull request.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.6:** An "Include recent logs" toggle attaches a context bundle to the task when enabled. It defaults on for **Bug fix** and off when **Feature request** or **Open issue** is selected; users may toggle it for any report kind. The initiating browser creates an authenticated diagnostic bundle job for the standard backend, frontend, and runtime sources, excluding ACP evidence, waits for a ready or partial archive, and leases the ZIP to a temporary path referenced in the task description. The archive follows the shared diagnostic bundle contract rather than a fixed set of log files.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.7:** Submitting the dialog creates the task in the chosen target workspace, clones the kandev repo if needed, and starts the agent on the first step. An existing dedicated **Improve Kandev** workspace is reused; when it does not exist, the target is chosen as defined in AC `.8`.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.8:** When no dedicated workspace exists, the dialog offers a "Create a dedicated Improve Kandev workspace" checkbox, checked by default. If checked, submitting creates the normal visible workspace named **Improve Kandev** and scopes the task, hidden workflows, and kandev repo there; the workspace is reused on later uses and persists across restarts. If unchecked, those resources use the active workspace instead. Once a dedicated workspace exists, it is reused.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.9:** When a workspace containing the hidden Improve Kandev workflows is selected, the desktop Kanban board shall show task cards from both workflows in unfiltered **All Workflows** view. On phones, both workflows shall remain reachable through the existing navigator even when no tasks match; their task cards appear when selected and tasks exist. When bootstrap creates a workflow in the active workspace, its live workflow event must preserve template identity so the board can recognize it without a page reload.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.10:** On first creation of a dedicated **Improve Kandev** workspace, bootstrap copies the GitHub connection from the default workspace: the valid active workspace recorded in user settings, otherwise the earliest-created workspace, otherwise the literal `default` workspace. The copy includes the connection's applicable PAT secret.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.11:** If the resolved default workspace has no GitHub connection, first-time bootstrap leaves the dedicated workspace without a GitHub connection.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.12:** First-time bootstrap copies no integration configuration other than the GitHub connection, no automations, and no user-created workflows or repositories. The new workspace contains only the normal bootstrap defaults.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.13:** When bootstrap reuses an existing dedicated **Improve Kandev** workspace, its existing user configuration, including its GitHub connection and other integration configuration, remains unchanged; bootstrap does not recopy or sync workspace configuration.


## System design

The migrated technical source is split into [part 1](../system-design/improve-kandev.md).
