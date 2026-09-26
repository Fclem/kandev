import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useTaskListFacetSelection } from "./use-task-list-facet-selection";

const FACET_KEY = "facet:plugin:tags";

const baseProps = {
  facetKeys: [FACET_KEY],
  requestedSort: FACET_KEY as `facet:${string}:${string}`,
  requestedGroup: FACET_KEY as `facet:${string}:${string}`,
  tasks: [{ id: "task-1" }] as never[],
  facetValues: {},
  onSortChange: vi.fn(),
  onGroupChange: vi.fn(),
};

describe("useTaskListFacetSelection", () => {
  // AC-PLUGINS-TASKLIST-FACETS-003.3: an unavailable facet retains the request.
  it("falls back without writing and restores when the facet becomes available", () => {
    const onSortChange = vi.fn();
    const onGroupChange = vi.fn();
    const props = { ...baseProps, facetKeys: [] as string[], onSortChange, onGroupChange };
    const { result, rerender } = renderHook((next) => useTaskListFacetSelection(next), {
      initialProps: props,
    });
    expect(result.current.sort).toBe("updated_desc");
    expect(result.current.group).toBe("state");
    expect(onSortChange).not.toHaveBeenCalled();
    expect(onGroupChange).not.toHaveBeenCalled();
    rerender({ ...props, facetKeys: [FACET_KEY] });
    expect(result.current.sort).toBe(FACET_KEY);
    expect(result.current.group).toBe(FACET_KEY);
    rerender(props);
    expect(result.current.sort).toBe("updated_desc");
    expect(onSortChange).not.toHaveBeenCalled();
  });

  it("writes a chosen facet or built-in value through the same preference handlers", () => {
    const onSortChange = vi.fn();
    const onGroupChange = vi.fn();
    const { result } = renderHook(() =>
      useTaskListFacetSelection({ ...baseProps, onSortChange, onGroupChange }),
    );
    act(() => result.current.selectSort("title_asc"));
    act(() => result.current.selectGroup("workflow"));
    act(() => result.current.selectSort(FACET_KEY));
    act(() => result.current.selectGroup(FACET_KEY));
    expect(onSortChange.mock.calls).toEqual([["title_asc"], [FACET_KEY]]);
    expect(onGroupChange.mock.calls).toEqual([["workflow"], [FACET_KEY]]);
  });
});
