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

| Panel (component) | Primitive family | Likely layer owners (component/file) | Status |
|---|---|---|---|
| chat / session tabs (`chat`) | DropdownMenu, Dialog, Popover, ContextMenu | `components/task/chat/` (message actions, session menu, queue panel controls, feedback popover) | to-wire |
| plan (`plan`) | Popover, Dialog | `components/task/plan/` (plan panel popovers, tiptap toolbar menus) | to-wire |
| terminal (`terminal`) | ContextMenu, Dialog | xterm right-click menu, terminal tab rename/destroy menu, `TerminalScriptsDropdown` (`dockview-header-actions.tsx`) | to-wire |
| files (`files`) | ContextMenu, Popover | file-tree context menu, hover actions | to-wire |
| changes / diff (`changes`, `diff-files`, `all-files`, `diff-viewer`, `file-editor`, `commit-detail`) | Popover, Dialog, ContextMenu | `components/diff/`, review-finding popovers, comment popups, `walkthrough-step-card` | to-wire |
| browser (`browser`) | Popover, Dialog | browser panel URL bar, inspect annotations popup | verify |
| pr-detail / mr-detail / review-detail | Popover, Dialog, DropdownMenu | review surfaces, action menus | verify |
| todos (`todos`) | DropdownMenu | todo list row menus | verify |
| vscode (`vscode`) | none (iframe host) | no owned layers expected | verify |
| dev-server (`terminal`) | none | no owned layers expected | verify |
| plugin panels (`plugin-panel`) | any (plugin-owned) | via `host.ui.registerFloatingOwnedLayer` (host-issued ownership validated against the plugin's task-panel portal) | to-wire |

## Mobile

Not applicable: the dockview workbench (and therefore floating panels) does
not render on phone viewports; the mobile task surface owns its own layers.

## Registration contract

- Host panels: `useFloatingOwnedLayer(layerRoot)` — idempotent unregister on
  Radix `onOpenChange(false)` AND React cleanup (unmount, navigation,
  ancestor teardown).
- Plugin panels: `host.ui.registerFloatingOwnedLayer(layerRoot: HTMLElement)
  => () => void` — the host validates `layerRoot` against the calling
  plugin's registered task-panel portal (ownership capability issued by the
  host, not trusted plugin ID closure); unregister is idempotent on close,
  unmount, and plugin unregistration (`unregisterPlugin` cleanup).

## Test matrix (task-03)

One real test per primitive family, each inside a floating window: Dialog,
Popover, DropdownMenu, ContextMenu, HoverCard, plus one plugin-panel layer.

## Audit procedure

During task-03, run a full pass over every panel content component listed
above; any Radix primitive found that opens a layer must either be added to
this table or be proven layer-free. A layer that can open while the window is
expanded and is not registered here is a collapse bug.
