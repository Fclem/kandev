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

## 5. Display copy in module-scope config objects — FIXED

Config objects at module scope hold catalog KEYS, translated at the render site.
`label: t("task:todo")` there would call `t()` at import, pinning the copy to
whatever locale was active at boot and never updating on a switch — a worse bug
than the hardcoded string, because it looks correct until someone switches
language.

**All 294 in-scope strings across 58 files are done.** Verify with:

```bash
cd apps/web && DUMP_MODULE_SCOPE=1 node scripts/externalize-strings.mjs components app
```

### The shape that kept recurring

An option list fed from two places at once — a static list we wrote, plus runtime
values we were handed (a user's agent-profile names, a repo slug, Docker's own
state text, an acronym like `CEO`). One field cannot be both, so these carry the
two separately:

- `lib/i18n/option-label.ts` — `OptionLabel` + `resolveOptionLabel(t, option)`
- `ExecutorEnvironmentStatus` — `labelKey` / `rawLabel`
- `ScriptPlaceholder` — `descriptionKey` / `description`

**Both fields are optional, so reading `item.label` directly type-checks and
renders blank.** That bit once, in the watch dialogs' `SelectField`. Always go
through the resolver — the shared `WatchSelectField` now does.

### Tooling

- `externalize-config-labels.mjs` rewrites the literal to its key and **renames**
  the property (`label` -> `labelKey`), so a consumer still reading `.label` fails
  to compile instead of rendering a raw key to a user.
- `fix-config-label-reads.mjs` reads `tsc` output and applies the mechanical half
  of each break, driven by compiler positions rather than re-parsing.
- `fix-missing-t.mjs` binds `t` where a rewrite introduced a call to it.

Hard limits, each learned from wrong output:

- **`lib/types/**` and `lib/api/**` are off limits** — those types describe the
  server's JSON, so a member name is a wire contract.
- **Fixture modules and keyboard key names are not copy.**
- **A named type annotation is a warning sign**; `--include-named-types` is opt-in
  per file, because such a type is often also satisfied by runtime data.
- **Component props are not config items.** A `SelectField`-style component
  receives `label={t(...)}` — already translated. Only the *item* types in its
  option arrays need the key treatment.
- **Never bulk-rewrite a call inside the helper that implements it.** Rewriting
  `t(x.descriptionKey)` into the new accessor also rewrote the accessor's own
  body, making it recurse until the stack blew. Only a render-time test caught it.

### Deliberately not translated (31 strings)

| Count | File | Why |
|---|---|---|
| 21 | `task/mobile/mobile-terminal-keybar-helpers.tsx` | Keyboard key names, printed on physical keys; the spec lists keyboard glyphs as out of scope |
| 5 | `app/demo/agent-messages/page.tsx` | Demo fixture data |
| 3 | `office/workspace/routing/components/provider-tier-mapping.tsx` | Label is typed `Tier`, a union used for matching — translating breaks the comparison |
| 2 | `app/layout.tsx` | Dead Next.js `metadata` export; `index.html` owns the real `<title>` |

---

## 6. Copy the lint guard cannot see — FIXED

`i18next/no-literal-string` only inspects **plain string literals in JSX**. Three
shapes therefore sat at zero findings while still shipping English:

| Shape | Why it was invisible | Example |
|---|---|---|
| Template literal | The rule never visits `TemplateLiteral` | ``aria-label={`Remove ${label}`}`` |
| Native dialog argument | Not JSX at all | `confirm("Remove workflow sync configuration?")` |
| Plain `.ts` module | The codemod only walked `.tsx` | `toast.error(\`Save failed: ${err}\`)` in `hooks/` |

`aria-label` mattered most: a screen reader reads it aloud, so these were the
least visible strings and the ones a sighted reviewer is least likely to catch.

**Fixed by** extending `externalize-strings.mjs`: `handleTemplateLiteral`
(+ `scripts/lib/template-literals.mjs`), `DIALOG_CALLEES` for
`confirm`/`alert`/`prompt`, and `.ts` in `listFiles`. 234 template literals, 13
dialog/toast arguments and 225 further literals were converted.

Placeholders are named from the expression (`{{label}}`, not `{{value0}}`), because
a translator reorders a sentence and cannot do that against anonymous slots.

### Two bugs this class hid

- **An empty trailing quasi matched the `""` sentinel.** Every template ending in
  `${x}` has a final empty chunk, and `""` is a comparison operand in nearly every
  file, so the sentinel guard rejected the whole string. Silently — the decline
  had no counter. Fixed by testing only non-empty chunks; this alone unblocked
  conversions across the tree.
- **`t` shadowed in the dialog path.** `handleToastCall` did not run the
  `tIsShadowed` check, so `terminals.map((t) => confirm(...))` emitted a `t("key")`
  that called the terminal object. `tsc` caught it as "not callable".

### Excluded on purpose

`EXCLUDED` in `externalize-strings.mjs`: `lib/api/**` and `lib/types/**` (the
backend's JSON shape — a translated default value changes what gets persisted;
`plan-api.ts` sent `title: "Plan"` to the server), `lib/state/layout-manager/**`
(panel titles are serialized into saved layouts), and test helpers/fixtures.

## 7. English pluralization inside an interpolation — FIXED

The codemod's first output turned ``${n} task${n === 1 ? "" : "s"}`` into

```ts
t("ns:task", { length: n, value1: n === 1 ? "" : "s" })   // "{{length}} task{{value1}}"
```

which renders correctly in English and is untranslatable everywhere else: the
plural rule lives at the call site, so a language with three or six plural forms
has nowhere to express them and a translator editing the catalog cannot fix it.

`fix-inline-plurals.mjs` converted all 12 to i18next's own mechanism — `count`
plus `_one`/`_other` keys — and `check-inline-plurals.mjs` (now in `i18n:check`)
fails on both the call-site form and the catalog form (`{{count}} task(s)`, or a
`{{count}}` message with no `_one`/`_other` sibling). 17 further count-bearing
messages gained plural forms; where English does not inflect (`{{count}} selected`)
both forms are identical so translators can still diverge.

Two needed a discriminant rather than a plural, per the copy-vs-logic rule:

- `Failed to ${noun} N tasks` with `noun` = `"archive"｜"delete"` → a `BulkAction`
  type selecting a whole message. A verb cannot be interpolated into a frame in
  languages that inflect or reorder it.
- `<Trans values={{ value2: expanded ? "shown" : "hidden" }}>` — English words in
  `values` are never translated. Split into two keys.

## 8. `t()` as a `<Trans>` text child — DOCUMENTED, NOT A BUG

~340 sites read:

```tsx
<Trans i18nKey="ns:commit">
  <IconCheck />
  {t("common:commit")}
</Trans>
```

`<Trans>` renders the **catalog message**; children only supply substitutes for
`<n>` tags. So the inner `t()` is inert — verified empirically in
`lib/i18n/trans-children.test.tsx`, which pins the render (`<span/> Commit`, once)
so a future react-i18next upgrade that starts rendering text children fails there
instead of duplicating copy across the UI.

It is redundant, not broken: two keys hold the same English and can drift apart in
translation. Left as-is deliberately — rewriting 340 render paths to remove dead
arguments is churn with real regression risk and no user-visible change.
