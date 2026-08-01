---
spec: docs/specs/ui/webkit-task-dialog-rendering.md
created: 2026-08-01
status: complete
---

# Implementation Plan: WebKit Task Dialog Rendering

## Overview

Classify the active rendering engine before React mounts, then opt only the Create Task dialog into
a WebKit-safe CSS path. Prove engine classification with unit tests and prove both the unchanged
Chromium path and the WebKit desktop/mobile geometry through focused browser tests.

## Confirmed root cause

`DialogContent` combines fixed centering with translated positioning and a `scale(0.95)` enter/exit
animation on the same element that contains the form text. WebKit rasterizes transformed,
composited text-bearing layers differently from Blink, producing the reported soft dialog while
the same Go-served SPA and fonts remain sharp in Chrome. The overlay and content also share
`z-50`, leaving their relative composited-layer ordering implicit. The Tauri window is opaque and
does not configure vibrancy or transparency.

## Frontend

### Rendering-engine marker

- Add `apps/web/lib/browser/rendering-engine.ts` with a pure classifier and a small document-marker
  helper.
- Classify a user agent as WebKit when it contains `AppleWebKit` and is not a desktop Blink runtime.
  Keep iPhone, iPad, iPod, and touch-capable iPadOS desktop-mode user agents on the WebKit path even
  when their browser brand is Chrome, Edge, or Firefox.
- Treat desktop `Chrome`, `Chromium`, `HeadlessChrome`, `Edg`, `OPR`, and `SamsungBrowser` tokens as
  Blink/non-WebKit compatibility UAs. Firefox and unknown engines fall back to `other`.
- In `apps/web/src/main.tsx`, apply `data-rendering-engine="webkit|other"` to the document root
  before boot-payload loading and React rendering. Do not persist the value.

### Create Task dialog rendering

- Add a semantic opt-in data attribute to the Create Task `DialogContent` in
  `apps/web/components/task-create-dialog.tsx`; do not modify the shared default motion in
  `apps/packages/ui/src/dialog.tsx`.
- Add a semantic nested-confirmation marker to
  `apps/web/components/discard-local-changes-dialog.tsx` so the WebKit stack can elevate that
  confirmation without styling by test IDs.
- Add scoped WebKit selectors and opacity-only keyframes in `apps/web/app/globals.css`.
- Under `html[data-rendering-engine="webkit"]`, override only the opted-in dialog's animation name
  so its keyframes never write `transform`; replace translated centering with `inset: 0`, automatic
  margins, and desktop `height: fit-content`; lower only its own overlay to `z-49` while keeping
  content at the shared `z-50` modal level; and elevate the nested discard confirmation to `z-53`.
- Preserve the current mobile `width: 100%` and `height: 100%`, desktop 900px width, 85vh maximum
  height, form overflow ownership, rounded corners, focus, and dismissal behavior.
- Leave the existing scale-and-fade utility classes active for non-WebKit engines.

### Mobile design contract

- **Desktop outcome:** Safari and WebKit-backed Tauri show the existing centered 900px Create Task
  form without transformed text; Chromium retains its current motion.
- **Mobile entry point:** the existing Kanban floating New Task action opens the same full-height
  dialog.
- **Nearest shipped exemplar:** the existing mobile Create Task surface and
  `MobileKanbanPage.mobileFab` remain the composition and entry-point baseline.
- **Hierarchy and primary action:** task name, prompt, repositories, agent profile, and submit
  controls remain in their existing order; task creation remains the primary action.
- **Presentation rationale:** this is a rendering workaround, so preserving the existing
  full-height mobile dialog is less disruptive than introducing a new drawer or route.
- **Geometry:** the dialog remains the single viewport-bound surface, keeps its existing internal
  form scroll owner and safe-area behavior, and introduces no document horizontal overflow.
- **Shared logic:** all task state, validation, selectors, and submit handlers remain unchanged;
  only engine classification and presentation CSS differ.
- **Mobile proof:** Pixel 5 E2E forces the WebKit marker and verifies full-viewport containment,
  accessible inputs, internal usability, and zero document horizontal overflow.

## Tests

- **What:** Safari/WKWebView/WebKitGTK, desktop Blink, Firefox, iOS branded browsers, iPadOS
  desktop mode, and unknown-UA classification.
  - **File:** `apps/web/lib/browser/rendering-engine.test.ts`
  - **How:** table-driven Vitest cases against the pure classifier plus marker-helper assertions.
- **What:** the marker is applied before React boot without persistence.
  - **File:** `apps/web/lib/browser/rendering-engine.test.ts`
  - **How:** pass a detached document root and navigator-like input to the marker helper and assert
    the exact `data-rendering-engine` value.

## E2E Tests

- **Scenario:** Chromium retains its existing Create Task scale motion and positioning when the
  runtime marker is `other`.
  - **File:** `apps/web/e2e/tests/task/create-task-webkit-rendering.spec.ts`
  - **What to verify:** the early marker is `other`, the dialog keeps the existing animation name,
    translated centering, dimensions, and content-over-overlay hit testing.
- **Scenario:** the WebKit override removes transforms while preserving centered desktop geometry.
  - **File:** `apps/web/e2e/tests/task/create-task-webkit-rendering.spec.ts`
  - **What to verify:** force the root marker to `webkit` before opening the dialog; assert the
    opacity-only animation name, identity `transform`/zero `translate`, explicit overlay/content
    stacking levels, 900px capped width, viewport-centered bounds, and focused task-name input.
- **Scenario:** a narrow WebKit Create Task dialog keeps its full-height usable composition.
  - **File:** `apps/web/e2e/tests/task/mobile-create-task-webkit-rendering.spec.ts`
  - **What to verify:** force the WebKit marker, open from `MobileKanbanPage.mobileFab`, assert
    viewport containment, task-name and prompt reachability, internal scrolling when required, and
    no document horizontal overflow.

## Implementation Tasks

- [x] [Task 01: Classify the rendering engine](task-01-rendering-engine-marker.md)
- [x] [Task 02: Apply WebKit-safe task dialog rendering](task-02-webkit-dialog-rendering.md)

Execution is sequential in the primary conversation. No subagent delegation is planned or
authorized.

## Risks

- Browsers do not expose a standardized rendering-engine API. The classifier therefore needs
  explicit compatibility-UA tests and must fail safe to the existing rendering path.
- Chromium E2E can prove selector behavior and geometry after forcing the root marker, but it
  cannot reproduce WKWebView rasterization. Final acceptance requires a macOS Tauri/Safari visual
  comparison at actual-size zoom (`Cmd+0`) after the automated checks pass.
- Transform-free absolute centering can accidentally stretch an auto-height desktop dialog. The
  WebKit desktop selector must set `height: fit-content`, while the narrow layout retains the
  existing full-height rule.
- The opt-in attribute deliberately limits blast radius. Other dialogs remain out of scope until
  they show the same reproduced WebKit defect.
