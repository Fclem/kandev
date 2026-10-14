# ADR-2026-10-10-periodic-omp-self-updates: Opt-in periodic OMP self-updates

**Status:** accepted
**Date:** 2026-10-10
**Area:** backend, frontend

## Context

Codex automation validates an isolated exact-version candidate before activation. OMP only exposes an in-place `omp update` operation following its configured channel. Stable registry metadata cannot establish whether that channel has a newer release. The user selected opt-in in-place automation and periodic configured-channel checks rather than stable-release-triggered updates.

## Decision

Extend the existing install-wide consent, background sweep, and maintenance job pipeline for built-in `omp-acp`. Execute only the trusted `omp update` and `omp acp` commands. Do not add a channel selector, target version, staged installation, or package-manager fallback.

The existing fifteen-minute background cadence drives OMP attempts independently of stable-release status. Gate on the persisted start time of the previous background pass that admitted an attempt (pass start times on both sides, one-minute slack, a clock step back counts as eligible; opt-out and opt-in never reset it), so restart cannot accelerate checks and the real cadence is one attempt per tick; attempts are never less than fourteen minutes apart. Successful ACP validation with the same known version is a no-change result for automatic jobs only. An updater error or an updater that reaches its own deadline is a failed check that is retried at the normal interval and pauses after three consecutive failures; a stall is the same transient class as an offline error. A failed, timed-out or versionless ACP validation after a successful updater, including an authentication-required probe, pauses at once because the installation may be in an unknown state. The probe runs on the pass context, not the updater's hard-timeout context, so a slow download cannot starve it; the updater deadline starts after admission. Backend shutdown cancels an admitted update and its package-manager processes instead of waiting up to the five-minute job timeout: output goes through Cmd-owned writers so the wait delay releases the pipes, the process group gets SIGTERM and then SIGKILL, and only a descendant that detached itself can survive. A cancelled pass context (not the hard-timeout child, whose deadline stays an updater deadline) classifies the attempt as interrupted even when the host probe reported a failure, because the probe swallows cancellation. The resulting interrupted result (also written when a stale running marker is found after a crash) counts as a failed check without pausing at once, because the next `omp update` repairs a partly applied installation. A pause lasts until saved opt-out followed by saved opt-in.

Consent authorizes an in-place mutation, not Codex's rollback-safe activation contract. Show this limitation next to the switch before consent. Revalidate consent and maintenance ownership immediately before invoking the updater. Once invocation begins, opt-out cannot undo that operation, but prevents subsequent checks. Publish changed capabilities only after healthy validation and retain the previous catalogue on failure without pretending the executable was restored.

This narrowly extends [the harness-owned update decision](2026-09-26-harness-owned-runtime-updates.md): its explicit/manual update and unchanged-version failure behavior remain intact. The automatic no-change exception does not apply to manual jobs or other external runtimes.

It also amends [ADR-2026-10-01](2026-10-01-agent-runtime-update-authority.md) for built-in `omp-acp` only. That decision restricts automatic updates to runtimes with verified safe staging, candidate isolation and restoration, and keeps vendor self-update commands manual. This decision accepts an in-place vendor updater with no isolated candidate and no restoration, because the operator explicitly opts in after an in-place and no-rollback warning. No other external runtime gains automation from it.

## Consequences

Configured-channel updates work without discovering a machine-readable channel release feed. A healthy no-change check invokes the updater and ACP probe every eligible interval; it is not evidence that stable latest or every possible release was installed. An updater that declines an installation (for example a Nix-owned one) and exits zero is indistinguishable from "no new release", so a no-change result keeps a bounded output excerpt and its copy says OMP reported no version change, never up to date. Outcomes retain observed versions and never invent an exact target. A probe failure may leave an unusable installation and requires operator recovery. Current processes are not restarted; future launches use the installation OMP changed. A manual OMP update leaves consent unchanged, unlike a pinned manual update, because OMP has no selection to invalidate.

## Alternatives Considered

- Stable-release gating: rejected by the user because it misses updates on the configured channel.
- Isolated activation: not selected because it requires a new installer/activation boundary rather than reusing OMP's supported updater.
- Parse human-readable `omp update --check` output: rejected because exit status does not reliably distinguish channel availability and text parsing is not a stable contract.
- Pause on the first updater error: rejected because a periodic, network-dependent updater fails transiently (offline or registry errors) and one such failure would silently end the feature. Bounded retry at the normal interval, pausing after three consecutive failures, limits repeated mutation while tolerating transient failure.
