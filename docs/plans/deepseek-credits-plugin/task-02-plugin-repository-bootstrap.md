---
id: "02-plugin-repository-bootstrap"
title: "Dedicated plugin repository bootstrap"
status: pending
wave: 0
depends_on: []
plan: "plan.md"
spec: "../../specs/deepseek-credits-plugin/spec.md"
---

# Task 02: Dedicated plugin repository bootstrap

## Intent

Create public `kdlbs/kandev-plugin-deepseek-credits` from the official plugin
template and materialize it as a sibling of the Kandev checkout. Rename the
plugin identity, set the host-version floor, trim example-only content, and
prove the template skeleton builds and packages. No product behavior in this
task.

## Owned paths

- The attached plugin worktree only (never a nested clone inside this monorepo).
- Template-owned manifest, package, CI, backend, and UI skeleton files.

## Dependencies

None.

## Acceptance

1. The repository is public, template-derived, and attached to this task as a
   sibling worktree of the Kandev monorepo checkout (via
   `add_branch_to_task_kandev` with the repository URL once it exists, or a
   local sibling materialization). The SDK `replace` in `go.mod` resolves:
   provide `../kandev` as a symlink to the monorepo checkout (or adjust the
   `replace` target to `../kdlbs-kandev/apps/backend`); keep the divergence
   minimal and documented.
2. Identity is renamed everywhere the template carries it:
   `manifest.yaml` (`id: kandev-deepseek-credits`, plus
   `display_name: "DeepSeek Credits"` — pinned, because the panel's
   Settings-path copy and task-07 navigation reference it — plus
   `description`, `author`), `README.md` (title and body references to
   `kandev-plugin-template`), `go.mod` module line, and the two server
   strings that carry the template name — `server/plugin.go`'s error
   string and `server/main.go`'s doc comment — which are disposed
   wholesale by task 04's rewrite (listed here so "everywhere" is
   literally true), `Makefile` (`BIN`,
   `PKG_OUT`, `VERSION`, and the `clean` target's tarball glob
   `kandev-plugin-template-*.tar.gz` → `kandev-deepseek-credits-*.tar.gz`),
   `ui/bundle.js` registration id AND its line-39 rename comment (both carry
   the template name; the comment is disposed by task-05's full-file
   rewrite, listed here so "everywhere" is literally true), and the
   workflow identity patterns in task acceptance 4.
   `min_kandev_version: "0.88.0"` (first release with authenticated plugin
   actions, PR #2117).
3. Example-only surfaces are trimmed consistently so the kept targets still
   run: `recipes/`, `package.json`, `package-lock.json`, `tsconfig.recipes.json`
   and the recipe-only `Makefile` targets (`test-recipes`,
   `typecheck-recipes`, `audit-recipes`) are removed; the `test` target is
   rewired to `test-backend` (task 05 adds the bundle test to it, since
   `test/bundle.test.mjs` does not exist yet at bootstrap), and the recipe
   path is dropped from the `test-backend` and `vet` targets (the template's
   `go test ./server/... ./recipes/source-control/server/...` and
   `go vet ./server/... ./recipes/source-control/server/...` fail with "no
   packages match" once `recipes/` is gone; provider-usage uses plain
   `go test ./server/...`). ALL non-recipe backend/package targets are KEPT
   — `test-backend`, `vet`, `build`, `package`, `package-host`,
   `verify-package`, `verify-package-host` (task-06 requires
   `make verify-package` for the five-platform CI check) — even though
   provider-usage's own Makefile names its targets differently. The manifest claims no capabilities beyond `runtime` + `ui`;
   any leftover template webhook/event/state scaffolding must not claim
   product behavior (task 06 owns the final manifest contract).
4. The template CI is retargeted for the trimmed repo AND the actions
   contract: the `base-floor` job (ci.yml, currently pinned to `ref: v0.86.0`)
   is bumped to `v0.88.0` and renamed, because this plugin's server implements
   `pluginsdk.HandleAction` (types added in `f218880ec`/PR #2117, first tagged
   `v0.88.0`) and will not compile against the v0.86.0 SDK. The `build.yml`
   SDK ref (`f218880ec…`) already satisfies the floor. The recipe
   steps are stripped from BOTH workflow files alongside the trim: in ci.yml's
   **verify** job and in release.yml's publish **Verify** step, drop `npm ci
   --ignore-scripts`, the `make audit-recipes` step, the
   `package-lock.json` cache path (`cache-dependency-path:
   plugin/package-lock.json`), AND the `cache: npm` input on the Setup Node
   step — with the lockfile deleted, `actions/setup-node@v6` with `cache: npm`
   and no `cache-dependency-path` hard-fails ("Dependencies lock file is not
   found", looked up in the workspace root, not `plugin/`). Keep
   `node-version` and setup-node itself so the bundle tests (`node --test`)
   still run. release.yml has THREE `kandev-plugin-template`
   name references, all renamed with the plugin: the prepare-job README sed
   (line ~114), the publish "Extract checksums" step's tarball glob
   (`tar -xzf kandev-plugin-template-*.tar.gz checksums.txt`, line ~204 —
   stale here means the extract matches nothing and `tar` fails at tag-push
   time), and the publish "Create GitHub release" `files:` glob
   (`plugin/kandev-plugin-template-*.tar.gz`, line ~211); all three become
   `kandev-deepseek-credits-<version>.tar.gz` spellings. release.yml only
   triggers on tag push / workflow_dispatch, so CI on PRs cannot catch a
   stale name. First CI run on the plugin repo must be green.
5. The Makefile `fmt` target is CHANGED to the CI-style failing check:
   `test -z "$$(gofmt -l .)"` — the template (and provider-usage) ship the
   bare `gofmt -l .` which exits 0 on unformatted files, so this is an
   explicit edit, not an inherited state.
6. The skeleton builds and packages: `make verify-package-host` produces a
   tarball whose contents are `manifest.yaml`, the host-platform executable
   under `server/`, `ui/bundle.js`, and a generated internal `checksums.txt`.

## Verification

Run from the plugin worktree:

```sh
make fmt
make vet
make test-backend
make build
make verify-package-host
grep -q 'test -z "$$(gofmt -l .)"' Makefile
```

Confirm the archive layout with `tar tzf kandev-deepseek-credits-<version>.tar.gz`.

## Risks

- **Repo creation requires a `kdlbs` maintainer.** The authenticated GitHub
  account has no `kdlbs` org membership, so `gh repo create
  kdlbs/kandev-plugin-deepseek-credits --template kdlbs/kandev-plugin-template
  --public` may be refused. Bootstrap request: have a maintainer create the
  repo from `kdlbs/kandev-plugin-template` (public, no initial content), then
  attach it. Fallback that does not block local work: create the public repo
  under the author account from the template and request a later transfer;
  the manifest `repo_url` is updated when the home moves.
- Template drift can invalidate guessed commands or archive layout; use the
  template's current documented targets.
- The plugin worktree must resolve the SDK `replace`; a missing `../kandev`
  checkout breaks `go build` and packaging.
