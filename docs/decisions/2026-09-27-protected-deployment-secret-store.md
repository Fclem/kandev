# ADR-2026-09-27-protected-deployment-secret-store: Protect deployment secret-store integrity from the backend service

**Status:** accepted
**Date:** 2026-09-27
**Area:** backend

Related: [GitHub repository stars requirements](../specs/integrations/requirements/github-repository-stars.md), [protected deployment secret store design](../specs/integrations/system-design/deployment-secret-store.md)

## Context

The backend needs read access to the encrypted, deployment-owned OAuth client-secret references. The unreleased layout stored those references and the deployment identity in the backend service's writable data directory. Restrictive file modes do not help, because the service account owns the parent directory and can replace files or directories. A compromised backend could therefore change revocation metadata, restore a revoked reference, or substitute identity material. This breaks the separation between runtime resolution and root-owned provisioning and revocation.

The data directory is intentionally service-owned. Making it read-only would break unrelated runtime state. The backend can also read `master.key`, so it can create valid sealed envelopes. A compromised backend could leave behind a plausible store whose integrity checks pass.

## Decision

Store deployment identity and the encrypted reference database under the fixed root-owned state root `/var/lib/kandev-deployment-secrets/`, outside the backend service home. Key each installation directory by installation ID (`installations/<installation_id>/`). A root-owned binding file maps the configured data directory to that ID: `bindings/<sha256 of the cleaned absolute data directory>`. The root supervisor and root-only CLI own all mutation. The backend has group read access only to its installation's public identity and database. Every resolver validates root ownership and non-writable modes on the full path before reading.

Keep the `<dataDir>/master.key` contract unchanged. That key cannot authorize supervisor requests or modify the protected store.

Do not migrate the data-directory layout. It never shipped in a release, and its contents cannot be authenticated. Root bootstrap creates new protected state and never reads, imports, or falls back to data-directory state. Installations configured from the unmerged branch run bootstrap and provision again.

## Alternatives considered

### Restrict permissions on files in the existing data directory

Rejected. The backend owns the parent and can replace the files or directory, defeating file-level permissions.

### Configure an arbitrary protected path

Rejected. A path setting would let the backend, supervisor, installer, backups, and CLI disagree. A single standard root-owned location keeps the trust boundary deterministic. Only the E2E runtime profile can override the root, for non-root test runners.

### Derive the installation directory from the data-directory path

Rejected. Keying the directory by installation ID makes the store self-describing: resolvers can require that the directory name, the `installation_id` file, and the binding match. The path hash serves only as a binding key.

### Put the protected store below the service home

Rejected. The service account can replace a child directory through its writable parent, regardless of child ownership/mode.

### Migrate legacy state, with or without operator attestation

Rejected. A backend that could replace the legacy files and read `master.key` could forge any state in them. Neither integrity checks nor an operator inventory restores trust, and no released installation holds that layout.

## Consequences

- Installers and service setup must run root bootstrap with the service user and preserve the layout's ownership and modes.
- The store uses SQLite rollback journaling so read-only backend access never requires write access to the directory.
- Missing, unbound, or invalid protected state makes the optional integration unavailable. It never falls back to the data directory.
- Backups must preserve the state root, its ownership, and `master.key`. Relocating the data directory requires bootstrap and provisioning for the new path.
