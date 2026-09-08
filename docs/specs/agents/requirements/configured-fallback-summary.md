---
status: active
system: agents
created: 2026-09-08
owners:
  - kandev
---

# Configured fallback summary requirements

## Overview

Agent profile rows on the Agents settings page expose the fallback policy saved
for each profile. The summary lets users distinguish strict operation, automatic
selection of the next model, and a configured explicit fallback without opening
the profile editor.

## Terminology

- **Strict fallback state:** `auto_fallback` is false and `fallback_model` is empty.
- **Automatic fallback state:** `auto_fallback` is true. The configured explicit
  fallback value is ignored by runtime precedence.
- **Explicit fallback state:** `auto_fallback` is false and `fallback_model` is
  non-empty.

## Requirements

### REQ-AGENTS-CONFIGURED-FALLBACK-SUMMARY-001: Show the configured fallback policy

**Intent:** Make the saved fallback policy visible in the profile list.

**User story:** As a Kandev user, I want each agent profile row to show its
configured fallback policy, so that I can understand how an unavailable start
model will be handled without opening every profile.

#### Acceptance criteria

- **AC-AGENTS-CONFIGURED-FALLBACK-SUMMARY-001.1:** When a profile row is shown
  on Settings > Agents, it shall render a fallback pill immediately after the
  model pill, with `fallback: none` for the strict fallback state, `fallback:
  next` for the automatic fallback state, or `fallback: <configured model>` for
  the explicit fallback state.
- **AC-AGENTS-CONFIGURED-FALLBACK-SUMMARY-001.2:** The fallback pill shall use
  the profile's saved `auto_fallback` and `fallback_model` values, and shall not
  change those values or infer runtime availability.
- **AC-AGENTS-CONFIGURED-FALLBACK-SUMMARY-001.3:** The fallback summary shall
  remain readable and wrap with the existing profile metadata on phone-sized
  screens without introducing document-level horizontal overflow.
- **AC-AGENTS-CONFIGURED-FALLBACK-SUMMARY-001.4:** The fallback summary shall
  be localized through the Agents translation namespace while preserving the
  configured model identifier as opaque product data.

## Out of scope

- Changing runtime fallback precedence or model selection.
- Editing profile fallback settings from the profile list.
- Showing executor availability, runtime effective models, or fallback warnings.
