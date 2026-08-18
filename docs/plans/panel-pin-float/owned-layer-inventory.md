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
| changes / diff | Dialog, AlertDialog | `changes-panel-dialogs.tsx` (Dialog/AlertDialog imports ~8-24; AlertDialog open ~56) | to-wire |
| changes / diff | DropdownMenu | `changes-top-bar.tsx` (DropdownMenu ~59-77) | to-wire |
| chat / session tabs (`chat`) | DropdownMenu, Dialog, Popover, ContextMenu | `chat/chat-input-toolbar-primitives.tsx` (~298), `chat/message-actions.tsx` (~212, ~264), `chat/session-menu.tsx`, `chat/queue-controls.tsx`, `chat/feedback-popover.tsx` (exact ranges confirmed during task-03 audit) | to-wire |
| plan (`plan`) | Popover, Dialog | `plan/plan-panel-popovers.tsx`, `plan/tiptap-toolbar-menu.tsx` (exact ranges confirmed during task-03 audit) | to-wire |
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
- Plugin panels: `host.ui.registerFloatingOwnedLayer(capability, layerRoot)
  => () => void` (**two arguments, matching the SDK/spec contract exactly** —
  the opaque `floatingOwnedLayerCapability` type is defined in the SDK, and
  the mobile rejection result and unregister semantics are declared here).
  **Per-panel capability channel:** the host issues an opaque ownership
  capability at `PluginTaskPanel` render time from a portal-instance
  generation (bound to the exact portal element/instance for that panel
  render, `useRef`-stable across benign re-renders, revoked on actual
  release and plugin unregistration, rotated on reacquire, absent on mobile
  where host registration is rejected), and `registerFloatingOwnedLayer`
  requires and validates that capability — a plugin rendering two task panels
  cannot register a layer root from one panel against the other, and a plugin
  ID closure alone is never trusted. Unregister is idempotent on close,
  unmount, and `unregisterPlugin` cleanup. Tested: same-plugin
  different-panel rejection, unmount revocation, release-reacquire rotation,
  benign re-render stability, mobile rejection, unregister cleanup.

## Test matrix (task-03)

One real test per primitive family, each inside a floating window: Dialog,
Popover, DropdownMenu, ContextMenu, HoverCard, plus one plugin-panel layer.

## Audit procedure

During task-03, run a full pass over every panel content component listed
above; any Radix primitive found that opens a layer must either be added to
this table or be proven layer-free. A layer that can open while the window is
expanded and is not registered here is a collapse bug.
