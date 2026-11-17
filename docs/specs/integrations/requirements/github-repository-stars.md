---
status: draft
system: integrations
created: 2026-09-17
owners:
  - Kandev
---
# GitHub Repository Stars Requirements

## Overview

Plugin repository links identify GitHub projects that users may want to support. The Settings > Plugins surface shall let an authenticated user star or unstar those repositories without leaving Kandev, using whichever builtin GitHub authentication the workspace and user already have, while preserving the existing plugin browsing and management flow when GitHub is unavailable.

## Requirements

### REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001: Plugin repository starring

**Intent:** A user can manage their GitHub star for a plugin's public GitHub repository from Settings > Plugins and see the current GitHub-owned state when the page loads.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.1:** When Settings > Plugins displays a valid `https://github.com/<owner>/<repo>` plugin repository and connection status reports `available=true`, the surface SHALL show a star control whose state reflects whether the resolved star actor (REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002) currently stars that repository. When `available=false`, it SHALL omit the control and any actionable CTA.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.2:** When the user activates an unstarred control, Kandev SHALL request GitHub to star the repository and update the control to starred only after GitHub confirms success; activating a starred control SHALL analogously unstar it.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.3:** The star state SHALL be refreshed when the plugin surface loads and SHALL update the frontend cache after a successful mutation so repeated renders do not require a second request for the same repository. Cache entries SHALL be scoped to the active workspace, authenticated user, star actor source, actor GitHub login, actor credential generation, and canonical repository, and stale status responses SHALL not overwrite a newer confirmed mutation, a different user, or a different actor with the same generation.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.4:** GitHub authentication, repository eligibility, and provider errors SHALL fail silently for this optional control: the plugin catalog, installed-plugin management, repository link, and page navigation SHALL remain usable, and a failed mutation SHALL not claim the opposite star state.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.5:** When connection status reports `available=true`, star controls SHALL be available to authenticated members wherever the repository is displayed, including marketplace entries, installed-plugin rows, and installed-plugin detail; they SHALL not grant plugin installation, configuration, lifecycle, or repository-management authority. When `available=false`, those surfaces SHALL omit the control and actionable CTA.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.6:** Star status and mutations SHALL require an explicit `workspace_id`, authorize active membership in that workspace at both controller and service layers, and resolve the star actor for the authenticated user and workspace without requiring the public catalog repository to be registered in the workspace. They SHALL never use a GitHub App installation identity. Missing or foreign workspace identity and the absence of a capable star actor SHALL fail closed before any provider call.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.7:** The backend star contract SHALL accept raw repository URLs and apply one strict parser: HTTPS, exact `github.com` host (case-insensitive), no credentials or port, no percent-encoding, exactly two path segments, and no query or fragment. The owner grammar SHALL be ASCII `[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?` (one-character names allowed; no leading/trailing hyphen). The repository grammar SHALL be ASCII `[A-Za-z0-9](?:[A-Za-z0-9._-]{0,98}[A-Za-z0-9_-])?` (one-character names allowed; `.`, `-`, and `_` are allowed anywhere except as the trailing dot, which would collide with the clone-URL suffix). A terminal `.git` is removed once before repository validation and canonicalization; `repo.git.git` is invalid. Owner and repository are lowercased with ASCII case folding for the canonical key. Unicode, dot-only, empty, traversal-like, encoded, confusable, or otherwise out-of-grammar names SHALL be rejected independently of frontend rendering.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.8:** Batch status hydration SHALL use a bounded, deduplicated request contract with explicit input limits and one shared 30-second deadline for provider work; per-repository failures or deadline expiry SHALL produce unknown results, and duplicate canonical repository keys SHALL cause at most one provider call.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.9:** Fine-pointer controls on non-mobile desktop SHALL use the existing 24px compact inline sizing. Any touch context, defined as a mobile viewport or a coarse primary pointer, SHALL use a minimum 44px touch target, including mobile viewports with fine pointers. Each surface SHALL expose keyboard activation, `aria-pressed`, a stable accessible name while busy, and localized live status feedback.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.10:** The dedicated personal star OAuth App connection SHALL remain an optional star source. When configured, it SHALL request and persist the `public_repo` capability; a dedicated connection without that capability or with unknown scope metadata SHALL not be used for star mutations and SHALL expose reconnect/re-consent through the dedicated flow. Reduced-scope or declined re-consent SHALL fail silently in the optional star control.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.11:** The backend service account MAY read deployment-owned OAuth client-secret metadata and ciphertext required at runtime, but SHALL NOT alter, replace, or reactivate revoked secret references or deployment identity material, including by replacing store files or their parent directory.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.12:** Deployment identity and secret references for the dedicated OAuth App SHALL exist only in the protected deployment store. Runtime resolution, root bootstrap, provisioning, and revocation SHALL never read, import, or fall back to deployment identity or secret-store state under the backend data directory. When protected state is missing, unbound, or fails ownership validation, the dedicated source SHALL report unavailable and star actor resolution SHALL continue with the remaining sources; the operator recovers through root bootstrap and provisioning, not through migration.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.13:** After a confirmed star-state transition, every visible count for the same canonical GitHub repository SHALL update without reloading: increment once for unstarred-to-starred and decrement once for starred-to-unstarred, never below zero. In-flight, failed, stale, duplicate, and status-hydration responses SHALL NOT adjust counts. The adjustment is session-local against the catalog count, not a live census of other GitHub users' activity; unknown counts SHALL remain unknown rather than being invented. Star/unstar SHALL remain usable when the count is unknown.

### REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002: Star actor resolution across GitHub authentication schemes

**Intent:** Starring works with every builtin GitHub authentication scheme Kandev supports, always acts as one disclosed human GitHub account, and never silently changes that account.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.1:** The star actor SHALL be resolvable from each builtin scheme: the dedicated personal star OAuth connection, the GitHub App user-to-server personal connection, a workspace personal access token (classic or fine-grained), a workspace `gh` CLI account, and legacy shared credentials (`GH_TOKEN`/`GITHUB_TOKEN` environment, a `GITHUB_TOKEN`/`github_token` secret, or the legacy `gh` login). The mock GitHub client SHALL serve stars only under the E2E profile.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.2:** Resolution SHALL use one fixed precedence for the authenticated user and workspace: the dedicated star connection, then the GitHub App personal connection, then the workspace PAT, `gh` CLI, or legacy shared credential. The first candidate that is active and not known to be incapable SHALL be the actor. A request SHALL never retry under a different actor after its chosen actor fails.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.3:** A GitHub App installation SHALL never be the actor. When a workspace connected through a GitHub App installation has no usable personal actor for the user, the status SHALL offer a Connect GitHub path: the dedicated flow when the deployment supports it, otherwise the existing GitHub App personal-connection flow.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.4:** Capability SHALL be evaluated per source. A token that reports classic OAuth scopes SHALL be capable only when its scopes include `repo` or `public_repo`. A fine-grained PAT or GitHub App user token SHALL be treated as capable until GitHub denies a star call (`403`); that denial SHALL mark the actor and credential generation incapable until the credential changes, and SHALL not be repeated for later requests. A GitHub App user token SHALL also require its App registration to hold `starring: write`.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.5:** Connection status SHALL disclose the actor's source, GitHub login, and whether the credential is shared across workspace members, without token material. The star control SHALL disclose the acting GitHub login through a localized tooltip and accessible description, and SHALL say when the account is shared.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.6:** When the actor is a workspace-shared credential, every active workspace member SHALL star as that shared GitHub account. Plugin installation, configuration, lifecycle, and repository-management authority SHALL remain unchanged.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.7:** A workspace credential change (source switch, PAT replacement, `gh` account change, disconnect) or a personal connection change SHALL change the actor scope, so star states cached for the previous actor are neither displayed nor overwritten by its late responses.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.8:** New Kandev-created GitHub App registrations SHALL request the `starring: write` user permission. Existing and imported registrations without it SHALL not be reported as App policy problems; they SHALL make the App personal connection incapable for stars only.

## Explicit exclusions

- Non-GitHub repository URLs and malformed repository URLs do not receive a star control.
- Kandev does not persist a separate star preference or star count; GitHub remains authoritative.
- This capability does not add ratings, reviews, telemetry, or marketplace ranking changes.
- GitHub App installation identities cannot star repositories; GitHub only accepts star calls from a user identity.
- Kandev does not change a workspace credential's scopes or a GitHub App's permissions; missing capability is remediated by the operator in GitHub or `gh`.

## System design

See [GitHub repository stars system design](../system-design/github-repository-stars.md), [GitHub repository star actor resolution design](../system-design/github-repository-star-actors.md), and [protected deployment secret store design](../system-design/deployment-secret-store.md).
