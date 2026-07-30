# i18n sweep — follow-ups

Issues surfaced during the migration that wrapping strings alone does not fix.
The lint guard is now an error in `apps/web/eslint.config.mjs` (task-40) and
reports zero across `components/` and `app/`.

Zero is not the same as complete: the guard only inspects JSX, so copy held in
module-scope config objects is invisible to it. §5 is the open work.

---

## 1. Type-to-confirm sentinels break under translation — FIXED

Destructive dialogs gate on the user typing an exact token that is also compared
with `===` (and sometimes sent to the backend). Where that token was baked into
translatable copy, a translated locale told the user to type a word the gate
would never accept — making the dialog impossible to satisfy, invisibly, until a
second language shipped.

Fixed by interpolating the sentinel so the displayed and compared token cannot
drift:

```tsx
const CONFIRM_TOKEN = "RESET";                    // sentinel, never translated
t("settings:typeToConfirm", { token: CONFIRM_TOKEN })
<Trans i18nKey="settings:typeToEnableTheConfirmButton" values={{ token: CONFIRM_TOKEN }}>
  Type <code>{CONFIRM_TOKEN}</code> to enable the confirm button.
</Trans>
```

Sites audited and corrected:
- `components/settings/system/factory-reset-dialog.tsx` (`RESET`) — copy had the
  token inline; now interpolated.
- `app/settings/executor/[id]/page.tsx` (`delete`) — had no sentinel constant at
  all; now `DELETE_CONFIRM_TOKEN`, interpolated into the instruction.
- `components/settings/system/restore-dialog.tsx` (`RESTORE`) — already correct;
  placeholder renamed to `token` for consistency.
- `components/settings/system/storage/storage-confirmation-dialogs.tsx`
  (`DEDICATED` / `ADOPT` / `DELETE`) — already correct; now shares the single
  `settings:typeToConfirm` message.

**Regression guard:** `lib/i18n/confirm-tokens.test.ts` asserts every confirm
message renders the token verbatim under both `en` and `pseudo`, that no confirm
message hardcodes a sentinel, and that the old broken keys stay deleted. Verified
to fail when the defect is reintroduced (under `pseudo` the hardcoded token
accents to `ŔĒŚĒŢ` while the gate still compares `RESET` — exactly the
production bug).

**Out of scope (correct as-is):**
`app/office/workspace/settings/components/danger-zone-section.tsx` compares the
input against the *workspace name* (user data, never translated).

---

## 2. Display strings used as logic sentinels — PARTIALLY FIXED

Fixed: `components/settings/layouts/layout-editor-toolbar.tsx` interpolated the
raw `direction` enum (`"left"`/`"above"`/…) into translated tooltips, so
non-English locales would render an English word mid-sentence. It now passes the
translated label (`t(label)`), which already existed a few lines above.

Still open — display strings that double as comparison keys. Each must either
stay untranslated (accepting English in every locale) or be decoupled by keying
off a stable id and translating only at render:

- `components/kanban/kanban-header.tsx` — `getHeaderTitle()` returns
  `"Home"` / `"Tasks"`, compared via `title === "Home"` to pick the topbar
  variant. Currently untranslated, so the header stays English in every locale.
- Command-palette `group` values (`"Navigation"`, `"Git"`, `"Panels"`, …) are
  `Map` keys in `command-panel.tsx` *and* rendered as group headings, across
  `session-commands.tsx` / `global-commands.tsx` / `homepage-commands.tsx`.
  Translating one file alone would fragment the grouping.
- `components/task-create-dialog-branch-utils.ts` — `computeBranchPrefix()`
  returns `"from: "` / `"current: "` / `"will switch to: "`, which are rendered
  as chip text *and* compared in `computeBranchTooltip()` (with tests asserting
  the literals).

Correctly left literal (backend contract values, not copy): executor types
(`local_docker`, …), TLS `"1"`/`"0"`, default executor/layout names persisted to
the DB, and `DEFAULT_CUSTOM_STEPS` workflow step names.

---

## 3. Module-scope translation is not locale-reactive — FIXED

`t()` at module scope resolves once at import, before a locale is active, and
never updates on a locale switch. This is the same failure mode as §5, which
tracks the config objects still holding raw English for exactly this reason.
Fixed:

- `components/kanban/swimlane-kanban-content.tsx` — `ORPHAN_STEP` became
  `orphanStep()`, resolved at call time (2 consumers updated).
- `components/task-create-dialog-footer.tsx` — the seven `REASON_*` constants
  became `reasonTitle()` / `reasonRepo()` / … getters; the test was updated to
  call them.

Audited clean: the `() => t(...)` helpers in `task-create-dialog*.tsx` and the
`workItems()` / `workspaceItems()` factories in the sidebar were already lazy.

**Convention going forward:** never assign `t()` to a module-level constant —
use a getter, or resolve inside the component.

---

## 4. Literal braces in copy — RESOLVED BY THE i18next MIGRATION

Under Lingui, `{{`/`}}` in copy was parsed as ICU syntax and had to be hoisted
and interpolated. i18next's interpolation is `{{name}}` and the affected strings
now live as catalog values rather than inline macro arguments, so the hazard is
gone. The `i18n:check` gate would catch a malformed placeholder as a missing key.

---

## 5. Display copy in module-scope config objects — MOSTLY DONE

Config objects at module scope hold catalog KEYS and are translated at the render
site. `label: t("task:todo")` there would call `t()` at import, pinning the copy to
whatever locale was active at boot and never updating on a switch — a worse bug
than the hardcoded string, because it looks correct until someone switches
language.

**286 of 325 strings across 47 files are done.** Re-check the remainder with:

```bash
cd apps/web && DUMP_MODULE_SCOPE=1 node scripts/externalize-strings.mjs components app
```

### Tooling

- `externalize-config-labels.mjs` rewrites the literal to its key and **renames**
  the property (`label` -> `labelKey`), so a consumer still reading `.label` fails
  to compile instead of rendering a raw key to a user.
- `fix-config-label-reads.mjs` reads `tsc` output and applies the mechanical half
  of each break (`{opt.label}` -> `{t(opt.labelKey)}`, plus cross-file type-member
  renames).
- `fix-missing-t.mjs` binds `t` where a rewrite introduced a call to it.

Hard limits, each learned from wrong output:

- **`lib/types/**` and `lib/api/**` are off limits.** Those types describe the
  server's JSON, so a member name is a wire contract.
- **Fixture modules and keyboard key names are not copy.** `*-mock-data`, `demo/`,
  and `Esc`/`Tab`/`PgUp` are excluded.
- **A named type annotation is a warning sign.** Such a type is frequently ALSO
  satisfied by runtime data, so `--include-named-types` is opt-in per file.
- **Component props are not config items.** A `SelectField`-style component
  receives `label={t(...)}` — already translated. Only the *item* types in its
  option arrays need the key treatment.

### The shape that keeps recurring

An option list fed from two places at once — a static list we wrote, plus runtime
values we were handed (a user's agent-profile names, a repo slug, an acronym like
`CEO`). One field cannot be both, so these carry the two separately:

- `lib/i18n/option-label.ts` — `OptionLabel` + `resolveOptionLabel(t, option)`
- `ExecutorEnvironmentStatus` — `labelKey` / `rawLabel` (Docker's own state text)
- `ScriptPlaceholder` — `descriptionKey` / `description` (backend-supplied)

### Out of scope (31 strings)

| Count | File | Why |
|---|---|---|
| 21 | `task/mobile/mobile-terminal-keybar-helpers.tsx` | Keyboard key names, printed on physical keys; the spec lists keyboard glyphs as out of scope |
| 5 | `app/demo/agent-messages/page.tsx` | Demo fixture data |
| 3 | `office/workspace/routing/components/provider-tier-mapping.tsx` | Label is typed `Tier`, a union used for matching — translating breaks the comparison |
| 2 | `app/layout.tsx` | Dead Next.js `metadata` export; `index.html` owns the real `<title>` |

### Still to do (39 strings, 11 files)

All of them hang off the shared preset shapes — `PresetOption` (declared twice:
`my-github/search-bar.tsx` and `my-gitlab/presets.ts`), `ScopePreset` in
`integrations/presets-scope-bar-base.tsx`, and the `SelectField`/`CategoryMeta`
props in the watch dialogs and automations config:

```
components/github/my-github/search-bar.tsx            9
components/github/issue-watch-dialog.tsx              6
components/github/review-watch-dialog.tsx             6
components/jira/jira-settings.tsx                     3
components/jira/my-jira/filter-pills.tsx              3
components/jira/my-jira/list-toolbar.tsx              3
components/automations/config-section.tsx             2
components/azure-devops/azure-devops-scope-bar.tsx    2
components/github/my-github/presets-scope-bar.tsx     2
components/gitlab/my-gitlab/presets-scope-bar.tsx     2
components/automations/trigger-picker.tsx             1
```

These need one coordinated change, not a sweep: the type moves in
`presets-scope-bar-base.tsx`, both `PresetOption` declarations, the three
integration scope bars, `use-default-query-presets.ts`, and four test files all
have to land together, and the props-vs-item distinction has to be made per
member rather than per file. Attempting it as a batch produced 58 compile errors
that would not converge, so it was reverted rather than landed half-done.
