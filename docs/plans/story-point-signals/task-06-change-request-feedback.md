---
id: "06-change-request-feedback"
title: "Change-request feedback read"
status: completed
wave: 4
depends_on:
  - "05-code-pr-summary"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-EFFORT-SIGNALS-004
  - REQ-PLUGINS-EFFORT-SIGNALS-005
acceptance_criteria:
  - AC-PLUGINS-EFFORT-SIGNALS-004.1
  - AC-PLUGINS-EFFORT-SIGNALS-004.2
  - AC-PLUGINS-EFFORT-SIGNALS-004.3
  - AC-PLUGINS-EFFORT-SIGNALS-004.4
  - AC-PLUGINS-EFFORT-SIGNALS-004.5
  - AC-PLUGINS-EFFORT-SIGNALS-004.6
  - AC-PLUGINS-EFFORT-SIGNALS-004.7
  - AC-PLUGINS-EFFORT-SIGNALS-005.1
system_design:
  - ../../specs/plugins/system-design/effort-signals-host.md
---

# Task 06: Change-request feedback read

## Summary

Add the exact read `ListChangeRequestFeedbackExact` returning provider-neutral
reviews and comments with bounded bodies for a task's linked GitHub pull
requests and GitLab merge requests, behind the existing
`host.v2.read:change_requests` workspace approval.

## In scope

- Proto RPC and SDK page types: `[]ChangeRequestFeedbackTask`, each task with
  provider results and nested `[]ChangeRequestFeedbackItem`; the provider result
  contains `provider`, `provider_state`, `provider_error`, and `items_omitted`.
  Register in `exactReadMethods` (`host_exact_queries.go`) and
  `exactReadTaskDataMethods` (`host_exact.go`), use `exactReadAdmission` and
  `exactPage`. Extend `TestHostSDKProtoParity` and re-verify
  `MinimumSessionActivityVersion` with `TestSessionActivityMinimumVersion`.
- Bounds: at most 20 tasks (`ResourceExhausted` above), at most the 50 newest
  items across providers per task, and a 7 MiB encoded-size budget measured on
  the same `json.Marshal` encoding used for the snapshot (`exact_read_snapshots.go`;
  5000-row and 8 MiB caps). `<`, `>`, and `&` encode as 6 bytes. Drop oldest
  items first and increment `items_omitted` for that item's provider.
- GitHub adapter over `ListPRReviews` and `ListPRComments`: review `author_kind`
  is `bot` for a `[bot]` login suffix and `unknown` otherwise, `updated_at`
  equals `created_at`, `resolved` absent; comments use `AuthorIsBot` and
  `UpdatedAt`. GitLab adapter over `GetMRFeedback` approvals and discussions.
  Extending provider readers is allowed but not required.
- Body sanitization and 4096-byte rune-boundary truncation with
  `body_truncated`.

## Out of scope

- Persisting comments; providers other than GitHub and GitLab; review-thread
  resolution for GitHub.

## Acceptance

- For a task linked to both providers, each single-source failure returns the
  successful provider's items, reports that provider `available` and the failed
  provider `unavailable`, and includes the coded provider error. The regression
  test covers both GitHub-fails/GitLab-succeeds and the reciprocal case.
  Successful providers with no linked requests report `available` and an empty
  item list; linked requests without an adapter report `unsupported`.
- A busy change request returns the 50 newest items across providers with
  omitted counts attributed per provider. Items with identical timestamps and
  provider-scoped IDs remain deterministically ordered by provider, repository,
  request number, kind, and ID. A page whose bodies are all `<` characters stays
  under the budget; 21 tasks fail `ResourceExhausted`.
- Declared resource without workspace approval is denied; a foreign-workspace
  task is rejected with no existence signal.
- No body, login, or token appears in captured logs.

## Verification

(cd apps/backend && make proto && go test ./internal/plugins/ ./pkg/pluginsdk/ ./internal/plugins/manifest/ -run 'TestListChangeRequestFeedbackExact|TestFeedbackProviderFailureIsPerProvider|TestFeedbackBoundsAndTruncation|TestFeedbackOrderingUsesProviderTieBreakers|TestHostSDKProtoParity|TestSessionActivityMinimumVersion')
```

## Files likely touched

- `apps/backend/internal/plugins/host_exact_queries.go` (+ new `host_exact_feedback.go`)
- `apps/backend/pkg/pluginsdk/exact_queries.go`, `exact_queries_grpc.go`
- `apps/backend/proto/kandev/plugin/v1/plugin.proto` and generated code
- Provider adapters under `internal/github/`, `internal/gitlab/` only if a narrow reader is needed
- `apps/backend/internal/plugins/host_exact.go`, `internal/plugins/manifest/min_version_policy.go` (verify only)

## Dependencies

Task 05.

## Risks

- Live fetch rate limits; each first page is a full provider fan-out because
  `exactPage` does not join concurrent calls.
- GitLab discussion shape differs from GitHub reviews; map to the same kinds
  without inventing review states.

## Parallelism

`sequential`

## Inputs

- Design section: Change-request feedback. Pattern: `ListChangeRequestEvidence`.

## Results

Completed. `ListChangeRequestFeedbackExact` (SDK `ListChangeRequestFeedback`) returns provider-neutral reviews and comments for a task's linked GitHub pull requests and GitLab merge requests, gated by `change_requests` and workspace admission. Bounds: at most 20 tasks, newest 50 items per task across providers, and a 7 MiB `json.Marshal` budget with `items_omitted` attributed per provider. Bodies are sanitized and truncated to 4096 UTF-8 bytes at rune boundaries. Each provider fails independently with a coded error; successful providers with no links report `available` with an empty list, and linked requests without an adapter report `unsupported`. GitLab approvals use `approval:<numeric user ID>` (decision: option B, current active state, not event history), so username changes and reapproval keep identity. GitLab reads use workspace-scoped credentials and reject missing or mismatched linked hosts. `MinimumSessionActivityVersion` remains 0.97.0.

Verification (parent rerun): `make proto`, then `go test -count=1 ./internal/plugins/ ./pkg/pluginsdk/ ./internal/plugins/manifest/ ./internal/gitlab/ -run 'TestListChangeRequestFeedbackExact|TestFeedbackProviderFailureIsPerProvider|TestFeedbackBoundsAndTruncation|TestFeedbackOrderingUsesProviderTieBreakers|TestHostSDKProtoParity|TestSessionActivityMinimumVersion|TestListChangeRequestFeedbackExactWorkspaceHost|TestFeedbackApprovalReaderRetainsUserID'` passed. Child reports: `make fmt lint` 0 issues; `go test ./internal/plugins/... ./pkg/pluginsdk/...` passed; a standalone HTTP smoke confirmed user ID 42 retained across a reapproval. PostgreSQL not run: `KANDEV_TEST_POSTGRES_DSN` unset. Provider matrix tests have no independent pre-implementation RED; the read-level RED/GREEN evidence is recorded in the child's report.

Amendment: the floor is now 0.98.0 (see Task 04 amendment). Round 2 review moved plugin GitHub feedback reads to the workspace automation credential, because plugin Host RPCs carry no user identity.
