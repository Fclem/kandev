# ADR-2026-10-03-session-user-activity-ledger: Session user-activity ledger and effort-signal reads

**Status:** proposed
**Date:** 2026-10-03
**Area:** backend, frontend, protocol

## Context

A story-point estimator plugin must weigh how much human effort a task took.
Kandev records agent turn time, messages, code statistics, and pull-request
rows, but no measurement of a person's engagement with a session. The plugin
Host data API also cannot tell a prompt a person wrote from one that a workflow,
an automation, or another task's agent generated, and it lacks turn timing,
changed-file counts, GitLab merge requests in `Task.PullRequests`, and pull-request
review and comment content.

## Decision

Add a core session user-activity ledger. The browser reports foreground
interaction as a timestamp-free beat over WebSocket; the server merges beats on
its own clock into per-session active intervals. Session creation records whether
capture was enabled; measured state is computed at read time from that stamp and
persisted capture-off gaps. The feature ships behind a restart-required runtime
flag that is off in every profile.

Expose the effort facts through additive, capability-gated Host reads rather
than a core estimator or a core story-point field: message `origin`, a session
activity and turn read behind a new `session_activity` resource (personal
engagement data must not reach plugins that only hold `sessions`), file counts
and PR size and timing fields, GitLab merge requests in `Task.PullRequests`, and
an exact change-request feedback read behind the existing workspace approval.
The estimator, its formula, and its pages live in an external plugin that uses
existing contribution points; no stats-page slot is added.

Measured and derived data stay separate. The ledger never backfills; a plugin
may derive a labelled proxy from message and turn timestamps for older sessions.

## Consequences

Effort analysis gets a measured signal, and consumers can distinguish unmeasured
history from measured zero. Core carries one small additive table, one runtime
flag, and one WebSocket action, plus about six additive Host reads. The
estimate is plugin-owned and is not shared task metadata, so core views cannot
sort by it.

Feedback reads make live provider calls under the user's credentials. The read
bounds itself to 20 tasks, 50 items per task, and a 7 MiB encoded budget, because
the exact-read snapshot caps (5000 rows, 8 MiB) apply to JSON-encoded size.
Plugins that read comment bodies receive third-party content and are
approval-gated.

## Alternatives Considered

- **Derive activity from messages and turns only:** No new capture, works on
  history, but cannot see reading, reviewing, or editing between prompts.
  Retained only as the plugin's labelled fallback.
- **Heartbeat with raw client timestamps:** Simpler client, but trusts client
  clocks and invites inflation from suspended machines.
- **Persist beats and aggregate later:** Unbounded growth; merged intervals are
  the smallest durable form.
- **Add a story-point field to the task model:** Couples core to one planning
  method and conflicts with the direction that provider-specific data lives in
  plugin state.
- **Add a core stats-page slot:** Gives range-selector reuse but widens core UI
  extension for one consumer; `host.ui` primitives already reproduce the style.
- **Filter `origin` in SQL:** Needs JSON metadata predicates with SQLite and
  Postgres parity for no benefit over plugin-side filtering.
- **Browser-only plugin:** Cannot compute or persist estimates when no browser
  is open.

## Related contracts

- [Session user activity requirements](../specs/tasks/requirements/session-user-activity.md)
- [Session user activity design](../specs/tasks/system-design/session-user-activity.md)
- [Effort signals requirements](../specs/plugins/requirements/effort-signals-host.md)
- [Effort signals design](../specs/plugins/system-design/effort-signals-host.md)
- [ADR 0043: capability-gated Host data API](0043-plugin-host-data-api.md)
- [ADR 0047: plugin conversation reads](0047-plugin-host-conversation-reads.md)
