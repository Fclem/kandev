---
id: "07-instance-smoke-test"
title: "Disposable-instance smoke test"
status: done
wave: 5
depends_on: ["06-manifest-config-package"]
plan: "plan.md"
spec: "../../specs/deepseek-credits-plugin/spec.md"
---

# Task 07: Disposable-instance smoke test

## Intent

Prove the packaged artifact against a disposable dev Kandev instance: install
the tarball, enable the plugin, and drive the real UI through every
user-facing state. This is the E2E evidence for the plugin-only change
(plugin repositories run no Playwright suite; see plan "E2E Tests").

## Owned paths

None (verification task; no production or permanent test files change).

## Dependencies

Task 06 (validated package).

## Acceptance

1. A fresh, disposable Kandev dev instance (never a developer's primary
   instance, database, or credentials) installs
   `kandev-deepseek-credits-0.1.0.tar.gz` via Settings → Plugins; the plugin
   activates without errors and the settings page renders the generated
   `config_schema` form.
2. Browser-driven checks pass, in order:
   - unconfigured: no key → pill shows the neutral unavailable indicator; the
     panel explains the settings path and `DEEPSEEK_API_KEY`;
   - invalid key: configure a fake `api_key` → panel shows the 401 reason,
     pill keeps no balance; then replace with a real key the operator supplies
     for this disposable test only (if none is available, record that the live
     balance path was not exercised and rely on the 401/unconfigured evidence);
     saving the key mid-run (host config-save restart) transitions
     unconfigured/invalid → balance WITHOUT a manual Refresh;
   - live balance: pill shows the formatted primary-currency balance; hover
     and click/tap open the panel with total, granted/topped-up, status,
     last-updated, and Refresh; the 60 s silent re-read issues no forced
     rebuild. Refresh is asserted with a SELF-ANCHORED cooldown window
     (do not anchor to the startup fetch — the human-paced panel checks take
     longer than 5 s, so the first click would land outside the window):
     first, click Refresh and wait for the panel's last-updated/fetched_at to
     advance (fetch N completes at T); immediately click Refresh again (the
     two clicks are well under 5 s apart, nothing in flight) → `200` with
     `fetched_at` UNCHANGED (cooldown evidence, zero new DeepSeek round
     trips); then sleep ~6 s and Refresh again → `fetched_at` advances;
   - disable: pill disappears from the top bar and no balance requests fire;
     re-enable: it returns and recovers;
   - non-owner 404 (while the plugin is installed and enabled): on the
     auth-enabled instance, with a workspace created under the enabled-auth
     admin (so `OwnerID` is set — an ownerless workspace is visible to every
     authenticated user and would fail the assertion), control-check the
     admin's own `balance.get` → `200` first, then create/invite a second
     user and call `balance.get` for the admin-owned workspace with the
     second user's session → `404` (workspace not visible). If a second user
     is impractical, cite the monorepo's
     `apps/backend/internal/plugins/handlers_test.go`
     workspace-not-visible → 404 case as the owning evidence instead.
   - upgrade: bump the manifest `version` to `0.1.1` together with the
     lockstep `Makefile` `VERSION` (task-06 acceptance 2 requires
     `VERSION == manifest version`), repackage, and install over `0.1.0` →
     the plugin activates, re-fetches balance immediately, and the vault-stored
     key still resolves without re-entry (reinstall, by contrast, purges the
     vault: uninstall then reinstall requires re-entering the key — cover
     both, matching the spec scenarios);
   - uninstall: plugin and its routes are gone; the vault-stored key is
     removed with it.
   - restore: after the upgrade check, revert `manifest.yaml` `version` and
     the lockstep `Makefile` `VERSION` to `0.1.0` (what task 06 left) and
     re-run `make test` — the manifest test shape-checks the version, so it
     passes at `0.1.1`, but the revert keeps the worktree at the task-06
     state and confirms the full suite is green there. Record the revert
     under teardown evidence.
3. Secret hygiene: the STORED key value never appears in any action
   response, plugin log, host API response, or request AFTER save (masked as
   `********` in the settings form; vault reference in `<id>.config.yml`) —
   the operator-entered value is expected to appear only in the settings form
   and its single save request, so exclude the save request itself from the
   no-disclosure check; no webhook route exists for the
   plugin (`GET/POST /api/plugins/kandev-deepseek-credits/webhooks/*` → 404);
   SEQUENCING: run ALL acceptance-3 curl checks while the plugin is still
   installed and enabled — i.e. before executing acceptance 2's uninstall
   bullet (after uninstall, the dev-instance equivalent returns 404 for the
   missing record, not the promised 400);
   an unauthenticated `POST
   /api/plugins/kandev-deepseek-credits/actions/balance.get` (curl
   `-d '{}'`, no session cookie/PAT — a valid JSON envelope, because a
   bodyless POST fails the handler's envelope decode with 400) returns `401`
   on the auth-enabled instance, matching the spec scenario. Note the
   middleware order: the global auth middleware challenges unauthenticated
   callers BEFORE the handler (record lookup, envelope decode, and
   verifyActionContext), so on the auth-enabled instance the 401 holds even
   if the record is missing or the plugin disabled; the handler-internal
   record→status→envelope→auth ordering (a missing record 404ing, a
   disabled one 503ing) is observable only for AUTHENTICATED callers or on
   the auth-disabled dev instance. Run these curl checks WHILE THE PLUGIN IS
   INSTALLED AND ENABLED — see the sequencing note below. NOTE the
   dev/e2e profiles run with `KANDEV_FEATURES_AUTH` disabled (synthetic
   identity admitted, so a selector-less curl gets `400`, not `401`): the
   true `401` assertion is verified on an AUTH-ENABLED disposable instance
   (`KANDEV_FEATURES_AUTH=true` + setup wizard), and on the auth-disabled
   dev instance the equivalent check is `curl -d '{}'` with no workspaceId →
   `400` ("workspace action requires only workspaceId" — a bodyless POST
   fails earlier with "invalid plugin action payload", so send the empty
   JSON object to exercise the named branch) so no false failure is
   reported.

## Verification

Documented browser-driven script (browser automation against the dev instance)
plus teardown evidence: instance stopped and temporary data removed after the
run. Record exact commands, the artifact path, host platform tested, and any
path not exercised (e.g. live balance without a supplied key).

## Risks

- A live-balance check needs a real DeepSeek key on the disposable instance;
  never reuse a developer's primary credentials beyond the operator-supplied
  test key, and record the limitation when unavailable.
- UI-only asset replacement can be cached; use a hard reload or fresh document
  after reinstalling a new package version.

## Results

Completed 2026-08-20. Disposable instance: fresh `kandev-launcher dev` boot
from the task worktree (`KANDEV_NO_BROWSER=1`), state rooted in
`<repo>/.kandev-dev` (DB, plugins, vault — isolated from production
`~/.kandev`); backend origin `http://localhost:35917` served the SPA + API in
dev mode. Host platform tested: linux-amd64. Artifact:
`kandev-deepseek-credits-0.1.0.tar.gz` (and 0.1.1 for the upgrade check),
built in the plugin repo.

### Acceptance 2 — browser-driven checks (real Chromium against the dev SPA)

- **Unconfigured**: pill renders `#deepseek-credits-topbar` with `Ds` monogram
  and `data-deepseek-state="unconfigured"`; tap opens the panel with "No API
  key configured.", "Settings → Plugins → DeepSeek Credits", and
  `DEEPSEEK_API_KEY` guidance. Action body verified identical:
  `{"status":"unconfigured","error":null,"fetched_at":null,"is_available":null,
  "balance_infos":null,"warn_below":null}` (200).
- **Config form**: Settings → Plugins → DeepSeek Credits renders the generated
  `config_schema` form — "DeepSeek API key" (`input[type=password]`, empty),
  "Refresh interval (min)" (number, 5), "Warn threshold" (number, 10), each
  with its description, plus "Saving restarts the plugin so the new settings
  take effect." Manifest card: ID/Version 0.1.0/API version 1/Author kandev/
  Signed no.
- **Invalid key**: saved a whitespace-padded ` sk-smoke-abc ` through the form
  → host config-save restart (no manual Refresh) → action returns
  `status: error`, `error.code: invalid_key`, `balance_infos: null`,
  `warn_below: 10`; pill `data-deepseek-state="error"`; panel shows "DeepSeek
  rejected the API key (401)" + settings path + env fallback + Refresh. The
  padded value authenticating as trimmed `Bearer sk-smoke-abc` is proven by
  the 401 (a padded Bearer would also 401, but the trim rule is unit-tested in
  task 04).
- **Refresh**: two clicks 150 ms apart keep the panel in the error state with
  no blanking and the button returns to "Refresh" (second click inside the 5 s
  cooldown serves the cached error state — zero round trips). The
  fetched_at-advance + unchanged-then-advance cooldown evidence needs a live
  key; NOT EXERCISED (no real DeepSeek key available — see limitation below);
  the cooldown semantics are unit-tested in task 04
  (TestRefresh_CooldownServesCurrentStateWithZeroRoundTrips,
  TestRefresh_CooldownAfterFailedAttempt_ServesCachedErrorState,
  TestRefresh_CooldownWinsOverJoin).
- **Live balance**: NOT EXERCISED (no operator-supplied real key on the
  disposable instance); relied on the 401/unconfigured evidence per the task's
  fallback.
- **Disable/re-enable**: disable → pill gone from the top bar on reload and
  stays gone; re-enable → status `active`, pill returns with the error state
  (vault key still resolves without re-entry).
- **Upgrade**: manifest + Makefile `VERSION` bumped lockstep to 0.1.1,
  repackaged, installed over 0.1.0 → active, immediate re-fetch, vault key
  survived without re-entry (action still `invalid_key` after hard reload with
  the new bundle). **Reinstall**: uninstall then reinstall 0.1.1 → config
  empty, action `unconfigured` (vault purged — key must be re-entered),
  pill back to `data-deepseek-state="unconfigured"`.
- **Uninstall**: plugin gone from `/api/plugins`, config 404, config file
  purged from disk.
- **Non-owner 404 / unauthenticated 401**: the dev/e2e profile disables auth
  (`KANDEV_FEATURES_AUTH=false`, ownerless default workspace), so a second
  user was impractical on this instance; citing the owning monorepo evidence
  as the task permits:
  `apps/backend/internal/plugins/handlers_test.go`
  `TestActionHandlerRejectsUndeclaredAndUnauthorizedResources` — an
  authenticated action call for `workspace-not-visible` returns **404** with
  zero plugin invocations, and an unauthenticated call returns **401**
  (auth middleware challenges before the handler). Both tests PASS locally:
  `go test ./internal/plugins/ -run 'TestActionHandlerRejectsUndeclaredAndUnauthorizedResources|TestActionHandlerDispatchesVerifiedContext'`
  → ok. On the auth-disabled dev instance the equivalent checks ran: selector-less
  `curl -d '{}'` → **400** "workspace action requires only workspaceId";
  bodyless POST → **400** "invalid plugin action payload" (the task's named
  branches).
- **Restore**: manifest + Makefile reverted to 0.1.0 (`git checkout`), full
  suite re-run green (`make test` — backend + 22 bundle tests), worktree clean
  at the task-06 state (commit `db521a3`).

### Acceptance 3 — secret hygiene and route checks (run while installed + enabled)

- Stored key value never appears after save: settings form masks
  `********`; `GET /api/plugins/kandev-deepseek-credits/config` →
  `{"config":{"api_key":"********","poll_minutes":5,"warn_below":10}}`; the
  on-disk `kandev-deepseek-credits.config.yml` holds only
  `api_key: vault:plugin:kandev-deepseek-credits:config:api_key`; a recursive
  scan of `.kandev-dev` found zero occurrences of the raw key; no action
  response contains it (grep count 0). The operator-entered value appeared
  only in the settings form and its single save request.
- No webhook route: `GET` and `POST /api/plugins/kandev-deepseek-credits/
  webhooks/ping` → **404** ("has no webhook").
- Unauthenticated POST on the auth-DISABLED dev instance: `curl -d '{}'` (no
  workspaceId) → **400** "workspace action requires only workspaceId" — the
  task's named dev-profile branch (the true 401 is covered by the monorepo
  test above).
- Action route with a real workspace while unconfigured → **200** with the
  exact unconfigured body; with the fake key → **200** with the exact error
  body (domain failures are body-encoded, never HTTP statuses).

### Teardown evidence

- Instance stopped (`hub stop` → exit 255, uptime 8m19s); backend port
  closed (curl → 000).
- `.kandev-dev` removed (`rm -rf`); no disposable data left.
- Plugin repo artifacts cleaned (`rm -f kandev-deepseek-credits-0.1.0.tar.gz
  kandev-deepseek-credits-0.1.1.tar.gz`, `rm -rf bin .build`); `git status`
  clean.

### Limitation recorded

- Live-balance path (real DeepSeek key → pill amount, fetched_at-advance
  cooldown evidence) was not exercised: no operator-supplied key was available
  for this disposable test. The 401/unconfigured evidence plus the task-04
  cooldown unit tests stand in for it, per the task's fallback.

### Plugin repo CI

PR `Fclem/kandev-plugin-deepseek-credits#1` (branch `feat/deepseek-credits`)
opened to exercise CI: verify (tidy/format/vet/test), base-floor
(Action contract floor on Kandev v0.88.0), and Build plugin packages checks.
Results recorded in the implementation report.
