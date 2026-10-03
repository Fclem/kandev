---
status: draft
system: agents
created: 2026-10-03
owners:
  - kandev
---

# Provider Limit Recovery Requirements

## Overview

A provider can stop a model because of a rate limit, a usage window, exhausted
credits, or a spend ceiling. Kandev classifies many of these failures and
often knows the reset time. Today a Kanban session still stops on a manual
recovery card. The profile's configured fallback model is used only when a
session starts, and the reset time does not schedule anything. When several
parallel sessions share one account, the user must restart each one by hand
after the reset.

This capability lets a user opt a concrete agent profile into two recoveries.
With the first, Kandev continues on the configured fallback model when a limit
stops the profile's model. With the second, Kandev resumes the stopped work
after a trusted reset. Kandev records a shared limit mark so that other work
using the same account and model does not launch into a known limit.

The agent system owns this contract because it owns provider classification,
profile model policy, and the agent capability surface that task and Office
execution consume. It amends three documents:
[no silent model fallback](no-silent-model-fallback.md) for post-failure
fallback, [agent stall recovery](agent-stall-recovery.md) for scheduling a
Kanban retry from a reset time, and
[Claude session-limit classification](claude-session-limit-classification.md)
for opted-in fixed profiles. All other rules in those documents still apply.

## Terminology

- **Limit failure:** a terminal provider failure classified as `quota_limited`.
  It also includes a `rate_limited` failure that the existing short same-model
  retry does not absorb.
- **Limit scope:** `account` applies to every model reached through one
  credential binding. `model` applies to one model reached through that
  binding.
- **Credential binding:** the non-secret identity of the account that a
  concrete profile uses, as defined by dynamic routing. A profile whose binding
  cannot be proven forms its own isolated binding.
- **Limit mark:** a durable record that a credential binding and limit scope
  are limited until a stated instant.
- **Trusted reset:** an instant that comes from structured provider data or a
  recognized provider notice. It is later than the observation time and at
  most seven days after it.
- **Opted-in profile:** a concrete profile with **Use fallback model when
  limited**, **Resume after reset**, or both enabled.
- **Eligible limit fallback:** the profile's explicit fallback model, when Use
  fallback model when limited is on, Require exact model is off, automatic
  fallback is off, and the fallback model is not empty.
- **Automatic launch:** work that Kandev starts without a user action at that
  moment. Examples are workflow auto-start, queued-message drain, replay of a
  deferred launch, automation runs, and Office scheduler dispatch.

## Requirements

### REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-001: Per-profile limit recovery settings

**Intent:** Each concrete profile states whether it falls back or resumes after
a provider limit. Upgrades preserve today's behavior.

**User story:** As a Kandev user, I want to choose limit recovery for each
agent profile, so that unattended work continues only where I allowed it.

#### Acceptance criteria

- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.1:** When a concrete profile is
  created, or an existing profile is upgraded, Use fallback model when limited
  and Resume after reset shall both be off.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.2:** The Fallback settings of the
  profile editor and the profile settings page shall show both switches with
  localized helper text. They shall be keyboard and touch operable on desktop
  and phone, with no horizontal document overflow.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.3:** When Require exact model or
  automatic fallback is on, or the fallback model is empty, Use fallback model
  when limited shall be disabled. Its saved value shall be retained, it shall
  be shown as inactive, and runtime shall treat it as off. Resume after reset
  shall remain available on every concrete profile, including strict profiles.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.4:** Save, reload, duplication, and
  supported profile export and import shall preserve both values. A partial
  update that omits a value shall preserve the saved value.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.5:** The collapsed Fallback settings
  summary shall state when limit recovery is on, and unsaved changes to either
  switch shall mark the section as dirty.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.6:** Dynamic profiles shall not show
  either switch. When a concrete profile runs as a dynamic candidate, both
  switches shall be ignored and the dynamic conductor's policy shall apply.

### REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-002: Shared limit marks

**Intent:** A known limit is recorded once and is visible to all later work
that would use the same account and model.

#### Acceptance criteria

- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.1:** When a concrete Kanban session
  or Office run ends with a limit failure, Kandev shall record a limit mark for
  its credential binding and limit scope. It shall do this whether or not the
  profile is opted in.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.2:** A recognized account-wide limit
  shall mark the `account` scope. Account-wide limits include a subscription
  session or usage window, exhausted credits, and a spend ceiling. Any other
  limit failure, including one with an unknown scope, shall mark the `model`
  scope for the model that failed.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.3:** A mark with a trusted reset
  shall last until that reset. A mark with a known reset more than seven days
  away shall last until that reset. A mark with no known reset shall last
  30 minutes.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.4:** A model shall count as limited
  while its model-scope mark or its binding's account-scope mark is active.
  A different credential binding shall never inherit the mark. If a profile's
  binding cannot be proven, the mark shall apply only to that profile.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.5:** When a turn on a binding and
  model completes successfully, Kandev shall clear that model-scope mark and
  the binding's account-scope mark.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.6:** When a mark's reset passes and
  automatic work is waiting on it, only one waiting session or run shall
  resume first as a probe. The others shall resume after the probe completes a
  turn successfully. If the probe hits the limit again, the mark shall be
  renewed and the others shall keep waiting. If the probe produces no result
  within 10 minutes, another waiter may probe.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.7:** Marks shall survive a backend
  restart. A mark shall persist only the opaque binding key, the scope, the
  model identifier, the error code, the reset or expiry instant, and
  timestamps. It shall not persist provider text, credentials, or prompts.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.8:** On Settings > Agents, a profile
  row whose model is limited shall show a localized `limited until <time>`
  indicator. Desktop and phone shall both show it, and it shall disappear
  when the mark expires or clears.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.9:** An agent can route several
  upstream providers through one profile with `<provider>/<model>`
  identifiers, as OMP and OpenCode do. For such an agent, an account-scope
  mark shall limit only the binding's models of the failed model's provider,
  so a fallback to another provider stays eligible. A failed model identifier
  without a provider prefix shall use model scope.

### REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-003: Visible fallback after a limit

**Intent:** An opted-in Kanban session continues on its configured fallback
model without user action, and the switch remains visible.

#### Acceptance criteria

- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.1:** A concrete Kanban session can
  end its turn with a limit failure on a model other than its fallback. When
  its profile has an eligible limit fallback that the current session catalog
  advertises and that is not limited, Kandev shall switch the session to that
  model. It shall continue the same session and conversation without user
  action.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.2:** If the failed turn produced no
  turn event, the continuation shall send the same user input once. Otherwise
  it shall send a continuation instruction and shall not send the original
  input again.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.3:** Each fallback decision shall add
  one durable model-selection warning naming the requested model, the
  effective model, and the provider limit reason. Reload and event replay
  shall not duplicate the warning. A later, separate decision shall add its
  own warning.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.4:** A strict profile, a profile with
  automatic fallback on, and a profile without an eligible limit fallback shall
  never switch models after a limit failure. Kandev shall never send a model
  that the session catalog does not advertise.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.5:** A failed turn shall cause at
  most one automatic fallback. Each of these outcomes shall continue under
  REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-004: the fallback is not advertised, it
  is limited, the switch fails, or the fallback turn also hits a limit.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.6:** After a fallback, the session
  shall stay on the fallback model until the user changes it or a new session
  starts. Cancelling, stopping, archiving, or deleting during the switch shall
  prevent the continuation.

### REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-004: Automatic resume after a trusted reset

**Intent:** Opted-in work stopped by a limit resumes by itself after the reset,
in its own session.

#### Acceptance criteria

- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.1:** A Kanban session on a profile
  with Resume after reset can end with a limit failure that no fallback
  resolves. When the failure has a trusted reset, the session shall show a
  waiting notice with the model, the localized reset time, and a **Cancel**
  action. It shall resume the same session on the limited model after the
  reset, following the continuation rule in AC 003.2.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.2:** When the provider supplies a
  retry delay in milliseconds, the reset shall equal the observation time plus
  exactly that delay. Kandev shall not resume before the reset instant.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.3:** When the reset is unknown, is
  more than seven days away, or Resume after reset is off, Kandev shall show
  the existing recovery card and shall not resume automatically. The card
  shall include a known reset time.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.4:** A wait shall survive a backend
  restart. A wait whose reset passed during downtime shall resume after
  startup, subject to the probe rule in AC 002.6.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.5:** Each of these actions shall
  cancel the wait and prevent the resume: **Cancel**, a manual prompt, stopping
  the session, moving the task to another workflow step, archiving the task, or
  deleting the task. A manual prompt shall be dispatched normally.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.6:** A session shall make at most
  three consecutive automatic waits without a successful turn. The next limit
  failure shall show the existing recovery card.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.7:** When a workflow step's
  automatic start fails on a limit, the step shall resume under the same rules.
  Its entry prompt shall be delivered exactly once.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.8:** A waiting session shall not
  occupy a session-ceiling slot. Its resume shall pass the same automatic
  launch admission as other automatic launches.

### REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-005: Automatic launches avoid known limits

**Intent:** While a limit is known, opted-in automatic work does not launch into
it. Users keep control of manual work.

#### Acceptance criteria

- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.1:** An automatic launch can target
  an opted-in profile whose model is limited. If the profile has an eligible
  limit fallback that is not limited, the launch shall start on the fallback.
  It shall show the warning defined in AC 003.3.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.2:** If no fallback applies, Resume
  after reset is on, and the mark has a trusted reset, Kandev shall defer the
  launch durably until that reset. The task shall show a localized `waiting
  for <model> limit reset at <time>` state. Otherwise the launch shall
  proceed.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.3:** A deferred launch shall replay
  once through the same admission gates as other automatic launches. A task
  that leaves the step, is archived, or is deleted shall drop the deferral
  without launching.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.4:** Manual launches and manual
  prompts shall use the requested model even while it is limited. The session
  shall show a non-blocking `limited until <time>` notice.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.5:** Launches on profiles that are not
  opted in shall keep their existing behavior while a mark is active.

### REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-006: Office runs

**Intent:** Office runs on opted-in profiles fall back and resume under the
same profile policy and shared marks, while keeping Office ownership of runs.

#### Acceptance criteria

- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-006.1:** An Office run can end with a
  limit failure on a concrete profile that has an eligible limit fallback. If
  the fallback is not limited, Kandev shall requeue the run once on the same
  execution profile with the fallback model. It shall do this before advancing
  to another routed provider or escalating the failure. The route attempt
  shall record the requested model, the effective model, and the reason.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-006.2:** A fallback launch shall apply
  the fallback model before inference. If the executor does not advertise the
  fallback, the launch shall fail before inference and continue under AC 006.3.
  It shall not continue on another model.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-006.3:** When no fallback applies, Resume
  after reset is on, and the mark has a trusted reset, the run shall park until
  that reset. The scheduler shall then dispatch it automatically, subject to
  the probe rule in AC 002.6.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-006.4:** Office dispatch for an opted-in
  profile whose model is limited shall follow AC 005.1 and AC 005.2. Runs on
  profiles that are not opted in shall keep existing Office routing, health,
  inbox, and retry behavior.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-006.5:** Clearing Office provider health
  shall not clear a limit mark, and clearing a limit mark shall not clear
  Office provider health.

### REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-007: Limit timing and scope evidence

**Intent:** Provider limit reports give a reset and a scope wherever the
harness exposes them.

#### Acceptance criteria

- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.1:** When a provider error carries a
  structured reset or retry delay, Kandev shall use it in this order: an
  absolute reset, a millisecond delay, a delay in seconds, and then a reset
  derived from the notice text. A millisecond delay shall keep millisecond
  precision.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.2:** An Anthropic `rate_limit_error`
  reported by `claude-acp` or `omp-acp` shall be classified as `rate_limited`.
  If the error reports a spend limit or `enforced_spend_limit_reached`, it
  shall be classified as `quota_limited` with account scope.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.3:** These limits shall carry account
  scope: Claude session limits, Codex usage limits, OpenCode period-usage and
  credit exhaustion, and Claude credit exhaustion. Other recognized limits
  shall carry model scope.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.4:** Unrecognized text, malformed
  timing, and timing for another session or prompt generation shall not
  create a reset. Existing classifications for other providers shall remain
  unchanged.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.5:** OMP reports a provider failure
  as assistant text and then ends the prompt with `end_turn`. When the last
  assistant message chunk of an `omp-acp` prompt is a strict Anthropic error
  envelope (`{"type":"error","error":{"type":"rate_limit_error",...}}`),
  Kandev shall end the turn as a terminal provider failure, not as a
  successful completion. A valid `retry-after-ms=<n>` suffix shall supply the
  millisecond delay. Ordinary assistant text, a quoted envelope inside other
  text, an envelope followed by more assistant output, and the same text from
  another agent shall not become a failure.
- **AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.6:** Persisted and displayed
  diagnostics shall not contain a provider request identifier such as
  `req_<id>`. A retry delay shall be kept only as a number.

## Out of scope

- Dynamic profiles and their candidates, which their conductor policies own.
- Adding a fallback chain longer than one explicit fallback model.
- Switching a session back to the profile model after its reset.
- A configurable maximum wait, a global or workspace switch, or a release
  toggle.
- Provider usage meters, subscription dashboards, or prediction before a limit
  is reached.
- Interrupting or changing the model of a running turn.
- New limit signatures without an observed or documented provider message.

## System design

- [Provider limit recovery design](../system-design/provider-limit-recovery.md)

## Implementation plans

- [Provider limit recovery](../../../plans/provider-limit-recovery/plan.md)
