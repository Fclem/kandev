---
status: draft
system: agents
created: 2026-11-19
requirements:
  - REQ-AGENTS-SESSION-CATALOG-001
---

# Fresh task-context catalog discovery

## Review boundary

The user selected this design direction after provider-contract research failed
to qualify generic utility-to-task catalog transfer. It replaces the acquisition
path of AC-AGENTS-SESSION-CATALOG-001.1; a later explicit user request authorized
its implementation. The [parent design](session-catalog-refresh.md) retains
publication, provider availability, restart, consent, recovery and desktop/phone
ownership.

Agents owns acquisition and provenance; Tasks owns authorization, execution and
publication. A successful saved-profile probe gates the operation but supplies
**no session candidate payload**. Each eligible host execution supplies its own
fresh observation. Installer identity and generic environment equivalence are
removed from candidate admission. No MAC/challenge or environment-attestation
transport is needed or approved.

## Evidence and rejected alternatives

Research found directory, environment, credential, project configuration and
session dependencies. OpenCode consumes GitLab credentials for provider/model
activation and directory agents for modes. Claude and Codex ACP also derive
permission modes from project/session settings. These inputs cannot be ignored
by a generic generated-key or credential-name exclusion.

Native Codex 0.169.0 research used the actual binary: replacing its configured
catalog file changed a new process's result, but not a retained process's
`model/list`. Its [catalog processor](https://github.com/openai/codex/blob/d32063f96c2dff1dc96af839b3952abb6acfeda4/codex-rs/app-server/src/request_processors/catalog_processor.rs#L304-L360)
uses a warm-cache strategy. Calling the live adapter again is not forced-fresh
configuration discovery. A fresh helper is therefore required for this native
implementation. A request to restart the live process remains separately consented.

Reject generic utility result transfer, installer-digest equality, snapshot hash
attestation, borrowed warm caches, copying task secrets to the settings utility,
and creating/loading a conversation to manufacture a catalog.

## Supported acquisition contract

A provider implementation must declare an internal versioned task-probe capability
and its supported model/mode surfaces. This is executable adapter capability,
not a provider-name switch in the publisher. Capability evidence includes actual
resolved runtime/version, launch shape, fresh acquisition method and forbidden
protocol operations. A shim pin, wire-schema version or unrelated installed CLI
is not evidence of the process implementation.

| Implementation | Planned acquisition | Initial eligibility |
| --- | --- | --- |
| Native Codex built-in unwrapped app-server, actual 0.169.0 and 0.171.0 (each exact version) | New helper process; initialize then complete `model/list`; no thread operations | Model surface only, after versioned process/protocol tests and actual runtime proof for that exact version |
| ACP, including Claude/Codex ACP/Gemini/OpenCode/Copilot/Pi/Muse | No qualified generic fresh read-only model/mode operation established | Notice-only until a concrete implementation satisfies this contract |
| Custom command, passthrough, wrapper, unrecognized runtime/version | No established safe fresh-probe launch shape | Notice-only |
| Deterministic mock adapter | Real helper/protocol fixture with distinct task-specific models and modes | Tests only; never production-provider qualification |
| Docker/SSH/Sprites/Kubernetes/plugin remote or unknown locality | No automatic task probe | Notice-only, regardless of a provider capability |

This matrix is a visible review constraint: it does not promise automatic candidate
publication for every ACP provider. It preserves a required ordinary production
host path rather than universal notice-only. Before completion, prove native
Local and Worktree positive paths with the actual qualified runtime. Additional
versions/providers require their own same-contract evidence, not permissive
version ranges. A supported empty model list is complete; unsupported mode
surface is explicit, never a fabricated empty supported mode catalog.

## Saved refresh and admission

Keep `POST /api/v1/agent-profiles/:id/model-catalog/refresh` and its no-overrides
request contract. The existing saved probe remains the editor response and
successful saved-source gate; draft/automatic/failed probes do not fan out.
After success, the narrow backendapp-injected consumer enumerates bounded pages
of initialized live concrete-profile sessions, including IDLE, and authorizes
each owning task **before any task-side request**, then again before publication.

Capture a request-owned saved-source token: concrete profile, saved launch-input
revision, runtime selection/binding currency, mutation fence and refresh generation.
The token has equality/currency semantics, not durable revision ordering. Carry
it through task acquisition and the existing conditional SQL publication. Profile
save/delete/disable, launch-input mutation, secret rebind/runtime activation or
newer refresh invalidates remaining work. A task launched under a different or
unavailable saved-source revision gets notice-only; never apply the current
profile's environment or CLI to an older execution to make it match.

Use one active helper per target instance and at most two per saved-refresh
request, with paginated admission and no detached worker. Saved probing retains
its existing bound. Following saved-probe success, fan-out has a 60-second total
deadline; each helper has a 15-second cap within the remaining budget. The
backend waits for one task read up to that cap plus agentctl's reap and pipe-drain
bounds and a margin, so it never abandons a request whose helper still holds the
slot. A newer refresh or saved mutation cancels the superseded request's fan-out.
Cancellation stops admission, cancels owned helpers and preserves earlier
committed writes. A busy target instance is contention, not that task's result:
write nothing, keep the prior record and report the publication partial.
Exhaustion is a sanitized partial/unavailable publication outcome, not all-session
success. Do not queue probes after the request ends or add automatic retries.

## Internal API and ownership

Add proposed backend-only HTTP action
`POST /api/v1/agent/catalog-candidates/discover` on the authenticated agentctl
instance server, with a typed native client method. This is distinct from the
existing startup terminal-discovery getter; neither becomes the other.

The backend derives the target from its authorized execution/runtime lease, never
from caller-supplied task IDs or URLs. Require current instance control-credential
authentication and a nonempty matching `X-Instance-ID`; reject legacy/auth-disabled
or unproven ownership. Explicitly deny this path in **every** generic browser/UI
proxy, including credential-injecting proxies. No frontend WS/MCP action can
forward it or choose its owner, command, credentials or protocol frames. Existing
profile/task authorization remains backend-owned; bearer authentication alone
must not become an unauthenticated browser discovery oracle.

Request fields: bounded request ID, expected manager ID, positive process
generation, native session ID and accepted session-source generation. No env,
executable, argv, cwd, secret IDs, helper command, arbitrary RPC or catalog input.
The backend obtains manager/process identity from the authenticated current
instance descriptor, not trust-on-first-use from the discovery reply.

Under the process manager's existing startup/ownership locks, capture the current
adapter, successful launch snapshot and a monotonically advancing session-source
epoch. `GetAdapterForGeneration` is the existing capture seam, but its generation
zero wildcard is not qualifying evidence. Increment/invalidate the session-source
epoch for same-process New/Load/Resume/replacement as well as native-ID changes;
same-native-ID replacement must not keep the old epoch. Do not hold startup or
settings locks across network/helper work.

Recheck actual manager/process/session-source identity after acquisition and at
reply admission. Backend independently rechecks runtime lease, execution,
attempt/source, saved-source token and access before its transactional
expected-profile/execution/prior-record write. A matching native session ID,
adapter pointer, request echo or old adapter getter alone is insufficient.
Missing descriptors/epochs mean unavailable, not a guessed owner.

Reply fields: request correlation, actual instance/manager/process/session-source
owner, capability/version, result status (`complete`, `unsupported`, `failed`),
explicit per-surface support/presence and normalized bounded model/mode choices.
No raw environment, argv, paths, credentials, config defaults, provider stderr or
helper launch material leaves agentctl. Use existing redacted errors/admission,
no-store replies and bounded decoding. Existing control credentials are reused
only for their established authentication, not a new key-derivation protocol.

## Fresh helper lifecycle

At the successful live process start, retain an immutable **private in-memory**
snapshot of the actual final executable, argv, `exec.Cmd.Environ()` and cwd after
all existing environment, stripping, runtime and wrapper stages. Preserve exact
unset/empty distinctions. Bind it to the process and session-source epoch; do not
serialize it to backend/SQL/events or reconstruct it from current backend env.
Retained/adopted processes without this evidence get notice-only.

The first qualified native implementation requires an unwrapped built-in app-server
launch with no prompt/one-shot/custom command arguments and a positively identified
supported version. The process manager launches a fresh helper from the sealed
snapshot, in the actual task cwd, with private stdin/stdout, bounded redacted
stderr and its own process group. Do not re-run preparation, install/update a
runtime, mutate the live command or change HOME/identity variables to imitate the
generic utility. Refuse unavailable executable/cwd or changed runtime identity;
never silently select another binary. Task credentials stay inside the owning
instance's existing trust boundary.

The helper is a **catalog observation**, not a task execution/session. Native
Codex may receive initialize/initialized and paginated model/list only. It must
not receive thread/start, thread/resume, thread/fork, prompt/turn, tool/config or
model setters. Do not attach it to the task transcript, journal, execution slot,
queue or live adapter, nor publish its model events as live advertisement. No
new conversation or native-history fallback is permitted. Provider-owned catalog
or credential-cache writes inherent in initialization are not advertised as a
filesystem sandbox; adapters requiring tool execution or unproven startup hooks
are unsupported rather than claimed side-effect-free.

Request completion/error/cancellation/owner invalidation kills and waits for only
the helper process group, closes pipes and releases admission. It must never kill
the active agent. Bound stdout and stderr draining without deadlock; clean up on
pipe/start/protocol errors and backend disconnect. No retained warm helper pool:
otherwise configuration caching would recreate the observed defect.

## Complete result and publication

Native pagination follows NextCursor until absent, with at most 16 pages,
1,024 aggregate choices and 1 MiB decoded provider payload within the helper
deadline. Repeated cursors, overflow, missing/null required data, malformed rows,
partial protocol failures or a nonterminal cursor at the bound are `failed`,
never complete-empty. Explicit `[]` on a supported surface is complete. Normalize
with existing catalog rules; preserve exact IDs/order and support per surface.
No synthetic modes from collaboration APIs or static dialect defaults.

Only a complete task-owned result can supply candidate choices. An unsupported
or failed task probe produces a revisioned notice without transferring the generic
profile list. It is not provider-startup unsupported/failure and must not overwrite
`acp_model_state.catalog_discovery` or settle a post-Resume observation deadline.

Version the refresh record to identify `source: task_probe_v1` and its accepted
owner/saved-source correlation. Keep existing `host_candidates` disposition for
wire/UI compatibility; it now means task-owned host candidates. Models/modes
come from that one target's result, not another session or the generic probe.
Invalidate previous generic-origin candidates via a revisioned notice/tombstone;
frontend must not reinterpret missing/legacy provenance as task-owned. Rollout
backfill clears old selectable candidates without clearing provider choices or
pretending a restart occurred. Existing per-session monotonic revisions,
single-key SQL CAS, postcommit scoped delivery and stale-HTTP guards remain.

Candidate activation continues to require explicit restart consent if not already
advertised by the live provider. A fresh helper never authorizes SetModel on a
warm live adapter or auto-applies a candidate. Terminal startup evidence alone
settles the replacement notice under the parent design.

## Implemented boundaries

- `process.Manager` seals `cmd.Path`, argv, `cmd.Environ()` and `cmd.Dir` when a
  generation starts. `DiscoverCatalogCandidates` captures and rechecks manager,
  positive generation, adapter, native ID and handshake epoch under `startMu`.
- `qualifyCatalogLaunch` is the capability table. Each exact Codex version has its
  own entry (`codex_app_server_model_list_v1@0.169.0` and `@0.171.0`) requiring the
  built-in unwrapped managed `@openai/codex@<version> app-server` launch without
  agent arguments and a `kandev/<version>` runtime user agent. There are no
  ranges; a regression test requires the reviewed default managed pin to be in
  the table, so a pin bump fails until it is requalified with native evidence.
  `mock_acp_catalog_v1` is the mock-agent fixture only.
- The instance route requires authenticated control and the exact `X-Instance-ID`.
  `client.ProxyTransport` refuses the action for every browser proxy.
- The backend client records handshake epochs from New/Load/Resume/Reset replies
  and reads the manager from the authenticated delivery descriptor. Lifecycle
  compares an in-memory saved-source digest recorded at host launch.
- `PublishProfileCatalog` stores `source: task_probe_v1` with the per-surface
  `probe`; failures use `task_discovery_unsupported` or `task_discovery_failed`.
  Startup retirement and the browser schema demote legacy candidates to notices.

## Verification and review gates

Task 01 owns API/proxy authorization, epoch and sealed-context capture, fresh
helper cleanup/pagination/capability, provenance cutover and backend publication.
Task 02 owns legacy-candidate rejection, per-surface UI projection and real
endpoint-to-native-helper browser proof.

Required behavioral RED/GREEN includes same-native-ID replacement, same-process
reload, zero/unknown generations, auth/header/proxy rejection, source/profile/
secret/runtime races, cancellation with partial commits and helper termination,
malformed/empty/mode-only/paginated results, two tasks with distinct configuration,
and absence of conversation/configuration mutations. A positive production-native
experiment must show old live catalog A, changed task catalog B, fresh helper B,
unchanged live advertisement A, then consented normal resume observing B with
original native conversation and draft retained. A retained-client model/list or
mock-only test cannot substitute for that experiment. Inspect actual protocol
frames to prove the helper never starts/resumes a thread or sends a prompt.

Keep PostgreSQL/CAS/race, complete desktop/phone collections, actual Docker
notice-only proof and the requested root format/typecheck/test/lint gates.
Historical generic-candidate browser passes are not provenance qualification.
Recorded evidence and any remaining external gate live in the work orders.
