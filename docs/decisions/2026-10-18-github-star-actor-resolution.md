# ADR-2026-10-18-github-star-actor-resolution: Resolve the star actor across every builtin GitHub authentication scheme

**Status:** accepted
**Date:** 2026-10-18
**Area:** backend, frontend

Supersedes
[ADR-2026-09-18-github-personal-star-oauth](2026-09-18-github-personal-star-oauth.md).

## Context

Plugin repository starring shipped behind a dedicated personal OAuth App. Deployments that have not provisioned that App, its protected client secret, and the root secret daemon report the capability unavailable, even when the workspace already holds a GitHub credential that GitHub accepts for `PUT /user/starred/{owner}/{repo}`. The common local install authenticates through the `gh` CLI with a classic `repo`-scoped token, so the star control never appears there.

GitHub accepts star mutations from classic OAuth tokens and classic PATs that carry `repo` or `public_repo`, from fine-grained PATs with the `Starring` user permission, and from GitHub App user access tokens whose App holds the `Starring` user permission. GitHub App installation tokens cannot star: they are not a user.

Kandev's existing personal-write resolution (`CredentialResolver.resolvePersonal`) already treats PAT, `gh` CLI, and legacy shared credentials as human identities that can supply authenticated-viewer semantics, and never lets an App installation impersonate a person.

## Decision

Starring resolves one **star actor** per request from every builtin GitHub authentication scheme, in a fixed precedence: the user's dedicated personal star OAuth connection, then the user's GitHub App user-to-server connection, then the workspace's human automation credential (PAT, `gh` CLI account, or legacy shared credential). The first candidate that is active and not known to be incapable is the actor. A GitHub App installation is never an actor.

Capability is evaluated per source. Classic OAuth-scoped tokens are capable only when GitHub reports `repo` or `public_repo` in `X-OAuth-Scopes`. Fine-grained PATs and App user tokens expose no scope header; they are treated as capable until GitHub denies a star call, after which that actor and credential generation are recorded incapable until the credential changes. App user tokens additionally require the App registration to hold `starring: write`.

The dedicated OAuth App remains a supported, optional per-user source with its existing persistence, flow, and protected deployment store. It no longer gates the feature.

A workspace-shared credential makes every active workspace member star as that shared GitHub account. The connection status discloses the actor's source, login, and whether it is shared, and the control discloses the acting login.

## Consequences

- Workspaces authenticated by `gh`, a PAT, or legacy credentials get working stars without OAuth App provisioning.
- App-installation workspaces star through the user's existing App personal connection when its App holds `starring: write`; new Kandev-created App registrations request it, and existing registrations are not flagged as policy problems for lacking it.
- Fine-grained and App user capability is learned from a side-effect-free denial (`403` on a star call). That denial is cached per actor generation so it is not repeated, and no request is retried under another actor.
- Shared credentials attribute stars to the shared GitHub account; this matches existing personal-write fallback and is disclosed in the UI rather than hidden.
- The cache scope and connection-status contract gain actor source and login so a credential switch never shows another actor's star state.

## Alternatives Considered

- **Keep the dedicated OAuth App as the only source:** rejected; it leaves the feature unavailable on installs that already hold a star-capable GitHub credential.
- **Restrict shared credentials to workspace admins:** rejected; it diverges from existing personal-write routing, and the actor is disclosed instead.
- **Probe fine-grained capability by starring and unstarring a sentinel repository:** rejected; it creates visible side effects on the user's GitHub account.
- **Fall back to the next source after a denial:** rejected; it silently changes who performed the action within one user gesture.
