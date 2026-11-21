---
status: draft
system: agents
created: 2026-11-10
requirements:
  - REQ-AGENTS-SESSION-CATALOG-001
  - REQ-AGENTS-SESSION-CATALOG-002
  - REQ-AGENTS-SESSION-CATALOG-003
---

# Session catalog refresh system design

## Purpose and boundaries

Agents owns refreshed provider catalogs and their relationship to a concrete
execution profile. Tasks owns authorization, restart/resume, and conversation
identity. This design adds publication of discovery observations, not a new
provider configuration authority.

[Profile discovery](profile-capability-discovery.md) remains pure.
[Dynamic provider options](../requirements/dynamic-provider-options.md) retains
live effective-state ownership. The new host-candidate projection is explicitly
not live provider state. Remote catalogs, startup selection, and dependent
options keep their present owners. No active requirement is being silently
relaxed: the separate publication operation cannot mutate effective settings.

## Verified implementation context

- `settings/controller/profile_discovery.go:FetchProfileDynamicModels` resolves
  saved or complete draft launch inputs and calls `ProbeProfileCapabilities`.
  Its internal result contains `ConfigOptions`, while the settings response is
  also enriched with host-CLI suggestions by `finishDynamicModels`. Neither
  result supplies task candidates; successful saved discovery gates task probing.
- `hostutility.AgentCapabilities` carries models, modes, current defaults, and
  normalized configuration choices. `ProfileCapabilityResult.ContextRevision`
  is a process-local context/cache identity, not a durable ordered revision.
- `orchestrator.handleSessionModelsEvent` serializes provider snapshots through
  `sessionSettingsSnapshotLock`, rejects stale execution/attempt/source reports,
  and persists `acp_model_state` plus runtime configuration and baselines.
- `lifecycle.SessionModelsSnapshot` is hydrated into the frontend selector.
  `lib/ws/handlers/session-models.ts` protects startup and explicit selections.
- `acp.Adapter.setModelWithConn` validates model IDs against `availableModels`
  and uses its own cached config options to choose the provider RPC. Updating
  SQL or React alone does not admit a newly discovered model to that adapter.
- `AgentExecution.ExecutorType` describes actual locality. The concrete running
  profile is `AgentExecution.AgentProfileID`; dynamic parent routing and desired
  executor configuration are not runtime identity.
- `useSessionRestart` already captures task/session, sends `session.stop` then
  `session.launch` with `intent: resume`, and guards duplicate requests,
  navigation, composer drafts, and pending submissions. Stop acknowledgment
  does not prove process exit; existing resume cleanup owns that boundary.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-SESSION-CATALOG-001 | Saved refresh operation; Eligibility; Persistence and ordering; Projection |
| REQ-AGENTS-SESSION-CATALOG-002 | Candidate selection; Explicit restart and settlement; Failure |
| REQ-AGENTS-SESSION-CATALOG-003 | Projection; Desktop and phone composition; Verification |

## Saved refresh operation

Add `POST /api/v1/agent-profiles/:id/model-catalog/refresh` in settings handlers.
It accepts no client-provided catalog, executable, task list, or launch override.
Require the existing concrete-profile execution/configuration authority used by
`requireProfileDiscoveryScope`. Resolve the saved provider/profile server-side.

The editor's explicit Refresh uses this operation only when its complete current
launch draft matches its saved baseline. A dirty launch draft or a new profile
continues to use the existing `/agent-models/:agentName/probe` endpoint and cannot
publish. The server independently pins and rechecks the saved launch context;
frontend equality is not authorization or freshness proof. Automatic editor
loads and runtime-update-triggered discovery retain the existing pure probe path.

Share the settings probe/conversion path rather than executing two settings
probes. Retain its editor response and pass a pinned saved-source token through
a narrow injected consumer wired in `internal/backendapp`. Settings must not
import orchestrator/lifecycle. That consumer obtains separate fresh task-context
results under [the acquisition contract](session-catalog-task-discovery.md).
Generic settings choices and host-CLI enrichment never become task candidates.

Only `status: ok` can publish. Require the same complete context/generation checks
as profile discovery before each session write. A save, secret rebind, runtime
activation, or newer refresh invalidates an older result. Keep the existing
bounded hostutility admission and cancellation owner; do not create a detached
unbounded fan-out. The request executes publication, cancellation stops remaining
work, and committed per-session updates remain valid. A later explicit refresh
can converge a partial batch. Report sanitized publication failure separately
from successful discovery; do not return false all-session success.

## Eligibility

Enumerate live task sessions by concrete execution profile in a bounded,
paginated repository query; do not scan all sessions on every refresh. Include
RUNNING, WAITING_FOR_INPUT, and IDLE sessions with a current initialized execution.
STARTING/CREATED sessions have not settled their initial catalog: do not import
choices; their normal initial discovery supplies it. Exclude terminal and
archived tasks/sessions. Recheck actual profile, execution identity, state, and
authorization immediately before writing.

Authorize each session's owning task with the existing task/session access seam.
Profile configuration authority does not grant access to every workspace using
that global profile. Filter inaccessible sessions without disclosing their IDs
or counts. Do not enter an unscoped internal context for fan-out.

Candidate publication requires actual `local` or `worktree`, with `local_pc`
only as a legacy alias, and a qualified task-probe capability with sealed current
execution context. [Fresh task-context discovery](session-catalog-task-discovery.md)
defines provider support, launch/source ownership, authentication and admission.
It replaces utility-to-task launch equivalence, not installer-sharing behavior.
Unknown locality, legacy evidence, unsupported capability or a changed saved
source yields notice-only. Physical host/profile equality and installer digests
are not qualifying proof. Do not use `isHostLocalExecutor`'s empty-type allowance.

Worktree isolation is not container isolation. `local_docker`, `ssh`,
`remote_docker`, `sprites`, `k8s`, and plugin-provided remote executors are always
notice-only. Dynamic sessions match the actual concrete profile, not the routing
parent or current profile setting.

## Persistence and ordering

Add optional typed task-session metadata `profile_catalog_refresh`, separate
from `acp_model_state`. The latter stays provider-derived, so existing provider
writers cannot erase a pending host observation or mistake probe defaults for
confirmed settings. This satisfies direct DB-backed selector refresh without
rewriting provider truth. No table migration or durable catalog history is needed.

The bounded record contains:

- Schema version and a monotonically increasing per-session publication revision.
- Concrete profile identity and a non-secret canonical digest of normalized
  model/mode choices, excluding probe defaults, timestamps, and context secrets.
- Execution/attempt/source identity captured at publication, for replacement
  settlement; use the existing settings identity conventions.
- Disposition `host_candidates` or `rediscovery_required`.
- For compatible host sessions only: models/modes and bounded catalog data from
  that target's fresh task probe, with versioned `task_probe_v1` provenance.
- Notice reason `new_candidates`, `executor_scoped`, or `context_unverified`.
  Remote records contain no host model names or choices.

Use a single metadata-key write and the existing per-session settings lock with
expected-owner checks. If execution ownership can move outside that lock, add a
repository conditional write that checks the joined current execution/profile
and expected prior metadata in the same transaction; checking only the prior
JSON is insufficient. Cover SQLite and PostgreSQL. Never replace the entire
session metadata map. Keep revision allocation and write atomic. A duplicate
semantic digest for the same still-pending context is a no-op; a new execution
or cleared notice is not the same pending context.

Do not persist process-local HMAC revisions as durable ordering authority.
No probe result changes `current_model_id`, current mode, `runtime_config`,
`runtime_config_overrides`, original-effective settings, config baseline, history,
queue, or runtime snapshot attributed to a turn.

Publish a session-scoped `session.catalog_refresh_updated` notification only
after a committed write. Reuse session subscription routing and task access;
never broadcast catalogs install-wide. Include the complete new record and
revision. A cleared notice is an explicit empty/tombstone projection with a
newer revision, not an omitted field. Boot/task-detail hydration carries the same
record and revision. Older live or HTTP hydration cannot overwrite newer state.

Persist provider availability independently at
`acp_model_state.catalog_discovery`, with its own monotonic per-session revision,
concrete profile, execution/attempt/source owner, and normalized result. Preserve
that revision across source rotations; retaining an old record does not transfer
its ownership to the new source. This projection carries explicit empty and
unsupported surfaces even when there is no pending host notice, without replacing
effective values, baselines, overrides, or the rest of `acp_model_state`.

Emit `session.catalog_discovery_updated` only after its expected-owner SQL write
commits. Its `{task_id, session_id, provider}` payload uses the same session-scoped
subscription boundary as the host observation. The host notification carries
`{task_id, session_id, record}`. Provider advertisement can update availability
without settling a host notice; only the explicit replacement discovery finalizer
can perform that settlement. Independent revision guards prevent delayed HTTP
snapshots or source-local counters from reviving stale selectable controls.

## Projection

Add the observation to the existing session-runtime view model and task-detail
hydration path. Keep one store owner and shared hooks; components do not fetch.
`ModelSelector` combines the separate candidate projection with the provider
catalog for navigation while deriving selected values and dependent controls
solely from existing provider/runtime state.

For a matching host refresh, show the refreshed candidate order and groups.
Coalesce exact values shared with the provider while retaining structured
occurrence identities. Provider-only choices remain in an identified current
session group; a host-only choice removed by a later refresh disappears. Keep a
removed selected model visible as the current label, without fabricating a
selectable candidate. Modes follow the same candidate-versus-provider rule.

Resolve the current execution from the existing live `sessionAgentctl` projection,
falling back to the full session snapshot only when live identity is absent.
Explicit postcommit provider evidence for that execution can advance its settings
source before HTTP metadata catches up. A different execution, older source, or
conflicting attempt cannot become current merely because its choices are present.

Never transplant probe `CurrentModelID`, `CurrentModeID`, or model-dependent
option defaults/choices into the live controls. A richer model navigation catalog
can be projected; Thinking, Speed, permission mode, and other effective options
remain tied to the running model's confirmed snapshot. Separate vendor host-CLI
suggestions added by `finishDynamicModels` cannot enter this observation.

## Candidate selection

A choice present in the provider catalog uses the existing handlers and provider
RPC. A host-only choice is visibly labelled as requiring refresh. Activating it
opens the session-refresh explanation/confirmation, not SetModel/SetConfigOption.
Do not weaken `validateAvailableModel`, inject a host catalog into adapter caches,
or turn an ordinary provider rejection into an automatic restart.

After rediscovery, the user selects a newly advertised choice through the normal
flow. Do not queue or silently auto-apply the earlier candidate after a restart:
permissions and dependent options can differ. If the provider still omits it,
show the executor result honestly. The original issue's unconditional automatic
selection promise is qualified by its selected maintainer comment, not claimed
for a provider that cannot rediscover the choice.

## Explicit restart and settlement

The notice action opens confirmation captured to task/session and notice revision.
Reuse the confirmation pattern in `ConfigChatRestartConfirmation`:
`ActionConfirmPopover` on desktop and `MobileActionConfirmation` on phone.
Explain restart and possible interruption; Cancel performs no operation. A newer
notice or different target invalidates an old confirmation.

On confirmation, reuse `useSessionRestart` and its store-scoped pending entry.
Do not add a second stop/resume implementation, fixed delay, retry loop, profile
refresh flag on resume, context reset, new-session fallback, or prompt. Normal
backend admission, resume cleanup, durable recovery, and authorization remain
binding, including an uncertain interrupted turn that requires existing recovery.
Busy restart is possible only after this explicit interruption warning.

Ordinary resume resolves current profile environment, CLI flags, and command
prefix through `resolveAgentProfile`, `resolveProfileLaunchTokens`, and the
existing environment resolver. The stored profile/runtime snapshots preserve
model/mode/config-option intent, not a frozen environment/flags/wrapper.
This feature neither adds frozen launch capture nor changes that resume policy.
Confirmation and public guidance must disclose current launch-input resolution
alongside interruption. A changed HOME/account or wrapper can make native restore
fail; retain saved native identity and surface existing recovery rather than
creating another conversation. Test a profile launch edit between initial launch
and confirmation, including a change while confirmation is open.

Keep the notice pending after the two request acknowledgments. Settlement requires
an explicit terminal discovery result from the current replacement
execution/attempt/source, not readiness or the presence of a nonempty list.
Validate the current owner and notice revision. A late old owner or a refresh
that arrived after that replacement began must not clear a newer notice. Capture
replacement-start ordering at the existing launch/settings identity boundary;
do not infer it from wall-clock timestamps or changed model names.

Task 01 must add a typed, owner-correlated terminal discovery result through the
agentctl session response/stream and lifecycle projection. Its outcomes are
`complete`, `unsupported`, and `failed`; complete carries normalized model and mode
surface observations, including explicitly empty supported surfaces. Preserve
surface presence/support separately from list length. A mode-only response is
terminal for mode discovery and explicitly reports unsupported model discovery.
Missing transient state is not an empty or unsupported result.

Produce that result after a successful New/Load/Resume handshake and applicable
settings settlement, even when `GetSessionModelState` returns nil or
`publishSettledConfigOptions` has nothing to emit. In particular, strict-policy
ACP LoadSession must report terminal empty/unsupported surfaces rather than
depending on the provider-restored-only empty model event. Native supported
transports must supply the same normalized outcome; unimplemented/legacy result
producers use the bounded no-report outcome below, never inferred success.
Task 01 owns these source, wire, runtime, and settlement changes and their tests.
Reuse accepted source/attempt identity guards and exclude stale finalizers.

For `complete`, replace stale candidate projection from that owner, clear the
matching notice atomically, and publish the revisioned terminal/tombstone state.
An explicitly empty supported surface clears its prior selectable choices while
preserving any last-known selected label as unverified display, not a provider
default. Unsupported surfaces cannot retain stale enabled selection controls.
An unchanged catalog is a successful check, not evidence of new models.
For `unsupported` or `failed`, end checking, retire stale host-only candidates,
and retain a revisioned notice with the terminal reason and ordinary recovery;
do not announce fresh choices. Ordinary unrelated provider reports retain pending
state. A newer notice always wins over a delayed terminal result.

The frontend owns a single store-scoped 60-second observation deadline keyed by
task/session, accepted restart attempt, and notice revision, beginning at Resume
acknowledgment. A missing terminal result changes Checking to a retryable
unavailable status, retaining the notice and existing runtime controls. It does
not cancel/restart the agent, fabricate unsupported evidence, or block normal
submission after request-pending guards release. Navigation/viewport changes do
not restart this deadline; a new explicit attempt gets a new deadline. Reload
recovers the durable notice without resurrecting an unbounded checking spinner.
A later matching current-owner result can still settle that notice. Test the
deadline with a controlled clock and suppressed result, not a fixed test sleep.
At Resume acknowledgment, read the current revisioned result before starting
checking: a result delivered before the ACK must not be overwritten by a new
spinner/deadline. Carry the accepted launch owner in the existing launch response
or its correlated session projection so the hook never guesses attempt identity
from the currently selected session. Task 01 owns that additive owner contract;
Task 02 tests result-before-ACK and ACK-before-result orderings.

## Desktop and phone composition

Entry is the existing task/quick-chat composer model control. Add a compact
notice/bubble adjacent to it and a short status row above the composer when
needed. Render the notice outside `shouldHideModelSelector` so empty discovery
cannot hide recovery. Reuse the same view model in task, preview, and Quick Chat
instances; scope to the captured session.

Nearest shipped exemplars: the mobile model configuration selector for model
navigation, and Configuration Chat's restart confirmation for interruption
consent. This is a short secondary action, not a new settings page or global
notification. Desktop uses the anchored confirmation; phone uses the existing
inset confirmation surface with wrapped explanatory text and a single scroll
owner. Existing composer/picker owners retain scrolling and dynamic viewport
behavior; add no nested composer scroller or fixed viewport panel.

Phone controls have at least 44px hit areas, including narrow fine-pointer
phones below 768px. Desktop ordinary actions retain 28px sizing. Reuse safe-area
padding and focus return. Loading/failure is announced in one localized status
region while the action accessible name stays stable. All copy belongs in the
existing task/agents locale namespaces, all supported translations and pseudo.
Full structural previews are in the [plan](../../../plans/session-catalog-refresh/plan.md#ascii-ui-preview).

## Failure and observability

A failed probe publishes nothing. A failed session write retains prior state and
is not advertised as committed. Publication can partially succeed; report an
aggregate sanitized error and retain committed updates rather than rolling back
another session's current catalog. No automatic retry or new background worker.

Failed Stop ends the restart sequence; failed/uncertain Resume exposes ordinary
Resume/recovery without discarding conversation or claiming the agent is still
running. Reconnect recovers committed metadata. Preserve native session identity
and all existing no-silent-fallback constraints if the provider cannot restore.

Use existing structured logs for discovery outcome, scoped publication failures,
and restart/resume. Log stable result/reason fields and ordinary correlation
identities only. No secret values, raw launch inputs, credentials, provider stderr,
new high-cardinality metric labels, or new telemetry subsystem.

## Verification

Two sequential vertical work orders cover host candidate publication and explicit
executor rediscovery. Tests must prove the adapter-cache boundary, not merely
that the frontend received more options. See the [work package](../../../plans/session-catalog-refresh/plan.md)
for exact commands, provider/executor matrix, race cases, and browser flows.

Related decisions remain unchanged:

- [Explicit profile model strictness](../../../decisions/2026-09-15-explicit-profile-model-strictness.md)
- [Provider-neutral catalog](../../../decisions/2026-11-03-provider-neutral-model-catalog.md)
- [Explicit resume settings](../../../decisions/2026-09-29-explicit-resume-settings.md)
- [Session restart composition](../../tasks/system-design/session-tab-restart.md)

The existing selection/recovery design remains unchanged. The revised acquisition
boundary and its rationale live in
[Fresh task-context discovery](session-catalog-task-discovery.md), including
provider capability limits, fresh helper ownership and rollout cutover. The old
[attestation proposal](session-catalog-launch-context-evidence.md) is superseded,
not an authorized implementation dependency.

## Superseded generic-transfer feasibility

Provider research found cwd/project, credentials, permission modes, protocol
and configuration/cache dependencies omitted by installer identity. Ordinary
utility and task working directories differ even without wrappers. Native
Codex's retained `model/list` can remain stale after a catalog-file update, while
a fresh process observes it. These findings invalidate earlier generic-candidate
qualification; they do not invalidate independent restart/terminal/UI evidence.

The user selected task-context design after completing provider-contract research.
The revised design preserves AC001.1 through qualified fresh task-side acquisition,
not universal notice-only, blanket generated-variable exclusions, copying task
secrets into the utility or adding cryptographic equality proofs. Initial native
support and explicitly unqualified ACP/custom surfaces follow the linked
compatibility matrix; production-native proof is recorded in the work orders.
