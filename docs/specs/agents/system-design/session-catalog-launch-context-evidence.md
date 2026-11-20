---
status: superseded
system: agents
created: 2026-11-16
requirements:
  - REQ-AGENTS-SESSION-CATALOG-001
---

# Session catalog launch-context evidence proposal

**Superseded, never implemented or approved.** The user selected
[fresh task-context discovery](session-catalog-task-discovery.md) after completed
provider research. This historical proposal is not an implementation dependency:
proving unequal generic/task inputs cannot produce qualified candidates.

**Review disposition:** the feasibility blocker is acknowledged as unresolved;
this transport is not approved. Do not implement it or further elaborate its
cryptography as the next implementation step. The user-owned product/design
choice is a genuine provider-specific catalog-dependency contract or a separately
reviewed discovery architecture using an authorized actual task context.
Universal notice-only requires a separate explicit scope reduction. Even a
future positive-provider contract would not itself approve this transport.

## Authorized callers and subjects

Only the backend executing an explicit saved-refresh publication may issue this
request. It first retains profile-administration authority and separately
authorizes each target task. The subject is the current concrete local/worktree
execution, not a browser-selected execution ID or routing parent.

Advertise a proposed internal `catalog_launch_context_v1` capability only after
this contract is reviewed. A missing capability/owner descriptor remains unknown;
do not discover support by sending secret-bearing requests to an arbitrary peer.

The proposed live operation is an authenticated instance-server POST,
`/api/v1/agent/catalog-context/attest`; the probe proof is an optional internal
envelope on the existing profile `Probe` operation. Neither is exposed as a
frontend WebSocket/MCP action, public proxy route, or unauthenticated ownership
proof. The live operation never spawns, configures or stops a process.

Require all of:

- Current control-credential authentication, including the invalidation channel
  captured by `instanceAuth`; reject auth-disabled/static legacy evidence that
  cannot establish the required credential-generation lifetime.
- A nonempty `X-Instance-ID` equal to the actual configured instance. Existing
  `instanceIDGuard` allows missing headers for older callers; this operation
  must not inherit that exception.
- A backend purpose grant over the complete request, in addition to Bearer auth.
  A proxy that injects control Authorization must not turn a browser's chosen
  comparison key into an environment-fingerprint oracle. Deny this route and its
  purpose headers in every generic browser-facing proxy as a second boundary.
- A lease-bound, current nonzero runtime epoch at the backend, plus exact
  instance ID, accepted process-manager ID and process generation. Obtain the
  manager ID from the existing authenticated delivery descriptor
  (`agentctl/client_delivery.go:123-153`), not from the first attestation reply.
  Missing manager identity is unknown, not trust-on-first-use.

The agentctl server validates its instance, manager and child generation under
the process startup lock and checks credential invalidation before responding.
The backend validates runtime epoch/source owner before and after the request
using its runtime lease. An echoed backend epoch or user/task/profile ID is not
independent server evidence; application authorization remains backend-owned.

## Wire envelope

Proposed version-1 request fields are role (`probe` or `live`), batch ID, unique
request nonce, issue/expiry times, opaque application binding digest, expected
instance/manager/process subject, comparison key, subject reply key and purpose
grant. IDs, digests and keys use fixed-length base64url encodings where applicable;
owner strings have bounded lengths. The application binding digest covers the
authorized principal, task/session, concrete profile, source owner and captured
profile/probe/binding revisions without sending those private inputs to agentctl.

The reply contains version/role/batch/nonce, the actual instance/manager/process
owner, launch-snapshot identity, context fingerprint, expiry and reply MAC.
The backend matches every field to its pending ledger entry and checks the
actual owner, not merely the MAC. No raw environment, argv, paths, source secret
IDs, comparison keys or reply keys are returned. Malformed/unknown versions fail
closed. A browser request cannot mint a backend purpose grant.

For `live`, manager identity and positive child generation must already be known.
For `probe`, the expected subject is the utility instance and current flight;
the successful reply binds a unique probe invocation and its actual launch
snapshot. Normal probe cleanup after successful discovery does not invalidate
that bounded observation. Failure before success produces no qualifying proof.

## Challenges, expiry and replay

Proposed policy for review: one random 256-bit public batch ID and an independent
secret 32-byte seed per explicit saved-refresh flight; a hard 120-second batch
lifetime bounded by caller cancellation; and a distinct random 256-bit request
nonce for each probe/live operation. A live nonce also expires after five
seconds. Backend acceptance uses monotonic deadlines; server issue/expiry checks
reject invalid or excessive lifetimes and inconsistent clocks. These private
proof lifetimes do not change the composer's post-Resume 60-second observation.
The existing probe operation still has its 60-second cap.

The backend keeps bounded request-owned pending/consumed state. Each nonce is
accepted once for its exact role, binding digest, instance and owner before
expiry. Proposed server replay bound: 256 nonce/role/expiry entries per instance
and current credential generation. Do not evict unexpired entries to admit more;
reject admission when full and discard expired entries on request. Reject a
repeated probe nonce before spawning. No durable table or detached cleanup worker.
Expiry, lost responses or cancellation cause unknown evidence; no renewal,
transport retry, probe rerun or session action is triggered automatically.

Existing bounded managed-runtime repair may already perform another probe.
Give each such existing attempt a fresh nonce/reply key under the same batch,
invalidate the failed attempt, and accept only the final successful proof. This
does not authorize an additional attempt or change existing recovery admission.

Before SQL CAS/publication, recheck task authorization, captured profile/probe
currency, source/process/binding evidence and nonce validity. Consume the proof
once; a failed CAS does not make it reusable. End/cancel/replace the flight by
discarding all pending grants and key references. Committed earlier session
updates retain their normal partial-publication semantics.

## Keys, domains and material exposure

Supersede the earlier suggestion to derive this protocol from
`hostutility.profileKey`. Do not expose or reuse that cache key, installer keys,
ownership-proof derivation, or provider credentials.

Proposed derivation contract:

- Generate a fresh independent 32-byte seed in backend memory. With the batch ID
  as salt, HKDF-SHA256 derives a 32-byte comparison key using info
  `kandev/session-catalog-context/v1/compare`.
- Derive separate 32-byte reply keys with length-delimited info containing
  `kandev/session-catalog-context/v1/reply`, role, target instance, expected
  manager/generation and request nonce. Probe and each live subject get
  different reply keys.
- Derive the request-purpose-grant key from the currently authenticated control
  credential, with instance ID as salt and info
  `kandev/session-catalog-context/v1/request-grant`. It remains within the
  backend client and authenticated server handler. The grant MAC covers the
  entire bounded canonical request, including derived material, owner binding,
  role, nonce and expiry. This new control-credential derivation also requires
  security review; it is not implicit approval to extend existing auth.
  Exclude the grant field itself from its MAC input.
- Each agentctl handler receives only its comparison key and subject reply key,
  never the batch seed, another subject's reply key, or a cache/provider key.
  None enters child environment, argv, ACP frames or saved configuration.
- The context fingerprint is HMAC-SHA256 with the comparison key and domain
  `kandev/session-catalog-context/v1/snapshot`. The reply MAC uses its subject key
  and domain `kandev/session-catalog-context/v1/response`, covering role,
  batch/nonce, binding, actual owner, fingerprint and expiry (excluding its own
  MAC). The purpose grant uses its key and domain
  `kandev/session-catalog-context/v1/request`.

Use shared byte-preserving length framing: fixed-width counts/integers and
length-prefixed raw string/byte fields, not delimiter concatenation or JSON
replacement of invalid UTF-8 environment/argv bytes. Verify MACs in constant
time. Common snapshot encoding excludes proof role/owner/nonce/time metadata
because ownership is separately bound by the reply MAC. It does not remove
similarly named variables from the actual child environment.

Keys, grants and fingerprints are internal/request-scoped: no business capability
cache, SQL/session metadata, frontend DTO, traces, debug ring, raw request/response
logging or metrics. Only sanitized proof availability may affect disposition.
Use no-store responses, bounded bodies (proposed 4 KiB request/2 KiB response),
static redacted errors and existing admission bounds. Go key references can be
released; do not promise perfect zeroization of runtime/network-buffer copies.

## Comparison and invalidation

Hash actual successful command inputs after the existing process builders:
`exec.Cmd.Environ()` with exact effective key presence/empty values; resolved
executable and available executable identity; complete final argv including
wrapper expansion; actual cwd/directory identity; and recorded runtime selection.
Canonical environment ordering is not another precedence resolver. Missing
executable/directory identity is unproven. No credential-name, prefix, cwd, argv,
model, or generated-key exclusion is approved.

Keep application-side binding evidence separate: capture winning source/binding
identity through the existing resolver, without retaining new raw secret values
or reimplementing precedence. Current secret `Get` and `Reveal` are separate
operations and `UpdatedAt` is not a numeric/atomic generation API. Do not infer
an atomic binding/value generation from two equal timestamp reads; unavailable
consistency evidence stays unknown. A new secret-store atomicity contract/schema
is outside this proposal's approval.

Native evidence describes the sealed launch snapshot, not a reconstruction with
today's ambient environment. A new probe captures its own actual inherited
environment, so drift before that probe participates in comparison. Drift during
the flight must be fenced by an owned ambient/configuration generation or an
explicit immutable-runtime contract. If none can prove stability, fail closed;
two observations or a short TTL do not establish atomicity. Do not silently add
an ambient watcher, copy task secrets/identities into the probe, or align builders.

Invalidate on credential rotation/invalidation, runtime lease retirement,
instance deletion/port reuse, manager/process replacement, startup/source
generation change, configuration revision change, saved-profile/probe currency
change, binding/update change, expiry or caller cancellation. Recheck the actual
owner under the existing publication fences. Unsupported/legacy/unknown proof
permits notice-only, never installer-digest fallback.

This mechanism attests a launch snapshot, not provider account state or semantic
catalog independence. Provider-loaded auth/project files and protocol/session
configuration remain consumer-contract questions; matching paths alone do not
prove their contents/generations equal. Positive qualification still requires the
supported-provider contract and real boundary tests.

## Review and verification gates

Before any implementation: review feasibility, purpose-grant auth and derivation,
proof lifetimes, binding/ambient consistency and the selected consumer contract.
Then require behavioral RED/GREEN for actual equivalent context and every stated
mismatch, missing auth/header/manager identity, wrong subject/role, proxy oracle
attempts, credential rotation, replay/expiry, runtime/manager/source replacement,
binding/ambient drift, cancellation and stale SQL CAS. Privacy tests must observe
the real request/response/logging boundary and reject leaked key/env material.
No proposal approval, production implementation or runtime verification has
occurred. Existing cache, Docker and PostgreSQL gates remain in the work orders.
