---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001
created: 2026-09-27
owners:
  - Kandev
---
# Protected Deployment Secret Store System Design

## Context and boundary

The [GitHub repository stars design](github-repository-stars.md) resolves the dedicated OAuth App client secret from a deployment-owned encrypted reference. The configured data directory is owned and writable by the backend service account, so file modes inside it cannot protect integrity: the account can rename or replace any child file or directory. Deployment identity and the reference store therefore live outside the service-owned home.

This design maps to `REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001` AC .11 and .12. The backend reads ciphertext and public identity; it cannot mutate the store, replace identity material, or redirect resolution to state it controls. The decision record is the [protected deployment secret store ADR](../../../decisions/2026-09-27-protected-deployment-secret-store.md).

## Layout

The state root is the fixed path `/var/lib/kandev-deployment-secrets/`. Production has no path setting.

| Path | Owner:group | Mode | Contents |
| --- | --- | --- | --- |
| `/var/lib/kandev-deployment-secrets/` | `root:root` | `0755` | State root |
| `bindings/` | `root:root` | `0755` | One file per bound data directory |
| `bindings/<binding-key>` | `root:root` | `0644` | The bound installation ID and a trailing newline |
| `installations/` | `root:root` | `0755` | Per-installation directories |
| `installations/<installation_id>/` | `root:<service group>` | `0750` | Store for one installation |
| `.../installation_id` | `root:<service group>` | `0640` | Installation ID; must equal the directory name |
| `.../installation_key.pub` | `root:<service group>` | `0640` | Ed25519 root public key |
| `.../installation_key` | `root:root` | `0600` | Ed25519 root private key |
| `.../deployment-secrets.sqlite` | `root:<service group>` | `0640` | `deployment_secret_refs` |

`<binding-key>` is the lowercase hexadecimal SHA-256 of the configured data directory after `filepath.Abs` and `filepath.Clean`. Symlinks are not resolved: a backend-controlled link can change the key only to a missing binding or to an installation whose directory its group cannot read. `<service group>` is the primary group of the system service account named at bootstrap.

The existing `<dataDir>/master.key` contract is unchanged (`DeploymentMasterKeyringOpenExisting`). That key decrypts sealed envelopes; it confers no authority to sign supervisor requests or modify the protected store.

## Resolution and validation

One resolver in `internal/secrets` serves the backend, supervisor, root CLI, and test seed command. Given the configured data directory, it:

1. Reads `bindings/<binding-key>` and validates the installation ID grammar (`ValidateDeploymentInstallationID`).
2. Opens `installations/<installation_id>/` and requires `installation_id` to equal the bound ID and the directory name.
3. Validates ownership before reading any content. Every path component from `/` through the target file must be owned by UID 0 and must not be writable by group or other. Hardlinked, symlinked, or non-regular store files are rejected. The checks use `Lstat`, and reads use `O_NOFOLLOW`.
4. Loads the public key, and for root callers only the private key, then confirms that the private key derives the stored public key.

Each failure returns a typed error. The backend logs the failure and reports `available=false`. The supervisor refuses to start. The CLI exits nonzero with the binding path and the expected ownership. No path falls back to the data directory: after this change, `DeploymentIdentityDir(dataDir)` and `DeploymentSecretStorePath(dataDir)` no longer exist.

## SQLite access

The protected store uses rollback journaling (`journal_mode=DELETE`), not the WAL mode that `db.OpenSQLite` applies. A WAL reader needs write access to the `-shm` file in the store directory, which would break the read-only boundary. Only the supervisor and the test-support seed command open the writer; it holds a single connection. The backend opens `mode=ro`. If a hot journal remains after a supervisor crash, the read-only open fails and the capability stays unavailable until the supervisor restarts and recovers the journal.

## Bootstrap

`kandev deployment-secret bootstrap --data-dir <path> --service-user <name>` is root-only and idempotent:

- **No binding.** Generate the installation ID and key pair. Write the installation directory under a temporary name in `installations/`, fsync it, and rename it to `<installation_id>`. Then write the binding through a temporary file and rename it into place. A crash leaves no binding, and the next run removes any orphaned temporary directories.
- **Binding exists.** Validate it as described above. Reapply ownership and modes only for the recorded service group, and report the existing installation ID.
- **The service user changed.** Fail and name the existing group.

Bootstrap never reads from the data directory. The supervisor creates the SQLite file on first start if it is absent.

## Unreleased layout cutover

The data-directory layout (`<dataDir>/deployment/` and `<dataDir>/deployment-secrets.sqlite`) never shipped in a release. The protected layout replaces it without migration. No component reads, imports, validates, or deletes those files. An installation that was configured from this unmerged branch must run bootstrap and provision again. Stale files are inert, and operators may delete them.

## Test seam

The `e2e` runtime profile (`KANDEV_E2E_MOCK=true`) may set `KANDEV_E2E_DEPLOYMENT_SECRET_ROOT` to replace the state root. In that profile, validation requires the effective UID instead of UID 0 from the override root downward and skips ancestors above it. Other profiles ignore the variable, and the default production resolver has no override parameter. `apps/web/e2e/fixtures/personal-star-deployment.ts` and `cmd/deployment-secret-seed` write the same layout under that root.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001` AC .11 | Layout; Resolution and validation; SQLite access |
| `REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001` AC .12 | Resolution and validation; Bootstrap; Unreleased layout cutover |

## Consequences and operational contract

- Installation order is service user, then `bootstrap`, then supervisor start, then `provision`, then backend restart.
- Relocating the data directory changes the binding key. The capability reports unavailable until the operator runs bootstrap and provision for the new path.
- Backup and restore must preserve `/var/lib/kandev-deployment-secrets/`, including ownership and modes, together with `<dataDir>/master.key`. A restored store is subject to the same ownership validation.
- Public operator documentation covers the layout, bootstrap flags, recovery, and backup.

## Related documents

- [GitHub repository stars requirements](../requirements/github-repository-stars.md)
- [GitHub repository stars system design](github-repository-stars.md)
- [ADR: protected deployment secret store](../../../decisions/2026-09-27-protected-deployment-secret-store.md)
