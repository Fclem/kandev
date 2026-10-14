---
status: active
system: agents
created: 2026-10-10
updated: 2026-10-14
owners:
  - Kandev
---

# Periodic OMP updates requirements

## Overview

Operators who run Oh My Pi (OMP) want it to stay current without opening Settings. Codex-style automation stages and validates an isolated exact version. OMP only offers an in-place updater that follows its configured channel, so this contract defines an explicit, opt-in exception for that harness-owned updater. Agents owns it because runtime consent, trusted update commands, maintenance exclusion, and capability publication belong to the agent runtime system.

This contract extends [runtime update notifications and automation](runtime-update-notifications.md) and [harness-owned runtime updates](runtime-updates.md). AC-AGENTS-RUNTIME-NOTIFY-002.2, 002.3, 002.4 and 002.6, and the notifications requirement's native/vendor activation exclusion, apply to the Kandev-managed runtime path. OMP's trusted in-place exception follows this contract instead; its passthrough command is the same `omp` binary the updater replaces. The exception does not extend automatic eligibility to any other externally owned agent.

## Terminology

- **Check:** One automatic run of OMP's trusted updater followed by an ACP validation of the resulting installation.
- **In place:** The updater modifies the installation OMP resolves on the host. Kandev cannot restore the previous executable.
- **Paused:** Consent is saved as enabled, but Kandev starts no further automatic checks.

## Requirements

### REQ-AGENTS-RUNTIME-NOTIFY-004: Opt-in periodic OMP updates

**Intent:** Keep an operator's OMP current on its configured channel while stating the in-place limitation before consent.

**User story:** As an operator, I want OMP to update itself periodically after I opt in, so that I do not have to open Settings to receive new releases.

#### Acceptance criteria

- **AC-AGENTS-RUNTIME-NOTIFY-004.1:** Automatic OMP updates shall be off initially. Only an operator with agent configuration permission shall be able to save or withdraw install-wide consent. Saved consent shall survive restart.
- **AC-AGENTS-RUNTIME-NOTIFY-004.2:** While OMP is enabled, available, opted in and not paused, Kandev shall start a check on the first eligible background pass and then at most once per fifteen-minute background interval, never less than fourteen minutes after the background pass that admitted the previous attempt. A restart shall not allow an attempt sooner than that. A deferral because maintenance is active shall not consume the interval.
- **AC-AGENTS-RUNTIME-NOTIFY-004.3:** A check shall follow OMP's own configured channel. Kandev shall not choose a channel or target version, and an unavailable, equal or older upstream stable reference shall not prevent a check.
- **AC-AGENTS-RUNTIME-NOTIFY-004.4:** When the updater succeeds and ACP validation reports a known version different from the previous known version, Kandev shall refresh capabilities and retain the previous and observed versions until a later change replaces them. When the previous version was unknown, Kandev shall record the validated version without claiming a change.
- **AC-AGENTS-RUNTIME-NOTIFY-004.5:** When the updater succeeds and ACP validation reports the same known version, Kandev shall record that OMP reported no version change. It shall not report this as a failure, as an installed release, or as proof that OMP is up to date, and it shall retain an excerpt of the updater output: the final 2 KiB as plain text, with escape sequences and control characters other than newline and tab removed.
- **AC-AGENTS-RUNTIME-NOTIFY-004.6:** When the updater exits with an error or reaches its time limit, or an attempt is interrupted by backend shutdown or process loss, Kandev shall retain a failed or interrupted result and keep its last validated capabilities. It shall try again at the normal interval and shall pause after three consecutive failed or interrupted checks. Backend shutdown shall cancel a running update and, on POSIX hosts, the package-manager processes it started other than any that detached themselves, shall not wait for natural completion, and shall return within a bounded time. On Windows hosts only the updater process is stopped.
- **AC-AGENTS-RUNTIME-NOTIFY-004.7:** When the updater succeeds but ACP validation fails, reports that authentication is required, times out or reports no version, Kandev shall pause automatic checks across restart until the operator saves opt-out and then saves opt-in. It shall keep its last validated capabilities and state that the installation may already have changed. A slow updater shall not shorten the time available for validation.
- **AC-AGENTS-RUNTIME-NOTIFY-004.8:** Before the updater starts, Kandev shall revalidate consent, trusted update recipe, agent availability and enablement, and exclusive maintenance ownership. If revalidation fails, Kandev shall start no updater, record no failure or pause, and leave the interval unconsumed. Opt-out saved after the updater starts shall stop later checks and shall not claim to undo the running update.
- **AC-AGENTS-RUNTIME-NOTIFY-004.9:** Concurrent checks and manual maintenance for OMP shall not run competing updaters. A manual OMP update shall not change saved consent or a pause. Existing sessions shall not be restarted and remote or container installations shall not be updated.
- **AC-AGENTS-RUNTIME-NOTIFY-004.10:** Changed results and pausing failures shall use the existing runtime-outcome notifications, preferences and per-attempt deduplication. Validated-version, no-change and non-pausing failure results shall create no notification. No OMP notification shall offer version selection, rollback or Kandev-default recovery.
- **AC-AGENTS-RUNTIME-NOTIFY-004.11:** Settings shall show the OMP switch, the configured-channel check interval, the in-place and no-rollback warning, and the latest result (changed, validated version, no change, running, failed or interrupted) with previous and observed versions but no target version. It shall also show the latest changed result until a later change replaces it, and the retained updater output excerpt as plain text in a collapsed disclosure for no-change and failed results. A paused state shall explain the opt-out, save, opt-in, save recovery and shall disappear once recovery is saved. The OMP row shall not show guidance that contradicts the switch, and the installed-agent card shall not present an automatic check as a manual update result, replace a finished manual result, or claim that OMP is up to date. A check that did not run shall show nothing. A failed save shall preserve the authoritative saved consent. All copy shall be localized in every shipped locale.
- **AC-AGENTS-RUNTIME-NOTIFY-004.12:** On a phone, the same switch, warning, results and the link to OMP's manual update control shall be reachable with at least 44px touch targets, one page scroll owner and no horizontal page overflow.

## Out of scope

- Selecting a channel or version, rollback, staged installation, guessed installers, or an automatic force update.
- Changing the manual OMP update contract or the pinned Codex automation contract.
- Updating a task container, remote executor or active process.

## Design and implementation

See the [OMP automatic updates design](../system-design/omp-automatic-updates.md), the [decision](../../../decisions/2026-10-10-periodic-omp-self-updates.md) and the [implementation plan](../../../plans/omp-automatic-updates/plan.md).
