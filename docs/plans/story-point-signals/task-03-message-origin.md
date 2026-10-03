---
id: "03-message-origin"
title: "Prompt origin and author filter"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-EFFORT-SIGNALS-001
  - REQ-PLUGINS-EFFORT-SIGNALS-005
acceptance_criteria:
  - AC-PLUGINS-EFFORT-SIGNALS-001.1
  - AC-PLUGINS-EFFORT-SIGNALS-001.2
  - AC-PLUGINS-EFFORT-SIGNALS-001.3
  - AC-PLUGINS-EFFORT-SIGNALS-001.4
  - AC-PLUGINS-EFFORT-SIGNALS-001.5
  - AC-PLUGINS-EFFORT-SIGNALS-001.6
  - AC-PLUGINS-EFFORT-SIGNALS-005.1
system_design:
  - ../../specs/plugins/system-design/effort-signals-host.md
---

# Task 03: Prompt origin and author filter

## Summary

Add `Origin` to the plugin `Message` DTO, derived at read time from stored
message metadata with the first-match table in the design, and `AuthorTypes` to
`MessageFilter`. A plugin can then count human prompts without parsing text.

## In scope

- `models.ClassifyMessageOrigin` and the `MessageOrigin` constants in
  `internal/task/models` (imported by `internal/plugins` and by orchestrator
  tests; neither package imports the other), called from
  `messageModelToDTO`, with the order `sender_task_id` -> `source` prefix
  `plugin:` -> `origin` of `github_pr_automation`, `ci_automation`, or the GitLab
  MR origin -> `workflow_message` -> bare `auto_start` -> `human`; agent rows are
  `agent`. A bare `auto_start` initial message (`recordInitialMessage`) is
  `automation` by decision and is covered by the creator test.
- Create `pkg/pluginsdk/host_parity_test.go` with `TestHostSDKProtoParity`: maintain
  an explicit SDK-method-to-Host-RPC mapping table (some SDK methods have names
  different from their RPCs). For every mapping, call the RPC over bufconn with a
  sentinel Host and assert it does not return `Unimplemented`; reflection on
  `grpcHostServer` is insufficient because promoted methods from
  `UnimplementedHostServer` appear present without handlers. Add a field
  round-trip table. Enumerate any existing Host method without an RPC/default as
  an explicit exception. Tasks 04 to 06 extend the mapping and round-trip tables.
- `Origin` in proto, SDK, gRPC mapping; `AuthorTypes` in `MessageFilter`,
  `PluginMessageFilter`, and `ListMessagesForPlugin`, counted toward the
  400-value bound.
- Enumerate every creator of user-authored messages (orchestrator, queue drain,
  workflow, PR/MR/CI automation, watchers, MCP peer messages, every
  `Messages().Send` and `StartOrPrompt` path) and confirm each marker is in the
  table. Confirm that plugin sends write `source` as `plugin:<id>`; if a path
  writes a bare source, stop and update the design first.
- Test the queue path: a queued prompt delivered by drain
  (`CreateUserMessageIdempotent` with `metadataWithoutQueueOnlyKeys`) and the
  workflow auto-start pre-recorded row both keep their origin, counted once.
- Exact `ListSanitizedMessagesExact` carries the same field if its DTO mirrors
  `Message`.
- Regenerate proto stubs with `make -C apps/backend proto`.

## Out of scope

- An SQL origin filter; browser conversation facade changes.

## Acceptance

- Table test: every marker row yields its origin, including PR automation as
  `automation` and the output of `workflowMessageMetadata` (which also carries
  `auto_start`) as `workflow`; unmarked user row `human`; agent row `agent`; no
  metadata key reaches the DTO.
- `TestUserMessageCreatorsHaveClassifiedOrigin` fails when a creator's marker is
  missing from the table.
- `AuthorTypes` narrows results and counts toward the 400-value bound.

## Verification

```bash
(cd apps/backend && make proto && go test ./internal/plugins/ ./internal/task/models/ ./pkg/pluginsdk/ -run 'TestClassifyMessageOrigin|TestMessageListAuthorTypesFilter|TestHostSDKProtoParity')
(cd apps/backend && go test ./internal/orchestrator/ -run 'TestQueuedPromptKeepsOrigin|TestUserMessageCreatorsHaveClassifiedOrigin')
(cd apps/backend && make fmt lint)
```

## Files likely touched

- `apps/backend/proto/kandev/plugin/v1/plugin.proto` and generated code
- `apps/backend/pkg/pluginsdk/data_types.go`
- `apps/backend/internal/plugins/host_data.go`, `host_data_mappers.go`, `host_exact_queries.go`
- `apps/backend/internal/task/models/models.go`, task repository `ListMessagesForPlugin`

## Dependencies

None.

## Risks

- Falling back to `human` hides an unmarked generator; the creator enumeration
  test is the guard.

## Parallelism

`sequential`

## Inputs

- Design section: Prompt origin.
- `apps/backend/internal/orchestrator/message_meta.go` (`UserMessageMeta.ToMap`),
  `event_handlers_github_pr_automation.go`, `event_handlers_github_ci_automation.go`,
  `event_handlers_gitlab_mr_automation.go`, `internal/backendapp/adapters_plugin_messenger.go`.

## Results

Pending.
