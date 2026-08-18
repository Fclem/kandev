# Owned-layer inventory (floating panels)

Required by `docs/specs/ui/panel-pin-float.md` (Focus ownership → Layer
inventory) and `docs/plans/panel-pin-float/task-03-floating-window-edge-bar.md`.

Every interactive Radix layer opened from inside a floating-capable dockview
panel MUST call `useFloatingOwnedLayer` (host) /
`host.ui.registerFloatingOwnedLayer` (plugin panels), or the floating window
collapses while the layer is open (a contract violation).

Legend for `Status`: `audited` = callsite confirmed in the baseline and the
hook is applied by this feature; `to-wire` = callsite confirmed, hook must be
applied during task-03; `verify` = candidate surface, confirm during task-03.

## Desktop dockview panels

| Panel (component) | Primitive family | Exact owner (component/file) | Status |
|---|---|---|---|
| changes / diff | Dialog, HoverCard | `changes-panel-header.tsx` (Dialog ~139-170; HoverCard ~306-311) | to-wire |
| changes / diff | Drawer | `changes-panel-header.tsx` (touch Drawer ~293-302) | to-wire |
| changes / diff | DropdownMenu | `changes-panel-header.tsx` (PullDropdown ~411-425) | to-wire |
| changes / diff | ContextMenu | `changes-tab.tsx` (ContextMenu ~118-136) | to-wire |
| changes / diff | DropdownMenu | `changes-top-bar.tsx` (DropdownMenu ~59-77) | to-wire |
| chat / session tabs (`chat`) | DropdownMenu, Dialog, Popover, ContextMenu | `components/task/chat/` — audit each file during task-03: message actions, session menu, queue controls, feedback popover | to-wire |
| plan (`plan`) | Popover, Dialog | `components/task/plan/` — plan panel popovers, tiptap toolbar menus | to-wire |
| terminal (`terminal`) | ContextMenu, Dialog | xterm right-click menu, terminal tab rename/destroy menu, `TerminalScriptsDropdown` (`dockview-header-actions.tsx`) | to-wire |
| files (`files`) | ContextMenu, Popover | file-tree context menu, hover actions | to-wire |
| browser (`browser`) | Popover, Dialog | browser panel URL bar, inspect annotations popup | verify |
| pr-detail / mr-detail / review-detail | Popover, Dialog, DropdownMenu | review surfaces, action menus | verify |
| todos (`todos`) | DropdownMenu | todo list row menus | verify |
| vscode / dev-server | none | no owned layers expected | verify |
| plugin panels (`plugin-panel`) | any (plugin-owned) | via `host.ui.registerFloatingOwnedLayer` (per-panel capability) | to-wire |

**Audit rule (task-03, blocking):** every Radix primitive that opens a layer
inside a floating-capable panel must be a row here with its exact file and
line range (or an explicit layer-free proof for the panel). `to-wire` and
`verify` are **audit-baseline states, not completion**: task-03 is accepted
only when every row is `audited` (source-confirmed registration wired) or the
panel is proven layer-free. A layer found during the audit that is not in this
table is a collapse bug until registered.

## Mobile

Not applicable: the dockview workbench (and therefore floating panels) does
not render on phone viewports; the mobile task surface owns its own layers.

## Registration contract

- Host panels: `useFloatingOwnedLayer(layerRoot)` — idempotent unregister on
  Radix `onOpenChange(false)` AND React cleanup (unmount, navigation,
  ancestor teardown).
- Plugin panels: `host.ui.registerFloatingOwnedLayer(layerRoot: HTMLElement)
  => () => void`. **Per-panel capability channel:** the host issues an opaque
  ownership capability at `PluginTaskPanel` render time (bound to the exact
  portal element/instance for that panel render, revoked on unmount and
  plugin unregistration), and `registerFloatingOwnedLayer` requires and
  validates that capability — a plugin rendering two task panels cannot
  register a layer root from one panel against the other, and a plugin ID
  closure alone is never trusted. `PluginUIApi` gains the method as a
  callable (outside the mapped component type); unregister is idempotent on
  close, unmount, and `unregisterPlugin` cleanup. Tested: same-plugin
  different-panel rejection, unmount revocation, unregister cleanup.

## Test matrix (task-03)

One real test per primitive family, each inside a floating window: Dialog,
Popover, DropdownMenu, ContextMenu, HoverCard, plus one plugin-panel layer.

## Audit procedure

During task-03, run a full pass over every panel content component listed
above; any Radix primitive found that opens a layer must either be added to
this table or be proven layer-free. A layer that can open while the window is
expanded and is not registered here is a collapse bug.
