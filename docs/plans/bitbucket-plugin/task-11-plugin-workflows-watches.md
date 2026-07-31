---
id: "11-plugin-workflows-watches"
title: "Plugin task, Git, linking, and watch workflows"
status: completed
wave: 3
depends_on: ["03-protocol-manifest-actions", "05-dynamic-composer-reference-sources", "06-plugin-owned-task-lifecycle", "07-provider-neutral-git-credentials", "10-cloud-dc-domain-auth"]
plan: "plan.md"
spec: "../../specs/bitbucket-plugin/spec.md"
---

# Task 11: Plugin task, Git, linking, and watch workflows

## Intent

Use released host seams to implement Bitbucket plugin actions/RPC handlers for task
launching, scoped Git credentials, PR linking/creation, and durable watches.

## Owned paths

- Attached `kdlbs/kandev-plugin-bitbucket` worktree: action/RPC handlers, task and
  Git workflow services, link storage, credential resolver, health poll integration,
  watch persistence/poller/recovery, events, and focused tests.

## Dependencies

Tasks 03, 05, 06, 07, and 10.

## Acceptance

1. Authenticated actions launch tasks from PRs, link/unlink tasks, create PRs from
   task branches, and resolve Git credentials without secrets crossing host boundaries.
2. Plugin `pull_request` search and authorization use the live Cloud/DC adapter for
   both composer search and message submission; they do not bypass host canonicalization.
3. Watches persist definition/cursor/dedupe/reservation/link/recovery state, use keyed
   mutex plus durable `creating` reservation, reconcile crashes, and emit
   `plugin.kandev-plugin-bitbucket.*` events.
4. Reset/delete previews cascade and removes only plugin/watch-owned task trees;
   manually linked and adopted tasks survive.

## Verification

```sh
make test
make vet
make build
```

## Risks

Concurrent polls and crash timing can duplicate tasks. Credential refresh and ownership
checks must fail closed; avoid adding Bitbucket logic to host `agentctl` PR automation.
