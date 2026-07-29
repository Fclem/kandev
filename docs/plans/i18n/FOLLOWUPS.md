# i18n sweep — follow-ups

Issues surfaced during the migration that wrapping strings alone does not fix.
Resolve before flipping the lint guard to error (task-40).

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
never updates on a locale switch. Fixed:

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
