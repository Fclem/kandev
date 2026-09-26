---
status: draft
system: ui
created: 2026-09-22
owners:
  - kandev
---

# Pinned prompt availability Requirements

## Overview

A task transcript opens on a bounded newest window and can later hold an older
page set, a merged around-window, or a combination of both. A long agent run can
push the user's last prompt out of every loaded window, so the transcript renders
no user prompt at all. The pinned prompt and the scroll-to-last-prompt control
are the reader's only direct route back to that prompt, so deriving them only
from the loaded window removes them exactly when they are needed.

UI owns this presentation contract: which prompt the session's pinned prompt and
transcript controls represent, and how the transcript reaches it. Message
storage, cursor semantics, and the bounded-window composition remain owned by
[transcript history visibility](task-prompt-transcript-visibility.md). The
rendered appearance of the pinned prompt remains owned by
[last-prompt pinning](last-prompt-pinning-regressions.md).

## Terminology

- **Last prompt:** The newest stored user-authored message of the active session.
  The synthetic task-description row is never the last prompt.
- **Loaded transcript window:** The messages the session currently holds in its
  transcript cache. It can be empty, a bounded newest window, an older page set,
  or a merged around-window. A cached message can also be filtered out of the
  rendered rows; it is still part of the loaded window.
- **Unloaded last prompt:** A known last prompt that is absent from the loaded
  transcript window and therefore not rendered. A prompt that is cached but
  filtered out of the rendered rows is not an unloaded prompt and gains no last-prompt
  affordance: it gains neither the control nor a jump, and the anchored bar's node and
  its height reservation both follow the rendered-row list, so such a prompt mounts no
  bar and reserves no offset.
- **Anchored bar:** The desktop-only pinned prompt surface defined by
  [last-prompt pinning](last-prompt-pinning-regressions.md).

## Requirements

### REQ-UI-PINNED-PROMPT-AVAILABILITY-001: Pinned prompt availability

**Intent:** The session's last prompt and the navigation back to it stay
available while the messages around it are unloaded.

**User story:** As a reader of a long transcript, I want the newest prompt and
the control that returns to it to remain available, so that a loaded-window
boundary never hides my own instructions from me.

#### Acceptance criteria

- **AC-UI-PINNED-PROMPT-AVAILABILITY-001.1:** When the active session has a stored last prompt that the panel resolves and that a non-empty loaded transcript window does not contain, and the scroll-to-last-prompt setting is on, the transcript shall offer the scroll-to-last-prompt control for that prompt; `.9` governs the empty window, where no affordance appears whatever the projection holds. The control's direction follows the resolved edge, so an unloaded prompt classified `visible` points upward; `.3` and `.4` govern the rows it precedes or follows. Resolution is the precondition `.6` governs, so this criterion applies once a prompt is resolved, from the projection, from a live prompt the client observed, or from the loaded window.
- **AC-UI-PINNED-PROMPT-AVAILABILITY-001.2:** When the scroll-to-last-prompt control is activated for an unloaded last prompt, the transcript shall load the messages around that prompt, place the prompt row at the same start-aligned position a loaded prompt reaches (the row's top at the transcript viewport top plus that row's own scroll margin), and report the in-flight jump with the existing in-progress indication. A session removed and recreated with the same identifier invalidates a pending or in-flight jump: no around response may merge or settle across that incarnation. A row the freshness rule selects must survive the transcript cache's own reconciliation, so that cache's signature treats an unparseable `updated_at` as unknown rather than as an authoritative change signal.
- **AC-UI-PINNED-PROMPT-AVAILABILITY-001.3:** When the last prompt precedes every rendered transcript row, the desktop anchored bar shall present that prompt's stored content, and the scroll-to-last-prompt control shall point upward. This criterion covers a prompt the transcript resolves as unloaded; a prompt the transcript renders as a row is governed by last-prompt pinning's viewport threshold, and a prompt that is cached but absent from the rendered rows by the Terminology's withholding plus `.1`.
- **AC-UI-PINNED-PROMPT-AVAILABILITY-001.4:** When the last prompt follows every rendered transcript row, the anchored bar shall remain closed and the scroll-to-last-prompt control shall point downward, under the same coverage `.3` states.
- **AC-UI-PINNED-PROMPT-AVAILABILITY-001.5:** When a session has no stored user prompt, or only the synthetic task-description row, the transcript shall present no last-prompt affordance and shall not present the task description as the last prompt.
- **AC-UI-PINNED-PROMPT-AVAILABILITY-001.6:** When the session's prompt projection is empty, unavailable, still loading, or older than the loaded window, the transcript shall fall back to the last stored prompt of the loaded window, which also wins an order tie unless the projection row's `updated_at` is strictly newer, so a prompt read that fails or lags cannot remove an affordance the loaded window already supports. Only an authoritative projection contributes a prompt, and only for the rows the panel's own successful read returned: a cache populated by transcript fan-out or older-prompt pagination contributes nothing while that read is unavailable, failed, or in flight, and contributes nothing for a successful empty response, so a stale older prompt is never presented as the last prompt. A cache row the read did not return is admitted only when an observed row or id is on record and that row orders strictly newer than the newest observation, so a prompt fetched before this client observed it still resolves after the loaded window loses it. A valid stored user prompt row the client observed live (the message-added event, a committed send, or the comment, review, and walkthrough create paths, all through `addMessage`) is admitted on its own, because the client observed that row and it is the session's newest prompt, and it stays admitted once a reconciled fetched window drops it from the transcript. In the state this requirement addresses (a non-empty window whose newest page holds no stored user row) that fallback is empty, so absent a live observation no last-prompt affordance is presented until a read succeeds; a prompt the client observed live takes precedence over both fallbacks and resolves before any successful read, while `.9` still withholds every affordance when the raw loaded window is empty; recovery follows the projection's existing path: the read is attempted once per session generation and once per connection-status change until it succeeds (so a failed read retries on the next status change), and once the marker is set no further request is issued on a connection transition. A failure while the socket stays connected has no retry: that dead end is accepted, no polling loop and no transcript-side failure surface are added, and the prompt-history panel's retry does not restore this panel's resolution because that read installs a non-authoritative cache from its own hook instance. The authority read's response path is terminal: a current session generation with success installs and sets the marker, a live update landing mid-flight being preserved by that upsert, with no mismatch retry; a current generation with failure leaves the marker unset and issues no mismatch retry, keeping the next status-change, generation, or remount attempt available; a stale session generation or an unmounted consumer writes, marks, and retries nothing. The read also leaves an in-flight older-prompt page intact: its loading state is request-local and its install advances neither the shared cursor nor the refresh generation. A deletion that lands while the read is in flight wins over the response: no prompt-cache writer may reinsert a row whose id the store recorded as deleted, and the writers follow one matrix: a named authority install filters, sorts, merges, and sets the marker, applying the response's metadata only when the session has no cache entry at all (a cleared but initialized cache keeps its cleared metadata); the page writers apply the incoming metadata and then repair a filtered cursor to the oldest retained row, clearing the cursor with `hasMore` false when a non-empty page retains no row, while a zero-row
page keeps the metadata it carried (the exhausted page's `has_more` false and empty cursor, which
is how the older-page path learns the history ended) rather than the prior values; row events insert when the id is absent and never touch pagination metadata, keeping their existing refresh-generation bump that invalidates an in-flight prompt-history page; every merge (row events, the authority install, and the around window) replaces a cached row only when the incoming version is strictly newer, keeps it on an equal version or an unparseable incoming version while still adopting a `prompt_index` that incoming row newly supplies, and replaces a cached row whose own version is unparseable, matching the comparator's branch order; and the removal path repairs a cursor naming the removed id and clears both when no row remains. Boot hydration hydrates only whitelisted cache and metadata fields and never sets the marker, the observed-prompt record (its ids and its newest key), or the deleted-id set (its payload's slice is replaced by the store's session default in any case). The observed-prompt record is per session generation: the purge clears it with the marker and the deleted-id set, the removal path prunes a removed row's id, and no observation, id, or floor survives into a recreated session. Setting the marker can therefore neither resurrect a removed prompt nor strand older pages behind a cursor the server can no longer resolve.
- **AC-UI-PINNED-PROMPT-AVAILABILITY-001.7:** Availability shall not change the loaded window: the transcript shall not insert the unloaded last prompt as a rendered row, shall not alter pagination cursors, and shall not load older or complete history as a side effect of resolving the last prompt. A user-initiated jump may add the around-window rows for that prompt, which can also change which older-history affordances the transcript offers; resolving the prompt alone shall not. A jump's around window and the retained newest window need not tile the run: for a run longer than their combined span the rows between them stay unloaded, as they already do for the prompt-history and dockview jumps.
- **AC-UI-PINNED-PROMPT-AVAILABILITY-001.8:** Desktop and phone shall keep their existing compositions: the phone keeps the scroll-to-last-prompt control and no anchored bar, and that control reaches an unloaded last prompt on both surfaces.
- **AC-UI-PINNED-PROMPT-AVAILABILITY-001.9:** While the loaded transcript window is empty, the transcript shall present neither the anchored bar nor the scroll-to-last-prompt control; `.3` and `.4` govern a non-empty window whose rendered rows bound the prompt.

## Compatibility

The last-prompt affordances keep the existing setting gates
(`show_anchored_prompt_bar`, `show_scroll_to_last_prompt`) and the existing
directional contract: the anchored bar opens only for a prompt above the
viewport, and the control points at the prompt's actual direction.

Existing behavior is preserved when the last prompt is loaded and rendered: the resolved
prompt is the loaded window's last stored prompt, and the control keeps its
immediate scroll. When a loaded prompt is not rendered, the affordance follows the
unloaded rules above.

`AC-UI-PINNED-PROMPT-AVAILABILITY-001.2` assumes the transcript is not already
serving another owner's target. While a host input is still supplied to the panel,
or while a dockview target recorded for this session and panel id has not been
consumed, activating the control is an intentional no-op: the transcript keeps
serving that target, the control stays available, and the next activation after
the state clears performs the jump as described. Two long-lived inert states are
accepted: a recorded target the transcript never consumes (the message stays
unrendered), and a host input whose supplier never clears it; each keeps the
control inert in that panel until it clears or the panel unmounts.

A jump that confirms the last prompt no longer exists server-side stops retrying:
the consumed target never re-requests on its own, and the prompt projection stays
owned by the prompt-history contract, which reconciles deletions from session
message events. When the client missed that event, the transcript keeps offering
the prompt and the reader's next activation re-confirms it with exactly one
request; this capability adds no deletion path of its own.

## Out of scope

- Widening or removing the bounded transcript window, and changing
  `message.list` cursors, ordering, or limits.
- Injecting the unloaded prompt row into the loaded window, or rendering any
  synthetic last-prompt row in the transcript.
- The pinned prompt's rendered formatting, source bounding, expansion, and
  mobile absence (owned by
  [last-prompt pinning](last-prompt-pinning-regressions.md) and
  [bounded user-message rendering](bounded-user-message-rendering.md)).
- Prompt persistence, prompt-history panel pagination, and prompt search.

## Implementation plans

- [Pinned prompt availability](../../../plans/pinned-prompt-availability/plan.md)
