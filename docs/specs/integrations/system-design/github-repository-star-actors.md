---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002
created: 2026-10-18
owners:
  - Kandev
---
# GitHub Repository Star Actor Resolution System Design

## Context and boundaries

This design is the actor-resolution part of the [GitHub repository stars system design](github-repository-stars.md). That document keeps the HTTP routes, URL parser, bounded status batch, dedicated OAuth App flow, frontend cache mechanics, and control geometry. This document owns which GitHub identity performs a star call, how its capability is known, and what the connection-status contract discloses. [ADR-2026-10-18-github-star-actor-resolution](../../../decisions/2026-10-18-github-star-actor-resolution.md) records the boundary.

The integration system owns every builtin credential source: `github_workspace_connections` (`ConnectionSourcePAT`, `ConnectionSourceGHCLI`, `ConnectionSourceGitHubAppInstallation`, migration-only `ConnectionSourceLegacyShared`), per-user `github_user_connections` (`ConnectionSourceGitHubAppUser`), and the dedicated `github_personal_star_connection`. `CredentialResolver` (`apps/backend/internal/github/auth_resolver.go`) already resolves the first four with workspace-epoch cache invalidation and returns an `AuthPrincipal` that names the human or App identity.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.1`, `001.3`, `001.6` | Connection-status contract, Frontend behavior, Resolution order |
| `AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.10`, `001.12` | Sources, Resolution order |
| `AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.1`, `002.2`, `002.3` | Sources, Resolution order |
| `AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.4` | Capability evaluation |
| `AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.5`, `002.6` | Connection-status contract, Frontend behavior, Permissions and security |
| `AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.7` | Actor scope and invalidation |
| `AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.8` | GitHub App registration permission |

## Sources

| `actor.source` | Origin | Token at runtime | Identity | Shared |
| --- | --- | --- | --- | --- |
| `personal_star_oauth` | `github_personal_star_connection` row, only when the deployment reports the dedicated flow available | Access secret via the existing `personalStarResolver` | Kandev user's consenting GitHub account | no |
| `github_app_user` | `github_user_connections` row under an App-installation workspace | `runtimeUserCredentialProvider.ResolveUser` (refreshing user-to-server token) | Kandev user's linked GitHub account | no |
| `pat` | Workspace PAT secret `WorkspacePATSecretKey` | `CredentialResolver.resolvePAT` | PAT owner | yes |
| `gh_cli` | Workspace `gh` host/login | `ResolveGHAccountToken` (`gh auth token`) | Selected `gh` account | yes |
| `legacy_shared` | Legacy factory: `GH_TOKEN`, `GITHUB_TOKEN`, secret `GITHUB_TOKEN`/`github_token`, or detected `gh` | Legacy factory client | The legacy token's account | yes |

`github_app_installation` is never an actor. The E2E `MockClient` stands in for every source only when `personalStarE2EControlsEnabled` holds (E2E profile plus mock client), as it does today.

## Resolution order

A new `CredentialPurposeStar` joins `validCredentialPurpose`. `StarActorResolver` (new, `internal/github/star_actor_resolver.go`) replaces the direct `personalStarResolver` call in `service_personal_star.go` and resolves per `(workspace_id, user_id)`:

1. **Dedicated connection.** If the deployment reports the dedicated flow available and the user's `github_personal_star_connection` is active and capable, it is the actor. An existing row that is inactive or incapable does not stop resolution; it only contributes `reconnect_required` to the status.
2. **CredentialResolver.** Otherwise call `CredentialResolver.Resolve` with `CredentialPurposeStar`. For this purpose `resolveAtEpoch` tries `resolveConfiguredPersonal` first (App user connection), and on no personal row falls through to `resolveAutomation`. An inactive personal row returns `ErrGitHubConnectionInvalid` and does not fall through, matching `CredentialPurposePersonalRead`. The star resolver rejects any result whose `Principal.Kind` is `AuthPrincipalApp`.
3. **Capability filter.** The resolved candidate passes through Capability evaluation. A candidate known incapable ends resolution with no actor; resolution never steps to a lower source because a higher one is incapable, so the actor is predictable from configuration.

The resolved actor carries the existing `ResolvedCredential` (client, principal, credential generation, package-private token) plus `shared` and `source`. Star calls use one `PersonalStarHTTPClient` built from the actor's token, so every source sends the same `GET`/`PUT`/`DELETE /user/starred/{owner}/{repo}` request. Within one status batch or mutation, the actor is resolved once and never replaced; an unauthorized response from `personal_star_oauth` keeps its existing single refresh-and-retry against the same connection, and no other source retries.

When no actor resolves, the status offers a connect path:

- `connect="personal_star_oauth"` when the deployment supports the dedicated flow and no dedicated connection is active and capable.
- Otherwise `connect="github_app_user"` when the workspace source is `github_app_installation` and the user has no active App personal connection.
- Otherwise `connect=null`.

## Capability evaluation

`StarCapabilityCache` is process-local and keyed by `(workspace_id, source, login, credential_generation, app_registration_id)`. It holds two kinds of entries:

- **Scope probe** for token sources (`pat`, `gh_cli`, `legacy_shared`). The first resolution calls `GET /user` with the actor token and reads `X-OAuth-Scopes`. A present header means classic scopes: `capable` iff it lists `repo` or `public_repo`; otherwise the result is `denied` with `remediation="token_scope"`. An absent header means a fine-grained token: the result is `unverified`, which is capable. Probe results live 10 minutes; a probe failure leaves the entry absent, so status reports `capable=false` with no remediation and the next status retries.
- **Denial** for any source. A `403` from a star `GET`, `PUT`, or `DELETE` records `denied` with `remediation` set to `token_scope` (token sources) or `app_permission` (`github_app_user`) until that key changes. A `404` on `GET` is "not starred", never a denial.

`github_app_user` is capable only when the App registration's permissions include `starring: write`. The permissions come from `AppClient.GetAuthenticatedApp` for that registration, cached with the registration's credential generation; absence yields `denied` with `remediation="app_permission"`. A user who has not yet accepted an updated App permission is caught by the denial rule.

`personal_star_oauth` keeps its persisted `public_repo` capability and does not use this cache.

Restart drops the cache: the next status re-probes and the next denied call re-records, both without side effects.

## Actor scope and invalidation

The actor scope is `(user_id, source, login, credential_generation)`. Workspace sources use `WorkspaceConnection.CredentialGeneration`; `github_app_user` uses `UserConnection.CredentialGeneration`; `personal_star_oauth` uses its connection generation; `legacy_shared` uses `0` plus login. A source switch, PAT replacement, `gh` account change, or disconnect already bumps the workspace epoch and generation in `CredentialResolver`, so the cache key changes and the next status reports a new scope. The frontend fences responses by this scope as described in Frontend behavior.

## GitHub App registration permission

`BuildDeploymentAppManifest` adds `"starring": "write"` to `DefaultPermissions` for new registrations. `importedAppPolicyProblems` and the lifecycle permission comparison in `app_registration_helpers.go` skip `starring` through an explicit optional-permission set, so existing and imported Apps without it keep their current policy status. Existing Apps gain star capability when the owner grants `Starring` in GitHub and users accept the update.

## Connection-status contract

`GET /api/v1/github/user/starred/connection?workspace_id=...` keeps `{user_id, credential_generation, active, capable, reconnect_required, available}` and adds:

```json
{
  "actor": {"source": "gh_cli", "login": "octocat", "shared": true},
  "connect": null,
  "remediation": null
}
```

- `available` is true when an actor is resolved, `connect` is non-null, or a dedicated connection exists; otherwise false (for example, no GitHub connection, or an App-installation workspace with no flow).
- `actor` is present whenever a candidate resolved, including when it is incapable, so the scope is stable; it is null when none resolved.
- `active` and `capable` describe that actor. For `personal_star_oauth`, both keep their committed-row meaning.
- `credential_generation` is the actor's generation.
- `remediation` is one of `token_scope`, `app_permission`, or null. It is informational and never actionable for members.
- No field carries token material, scopes text, or installation identifiers.

Truth table, extending the existing table with the non-dedicated rows:

| `available` | `actor` | `capable` | `connect` | UI |
| --- | --- | --- | --- | --- |
| false | any | any | null | Omit star and CTA |
| true | set | true | any | Hydrate and show star/unstar, disclosing `actor.login` |
| true | set | false | null | Omit star and CTA (operator remediation only) |
| true | null | false | `personal_star_oauth` | `Connect GitHub`, starting the dedicated flow |
| true | null | false | `github_app_user` | `Connect GitHub`, starting `POST /api/v1/github/personal-connection/start` |

`reconnect_required` keeps its dedicated meaning and only adds `Reconnect GitHub` for that flow.

## Frontend behavior

- `GitHubStarConnectionStatus` (`apps/web/lib/types/github-star.ts`) gains `actor`, `connect`, and `remediation`.
- `resolvePluginStarControlMode` maps the truth table above.
- `pluginStarKey` adds `actor.source` and `actor.login` alongside the existing fields, so a source or account change never reads or writes another actor's entry.
- The `github_app_user` CTA calls the existing `startGitHubPersonalConnect` and navigates to its URL. Its callback lands on `/settings/integrations/github`, so no new return-path contract is added.
- The star button keeps its stable accessible name. A localized tooltip and `aria-description` read "Starring as @login", or "Starring as @login (shared workspace account)" when `actor.shared` is true.
- After a failed mutation, the hook refetches connection status once, so a newly recorded denial removes the control without a toast.
- Copy is added in all seven locales.

## Confirmed count synchronization

`AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.13` is implemented in the existing frontend plugin-stars slice, without new GitHub calls or backend contracts. `MarketplaceEntryRow` currently renders `entry.stars` independently of `PluginRepositoryStar`; that separation leaves the catalog count unchanged after a successful mutation.

Add a session-local count projection keyed by canonical repository, not actor: the aggregate repository count is shared across workspaces and accounts. Seed each repository once from the first known catalog count; all rows for the canonical repository read that projection. A null catalog count remains null until a known seed is available. Neither status hydration nor rerendering applies a delta or overwrites an existing seed.

On an accepted mutation result, compare the previous confirmed boolean with the returned boolean and adjust the repository count by `Number(next) - Number(previous)`, clamped at zero. Update the boolean and count in the same store transaction, only after the existing actor/epoch fence accepts the result. Use a distinct mutation-result action or retain request kind until commit, so status responses never adjust counts. A completed mutation epoch must be consumed so a duplicate delivery cannot apply twice. Errors and unknown results leave the count untouched.

Clearing an actor connection discards its boolean state but preserves the aggregate projection. A full application reload discards this non-persistent projection and resumes catalog counts. No live count polling, marketplace re-ranking, or external-user count reconciliation is introduced. Existing `formatNumber` formatting and mobile geometry remain unchanged.

## Permissions and security

Workspace membership is authorized at controller and service layers before resolution, exactly as for the dedicated flow. A shared credential lets every active member star as that account; this matches `CredentialPurposePersonalWrite` fallback to human PAT/CLI sources and is disclosed by `actor.shared`. Star resolution never consults the App installation. Tokens never reach the browser. The `GET /user` probe and star calls use the same bounded 30-second provider deadline as status hydration.

## Persistence and observability

There is no new persistence: capability caches are in memory, and GitHub stays authoritative for stars. The structured log on actor resolution records `source` and `outcome` (`resolved`, `denied`, `unverified`, `none`), but no login, repository, or token. Counter `github_star_actor_resolutions_total` is labelled by the closed `source` and `outcome` sets.

## Related contracts

- [GitHub repository stars system design](github-repository-stars.md) for routes, parser, batch limits, dedicated flow, and cache mechanics.
- [Workspace GitHub authentication design](github-authentication-01.md) for credential sources and `CredentialResolver`.
- [GitHub REST starring](https://docs.github.com/en/rest/activity/starring) for token types and the `Starring` permission.
