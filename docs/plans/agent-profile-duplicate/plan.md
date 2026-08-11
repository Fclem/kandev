---
spec: docs/specs/agents/profile-duplicate.md
created: 2026-08-11
status: complete
---

# Implementation Plan: Agent Profile Duplicate

## Overview

Add a one-click "duplicate" for agent profiles on the settings agents surface
(`/settings/agents` profile list and the per-profile settings page). A new
interlock-protected endpoint `POST /api/v1/agent-profiles/:id/duplicate`
copies the source profile's full configuration (model, mode, config options,
CLI flags, env vars, launcher prefix, auto-approve flags, enabled state, MCP
config row) into a fresh row named `<source> Copy`, broadcasts the existing
`agent.profile.created` WS event, and returns the new profile DTO.

## Backend

### Controller: `DuplicateProfile`

Add to `apps/backend/internal/agent/settings/controller/profile_crud.go`:

- `type DuplicateProfileRequest struct { ID string }`.
- `func (c *Controller) DuplicateProfile(ctx, req) (*dto.AgentProfileDTO, error)`:
  1. `repo.GetAgentProfile(ctx, req.ID)`; map the "agent profile not found"
     error to `ErrAgentProfileNotFound` exactly like `DeleteProfile` does
     (the sqlite store wraps `sql.ErrNoRows` in a message containing
     "agent profile not found").
  2. Build a fresh `models.AgentProfile` copying every configuration field:
     `AgentID`, `AgentDisplayName`, `Model`, `FallbackModel` (trimmed),
     `AutoFallback`, `Mode`, `ConfigOptions` (through
     `profileconfig.SanitizeConfigOptions` on a copied map),
     `AllowIndexing`, `AutoApprove`, `CLIPassthrough`, `CLIFlags` (deep
     copy of the slice), `EnvVars` (deep copy of the slice, keeping
     `SecretID` refs), `CommandPrefix`, `UserModified: true`, and the office
     enrichment configuration fields (`WorkspaceID`, `Role`, `Icon`,
     `ReportsTo`, `SkillIDs`, `DesiredSkills`, `MaxConcurrentSessions`,
     `CooldownSec`, `SkipIdleRuns`, `FailureThreshold` (copied pointer),
     `ExecutorPreference`, `BudgetMonthlyCents`, `Settings`, `Permissions`).
     Do NOT copy runtime state: `Status`, `PauseReason`, `LastRunFinishedAt`,
     `ConsecutiveFailures`, `DeletedAt`, `MigratedFrom`, `CustomPrompt`,
     `DangerouslySkipPermissions`.
  3. Name: `strings.TrimSpace(source.Name) + " Copy"`. The name is persisted
     data, not UI copy (same convention as seeded executor/repository names),
     so the suffix is a plain string in Go.
  4. `repo.CreateAgentProfile(ctx, clone)` — the store sets a new UUID,
     timestamps, and forces `Enabled=true` (existing store invariant).
  5. If `!source.Enabled`, call `repo.UpdateAgentProfileEnabled(ctx,
     clone.ID, false)` and set `clone.Enabled = false` so the copy inherits
     the source's selection state.
  6. Copy the MCP config row when the source has one:
     `repo.GetAgentProfileMcpConfig(ctx, source.ID)`; tolerate
     `sql.ErrNoRows`/nil; when non-nil, deep-copy `Servers` and `Meta` maps
     into a new `models.AgentProfileMcpConfig{ProfileID: clone.ID, ...}` and
     `repo.UpsertAgentProfileMcpConfig`. When the source has no row, leave
     the copy without one — the existing default-config semantics and boot
     `EnsureDefaultMcpConfig` cover MCP-supporting agents.
  7. Return `toProfileDTO(clone)`.

Env-var secret refs are copied verbatim; they were validated when the source
was created/updated, so no re-validation is needed.

### Handler + route

In `apps/backend/internal/agent/settings/handlers/handlers.go`:

- Register `api.POST("/agent-profiles/:id/duplicate", h.interlock,
  h.httpDuplicateProfile)` next to the other `/agent-profiles/:id` routes.
- `httpDuplicateProfile`: require `:id`, call
  `controller.DuplicateProfile`, map `ErrAgentProfileNotFound` → 404, other
  errors → 500; on success broadcast
  `ws.ActionAgentProfileCreated` with `{"profile": resp}` (same payload
  shape as `httpCreateProfile`) and return the DTO with 200.

### Tests (backend)

- Controller, new file
  `apps/backend/internal/agent/settings/controller/profile_duplicate_test.go`
  (using the existing `newTestController` + `newFakeStore`):
  - full-config copy: source with model/fallback/mode/config options/CLI
    flags/env vars/command prefix/auto-approve/cli-passthrough duplicates
    with equal fields, new ID, `Default Copy` name, `user_modified` true;
  - disabled source → disabled copy;
  - MCP config row copied to the new profile ID with equal enabled/servers/meta
    (extend the shared `fakeStore` in `reconciler_test.go` with an
    `mcpConfigs` map + working `GetAgentProfileMcpConfig` /
    `UpsertAgentProfileMcpConfig`, additively — other tests only observe
    the existing nil behaviour);
  - unknown ID → `ErrAgentProfileNotFound`;
  - empty source name (`""` → `" Copy"` is acceptable; source names are
    non-empty by validation, but pin the suffix behaviour).
- Handler, `apps/backend/internal/agent/settings/handlers/interim_settings_interlock_test.go`:
  add `{method: POST, path: "/api/v1/agent-profiles/profile-1/duplicate"}`
  to the route list that must return 403 without the interlock token.
- Optional (only if a clean stub is easy): a handler test asserting 404
  mapping and the `agent.profile.created` broadcast, following
  `agent_update_handlers_test.go`'s controller-stub pattern.

## Frontend

### API action

In `apps/web/app/actions/agents.ts` add:

```ts
export async function duplicateAgentProfileAction(profileId: string): Promise<AgentProfile> {
  const raw = await agentSettingsRequest<unknown>(
    `${apiBaseUrl}/api/v1/agent-profiles/${profileId}/duplicate`,
    { method: "POST" },
  );
  return normalizeAgentProfile(raw);
}
```

### List page: per-row Duplicate button

- `apps/web/app/settings/agents/profile-list-item.tsx`: accept an
  `onDuplicate: (profile: AgentProfile) => void` prop; render an icon-only
  button (copy icon, e.g. `IconCopy` from `@tabler/icons-react`) outside the
  link, next to the enabled switch, with `aria-label` / tooltip via
  `t("agents:duplicateProfileNamed", { name: profile.name })` and
  `data-testid={`duplicate-profile-${profile.id}`}`.
- `apps/web/app/settings/agents/page.tsx`: thread `onDuplicate` through
  `AgentProfilesSection`; add a `handleDuplicateProfile` wired like
  `useProfileEnabledToggle` (POST via the action, then merge the returned
  profile into `settingsAgents` + `agentProfiles` store slices atomically via
  `useAppStoreApi().setState`, appending the new profile to its agent's
  `profiles`; toast success/failure via `agents:duplicateProfileSuccess` /
  `agents:failedToDuplicateProfile`). The `agent.profile.created` WS handler
  already upserts the same profile; the direct merge keeps the UI consistent
  even if WS is delayed.

### Profile settings page: header Duplicate button

- `apps/web/components/settings/agent-profile-page.tsx`: add a Duplicate
  button to `ProfileEditorHeader` (copy icon + `t("agents:duplicate")`,
  `data-testid="duplicate-profile-header"`). On success: toast
  `agents:duplicateProfileSuccess` and
  `window.location.assign(`/settings/agents/${agentName}/profiles/${newId}`)`
  so the user lands on the copy to edit it. Wire it through the same
  action; the agent name is available from the page's agent lookup.

### i18n

Add to `apps/web/src/locales/en/agents.json`:

- `"duplicate": "Duplicate"`
- `"duplicateProfile": "Duplicate profile"`
- `"duplicateProfileNamed": "Duplicate {{name}}"`
- `"duplicateProfileSuccess": "Profile duplicated"`
- `"failedToDuplicateProfile": "Failed to duplicate profile"`

Regenerate the pseudo locale (`pnpm --filter @kandev/web i18n:pseudo`). Real
locales (`zh-cn`, `pt-pt`) fall back to en; key-parity is a warning there,
missing en keys are a hard error.

### Tests (frontend)

- `apps/web/app/settings/agents/profile-list-item.test.tsx`: extend with a
  case asserting the Duplicate button renders and calls `onDuplicate` with
  the profile (mirroring the existing toggle tests).
- `apps/web/lib/api/domains/agent-profile-normalize.test.ts` or the actions
  layer: only if an existing action-test pattern exists; otherwise the
  component test plus E2E covers the contract.

## E2E

New `apps/web/e2e/tests/settings/agent-profile-duplicate.spec.ts` modeled on
`agent-profile-delete.spec.ts`:

- Create a source profile via `apiClient.createAgentProfile(agent.id,
  "Dup Me", { model: agent.profiles[0].model, cli_flags: [...], command_prefix: ... })`.
- `testPage.goto("/settings/agents")`, wait for the profile list, click the
  duplicate button on the "Dup Me" row (`duplicate-profile-<id>`).
- Expect a "Dup Me Copy" row to appear without reload.
- Navigate to the copy's profile page and assert the copied model/cli flag
  values render.
- `finally`: delete both profiles via `apiClient.deleteAgentProfile(..., true)`.

## Implementation Waves

Execution is sequential in the primary conversation; no subagents are
authorized.

Wave 1:

- [x] [Task 01: Backend duplicate endpoint](task-01-backend-duplicate-endpoint.md)

Wave 2:

- [x] [Task 02: Frontend duplicate UI](task-02-frontend-duplicate-ui.md)

Wave 3:

- [x] [Task 03: E2E duplicate flow](task-03-duplicate-e2e.md)

## Risks

- The store's `CreateAgentProfile` forces `Enabled=true`; the disabled-source
  case needs the follow-up `UpdateAgentProfileEnabled` call — pinned by a
  test so a future change to that invariant cannot silently drop the state.
- `GetAgentProfileMcpConfig` returns different "absent" shapes across the
  sqlite store (`sql.ErrNoRows`) and the shared fake store (`nil, nil`);
  the controller must tolerate both.
- The list-page direct store merge and the WS `agent.profile.created`
  broadcast can both insert the copy; the WS handler upserts by ID so this
  is safe (verify while implementing).
