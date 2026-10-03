# ADR-2026-10-03-opt-in-provider-limit-recovery: Opt-In Limit Recovery for Concrete Profiles

**Status:** accepted
**Date:** 2026-10-03
**Area:** backend, frontend

## Context

Issue #3811 reports that a provider rate limit or quota stops a Kanban session
on a manual card. The card persists even when the profile has a fallback model
and the provider reported a reset. With several parallel sessions on one
subscription, every session stops within minutes and must be restarted by
hand after the reset.

The relevant rules were set by earlier decisions.
[ADR-2026-07-29](2026-07-29-agent-stall-user-controlled-recovery.md) and
[ADR-2026-08-08](2026-08-08-provider-neutral-agent-error-recovery.md) keep
concrete Kanban recovery user-controlled, apart from a short same-model retry.
[ADR-2026-08-13](2026-08-13-dynamic-agent-profile-routing.md) gives
cross-candidate fallback, reset waits, and shared health to dynamic profiles
behind an experimental flag. Office runs have their own provider health and
parking. A concrete profile's `FallbackModel` applies only when the requested
model is missing from the start catalog.

## Decision

A concrete agent profile may opt into two limit recoveries. Both are off by
default:

- **Use fallback model when limited.** This applies the profile's explicit
  fallback after a limit failure. It is never allowed when Require exact model
  or automatic fallback is on. The fallback must be advertised, and every
  switch records one durable model-selection warning.
- **Resume after reset.** This resumes the same session, or requeues the same
  Office run, after a trusted reset no more than seven days away.

Every limit failure on a concrete profile attempts to record a shared limit
mark; it becomes active for recovery only after the durable circuit-store write
succeeds. The mark is keyed by the existing credential binding fingerprint,
with `account` or `model` scope, and is stored in the existing resource circuit
table. Recording does not depend on `features.dynamicAgentRouting`. If the
write fails, do not act on that unpersisted update: preserve the existing
failure surface and any prior durable mark. Only opted-in profiles change
automatic behavior because of a durable mark. Automatic launches use an
eligible fallback when available, wait only when Resume after reset is on and
a reset is trusted, and otherwise follow existing admission rules with the
mark retained. Manual launches and prompts on any profile always keep the
requested model and show a notice without deferral.

Kanban session waits use independently keyed entries in the task's
`deferred_launch` metadata, so several sessions on one task can wait
concurrently. Automatic launch deferral keeps the existing single task-owned
launch slot and explicit conflict behavior. Office waits reuse run parking and
`earliest_retry_at`. Dynamic profiles keep their conductor policies, and
concrete candidates ignore the new switches.

The exact probe lease is persisted with its waiting session or launch and
transferred to the started session's turn owner. A restart restores that token;
a stale owner cannot release a later probe's lease.

## Consequences

- Opted-in concrete profiles get automatic recovery without enabling
  experimental dynamic routing. Upgraded installations behave as before.
- The circuit registry and binding resolver now have a consumer outside
  dynamic routing. Their schema and restore path must stay available when the
  dynamic flag is off.
- Account-wide limits stop fallback attempts that cannot succeed. A limit with
  an unknown scope may cost one fallback attempt that hits the same account
  limit.
- A profile whose credential binding is unprovable shares marks with no other
  profile.
- A trusted reset up to seven days away can hold opted-in work for days. A
  longer known reset remains the mark expiry and is shown, but does not create
  an automatic wait. New automatic launches follow existing admission rules.

## Alternatives Considered

- **Enable dynamic routing for this case.** Rejected: it needs an experimental
  flag and a dynamic profile with a different concrete candidate. It does not
  model a profile-local fallback model.
- **Turn on automatic recovery by default.** Rejected: it would change upgraded
  installations without consent, and the issue requires a fallback that is
  "configured and enabled".
- **Add a new provider-health table shared with Office.** Rejected: the
  circuit store already provides durable, binding-scoped expiry with exclusive
  probes. `office_provider_health` is workspace-scoped and records quota at
  provider scope.
- **Add a configurable maximum wait.** Rejected by product decision: the
  existing seven-day routing bound applies.
