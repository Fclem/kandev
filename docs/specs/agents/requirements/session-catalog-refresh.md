---
status: draft
system: agents
created: 2026-11-10
owners:
  - kandev
---

# Session catalog refresh requirements

## Overview

Refreshing a saved profile's model discovery should reach its existing task
sessions without killing an agent process manually. Agents owns this capability
because it owns provider capabilities and profile identity. Tasks retains session
identity, conversation storage, and restart admission.

Source: [issue 5237 and the selected maintainer comment](https://github.com/kdlbs/kandev/issues/5237#issuecomment-6773837684).
The comment qualifies the original issue: host observations may update compatible
host-session metadata, remote observations remain executor-owned, and a restart
requires a user action. Equal model lists across executors are not promised.

This proposal adds a saved-profile publication operation. Ordinary
[profile discovery](profile-capability-discovery.md) remains a side-effect-free
editing aid. [Dynamic provider options](dynamic-provider-options.md) continues
to own effective model-dependent options and confirmed live application.

## Terms

- **Provider catalog:** Choices advertised by the running session's agent.
- **Refreshed profile catalog:** A successful probe of a saved concrete profile
  in the Kandev host context, not an unsaved draft or a separate vendor CLI list.
- **Compatible host session:** A session actually running on the host with
  matching provider and launch settings. Local containers are isolated executors,
  not host sessions. Unknown compatibility is not a match.
- **Candidate:** A host-discovered choice not yet advertised by the live agent.
  It is not proof that the live agent can select that choice.

## Requirements

### REQ-AGENTS-SESSION-CATALOG-001: Saved refresh reaches existing sessions

**Intent:** A user sees refreshed choices without silently changing active work.

#### Acceptance criteria

- **AC-AGENTS-SESSION-CATALOG-001.1:** An explicit successful model-list refresh
  for an unchanged saved concrete profile shall update the displayed available
  models and modes of its compatible host sessions, including sessions currently
  producing output and sessions waiting for input, without restarting them.
- **AC-AGENTS-SESSION-CATALOG-001.2:** Refresh shall preserve the selected model,
  mode, dependent option values, profile defaults, session identity, conversation,
  prompt queue, unsent composer draft, and active turn. A removed selected choice
  shall remain identifiable without selecting a replacement.
- **AC-AGENTS-SESSION-CATALOG-001.3:** Unsaved drafts, new profiles, automatic
  editor reads, agent-wide probes, failed probes, and separate host-CLI model
  suggestions shall not publish session updates. A profile edit during refresh
  shall prevent that older refresh from publishing.
- **AC-AGENTS-SESSION-CATALOG-001.4:** Refresh shall affect only the same concrete
  execution profile's unarchived live task sessions accessible to the caller.
  Another profile, a dynamically routed session using another concrete profile,
  a foreign workspace, or a terminal session shall remain unchanged.
- **AC-AGENTS-SESSION-CATALOG-001.5:** A remote or isolated executor shall retain
  its own discovered choices. A successful saved-profile refresh shall instead
  expose a notice that the profile was refreshed and that checking this executor
  requires an explicit session refresh. Unknown host compatibility shall use the
  same conservative notice rather than importing host choices.
- **AC-AGENTS-SESSION-CATALOG-001.6:** Published choices and notices shall survive
  reload and reconnect. Repeating an identical refresh shall not accumulate
  notices. A late older refresh or older session report shall not restore a
  superseded catalog or clear a newer refresh notice.

### REQ-AGENTS-SESSION-CATALOG-002: Explicit executor rediscovery

**Intent:** Users can refresh the running agent when its own catalog is stale,
without process management outside Kandev.

#### Acceptance criteria

- **AC-AGENTS-SESSION-CATALOG-002.1:** Choices already advertised by the live agent
  shall remain selectable through normal model/mode configuration. A host-only
  candidate shall be distinguishable as requiring session refresh. Attempting
  that candidate shall offer refresh without sending an unadvertised selection
  or claiming that the candidate is active.
- **AC-AGENTS-SESSION-CATALOG-002.2:** A notice beside the composer model selector
  shall expose a Refresh session action. Before restarting, Kandev shall explain
  that refresh restarts the agent and can interrupt its current turn. Cancel
  shall leave the runtime unchanged. No profile refresh, render, reconnect, or
  transition to idle shall restart an agent automatically.
- **AC-AGENTS-SESSION-CATALOG-002.3:** After explicit confirmation, Kandev shall
  restart and rediscover the captured session on its existing executor through
  normal resume and recovery rules, preserving its conversation, task/workspace,
  saved model/mode/config-option overrides, and unsent input. It shall not reset
  context, switch profile identity, send a prompt, or replay an uncertain
  interrupted turn to obtain a catalog. Confirmation shall state that ordinary
  resume resolves the profile's current environment, CLI flags, and wrapper;
  these launch settings are not frozen with the conversation. A restore failure
  under those inputs shall retain the saved conversation and expose recovery,
  not create a new conversation.
- **AC-AGENTS-SESSION-CATALOG-002.4:** A restart request acknowledgment alone shall
  not clear the notice or announce fresh choices. An owner-correlated terminal
  discovery result shall end checking: a complete catalog, including an explicitly
  empty or mode-only result, shall replace stale candidate choices; unsupported
  discovery shall be reported as unsupported, not as fresh choices. If no
  correlated result arrives within 60 seconds after the Resume acknowledgment,
  checking shall end with a retryable unavailable status and the retained notice,
  without stopping the session or retrying automatically. If the provider still
  omits a desired choice, Kandev shall report that the executor did not advertise
  it; normal selection shall remain limited to that executor's choices.
- **AC-AGENTS-SESSION-CATALOG-002.5:** Rejected Stop, rejected Resume, timeout,
  disconnected transport, and blocked recovery shall remain visible with the
  existing recovery actions and retained conversation. Refresh shall not retry
  automatically, act on a newly selected sibling, or permit duplicate restarts
  and conflicting submissions while restart requests are pending.

### REQ-AGENTS-SESSION-CATALOG-003: Equivalent refresh controls

**Intent:** Desktop and phone users can discover and complete the same flow.

#### Acceptance criteria

- **AC-AGENTS-SESSION-CATALOG-003.1:** The composer shall retain its current model
  label and expose a localized, keyboard-accessible refresh notice and action.
  The notice shall remain reachable when model discovery is empty or the normal
  model trigger is unavailable. Pending and failed operations shall have a live
  status announcement distinct from the action's stable accessible name.
- **AC-AGENTS-SESSION-CATALOG-003.2:** Phone and coarse-pointer users shall reach
  the same notice, candidate distinction, confirmation, loading, and recovery
  outcomes without hover. Confirmation shall use the existing phone confirmation
  surface with at least 44px action targets, safe-area clearance, and no document
  horizontal overflow. Desktop controls shall retain normal compact sizing.
- **AC-AGENTS-SESSION-CATALOG-003.3:** Navigation to another session, viewport
  changes, duplicated notifications, and reload shall not transfer refresh state,
  confirmation, or a restart to a different session. Dismissing confirmation
  shall return focus to its trigger when it still exists.

## Out of scope

- Automatically restarting agents, background remote probing, or making a host
  catalog authoritative for a remote executor.
- Bypassing provider-advertisement validation or importing probe defaults as live
  model, mode, permission, dependent-option, or turn-attribution state.
- Applying an unsaved profile, changing resume-settings policy from issue 4994,
  updating packages, or changing runtime feature flags.
- Guaranteeing that a provider/account exposes identical models before and after
  restart; unsupported discovery remains explicitly unsupported.
- New notifications in the global inbox, telemetry projects, or protocol-specific
  model-name interpretation.

## Implementation plan

- [Session catalog refresh](../../../plans/session-catalog-refresh/plan.md)
