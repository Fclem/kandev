# Owned-layer inventory (floating panels)

Required by `docs/specs/ui/panel-pin-float.md` (Focus ownership → Layer
inventory) and `docs/plans/panel-pin-float/task-03-floating-window-edge-bar.md`.

Every interactive Radix layer opened from inside a floating-capable dockview
panel MUST call `useFloatingOwnedLayer` (host) /
`host.ui.registerFloatingOwnedLayer` (plugin panels), or the floating window
collapses while the layer is open (a contract violation).

Legend for `Status`: `audited` = callsite confirmed in the baseline and the
hook is applied by this feature (or layer-free proof); `to-wire` = callsite
confirmed, hook must be applied during task-03; `verify` = candidate surface,
confirm during task-03. **Source audit status (revision 39): COMPLETE —
every row below is file/line-anchored from live source (scout audit
2026-08-18 + parent verification 2026-08-18: model-config-selector.tsx
587-614 added, the github/gitlab/review reachable surface is a bounded
per-file enumeration with exact ranges via the AST gate, settings/connection
surfaces are declared out-of-scope with panel-reachability proofs); the
former directory-level chat/plan rows are replaced with the
real owners; browser/todos/vscode/dev-server carry layer-free proofs; the
`@kandev/ui` wrappers (not direct @radix-ui imports) are the primitive
channel.**

## Desktop dockview panels

| Panel (component) | Primitive family | Exact owner (component/file) | Status |
|---|---|---|---|
| changes / diff | Dialog, HoverCard | `changes-panel-header.tsx` (Dialog ~139-170; HoverCard ~306-311) | to-wire |
| changes / diff | Drawer | `changes-panel-header.tsx` (touch Drawer ~293-302) | to-wire |
| changes / diff | DropdownMenu | `changes-panel-header.tsx` (PullDropdown ~411-425) | to-wire |
| changes / diff | ContextMenu | `changes-tab.tsx` (ContextMenu ~118-136) | to-wire |
| changes / diff | Dialog, AlertDialog | `changes-panel-dialogs.tsx` (DiscardDialog AlertDialog 56-80 decl 41; AmendDialog Dialog 105-149 decl 95; ResetDialog Dialog 223-289 decl 192) — CONFIRMED | to-wire |
| changes / diff | DropdownMenu | `changes-top-bar.tsx` (DropdownMenu ~59-77) | to-wire |
| chat (`chat`) | Drawer | `chat/chat-input-toolbar-primitives.tsx` 298-322 (McpIndicator, decl 241) — CONFIRMED | to-wire |
| chat (`chat`) | Dialog | `chat/messages/message-actions.tsx` 212-240 (MessageDebugDialog, decl 197) — CONFIRMED | to-wire |
| chat (`chat`) | Drawer | `chat/messages/message-actions.tsx` 264-283 (MessageTimestamp, decl 244) — CONFIRMED | to-wire |
| chat (`chat`) | Drawer | `chat/messages/message-comment-surface.tsx` 265-283 (comment composer) | to-wire |
| chat (`chat`) | Dialog | `chat/messages/agent-plan-message.tsx` 109-116 (PlanMessageDialog) | to-wire |
| chat (`chat`) | HoverCard | `chat/messages/chat-message.tsx` 261-275 (PromptPreview mention) | to-wire |
| chat (`chat`) | HoverCard | `chat/context-items/context-chip.tsx` 121-145 (ContextChip) | to-wire |
| chat (`chat`) | Dialog | `chat/context-items/image-item.tsx` 147-152 (ImageItem preview) | to-wire |
| chat (`chat`) | Dialog | `chat/image-preview-dialog.tsx` 47-61 (ImagePreviewDialog) | to-wire |
| chat (`chat`) | Popover | `chat/context-popover.tsx` 312-318 (ContextPopover; session-menu surface) | to-wire |
| chat (`chat`) | AlertDialog | `chat/reset-context-button.tsx` 65-83 (ResetContextButton) | to-wire |
| chat (`chat`) | DropdownMenu | `chat/implement-plan-button.tsx` 113-148 (ImplementPlanButton) | to-wire |
| chat (`chat`) | HoverCard + Popover | `chat/todo-indicator.tsx` 161-172 / 177-189 (TodoIndicator) | to-wire |
| chat (`chat`) | HoverCard | `chat/tiptap-mention-extension.tsx` 142-154 (mention chip) | to-wire |
| chat (`chat`) | DropdownMenu | `sessions-dropdown.tsx` 273-343 (SessionsDropdown; doc's session-menu.tsx does NOT exist) | to-wire |
| chat (`chat`) | DropdownMenu | `mode-selector.tsx` 176-196 (ModeSelector) | to-wire |
| chat (`chat`) | Popover | `model-selector.tsx` (custom trigger) → `components/model-config-selector.tsx` 587-614 (Popover open/onOpenChange + PopoverContent 598) — the REAL layer owner (model-selector.tsx:9-14 imports it) | to-wire |
| chat (`chat`) | none | queue controls (SubmitButton/cancel, chat-input-toolbar-primitives.tsx 144-193) = plain buttons, layer-free | audited |
| plan (`plan`) | custom floating (non-radix) | `plan-selection-popover.tsx` 287-319 (createPortal div — owned-layer treatment DECIDED: host `useFloatingOwnedLayer` lease with explicit open/close lifecycle; doc's plan-panel-popovers.tsx does NOT exist) | to-wire |
| plan (`plan`) | Popover | `task-plan-revisions.tsx` 137-174 (TaskPlanRevisions) | to-wire |
| plan (`plan`) | Dialog | `task-plan-revisions.tsx` 495-530 (revert-confirm) | to-wire |
| plan (`plan`) | Dialog | `task-plan-diff-dialog.tsx` 63-106 (PlanRevisionDiffDialog) | to-wire |
| plan (`plan`) | Dialog | `task-plan-preview-dialog.tsx` 61-134 (PlanRevisionPreviewDialog) | to-wire |
| plan (`plan`) | none | plan-tab.tsx = layer-free tab header; no radix tiptap toolbar menu exists | audited |
| terminal (`terminal`) | ContextMenu | `terminal-tab.tsx` 187-220 (rename/destroy; ContentMenuContent helper 368-385) | to-wire |
| terminal (`terminal`) | Popover | `close-terminal-confirm-popover.tsx` 43-107 (decl 28) | to-wire |
| terminal (`terminal`) | DropdownMenu | `terminal-reopen-menu.tsx` (TerminalReopenMenuItems decl 35) | to-wire |
| terminal (`terminal`) | DropdownMenu | `parked-terminals-menu.tsx` (ParkedTerminalsMenu decl 18) | to-wire |
| terminal (`terminal`) | none | shell-terminal.tsx / passthrough-terminal.tsx / terminal-panel.tsx: no radix overlay (xterm right-click = native handling — verify separately) | verify |
| files (`files`) | ContextMenu | `file-context-menu.tsx` 299-319 (FileContextMenu, decl 336) | to-wire |
| files (`files`) | AlertDialog | `file-context-menu.tsx` 321-329 (DeleteConfirmDialog) | to-wire |
| files (`files`) | ContextMenu (+Sub) | `file-tree-editor-menu.tsx` 123-141 (OpenInEditorMenuItems) | to-wire |
| files (`files`) | DropdownMenu | `editors-menu.tsx` 100-104 / 205-210 (EditorIconMenu / EditorsMenu) | to-wire |
| files (`files`) | none | files-panel.tsx / file-browser.tsx / file-viewer-header.tsx layer-free at root (delegate above) | audited |
| browser (`browser`) | none | LAYER-FREE PROOF: plain `<Input>` URL bar + buttons + iframe; inspector/inspect-button.tsx + inspector/annotations-panel.tsx contain no overlay | audited |
| pr-detail / mr-detail / review-detail | none | review-detail-panel.tsx = layer-free router; real surfaces below | audited |
| pr / mr (`github`) | DropdownMenu | `github/pr-merge-button.tsx` 239-259 (PRMergeButton) | to-wire |
| pr / mr (`github`) | Popover + DropdownMenu | `github/pr-topbar-button.tsx` 208-222 / 288-303 / 309-333 (PRTopbarButton) | to-wire |
| pr / mr (`github`) | Popover + Drawer | `github/pr-status-chip.tsx` 251-274 / 341-368 / 384-409 / 418-439 (PRStatusChip) | to-wire |
| review (`review`) | Dialog | `review/review-dialog-surface.tsx` 100-162 (ReviewDialogSurface) | to-wire |
| review (`review`) | DropdownMenu | `review/review-top-bar.tsx` 79-97 (settings) | to-wire |
| review (`review`) | DropdownMenu | `review/review-diff-toolbar.tsx` 224-292 (FileDiffToolbar) | to-wire |
| pr / mr / review reachable surface (github) | Dialog, Popover, Drawer, Select | bounded enumeration of overlay-bearing non-test files reachable from pr-detail/mr-detail/review-detail: `github/pr-ci-popover.tsx`, `github/pr-ci-automation-rows.tsx`, `github/pr-mergeability-row.tsx`, `github/pr-mergeability-notice.tsx`, `github/issue-watch-dialog.tsx`, `github/review-watch-dialog.tsx`, `github/repo-filter-selector.tsx`, `github/my-github/save-preset-dialog.tsx`, `github/my-github/presets-sidebar.tsx`, `github/my-github/presets-scope-bar.tsx`, `github/my-github/list-toolbar.tsx`, `github/my-github/quick-task-launcher.tsx`, `github/my-github/issue-list.tsx` | to-wire (ranges via AST gate) |
| pr / mr / review reachable surface (gitlab) | Dialog, Popover, Drawer | bounded enumeration reachable from mr-detail: `gitlab/mr-ci-popover.tsx`, `gitlab/mr-status-chip.tsx`, `gitlab/mr-status-chip-drawer.tsx`, `gitlab/mr-status-chip-popover.tsx`, `gitlab/mr-status-chip-trigger.tsx`, `gitlab/mr-topbar-button.tsx`, `gitlab/mr-merge-button.tsx`, `gitlab/mr-reviewer-control.tsx`, `gitlab/mr-automation-controls.tsx`, `gitlab/mr-task-icon.tsx`, `gitlab/watch-dialog.tsx`, `gitlab/delete-watch-dialog.tsx`, `gitlab/task-mr-link-dialog.tsx`, `gitlab/my-gitlab/list-toolbar.tsx`, `gitlab/my-gitlab/presets-sidebar.tsx`, `gitlab/my-gitlab/presets-scope-bar.tsx`, `gitlab/my-gitlab/save-preset-dialog.tsx` | to-wire (ranges via AST gate) |
| pr / mr / review reachable surface (review) | Dialog, DropdownMenu, Popover | bounded enumeration reachable from review-detail: `review/review-dialog.tsx`, `review/review-diff-list.tsx`, `review/review-file-tree.tsx`, `review/review-comments-overview.tsx`, `review/review-findings-overview.tsx`, `review/review-findings-button.tsx`, `review/review-fix-comments-button.tsx`, `review/review-pr-selector.tsx`, `review/walkthrough-overlay.tsx`, `review/review-repository-identity.ts` (no overlay — identity helper) | to-wire (ranges via AST gate) |
| github/gitlab settings + connection surfaces | Dialog, Select, Drawer | OUT OF SCOPE (settings-page surfaces, not rendered inside dockview panels): github-app-connection-panel, github-connection-dialog, github-app-import-guide, github-app-registration-list, github-access-help, github-cli-form, action-presets-section, default-queries-section, github-repo-scope-section, github-connection-settings-form, gitlab-settings — the AST gate asserts none is reachable from a floating-capable panel; a panel-reachability proof is recorded per file | to-wire (proofs via AST gate) |
| todos (`todos`) | none | LAYER-FREE PROOF: TodosContent = plain list; TodoIndicator popovers belong to the chat chip, not todo rows; no row menu | audited |
| vscode / dev-server | none | LAYER-FREE PROOF: vscode-panel.tsx + dev-server-preview-button.tsx import/render no overlay primitive | audited |
| plugin panels (`plugin-panel`) | any (plugin-owned) | via `host.ui.registerFloatingOwnedLayer` (per-panel capability) — capability DOES NOT exist yet (sdk index.ts:492-494, host-api.ts 205+/373+, plugin-task-panel.tsx); task-03 deliverable; plugin-task-panel.tsx root is layer-free | to-wire |

**Audit rule (task-03, blocking):** every Radix primitive that opens a layer
inside a floating-capable panel must be a row here with its exact file and
line range (or an explicit layer-free proof for the panel). `to-wire` and
`verify` are **audit-baseline states, not completion**: task-03 is accepted
only when every row is `audited` (source-confirmed registration wired) or the
panel is proven layer-free. A layer found during the audit that is not in this
table is a collapse bug until registered. **Exact-anchor mechanism for the
broad reachable-surface rows (github/gitlab/review): the ACCEPTANCE
SOURCE OF TRUTH is a COMMITTED GENERATED ANCHOR ARTIFACT
(`apps/web/config/owned-layer-inventory.generated.json`, declared schema:
`{ file, component, primitiveFamily, renderStart, renderEnd, inventoryRowId }`)
produced by the AST generator from live source and committed with the
feature; the AST gate (`check-owned-layer-inventory.mjs`) is
VALIDATION-ONLY — it compares the generated artifact against BOTH the
live source (anchors must still resolve to the same primitive) AND the
inventory rows (every artifact entry maps to a row, every row has at
least one artifact entry or a layer-free proof), and fails on any drift
or a discovered primitive without a row; out-of-scope settings/connection
rows flip to `audited` only when their panel-reachability proofs are
attached (the gate asserts no overlay is reachable from a floating-
capable panel). The markdown is the human-readable view; the generated
artifact is the acceptance source.** **GENERATOR LIFECYCLE is
REPRODUCIBLE: `generate-owned-layer-inventory.mjs` (named, with exact
command `node scripts/generate-owned-layer-inventory.mjs`, deterministic
source globs + canonical ordering + schema) runs BEFORE validation in the
developer and CI targets; CI fails if the generator's output differs from
the committed artifact (uncommitted-diff failure) and then runs the
validator against the committed artifact; authors regenerate after every
source change to an owned-layer file, before commit/PR; a stale-artifact
fixture is tested.** **GENERATOR COMPLETENESS IS INDEPENDENTLY CHECKED:
the generator's input set is asserted to EQUAL an independently derived
set — every panel-content file reachable from the desktop component
registry / import graph (broad source scan) must appear in the generator
input globs; a newly added panel directory outside the globs is caught by
this set-equality assertion (test: add an out-of-glob panel dir, the gate
fails); diff-only validation can never bless an incomplete artifact.**

## Mobile

Not applicable: the dockview workbench (and therefore floating panels) does
not render on phone viewports; the mobile task surface owns its own layers.

## Custom (non-Radix) portal ownership

`plan-selection-popover.tsx:287-319` portals to `document.body` outside the
floating window subtree. It MUST use the host `useFloatingOwnedLayer` lease
with an explicit open/close lifecycle (register on open, unregister on
close/unmount — same pending/refcount/generation semantics as Radix layers);
a custom portal that cannot prove an open-state transition is NOT an owned
layer and can be collapsed by outside focus/pointer while open. **The lease
is bound to the FLOATING-WINDOW ROOT TOKEN (`{groupLogicalId, generation}`),
not to the panel's DOM location: panel content is portaled by
PanelPortalHost into entry.element siblings OUTSIDE Dockview
(panel-portal-host.tsx:28-57), so region checks use the floating overlay
root/window lease; the token propagates through adopted portal content and
is REQUIRED for custom registration (a body-portal without a valid token is
not owned); a lease keyed only by component/capability can count an
unrelated body portal as owned — rejected. **DOCKED-PANEL RULE: the
floating lease is NULLABLE — a docked plan panel opens its popover with NO
lease and the owned-region coordinator is a NO-OP for it (no floating
window to collapse). DOCKED-TO-FLOATING HANDOFF: if a layer is ALREADY
open when its group floats, the float transaction performs an ATOMIC
handoff — existing open registrations are transferred to the new window's
lease/generation BEFORE the persistent content is reparented (an
open=true transition is NOT re-fired; the registration moves with its
lease identity); float-while-open and open-at-the-same-frame-as-float
races are covered by tests; there is no window where an already-open
layer is unowned during the transition frame.** **The swap is ONE
coordinator critical-section operation,
`transferFloatingLayerLease(portalInstanceKey, oldLease, newLease,
generation)`: validates the current entry, replaces the map entry in a
SINGLE store update (never delete-then-set — a synchronous
`useFloatingOwnedLayer` read can never observe a null window), returns
the new lease to both owner trees, and rejects stale callbacks (wrong
generation); deterministic read-before/during/after transfer test
asserts no null window and exactly one owner at every instant.** **TRANSPORT:
`FloatingWindowLeaseProvider` is rendered at the WORKBENCH ROOT — ABOVE
BOTH PanelPortalHost and the floating overlay, because React context
follows the React OWNER tree, not the destination DOM node: a provider
inside the overlay is NOT an ancestor of content rendered by the sibling
PanelPortalHost even after the DOM element is reparented
(panel-portal-host.tsx:31-54, 82-115). The provider holds
`Map<portalInstanceKey, lease>`; the floating overlay registers its lease
per groupLogicalId + generation; `useFloatingOwnedLayer` looks up BY
PORTAL INSTANCE KEY (never DOM ancestry); a lease-read-before-and-after-
DOM-reparent test is required.** **portalInstanceKey is an EXPLICIT
contract: `PortalEntry` gains an instance-key field (opaque, per portal
instance, independent of panelId — createPortal currently keys by panelId,
panel-portal-manager.ts:41-52); `renderPanel`/`PanelPortalHost` pass it
through React context around each createPortal render so content reads the
SAME key the overlay registration uses; key rotation on reacquire and the
envId update are ONE atomic store update in PanelPortalManager (the current
re-acquire path updates api/params/component but not envId — fixed);
`useFloatingOwnedLayer` is an explicit NO-OP outside a desktop workbench
provider (mobile task layout, session-mobile-layout.tsx — no floating
window exists); tests: docked, floating, mobile, re-acquire rotation.**
**portalInstanceKey is CONTEXT/PROVIDER IDENTITY ONLY: createPortal's
React key REMAINS the stable panelId for the entry's lifetime — rotating
the React key on reacquire would REMOUNT TaskPlanPanel/terminal/editor/
plugin trees and violate content liveness; the instance key rotates only
in the lease map; true portal RELEASE (which may remount) is
DISTINGUISHED from RE-ACQUIRE (no remount); PTY/editor liveness + context-
key rotation tests.**
Tests: panel reparent + body-
portal outside pointer and focusout.** A real
custom-portal test (open → outside pointerdown → no collapse → close →
collapse) is specified; this row flips to `audited` only after the wiring
lands.

## Registration contract

- Host panels: `useFloatingOwnedLayer(layerRoot)` — idempotent unregister on
  Radix `onOpenChange(false)` AND React cleanup (unmount, navigation,
  ancestor teardown).
- Plugin panels: `host.ui.registerFloatingOwnedLayer(capability,
  openHandlers) => () => void` (**per-open handshake, matching the SDK/spec
  contract exactly**: the plugin spreads the returned `onOpenChange` onto
  the Radix root — a body-portaled root node alone is NOT accepted as
  ownership proof; registration happens on open=true, unregistration on
  open=false / onDismiss / cleanup. The opaque
  `floatingOwnedLayerCapability` type is defined in the SDK, and
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
Popover, DropdownMenu, ContextMenu, HoverCard, **Drawer (real Drawer test —
McpIndicator/message-timestamp rows), plus one plugin-panel layer;
AlertDialog is explicitly covered BY the Dialog test family (same
owned-layer lease wiring) and flagged in the test name.**

## Audit procedure

During task-03, run a full pass over every panel content component listed
above; any Radix primitive found that opens a layer must either be added to
this table or be proven layer-free. A layer that can open while the window is
expanded and is not registered here is a collapse bug.
