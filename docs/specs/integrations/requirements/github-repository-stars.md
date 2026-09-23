---
status: draft
system: integrations
created: 2026-09-17
owners:
  - Kandev
---
# GitHub Repository Stars Requirements

## Overview

Plugin repository links identify GitHub projects that users may want to support. The Settings > Plugins surface shall let an authenticated user star or unstar those repositories without leaving Kandev, while preserving the existing plugin browsing and management flow when GitHub is unavailable.

## Requirements

### REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001: Plugin repository starring

**Intent:** A user can manage their GitHub star for a plugin's public GitHub repository from Settings > Plugins and see the current GitHub-owned state when the page loads.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.1:** When Settings > Plugins displays a valid `https://github.com/<owner>/<repo>` plugin repository and connection status reports `available=true`, the surface SHALL show a star control whose state reflects whether the configured personal GitHub identity currently stars that repository. When `available=false`, it SHALL omit the control and any actionable CTA.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.2:** When the user activates an unstarred control, Kandev SHALL request GitHub to star the repository and update the control to starred only after GitHub confirms success; activating a starred control SHALL analogously unstar it.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.3:** The star state SHALL be refreshed when the plugin surface loads and SHALL update the frontend cache after a successful mutation so repeated renders do not require a second request for the same repository. Cache entries SHALL be scoped to the active workspace, authenticated user, personal GitHub credential generation, and canonical repository, and stale status responses SHALL not overwrite a newer confirmed mutation or a different user with the same generation.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.4:** GitHub authentication, repository eligibility, and provider errors SHALL fail silently for this optional control: the plugin catalog, installed-plugin management, repository link, and page navigation SHALL remain usable, and a failed mutation SHALL not claim the opposite star state.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.5:** When connection status reports `available=true`, star controls SHALL be available to authenticated members wherever the repository is displayed, including marketplace entries, installed-plugin rows, and installed-plugin detail; they SHALL not grant plugin installation, configuration, lifecycle, or repository-management authority. When `available=false`, those surfaces SHALL omit the control and actionable CTA.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.6:** Star status and mutations SHALL require an explicit `workspace_id`, authorize active membership in that workspace at both controller and service layers, and resolve the authenticated user's strict personal GitHub star connection without requiring the public catalog repository to be registered in the workspace. They SHALL never silently substitute a GitHub App installation or the existing GitHub App user-to-server connection. Missing or foreign workspace identity, missing personal star connection, inactive connection, and App principals SHALL fail closed before any provider call.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.7:** The backend star contract SHALL accept raw repository URLs and apply one strict parser: HTTPS, exact `github.com` host (case-insensitive), no credentials or port, no percent-encoding, exactly two path segments, and no query or fragment. The owner grammar SHALL be ASCII `[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?` (one-character names allowed; no leading/trailing hyphen). The repository grammar SHALL be ASCII `[A-Za-z0-9](?:[A-Za-z0-9._-]{0,98}[A-Za-z0-9_])?` (one-character names allowed; no trailing dot or hyphen). A terminal `.git` is removed once before repository validation and canonicalization; `repo.git.git` is invalid. Owner and repository are lowercased with ASCII case folding for the canonical key. Unicode, dot-only, empty, traversal-like, encoded, confusable, or otherwise out-of-grammar names SHALL be rejected independently of frontend rendering.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.8:** Batch status hydration SHALL use a bounded, deduplicated request contract with explicit limits and per-repository unknown results for provider failures; duplicate canonical repository keys SHALL cause at most one provider call.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.9:** Fine-pointer controls on non-mobile desktop SHALL use the existing 24px compact inline sizing. Any touch context, defined as a mobile viewport or a coarse primary pointer, SHALL use a minimum 44px touch target, including mobile viewports with fine pointers. Each surface SHALL expose keyboard activation, `aria-pressed`, a stable accessible name while busy, and localized live status feedback.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.10:** The personal GitHub star connection SHALL use a dedicated OAuth App authorization that requests and persists the `public_repo` capability. Existing GitHub App user-to-server connections, connections without that capability, and connections with unknown scope metadata SHALL not be used for star mutations. They SHALL expose reconnect/re-consent through the personal connection flow; reduced-scope or declined re-consent SHALL fail silently in the optional star control.

## Explicit exclusions

- Non-GitHub repository URLs and malformed repository URLs do not receive a star control.
- Kandev does not persist a separate star preference or star count; GitHub remains authoritative.
- This capability does not add ratings, reviews, telemetry, or marketplace ranking changes.
- GitHub App installation identities and existing GitHub App user-to-server personal connections cannot star repositories on behalf of this feature; a dedicated personal OAuth App connection is required.

## System design

See [GitHub repository stars system design](../system-design/github-repository-stars.md).
