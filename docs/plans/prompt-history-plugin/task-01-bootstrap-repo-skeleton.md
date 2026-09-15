---
id: "01-bootstrap-repo-skeleton"
title: "Bootstrap repo and installable skeleton"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-PLUGIN-001
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-PLUGIN-001.1
  - AC-PLUGINS-PROMPT-HISTORY-PLUGIN-001.2
  - AC-PLUGINS-PROMPT-HISTORY-PLUGIN-001.3
  - AC-PLUGINS-PROMPT-HISTORY-PLUGIN-001.4
system_design:
  - ../../specs/plugins/system-design/prompt-history-plugin.md
---

# Task 01: Bootstrap Repo and Installable Skeleton

## Summary

Create `kdlbs/kandev-plugin-prompt-history` from `kdlbs/kandev-plugin-template`
with the production identity and a minimal installable plugin: a manifest
declaring only `api_read: ["messages"]`, a no-op Go backend, and a UI bundle
that registers a placeholder prompt-history task panel. The package must
install, activate, and uninstall cleanly on a disposable development
instance.

## In scope

- Creating the public `kdlbs/kandev-plugin-prompt-history` repository from
  the template, preserving its packaging, test, and release safeguards.
- Renaming the identity in all four synchronized places (manifest `id`,
  `go.mod` module, Makefile `BIN`/`PKG_OUT`/`VERSION`,
  `window.registerKandevPlugin` id) and setting `display_name`, `author:
  "kandev"`, and `repo_url`.
- The manifest: `api_version: 2`, `min_kandev_version: "0.95.0"` (the first
  release carrying the #3588 browser conversation facade; confirm at release
  cut), `capabilities: { api_read: ["messages"] }`, `ui.bundle: "/ui/bundle.js"`,
  all five platform executables; webhooks, actions, `config_schema`, events,
  `state`, `secrets`, `agent_invoke`, providers, and agent tools removed.
- `server/`: no-op `pluginsdk.UnimplementedPlugin`.
- `ui/bundle.js`: placeholder task panel registration (`registerTaskPanel`,
  `mobileEnabled: true`, localized title) and the translation-catalog
  skeleton (en, pt-pt, zh-cn, zh-hk, zh-tw, pseudo).
- Pinned SDK reference: bump the template's pinned kandev ref (CI checkout
  `ref:`, currently `f218880e`, which predates the facade) and the local
  sibling checkout to the PR #3588 merge commit (`2b1d0cf7d`) or later, so
  the pinned `@kandev/plugin-sdk` carries the conversation types; point the
  `go.mod` `replace` and `KANDEV_SDK` at the monorepo worktree (the template
  assumes a sibling directory literally named `kandev`; update the reference
  to the worktree's directory name).

## Out of scope

- The parity panel implementation (Task 02).
- Parity proof and cross-platform packaging verification (Task 03).
- Release tagging and marketplace catalog entry.

## Acceptance

- The repository exists, its default branch contains the template-derived
  tree with the production identity, and the manifest, Go module, Makefile,
  and UI registration id are synchronized on `kandev-plugin-prompt-history`.
- `make verify-package` passes in the plugin repo.
- The packaged archive installs on a disposable development instance, the
  placeholder panel appears in the desktop add-panel menu and the mobile
  Panels picker, and disabling plus re-enabling the plugin restores the
  registration without error.

## Verification

```bash
cd ../kandev-plugin-prompt-history   # sibling of the monorepo worktree
make vet test
test -z "$(gofmt -l .)"              # make fmt is advisory (lists, exits 0)
make verify-package
```

Disposable-instance smoke (from the monorepo worktree):

```bash
(cd apps && pnpm install --frozen-lockfile)
cd apps/web && pnpm e2e:raw -- e2e/tests/plugins/prompt-history-plugin.spec.ts
```

The last command is the existing fixture spec run only to boot the e2e
test-base backend and web assets; the placeholder install smoke is a manual
or scripted check against the same disposable instance (install the
`kandev-plugin-prompt-history-0.1.0.tar.gz` built by `make package-host`, open
a task, confirm the placeholder panel in the "+" menu, disable and re-enable
the plugin in Settings > Plugins).

## Files likely touched

- `kdlbs/kandev-plugin-prompt-history/manifest.yaml`
- `kdlbs/kandev-plugin-prompt-history/go.mod`
- `kdlbs/kandev-plugin-prompt-history/Makefile`
- `kdlbs/kandev-plugin-prompt-history/server/plugin.go`
- `kdlbs/kandev-plugin-prompt-history/ui/bundle.js`
- `kdlbs/kandev-plugin-prompt-history/README.md`
- `kdlbs/kandev-plugin-prompt-history/.github/workflows/*`

## Dependencies

None.

## Risks

- Repository creation is an external GitHub action; if `gh` cannot create
  `kdlbs/kandev-plugin-prompt-history`, stop and report the bootstrap
  request instead of substituting a directory in the monorepo.
- The template `go.mod` `replace` and `KANDEV_SDK` resolve the SDK against a
  sibling directory literally named `kandev`; the monorepo worktree is named
  `kdlbs-kandev`, so the references are updated to the worktree path as part
  of the identity rename.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/prompt-history-plugin.md)
  REQ-PLUGINS-PROMPT-HISTORY-PLUGIN-001.
- [System design](../../specs/plugins/system-design/prompt-history-plugin.md),
  Package and identity.
- `kdlbs/kandev-plugin-template` (source tree), `kdlbs/kandev-plugin-voice`
  (identity and packaging conventions of an official plugin).
- `apps/backend/internal/plugins/manifest` (manifest validation rules).

## Results

Pending.
