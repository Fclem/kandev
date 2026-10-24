---
status: active
system: plugins
created: 2026-10-24
owners:
  - kandev
---

# Plugin Sidebar Task Facets

## Scope

The left sidebar's workspace-scoped saved task views expose active plugin task-list facets
in **Filters and sort → Group by**. This is separate from `/tasks` preferences and does not
add sidebar facet sorting or filtering. Plugins use the existing synchronous
`registerTaskListFacet({ id, label, getValues, subscribe? })` contract without a second API.

### REQ-PLUGINS-SIDEBAR-TASK-FACETS-001: Saved sidebar facet grouping

- **AC-PLUGINS-SIDEBAR-TASK-FACETS-001.1:** Built-in Group choices retain their current behavior and order. Active facets follow them,
  displaying the plugin's registered label verbatim (for example, **Tag**).
- **AC-PLUGINS-SIDEBAR-TASK-FACETS-001.2:** The requested grouping is encoded as `facet:<pluginId>:<facetId>` in the saved view's
  `group` field and in an unsaved view draft. Saving, reloading, switching views and switching
  workspaces preserve the appropriate workspace's selection.
- The host first obtains the ordinary bounded sidebar page using existing filters, sort
  chain, pinning and sibling order. Facet grouping is client-side over that accepted page,
  not a workspace-wide fetch or a server-side facet query. Groups and counts describe only
  loaded page members. Pagination continues to select source task rows before grouping.
- The explicit sidebar workspace id and each page task id are passed to `getValues`.
  Facet data from a previous workspace is not reused.
- Only nonempty groups render. Value groups are ordered by label, case-insensitively.
  Distinct value identifiers remain distinct even if labels are equal.
  A value's optional color appears as a decorative heading swatch on desktop and mobile,
  subject to the host's existing bounded CSS color validation. Missing/unsafe colors and
  Unassigned add no swatch; built-in headings remain unchanged.
- A task returning multiple valid values appears once in each matching group. Repeated
  identifiers from one callback do not duplicate a row within that group.
- A task with no valid value appears in one trailing **Unassigned** group. Blank labels or
  identifiers do not create groups; a plugin value named `Unassigned` cannot collide with
  the host fallback.
- Each section rebuilds its tree only from its own matching tasks. A child is nested only
  when its parent is present in that section; otherwise it is a root. A parent's membership
  never implicitly includes its descendants. Rows keep their task identity for actions and
  selection, while React section/row identity and parent collapse identity are section-local.
  Shift-click ranges use the clicked section-qualified row occurrence for both endpoints;
  bulk operations deduplicate the resulting canonical task IDs. A later copy of a task must
  never resolve to its first copy in another section.
- Group and subtask collapse are independent across facet values and across facets. A child
  absent from one group does not affect that group's count or expose an empty subtask toggle.
- Plugin `subscribe` notifications reproject values and labels without refetching tasks.
  Registry registration/unregistration uses the existing reactive plugin lifecycle.
- If a requested facet is unavailable, the list and Group control temporarily use **None**.
  They do not write that fallback to the saved view or draft. Registering the facet again
  restores the original grouping automatically. Callback errors remain contained by the
  existing facet resolver and resolve that task to **Unassigned**.

## Implementation boundaries

`GroupKey` extends the existing built-in string union with the existing namespaced facet
option type. Saved-view wire mapping already stores string group values; no backend schema
or plugin SDK signature change is required. The sidebar query substitutes built-in `none`
and omits disclosure for facet requests so one section's collapse cannot hide another
section's copy at the source. The accepted page is then reprojected using
`useTaskListFacets`, including its registry ownership, validation and subscriptions.

Facet groups own section-local child maps; built-in groups retain their existing shared
child map. Rendering, counts, DnD membership, memoization and visible-range traversal consume
the appropriate map. Collapse keys encode both the facet section and parent task id.

## Regression coverage

- `sidebar-facet-groups.test.ts`: memberships, nonempty fallback, ordering, source filters,
  sorting, pinning, sibling order, per-section hierarchy and collapse identities.
- `use-sidebar-facet-grouping.test.tsx`: live membership/label updates, workspace changes,
  registry removal/restoration, subscription cleanup, built-in behavior and shared child maps.
- `group-picker.test.tsx`: appended options, encoded selection and unavailable fallback.
- `sidebar-view-wire.test.ts`: saved-view and draft round trips including facet collapse keys.
- `use-sidebar-task-page.test.tsx`: built-in server query and local disclosure for saved facets.
- `task-switcher-facet-selection.test.tsx`: modifier clicks on repeated rows select only the
  intended rendered range, preserve occurrence anchors and deduplicate bulk task IDs.
- `use-task-multi-select-range.test.ts`: repeated anchors/targets, missing occurrences and
  canonical-ID deduplication, alongside unchanged built-in range behavior.
