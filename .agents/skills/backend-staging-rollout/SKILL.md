---
name: backend-staging-rollout
description: Sync the local repo from fork-source (kdlbs/kandev), rebase a staging branch carrying open PRs onto the updated main, rebuild the kandev backend runtime bundle, and replace the running systemd service. Use when the user asks to update the repo from the fork, rebase the staging branch onto new main, or rebuild and replace the kandev backend.
---

# Backend Staging Rollout

Rolls the latest upstream main plus a set of target PRs into a fresh staging branch, builds the full runtime bundle, and hot-swaps the user's running kandev backend service.

## Steps

1. **Sync main from fork-source.**
   - Ensure the `fork-source` remote exists (`git remote add fork-source git@github.com:kdlbs/kandev.git` if missing), then `git fetch fork-source --prune`.
   - `git checkout main && git merge --ff-only fork-source/main`.
   - Record the new main SHA; `git rev-parse fork-source/main`.

2. **Determine what the staging branch must carry.**
   - For each target PR (`gh pr view <N> --repo kdlbs/kandev --json state,headRefOid,mergeable`):
     - **MERGED** → its content is already in main; skip it (re-merging the old head duplicates or conflicts).
     - **OPEN** → fetch its current head (`git fetch fork-source <headRefOid>`) and note the SHA; the head moves between rounds.
   - Verify a merged PR's files exist in main before skipping: `git cat-file -e fork-source/main:<path>` per changed file, or `git diff --stat <old-pr-head> <merge-commit>` on the PR's own paths.

3. **Create the new staging branch.**
   - `git checkout -b staging/<slug>-<round>` from updated main, e.g. `staging/pr-2738-2741-3`.
   - Merge each open PR head with `--no-ff -m "Merge PR #<N>: <title> (rebase onto updated main)"`.
   - No conflicts expected; if any, resolve and keep the PR content identical to the head.

4. **Build the runtime bundle — never the Go binary alone.**
   - `make runtime-bundle` from the repo root (web build → `sync-embedded-web` → `make -C apps/backend build-runtime`).
   - The `kandev` binary **embeds `apps/web/dist`**; the installed service bundle has no filesystem web assets. A bare `build-kandev` ships the previous UI. The generated embedded assets are gitignored, so the tree stays clean.
   - Requires `apps/node_modules` (run `pnpm install --frozen-lockfile` from `apps/` first if missing).

5. **Verify the bundle.**
   - `./dist/kandev/bin/kandev --version` → matches `git describe --tags --always --dirty` on the staging head.
   - Spot-check PR markers: grep the embedded assets for frontend features, `strings dist/kandev/bin/kandev | grep -c <backend-marker>` for backend features.
   - `git status -s` empty.

6. **Locate the running service.**
   - The backend runs as a systemd user service: `systemctl --user status kandev` (needs `XDG_RUNTIME_DIR=/run/user/$(id -u)` and `DBUS_SESSION_BUS_ADDRESS=unix:path=$XDG_RUNTIME_DIR/bus`).
   - Bundle dir: `KANDEV_BUNDLE_DIR` in the unit (a workspace checkout's `dist/kandev`, e.g. `~/.kandev/repos/workspaces/<uuid>/github/kdlbs/kandev/dist/kandev`).
   - Do **not** touch that workspace checkout's git state (it may hold uncommitted work); build in the dev repo and copy binaries.
   - Do not disturb the user's interactive `kandev-launcher dev` session if present; replace only the systemd service.

7. **Back up, swap, restart.**
   - Back up the running bundle: `cp -a <bundle>/bin <bundle>.bak-<YYYYMMDD>-r<N>/`.
   - `rm -rf <bundle>/bin && cp -a <repo>/dist/kandev/bin <bundle>/bin` (six files: `kandev`, `agentctl`, four `agentctl-<os>-<arch>` helpers).
   - Update version metadata: `launcher_version` in `~/.kandev/service/install.json`, and `KANDEV_VERSION` in `~/.config/systemd/user/kandev.service` (sed the exact `KANDEV_VERSION=` line only — never the `KANDEV_TRUSTED_PROXIES` line).
   - `systemctl --user daemon-reload && systemctl --user restart kandev`.

8. **Verify the running backend.**
   - Backend startup takes ~15s: wait for the child (`pgrep -P <MainPID> -f __backend`), then check `ss -tlnp` for its listener.
   - `curl http://[::1]:<port>/health` → JSON with `"version"` matching the new build; web root serves 200.
   - Logs: `grep -E "backend ready|Web SPA" ~/.kandev/logs/backend-logs.log | tail`.
   - Confirm env survived: dump `/proc/<child>/environ` and grep with **exact-name patterns** (`grep '^KANDEV_'`) — substring greps like `PROXY` silently miss `KANDEV_TRUSTED_PROXIES`.
   - Report the version, port, rollback backup path, and any sessions interrupted by the restart.

## Notes

- The service port (`KANDEV_SERVER_PORT`, e.g. 38429) is config, not the binary; it survives restarts.
- Rollback = restore the timestamped `bin` backup and restart.
- Leave staging branches local unless the user asks to push.
