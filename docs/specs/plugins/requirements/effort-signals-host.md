---
status: draft
system: plugins
created: 2026-10-03
owners:
  - kandev
---

# Effort Signals Host Requirements

## Overview

A plugin that estimates story points from how much human effort a task took
needs facts the Host data API does not expose today: which prompts a person
wrote, how long a person worked in each session, per-turn timing, changed-file
counts, and pull-request activity. This capability adds those facts as
additive, capability-gated reads. The estimator itself, its heuristic, and its
pages live in an external plugin repository and are not specified here.

The plugin system owns these reads because it owns the Host data API and its
authorization. The [task system](../../tasks/requirements/session-user-activity.md)
owns the activity signal that one read projects.

## Terminology

- **Prompt origin:** The provenance of a user-authored message: `human`,
  `workflow`, `automation`, `peer_task`, or `plugin`.
- **Effort signal:** A fact about human or agent work on a task that an
  estimator can consume.
- **Activity rollup:** Per-session measured user active time; human prompt counts
  are derived separately from message origin.

## Requirements

### REQ-PLUGINS-EFFORT-SIGNALS-001: Prompt provenance

**Intent:** Let a plugin count prompts a person wrote and exclude prompts that
Kandev, a workflow, or another task's agent generated.

**User story:** As a plugin author, I want each user-authored message to carry
its origin, so that I can count human prompts without parsing message text.

#### Acceptance criteria

- **AC-PLUGINS-EFFORT-SIGNALS-001.1:** Each message returned by
  `Messages().List` shall carry a non-empty `origin` of `human`, `workflow`,
  `automation`, `peer_task`, `plugin`, or `agent`, so an empty value means the
  Host predates the field.
- **AC-PLUGINS-EFFORT-SIGNALS-001.2:** The origin of a user-authored message
  shall be the first matching rule of: sent by another task's agent
  (`peer_task`); sent through a plugin (`plugin`); created by pull-request,
  merge-request, or CI automation (`automation`); created by workflow step
  automation (`workflow`), even though it also carries an auto-start marker;
  created by any other automated trigger (`automation`).
- **AC-PLUGINS-EFFORT-SIGNALS-001.3:** A user-authored message matching none of
  those rules shall have origin `human`; an agent-authored message shall have
  origin `agent`.
- **AC-PLUGINS-EFFORT-SIGNALS-001.4:** `MessageFilter` shall accept
  `AuthorTypes` and return only messages whose author type matches; an empty
  list shall be unconstrained.
- **AC-PLUGINS-EFFORT-SIGNALS-001.5:** The origin field shall be derived at read
  time and shall not alter stored messages, and no new metadata shall leak into
  the sanitized DTO.
- **AC-PLUGINS-EFFORT-SIGNALS-001.6:** A message queued and later delivered
  shall be counted once with the origin of its original submission.

### REQ-PLUGINS-EFFORT-SIGNALS-002: Session activity and turn timing

**Intent:** Expose measured user active time and agent turn timing per session.

#### Acceptance criteria

- **AC-PLUGINS-EFFORT-SIGNALS-002.1:** `Sessions().UserActivity` shall return,
  per requested session, active milliseconds, interval count, first and last
  activity timestamps, and an `activity_measured` flag. Active milliseconds
  shall be the sum of per-actor active time, so sessions worked by several
  people report aggregate person-time.
- **AC-PLUGINS-EFFORT-SIGNALS-002.2:** For a session whose activity is not
  measured, the read shall return zero values with `activity_measured=false`
  rather than omitting the session or reporting measured zero.
- **AC-PLUGINS-EFFORT-SIGNALS-002.3:** `Sessions().Turns` shall return turn ID,
  session ID, start time, and nullable completion time for requested sessions in
  deterministic `(started_at, id)` order with opaque cursors.
- **AC-PLUGINS-EFFORT-SIGNALS-002.4:** Both reads shall accept session IDs and
  task IDs and fail with `InvalidArgument` when the two lists together exceed
  400 values.
- **AC-PLUGINS-EFFORT-SIGNALS-002.5:** Both reads shall require the
  `api_read: session_activity` resource. A plugin that declares only
  `sessions` shall receive `PermissionDenied`, and declaring the new resource
  shall be visible in the plugin's capability review.
- **AC-PLUGINS-EFFORT-SIGNALS-002.6:** Turn metadata, runtime configuration,
  and usage payloads shall not appear in the turn DTO.

### REQ-PLUGINS-EFFORT-SIGNALS-003: Code-change and pull-request summary

**Intent:** Give an estimator a file-count and pull-request-timing signal that
today's line-only statistics lack.

#### Acceptance criteria

- **AC-PLUGINS-EFFORT-SIGNALS-003.1:** `SessionCodeStats` shall add committed
  and peak-pending changed-file counts, with the same availability semantics as
  the existing line counts.
- **AC-PLUGINS-EFFORT-SIGNALS-003.2:** The documentation shall state whether each
  file count is a sum of per-commit counts or a distinct-path count, and the
  semantics shall not change after release.
- **AC-PLUGINS-EFFORT-SIGNALS-003.3:** `TaskPullRequest` shall add creation
  time, changed-file count, approved-reviewer count, and a size-observed flag
  without changing any existing field. Review-comment text counts are provided
  only by the feedback read.
- **AC-PLUGINS-EFFORT-SIGNALS-003.4:** `ChangedFiles` and
  `ApprovedReviewerCount` shall carry no value for a request that has never
  been synced, which a plugin can distinguish from zero. `Additions` and
  `Deletions` of a GitLab merge request, whose stored row has no size, shall be
  reported with size-observed false rather than as a measured zero.
- **AC-PLUGINS-EFFORT-SIGNALS-003.5:** A plugin built against an earlier SDK
  shall continue to compile and decode these messages.
- **AC-PLUGINS-EFFORT-SIGNALS-003.6:** `Task.PullRequests` shall include a
  task's linked GitLab merge requests with provider `gitlab`. `PullRequestsPartial`
  shall be presence-aware: a new Host shall set false when all provider sources
  were read and true when any source cannot be read; an older Host's omitted
  field shall remain distinguishable as unknown. A source failure shall not fail
  the task read or silently hide the partial result.

### REQ-PLUGINS-EFFORT-SIGNALS-004: Change-request feedback

**Intent:** Let an estimator see review and discussion activity on a task's
pull or merge requests, provider-neutrally.

#### Acceptance criteria

- **AC-PLUGINS-EFFORT-SIGNALS-004.1:** A new exact read shall return, for each
  requested task, its linked GitHub pull requests and GitLab merge requests'
  reviews and comments with provider, repository, change-request number, kind,
  author login, author kind (`human`, `bot`, or `unknown`), review state,
  resolved state when known, creation and update times, and sanitized body.
  Each task result shall include a provider status for each provider association
  source queried. An update time shall equal the creation time when the provider
  reports none.
- **AC-PLUGINS-EFFORT-SIGNALS-004.2:** Each body shall be limited to 4096 bytes
  on a UTF-8 boundary and carry a `body_truncated` flag.
- **AC-PLUGINS-EFFORT-SIGNALS-004.3:** The read shall require the
  `change_requests` read resource and its separate workspace approval; a
  declared resource without approval shall be denied.
- **AC-PLUGINS-EFFORT-SIGNALS-004.4:** A task result shall report provider
  availability separately for each queried association source: `available`
  when the source and feedback read succeeded (including no linked requests),
  `unavailable` when the source or feedback read failed, or `unsupported` when
  associated requests exist but no feedback adapter is implemented. One
  provider's failure shall not hide another provider's successful items; no
  provider failure shall fail the whole page or return stale data as current.
- **AC-PLUGINS-EFFORT-SIGNALS-004.5:** The read shall page deterministically
  using a total item order of `(created_at, provider, repository_id, number,
  kind, item_id)`, fail with `ResourceExhausted` above 20 requested tasks, and
  return at most the 50 newest items across providers per task, further limited
  so the encoded page stays within the snapshot byte cap. Each task's provider
  status shall carry its own omitted-item count so truncation and partial
  availability remain attributable; one busy change request cannot fail the
  read.
- **AC-PLUGINS-EFFORT-SIGNALS-004.6:** Bodies, author logins, and provider
  tokens shall not appear in logs or metric labels.
- **AC-PLUGINS-EFFORT-SIGNALS-004.7:** A task outside the approved workspace
  shall be rejected without revealing whether it exists.

### REQ-PLUGINS-EFFORT-SIGNALS-005: Compatibility and consumer proof

**Intent:** Ship the reads as additive contracts proven by a consumer that uses
only public surfaces.

#### Acceptance criteria

- **AC-PLUGINS-EFFORT-SIGNALS-005.1:** Every addition shall be additive under
  the current plugin API version and keep the Go SDK, proto, gRPC server, and
  documentation in sync. Only a manifest that declares `session_activity`
  shall require a minimum Kandev version, and that floor shall be the release
  that ships every addition of this capability. A new RPC on an older Host shall
  answer `Unimplemented`; a new field on an existing RPC shall be detectable as
  absent (`origin` empty, optional counts unset, `pull_requests_partial` unset).
- **AC-PLUGINS-EFFORT-SIGNALS-005.2:** A fixture plugin shall use only public
  SDK surfaces to read origins, activity rollups, turns, code and
  pull-request summaries, and change-request feedback, and shall persist a
  derived value in its own plugin state.
- **AC-PLUGINS-EFFORT-SIGNALS-005.3:** The fixture shall render a plugin-owned
  route, a task panel, and a task-card indicator from those reads using only
  existing contribution points and `host.ui` primitives, with no new core slot.
- **AC-PLUGINS-EFFORT-SIGNALS-005.4:** Public plugin documentation shall list
  each new read, its capability, bounds, and failure states.

## Out of scope

- The estimation heuristic, calibration, story-point scale, and plugin pages.
- A first-class story-point field on tasks, or writing estimates into core task
  records.
- A new slot inside the core `/stats` page.
- Capturing user activity, which the
  [task system](../../tasks/requirements/session-user-activity.md) owns.
- Providers other than GitHub and GitLab.
- A historical replay API for events.
