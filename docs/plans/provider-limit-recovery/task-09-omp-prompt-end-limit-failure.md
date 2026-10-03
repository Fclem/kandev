---
id: "09-omp-prompt-end-limit-failure"
title: "OMP prompt-end limit failure"
status: pending
wave: 2
depends_on:
  - "01-classify-limit-timing-and-scope"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-007
acceptance_criteria:
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.5
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.6
system_design:
  - ../../specs/agents/system-design/provider-limit-recovery.md
---

# Task 09: OMP Prompt-End Limit Failure

## Summary

OMP reports an Anthropic limit as an assistant `agent_message_chunk` and then
ends the prompt with `end_turn`, so Kandev records a successful turn today.
Convert a strict, final Anthropic error envelope from `omp-acp` into a terminal
provider failure. The failure carries the exact `retry-after-ms` delay and no
request ID, which lets the classifier and later recovery tasks act on it.

## In scope

- Candidate marking in `adapter_updates.go`. It applies to `omp-acp` only,
  matches a strict `429 {...rate_limit_error...}` envelope with an optional
  `retry-after-ms=<digits>` suffix, and is cleared by any later assistant
  chunk.
- A prompt-end branch in `adapter_prompt.go`, next to the Cursor and Codex
  branches. It emits `EventTypeError` with `ProviderError{Source: omp_acp,
  ProviderID: omp-acp, ModelID, Message, RetryAfterMs, OccurredAt}` and
  cancels async completion.
- The new `ProviderErrorSourceOMPACP` constant and its validation.
- An end-to-end Go test at the ACP boundary. It feeds the exact observed
  frames from the investigation into the adapter and passes the result
  through `classifyKanbanFailure`.

## Out of scope

- Fallback, waits, and marks (Tasks 03-07). Without them, the result is the
  existing recovery card.
- Other OMP upstream providers until their failure text is captured.

## Acceptance

1. The observed OMP frames for the issue line produce one terminal error
   event and no completion event. Classification gives account-scope
   `quota_limited` with reset `OccurredAt + 274579000ms`, and the persisted
   diagnostic has no `req_` ID.
2. These cases still complete normally: an envelope followed by more assistant
   output, an envelope quoted inside other text, ordinary text, the same text
   from `claude-acp` chunks, and malformed JSON.
3. Duplicate or late frames for an earlier prompt generation do not fail the
   current prompt.

## Verification

Run this complete block from the repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run 'TestOMPPromptEnd' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/adapter/transport/acp -count=1)
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run 'TestClassifyKanbanFailureOMP' -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/{adapter_updates.go,adapter_prompt.go,adapter_prompt_test.go}`
- `apps/backend/internal/agentctl/types/streams/provider_error.go`
- `apps/backend/internal/orchestrator/event_handlers_transient_omp_test.go`

## Dependencies

Task 01 (OMP rules, `RetryAfterMs`, request-ID redaction).

## Risks

- **Visible change for every OMP profile.** A spend-limit turn now shows the
  recovery card instead of a "completed" turn containing error JSON. This is
  intended. Mention it in the PR and the changelog.
- **Format drift.** The envelope match is pinned to OMP 18.5.0 output. A
  format change falls back safely to today's completion behavior.

## Parallelism

`parallel-safe` with Task 02 and Task 03. The files are disjoint.

## Inputs

- System design: OMP prompt-end failure conversion.
- `cursorRetriableFailureAt` and `codexCapacityFailure` patterns.
- Investigation by task `b447f478-8681-4edd-a846-b4e6ea7cd658`, which
  observed the frames.

## Results

Pending.
