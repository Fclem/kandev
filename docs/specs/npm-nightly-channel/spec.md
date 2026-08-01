---
status: shipped
created: 2026-07-31
owner: kandev
---

# npm nightly channel

## Why

Users who want fixes from `main` currently have to build Kandev themselves or wait for the next
stable release. Maintainers also lack one supported prerelease path that exercises the same npm
launcher and native runtime packages users install in production.

## What

- Kandev publishes an npm nightly from `main` at 12:00 UTC when `main` contains commits after the
  latest stable release and that exact commit has not already been published.
- A nightly for stable `X.Y.Z` and commit `abcdef123456...` has version
  `X.Y.(Z+1)-nightly.shaabcdef123456`.
- The 12-hex abbreviation is an accepted compactness trade-off. Before an existing version can
  skip publication, Git must resolve that abbreviation to the exact scheduled commit; an ambiguous
  prefix or identity mismatch fails closed for maintainer resolution.
- Every nightly publishes `kandev` and all five `@kdlbs/runtime-*` packages at the same immutable
  version under the npm `nightly` dist-tag. The stable `latest` dist-tag does not move.
- Users persistently install the channel with `npm install -g kandev@nightly`. The command
  `npx -y kandev@nightly` runs a transient Nightly copy without changing a global installation.
- Settings > System > Updates exposes Stable and Nightly for a verified, Kandev-managed npm or
  npx user service. Stable is the default and the setting is install-wide.
- Desktop, Homebrew, local-checkout, unknown, unmanaged, and system-service installations remain
  on the Stable channel.
- Stable update discovery continues to use GitHub Releases. Nightly discovery follows npm's
  `kandev@nightly` dist-tag.
- Applying an npm/npx update resolves the mutable tag first, then installs the exact immutable
  version.
- Nightly publication creates no Git tag, GitHub Release, changelog commit, desktop update,
  container tag, or Homebrew update.

Decision: [ADR-2026-07-31-npm-nightly-release-channel](../../decisions/2026-07-31-npm-nightly-release-channel.md).

Implementation plan: [npm nightly channel](../../plans/npm-nightly-channel/plan.md).

## Data model

The install-wide `settings` table contains key `updates_channel`. Its value is the UTF-8 string
`stable` or `nightly`; a missing or invalid value reads as `stable`.

The existing stable target cache remains in these `kandev_meta` keys:

- `latest_version`
- `latest_version_url`
- `latest_version_checked_at`

Nightly uses an isolated cache:

- `latest_version_nightly`
- `latest_version_nightly_url`
- `latest_version_nightly_checked_at`

Changing channel never reinterprets cached data from the other source.

## API surface

`GET /api/v1/system/updates` and `POST /api/v1/system/updates/check` retain their current fields
and add:

```json
{
  "channel": "stable",
  "channel_editable": true,
  "channel_unsupported_reason": ""
}
```

`PATCH /api/v1/system/updates/channel` accepts:

```json
{ "channel": "nightly" }
```

It returns the complete updates response for the selected channel. Invalid channel names return
`400`. Selecting Nightly for an unsupported installation returns `409`. Persistence or resolver
failures return `500` or `502` using the existing System handler error conventions.

The npm registry contract is the public metadata document for package `kandev`. The resolver reads
`dist-tags.nightly`, requires a valid SemVer, and requires the same version to exist in `versions`.

## Permissions

Anyone allowed to view System settings may read update status. The existing admin guard applies to
manual checks, channel changes, and update application. Channel choice is shared by all users of
the installation.

## Failure modes

- A GitHub or npm discovery failure preserves that channel's previous cache and surfaces the stale
  checked time plus the request error.
- A malformed or missing npm `nightly` tag fails closed; it is never offered or installed.
- A scheduled run with no commits after the stable tag exits successfully without building.
- A scheduled retry for an already-published commit exits successfully only when the main package
  and `nightly` tag agree.
- Before building, the workflow resolves the commit prefix in the current `nightly` tag. A
  scheduled or rerun commit at or behind that published commit is superseded and exits without
  building; divergent or unresolvable history fails closed.
- A 12-hex collision makes Git abbreviation resolution ambiguous, so the run fails closed instead
  of treating the colliding commit as already published.
- After acquiring the shared npm publication slot, a Nightly run rechecks `kandev@latest`. If a
  Stable publish moved the baseline while Nightly was building, the stale run exits without
  publishing.
- The same locked preflight requires `kandev@nightly` to equal the value observed before building;
  an overlapping run that already moved the tag supersedes the stale run.
- Runtime packages publish before `kandev`. If any runtime fails, the main launcher is not
  published, so no visible launcher references missing exact dependencies.
- Trusted-publisher OIDC is used only by `npm publish --tag nightly`. An existing version whose
  `nightly` tag does not match fails with recovery guidance because OIDC cannot run
  `npm dist-tag add`.
- GitHub's scheduled start may be later than 12:00 UTC; delayed execution does not change the
  deterministic version.

## Persistence guarantees

- Channel choice and both target caches survive backend restarts.
- One source failure never overwrites the other source's cache.
- Each full commit deterministically maps to one npm version, so a retry never creates a second
  version for the same source state. The accepted 12-hex collision case halts automatic
  publication rather than mapping a second commit to the existing package.
- npm nightly versions are immutable and retained; this feature performs no automated deletion.

## Scenarios

- **GIVEN** stable `0.82.0` and `main` commit `abc123def456...`, **WHEN** the nightly schedule runs,
  **THEN** all six packages publish as `0.82.1-nightly.shaabc123def456` under `nightly` and
  `kandev@latest` is unchanged.
- **GIVEN** `main` points at the latest stable tag's commit, **WHEN** the nightly schedule runs,
  **THEN** it exits successfully without a platform build or npm publication.
- **GIVEN** the current `main` nightly already exists and `kandev@nightly` points to it, **WHEN**
  the schedule runs again, **THEN** it exits successfully without rebuilding.
- **GIVEN** a previous run published only some runtime packages, **WHEN** the same commit retries,
  **THEN** matching packages are accepted, missing packages publish, and `kandev` publishes last.
- **GIVEN** no channel setting, **WHEN** a user opens Updates, **THEN** Stable is selected and the
  target comes from GitHub Releases.
- **GIVEN** a verified npm managed user service, **WHEN** an admin selects and saves Nightly,
  **THEN** the setting survives reload and the target resolves from `kandev@nightly`.
- **GIVEN** a Homebrew, Desktop, unmanaged, system-service, local, or unknown installation,
  **WHEN** Updates renders, **THEN** Nightly is unavailable with a visible reason and Stable stays
  effective.
- **GIVEN** an installed nightly whose SHA sorts lexically after the new target SHA, **WHEN** npm's
  `nightly` tag changes, **THEN** the new unequal target is offered; SHA text is not treated as a
  chronological counter.
- **GIVEN** a user running a nightly selects Stable, **WHEN** a valid stable target differs,
  **THEN** the UI offers an explicit return to that exact stable version without announcing it as
  a normal upgrade notification.
- **GIVEN** a Pixel 5 viewport, **WHEN** the user selects Nightly and saves, **THEN** the same
  persisted outcome is reachable through 44px rows with no horizontal document overflow.

## Out of scope

- Homebrew `HEAD`, a nightly formula, or a second tap
- Desktop nightly updater feeds
- GHCR nightly tags
- Nightly GitHub Releases or Git tags
- Automatic update application
- Per-user channels
- Additional beta/canary channels
- Timestamped versions
