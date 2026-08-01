# ADR-2026-07-31-npm-nightly-release-channel: Publish deterministic npm-only nightlies

**Status:** accepted
**Date:** 2026-07-31
**Area:** workflow, backend, frontend, cli

## Context

Kandev's stable release workflow intentionally gives npm, Homebrew, GitHub Releases, Desktop, and
containers one shared `X.Y.Z`. Users also need prerelease access to current `main`, but Homebrew
and Desktop require separate mutable-feed and signing designs. npm already has six coordinated
packages, trusted publishing bound to `.github/workflows/release.yml`, and native dist-tags.

A commit-derived prerelease must be deterministic for retries and valid even when the abbreviated
SHA is digits-only or begins with zero. Update discovery must not sort SHA text as time.

## Decision

Kandev has an npm-only `nightly` channel. A stable baseline `X.Y.Z` and current `main` SHA produce
`X.Y.(Z+1)-nightly.sha{first-12-lowercase-hex}`. Time and git-describe's `g` marker are excluded:
the commit is the identity, and the `sha` prefix makes the SemVer identifier unambiguously
nonnumeric.

The 12-hex abbreviation is an accepted compactness trade-off. Before an already-published version
can skip a run, the workflow resolves the abbreviation against full `main` history and requires the
resolved commit to equal the scheduled SHA. Git rejects an ambiguous abbreviation, so a prefix
collision fails closed for maintainer resolution rather than silently treating a different commit
as published.

The existing release workflow owns both stable and nightly npm publication because npm allows one
trusted publisher per package and validates the workflow filename. Scheduled nightlies publish all
five runtime packages before the launcher with `npm publish --tag nightly`; stable `latest` tags
remain untouched. Nightly jobs do not enter the Git tag, GitHub Release, Desktop, container, or
Homebrew graph. Stable and Nightly publication is serialized. Before building, a Nightly target
must be newer in `main` ancestry than the published Nightly; after acquiring the publication slot,
both the Stable baseline and previously observed Nightly tag must still match.

The backend owns an install-wide Stable/Nightly preference. Stable remains the default and resolves
GitHub Releases. Nightly resolves npm's `kandev@nightly` target and is selectable only for verified
managed npm/npx user services. Update intents always contain an exact version. Nightly-to-nightly
availability follows dist-tag inequality, not SemVer ordering of SHA text.

## Consequences

Users get one documented prerelease path without weakening stable channels or Desktop signing.
One full commit deterministically maps to one immutable version, making scheduled retries safe and
observable. A collision in the accepted 12-hex namespace blocks automatic publication and needs
maintainer resolution. Publishing six packages can still fail partially, so
runtime-first/main-last order and tag-consistency checks are required.

npm accumulates immutable nightly versions. Homebrew and Desktop users do not receive channel
parity in this iteration. The release workflow must explicitly gate every stable-only job when
handling scheduled events.

## Alternatives Considered

- **Timestamp plus SHA:** gives chronological SemVer ordering but creates different immutable
  versions for the same commit and complicates retries.
- **Raw abbreviated SHA:** can become a numeric SemVer identifier with an illegal leading zero;
  `sha` is a small explicit validity guard.
- **Full 40-hex SHA:** eliminates abbreviation collisions but makes every user-visible package
  version substantially longer; the shorter identity plus fail-closed ambiguity check is preferred.
- **Separate nightly workflow:** cleaner YAML isolation, but it conflicts with npm's single trusted
  publisher configuration for the existing six packages.
- **GitHub prereleases:** would duplicate stable release artifacts and feeds when npm is the only
  requested consumer.
- **Homebrew `HEAD` or a nightly formula:** viable future designs, but they need source builds or a
  separate mutable formula/tap and do not match the current immutable-asset formula.
