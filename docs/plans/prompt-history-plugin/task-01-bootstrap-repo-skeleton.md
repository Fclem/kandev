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
  "kandev"`, and `repo_url`; renaming the template's own package-name
  references with it (`release.yml` README sed pattern, `release.yml`
  Extract checksums `tar -xzf` glob, `release.yml` release-asset glob,
  `Makefile` `clean` archive glob).
- Replacing the template `README.md` demo-surface documentation (the
  `/template` nav route, host-component page, webhook and `config_schema`
  surfaces) with plugin-specific content - identity, `api_read:
  ["messages"]`, install via Settings > Plugins, the sibling-worktree dev
  layout, and the make targets - keeping the
  `kandev-plugin-prompt-history-0.1.0.tar.gz` archive name mentioned once
  so the `release.yml` README sed keeps applying. Also removing the
  source-control recipe pitch, the `recipes/`+`package.json`+
  `tsconfig.recipes.json` Layout entries, the "hand-written, no build
  step" `ui/bundle.js` paragraph (Task 02's esbuild toolchain replaces
  it), and the recipe-only `npm ci --ignore-scripts` install line, and
  keeping the "Developing against the SDK" heading that `go.mod`'s
  comment cites.
- The manifest: `api_version: 2`, `min_kandev_version: "0.95.0"` (the first
  release carrying the #3588 browser conversation facade; confirm at release
  cut), `capabilities: { api_read: ["messages"] }`, `ui.bundle: "/ui/bundle.js"`,
  `ui.styles: ["/ui/plugin.css"]`, a one-line `description`,
  `categories: ["tools"]`, all five platform executables; webhooks, actions,
  `config_schema`, events, `state`, `secrets`, `agent_invoke`, providers,
  and agent tools removed.
- `server/`: no-op `pluginsdk.UnimplementedPlugin`.
- `ui/bundle.js`: placeholder task panel registration
  (`registerTaskPanel` with `title`, `titleKey`, `mobileEnabled: true`, a
  bundled icon component, and no `visible` predicate) and the
  translation-catalog skeleton (en, pt-pt, zh-cn, zh-hk, zh-tw, pseudo),
  plus a placeholder `ui/plugin.css` (the real stylesheet lands in Task 02).
- Pinned SDK reference: the template's `go.mod` `replace` and
  `KANDEV_SDK` are plain paths with no ref (the template's root
  `package.json` `file:` devDependency goes away with the recipe slice;
  the SDK `file:` dependency exists only in the `ui/package.json` Task 02
  creates); only the CI checkout `ref:` values pin `f218880e`, which
  predates the facade. Bump every CI `ref:` to the PR #3588 merge commit
  (`2b1d0cf7d`) or later, rename every CI checkout `path: kandev` to
  `kdlbs-kandev` (the `ci.yml` verify job, the `ci.yml` `base-floor` job,
  `release.yml`, and `build.yml`), add `apps/packages/plugin-sdk` to
  `build.yml`'s sparse checkout (Task 02's `make ui-install` resolves the
  `file:` dependency against it), and point the `go.mod` `replace` and
  `KANDEV_SDK` at the monorepo worktree (the template assumes a sibling
  directory literally named `kandev`; update the references to the
  worktree's directory name; the `ui/package.json` created in Task 02
  re-adds the `file:` dependency at the worktree path).
- Strip the template's recipe slice (the `recipes/` directory,
  `tsconfig.recipes.json`, the Makefile
  `test-recipes`/`typecheck-recipes`/`audit-recipes` targets and the
  `./recipes/...` paths in `go test` and `go vet`, the root
  `package.json`/`package-lock.json`, and the `make audit-recipes` steps in
  both `ci.yml` and `release.yml`): this is a panel-only plugin with no
  recipes. Also remove the template's root `npm ci --ignore-scripts` and
  `Set up Node` (cache `plugin/package-lock.json`) steps from `ci.yml` and
  `release.yml` (the Go-only skeleton needs no Node; Task 02 re-adds pnpm
  setup plus the `kandev-plugin-voice` UI CI steps to all three
  workflows). After the strip, `make test` is `test-backend` (Go-only);
  Task 02 changes it to `test-backend test-ui`. Keep the `base-floor` CI
  job (it builds the backend against the declared minimum SDK) but point
  its `ref:` at the PR #3588 merge commit (`2b1d0cf7d`) as a stand-in for
  the floor until the floor release is cut, then update it to the floor
  tag; rename the job to match its new ref (the template names it after
  the pinned ref, "Default template on Kandev v0.86.0"), and rewrite the
  sibling-path checkout comment in `ci.yml` (it cites
  `plugin/../kandev/apps/backend`) to `kdlbs-kandev` (`build.yml` and
  `release.yml` carry no such comment).

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
- CI is green on the first push of the new repository (the stripped
  template tree plus the renames and bumps must leave all three workflow
  files passing).

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
- The template's `go.mod` `replace`, `KANDEV_SDK`, and CI
  `path: kandev` all assume a sibling directory literally named
  `kandev` (its root `package.json` `file:` devDependency does too, but
  that file is removed with the recipe slice); the monorepo worktree is
  named `kdlbs-kandev`, so Task 01 updates the `go.mod`/`KANDEV_SDK`/CI
  references and Task 02's `ui/package.json` re-adds the `file:`
  dependency at the worktree path.

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
