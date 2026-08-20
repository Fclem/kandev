---
id: "01-design-package"
title: "DeepSeek credits plugin design package"
status: done
wave: 0
depends_on: []
plan: "plan.md"
spec: "../../specs/deepseek-credits-plugin/spec.md"
---

# Task 01: DeepSeek credits plugin design package

## Intent

Produce the durable design package for the DeepSeek credits plugin: feature
spec, implementation plan, and task files, committed in the Kandev monorepo
following the `bitbucket-plugin` precedent.

## Owned paths

- `docs/specs/deepseek-credits-plugin/spec.md`
- `docs/plans/deepseek-credits-plugin/plan.md`
- `docs/plans/deepseek-credits-plugin/task-01..07-*.md`

## Acceptance

1. The spec describes observable behavior, the action/manifest/API contracts,
   failure modes, persistence guarantees, and GIVEN/WHEN/THEN conformance
   scenarios; it names `kdlbs/kandev-plugin-deepseek-credits` as the target
   repository and `kandev-deepseek-credits` as the plugin id.
2. The plan names exact files, dependency order, tests, the E2E boundary
   (bundle tests + disposable-instance smoke test; plugin repos run no
   Playwright suite), and the repo-ownership risk.
3. Task files 02–07 carry the required frontmatter, acceptance conditions,
   exact verification commands, dependencies, and risks, and are each
   independently executable.

## Results

Produced 2026-08-19: spec (`docs/specs/deepseek-credits-plugin/spec.md`),
plan (`docs/plans/deepseek-credits-plugin/plan.md`), task files 01–07
(`docs/plans/deepseek-credits-plugin/`). Research grounding: the
`kandev-plugin-provider-usage` repository (pill/hover-panel anatomy, poller,
Makefile, bundle tests), the `kandev-plugin-template` repository, the monorepo
plugin contracts (`apps/backend/pkg/pluginsdk`, manifest actions model,
`apps/web/lib/plugins/host-api.ts`, `docs/public/plugins-authoring.md`),
DeepSeek's official `GET /user/balance` documentation, and git history pinning
authenticated plugin actions to host `v0.88.0` (PR #2117).
