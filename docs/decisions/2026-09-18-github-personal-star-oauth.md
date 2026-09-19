# ADR-2026-09-18-github-personal-star-oauth: Use a dedicated OAuth App for personal stars

**Status:** accepted
**Date:** 2026-09-18
**Area:** backend

## Context

GitHub repository starring is a user-attributed operation. Kandev's current personal GitHub flow is GitHub App user-to-server authorization, while `public_repo` is an OAuth App scope. The existing flow does not provide a durable scope/capability record that can distinguish a token authorized for starring from one authorized only for the current integration features. Reusing the existing personal-write resolver would also fall back to an App installation identity, which cannot represent the user's personal star action.

## Decision

Personal plugin-repository starring uses a separate OAuth App connection per workspace/user. The deployment operator configures a dedicated OAuth App client ID and encrypted client-secret reference through the typed GitHub integration configuration. The flow has its own state records, callback identity, encrypted token secret IDs, and persistence table; it does not reuse or mutate the existing GitHub App `UserConnection` row or personal auth flow.

The dedicated connection stores non-secret normalized granted scopes and capability status as metadata, while access and refresh tokens remain only in the encrypted secret store referenced by secret IDs. The authorization request explicitly requests `public_repo`; the callback verifies the dedicated flow and persists token secret IDs, scopes, user identity, status, and a new credential generation atomically. Scope metadata survives refresh and generation replacement.

Reconnect/re-consent is preserve-and-remain-usable: starting a replacement does not invalidate the currently valid connection; a failed, declined, expired, or reduced-scope callback leaves the old connection and generation unchanged. A connection is replaced only after a verified callback grants `public_repo` and its new secrets/metadata commit succeeds. If no prior valid connection exists, the capability remains unavailable.

## Consequences

- Starring has a truthful personal actor and an explicit capability boundary.
- The backend needs dedicated OAuth App configuration, flow/callback state, encrypted token persistence, scope metadata, generation-aware replacement, reconnect status, and focused tests.
- Users may need one additional GitHub authorization even when a GitHub App is already connected.
- App-backed workspaces can still use the feature when the user completes the dedicated personal OAuth connection; App installation credentials are not sufficient.
- Existing GitHub App user-to-server connections and PR-write routing remain unchanged.

## Alternatives Considered

- **Reuse GitHub App user-to-server authorization:** rejected because `public_repo` is an OAuth App scope and the current flow is bound to an active App registration.
- **Use the workspace App installation identity:** rejected because it changes the actor from the user to the deployment/App and cannot implement personal starring semantics.
- **Encrypt scope metadata with tokens:** rejected because scopes are non-secret capability metadata; keeping them in the connection row makes capability queries and re-consent status explicit without exposing token material.
- **Invalidate an existing connection when reconnect starts:** rejected because a failed re-consent would unnecessarily remove a still-valid personal capability. Replacement is atomic after successful consent instead.
- **Infer capability from successful API calls:** rejected because it makes authorization state implicit, cannot support reconnect/re-consent UX, and risks repeated side effects.
