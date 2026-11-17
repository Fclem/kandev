---
id: "07-star-actor-e2e"
title: "Prove multi-scheme star flows"
status: pending
wave: 7
depends_on:
  - 06-star-actor-controls
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.2
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.4
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.1
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.3
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.4
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.5
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.6
system_design:
  - ../../specs/integrations/system-design/github-repository-star-actors.md
---
# Task 07: Prove multi-scheme star flows

## Summary

Prove the multi-scheme actor end to end in the browser, using the E2E-gated mock provider, and update public documentation that currently states that stars use only the dedicated OAuth App. These flows need no deployment-secret fixture: the workspace-credential and App-personal actors work without the dedicated flow.

## In scope

- Extend `apps/web/e2e/tests/auth/plugin-repository-stars.spec.ts` (auth project, existing `backendFixture` restart pattern) with:
  - A workspace PAT actor: an admin and a member both see shared disclosure, star, unstar, and reload state.
  - An App-installation workspace without a personal connection: Connect GitHub calls the personal-connection start.
  - An App personal-connection actor: personal disclosure and star.
  - An incapable actor: no control and no CTA.
  - A mid-session `403`: the control disappears silently, and no request repeats under another actor.
- Extend `apps/web/e2e/helpers/api-client.ts` mock controls for workspace source, App personal connection, scope-probe outcome, and `403` injection from Task 05.
- Update `docs/public/integrations.md` (routing table row and the "strict exception" paragraph), `docs/public/plugins-marketplace.md` (availability, disclosure, shared account), and `docs/public/feature-status.md`. Add `Starring` to the App permission list where the docs enumerate it.
- Update `apps/web/e2e/README.md` if the star fixture guidance changes.

## Exclusions

- Real GitHub calls or credentials; the LAN demo is not a test target.
- New Playwright projects or geometry assertions (covered by Task 03).

## Acceptance

1. The auth spec passes every scenario above against the mock provider, with causal waits and no `waitForTimeout`.
2. The public-docs validators pass. No public page still claims that stars require the dedicated OAuth App or never use PAT/`gh` credentials.

## Verification

```bash
(cd apps/web && pnpm e2e:raw --project=auth tests/auth/plugin-repository-stars.spec.ts)
(cd apps/web && pnpm e2e:raw --project=chromium tests/settings/plugin-repository-stars.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Likely files

`apps/web/e2e/tests/auth/plugin-repository-stars.spec.ts`, `apps/web/e2e/tests/settings/plugin-stars-helpers.ts`, `apps/web/e2e/helpers/api-client.ts`, `apps/web/e2e/README.md`, `docs/public/{integrations,plugins-marketplace,feature-status}.md`.

## Risks

- The mock must model the actor precedence faithfully, or the E2E proves only the mock; keep the resolution logic in production code and mock only the GitHub HTTP boundary.
- Real-GitHub behavior for App user tokens with `Starring` cannot be proven here; record it as a residual gap in Results.
