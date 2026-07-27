# i18n sweep — follow-ups

Issues surfaced during the migration that are NOT fixed by wrapping strings.
Resolve before flipping the lint guard to error (task-40).

## 1. Type-to-confirm sentinels break under translation (correctness)

`apps/web/app/settings/executor/[id]/page.tsx` gates executor deletion on
`deleteConfirmText !== "delete"`, while the instruction sentence around it was
wrapped for translation (`<Trans>Type "delete" to confirm deletion…</Trans>`).

Under a real locale the user is told to type a translated word, but the gate
still compares against the literal `"delete"` — the confirm dialog becomes
impossible to satisfy.

**Fix:** hoist the sentinel to a constant and interpolate it into the sentence
so the displayed word and the compared word cannot drift:

```tsx
const CONFIRM_WORD = "delete"; // sentinel — intentionally NOT translated
...
<Trans>Type {CONFIRM_WORD} to confirm deletion</Trans>
...
disabled={deleteConfirmText !== CONFIRM_WORD}
```

Audit every other type-to-confirm / string-compared-literal dialog for the same
pattern before task-40. Search: `grep -rn '!== "delete"\|=== "delete"\|confirmText' apps/web`.

## 2. Display strings used as logic sentinels

Reported by migration agents; each must stay untranslated OR be decoupled from
the comparison:

- `components/kanban/kanban-header.tsx` — `getHeaderTitle()` returns
  `"Home"` / `"Tasks"`, compared via `title === "Home"` to pick the topbar
  variant. Left untranslated (so the header is English in every locale).
  Proper fix: return a stable key and map it to a translated label at render.
- Executor type values (`local_pc`, `worktree`, `local_docker`,
  `remote_docker`, `sprites`) and TLS select values `"1"`/`"0"` — correctly
  left literal (backend contract values, not copy).
- `setName("Local Docker")` / `setName("Remote Docker")` in
  `app/settings/executor/new/page.tsx` — serialized to the backend as the
  executor name, correctly left literal even though the adjacent Select option
  labels were translated.

## 3. Module-scope translation is not locale-reactive

`ORPHAN_STEP.title` (`components/kanban/`) is translated with the global `t`
macro at module load, so it will not update on a runtime locale switch. Same
applies to any other module-level `t\`\`` constant. Prefer `msg\`\`` descriptors
resolved at render (`const { t } = useLingui(); t(descriptor)`) where the value
flows through a component.

## 4. ICU brace escaping

Literal `{{` / `}}` in copy is parsed as ICU syntax by the catalog. Where a
string must show braces (e.g. a prepare-script placeholder example), hoist the
token to a const and interpolate it. One instance already handled in
`app/settings/executor/[id]/profile/[profileId]/page.tsx`.
