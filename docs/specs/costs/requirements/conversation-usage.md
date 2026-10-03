---
status: draft
system: costs
created: 2026-09-24
owners:
  - kandev
---

# Built-in conversation usage requirements

## Overview

Users can inspect recorded tokens and cost in chat without a session-cost plugin or Office.
This capability extends the existing [task usage ledger](../../task-cost-ledger/spec.md).
The costs system owns accounting, provenance, aggregation, and its conversation display.
The [native Codex integration](../../agents/requirements/codex-app-server.md) supplies observations.

## Terms

- **Last response:** The latest attributable upstream model completion in a turn.
- **Turn usage:** Recorded work for that user turn, with direct and child work separated.
- **Session usage:** New work recorded for the Kandev session, excluding inherited fork history.
- **Calculated cost:** Tokens priced by a versioned rate catalogue, not an invoice amount.
- **Provider estimate:** A provider's explicitly estimated amount, with its own scope and currency.

## Requirements

### REQ-COSTS-CONVERSATION-USAGE-001: Attributed token measurements

#### Acceptance criteria

- **AC-COSTS-CONVERSATION-USAGE-001.1:** The system shall distinguish response, turn, session, and context-window usage. It shall never label last-response usage as whole-turn usage.
- **AC-COSTS-CONVERSATION-USAGE-001.2:** Repeated observations, resume replay, and inherited fork history shall not increase recorded usage twice.
- **AC-COSTS-CONVERSATION-USAGE-001.3:** Cached input and reasoning output shall appear as breakdowns when the provider includes them in input/output totals. Totals shall not count those tokens twice.
- **AC-COSTS-CONVERSATION-USAGE-001.4:** Missing, partial, estimated, pending, and measured-zero usage shall remain distinguishable after reload.
- **AC-COSTS-CONVERSATION-USAGE-001.5:** Child usage shall retain its own thread and turn identity. Late child usage shall update the originating work, never the currently active unrelated turn.

### REQ-COSTS-CONVERSATION-USAGE-002: Honest cost provenance

#### Acceptance criteria

- **AC-COSTS-CONVERSATION-USAGE-002.1:** Cost displays shall distinguish provider-reported monetary values, provider estimates, calculated list-price estimates, and unavailable prices.
- **AC-COSTS-CONVERSATION-USAGE-002.2:** A provider amount without established monetary units shall not appear as USD. Missing prices shall not appear as free usage.
- **AC-COSTS-CONVERSATION-USAGE-002.3:** A thread-level provider estimate shall not be divided into invented response or turn costs, or added to calculated usage totals.
- **AC-COSTS-CONVERSATION-USAGE-002.4:** Calculated costs shall retain the model and pricing snapshot used. Subscription usage estimates shall not claim to be extra invoice charges.

### REQ-COSTS-CONVERSATION-USAGE-003: Built-in chat display

#### Acceptance criteria

- **AC-COSTS-CONVERSATION-USAGE-003.1:** A turn's usage control shall show its tokens and cost, with separate last-response detail when available.
- **AC-COSTS-CONVERSATION-USAGE-003.2:** A session usage control shall show recorded totals without installing a plugin or enabling Office.
- **AC-COSTS-CONVERSATION-USAGE-003.3:** Desktop and phone users shall reach the same detail and provenance. Phone disclosure shall use a touch-accessible drawer with one scroll owner.
- **AC-COSTS-CONVERSATION-USAGE-003.4:** Delayed accounting and fetch errors shall appear as pending or unavailable states. They shall not block the agent or erase known values.
- **AC-COSTS-CONVERSATION-USAGE-003.5:** When the composer is shown, available, pending, or failed usage shall have one icon-only Usage control in the status row above the composer, alongside transcript navigation and sharing. An archived transcript shall retain the same control in its read-only banner. The control shall have a localized accessible name, and the transcript footer shall not show a second Usage control.

### REQ-COSTS-CONVERSATION-USAGE-004: Compatible durable accounting

#### Acceptance criteria

- **AC-COSTS-CONVERSATION-USAGE-004.1:** Existing ledger rows, ACP totals, task totals, Office budgets, and plugin readers shall retain their established meaning.
- **AC-COSTS-CONVERSATION-USAGE-004.2:** Turn and response reads shall enforce task/session ownership. Unknown ownership shall never attach usage to another session.
- **AC-COSTS-CONVERSATION-USAGE-004.3:** Accounting shall preserve recorded work through restart, including interrupted turns. Incomplete recovery shall remain visible rather than invent missing measurements.

### REQ-COSTS-CONVERSATION-USAGE-005: Message and turn metadata

Users can inspect timing, tokens, and cost for a turn from its messages. Measures with no value for that turn stay absent.

#### Acceptance criteria

- **AC-COSTS-CONVERSATION-USAGE-005.1:** The message metadata window shall show the turn's wall-clock time, time to first token (TTFT), tokens per second (TPS), input, output, and cached tokens, and cost without any user setting. Each value shall be labelled as turn-scoped, and a value with no measurement shall be omitted rather than shown as zero.
- **AC-COSTS-CONVERSATION-USAGE-005.2:** A per-user setting shall select which items the hover metadata line shows. The items, in their fixed display order, are session configuration, timestamp, wall-clock duration, TTFT, TPS, tokens, and cost. The initial selection shall be session configuration, timestamp, and duration, so a user who never changes the setting sees the line as before. An empty selection shall be valid and hide every selectable item.
- **AC-COSTS-CONVERSATION-USAGE-005.3:** The selection shall persist across reload and apply on every client of the same account. Selecting or clearing an item shall not change which data the system collects or stores.
- **AC-COSTS-CONVERSATION-USAGE-005.4:** TTFT, TPS, tokens, and cost shall appear on the last agent message of a completed turn that shows message actions, and not on earlier messages of that turn, on tool-call or thinking entries, or while the turn is running. Open clients shall receive them without a reload. Wall-clock duration shall stay on the user prompt message.
- **AC-COSTS-CONVERSATION-USAGE-005.5:** TTFT shall be the interval from the backend's receipt of the lifecycle `turn_started` event to its receipt of the first non-empty, non-diagnostic agent output, message or reasoning; provider diagnostic/error text shall not count as output. It is an observed turn-start-to-output interval, not prompt-dispatch-to-output latency; it excludes delay before `turn_started` reaches the backend and can include output-event delivery latency. TPS shall be the turn's own output tokens, including reasoning and excluding child work, divided by the time from first output to turn completion, an average turn rate that includes tool time. TPS shall be omitted when output tokens are estimated, partial, incomplete (`direct.output_complete` is false), or zero, or when that time is not positive. A `UsageTurn` with completeness `unknown` remains eligible only when its direct output is complete; `estimated` and `partial` turns are omitted. A turn whose timing was not observed, including one that spanned a backend restart or was cancelled, shall show neither TTFT nor TPS.
- **AC-COSTS-CONVERSATION-USAGE-005.6:** Cost shall come from the task usage ledger with the provenance rules of REQ-COSTS-CONVERSATION-USAGE-002. An unpriced or pending turn shall show no cost, never zero cost. This capability shall not add a way for plugins to supply cost.
- **AC-COSTS-CONVERSATION-USAGE-005.7:** The setting shall be editable in the Conversation chat settings on desktop and phone with localized labels. A failed save shall leave the stored selection unchanged and show an error.

## Out of scope

- Invoices, billing reconciliation, subscription purchase, and pricing claims for undocumented amounts.
- Retroactive reconstruction of missing historical response measurements.
- Replacement of third-party cost plugins or their independent features.
- Changes to permission approval actions or their desktop sizing.
- A plugin-supplied cost contract, provider-reported TTFT or TPS, and instance-level gating of the metadata display.

## Implementation plans

- [Native Codex support](../../../plans/codex-app-server/plan.md)
- [Chat Usage control placement](../../../plans/conversation-usage-toolbar/plan.md)
- [Message and turn metadata](../../../plans/message-turn-metadata/plan.md)
