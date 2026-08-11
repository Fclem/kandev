import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { MockInstance } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useTaskTitleSelectionRestore } from "./use-task-title-selection-restore";

const LONG = "T".repeat(60);

function Harness({ initial }: { initial: string }) {
  const [value, setValue] = useState(initial);
  const { inputRef, clampChange } = useTaskTitleSelectionRestore(value);
  return (
    <input
      ref={inputRef}
      data-testid="title"
      value={value}
      onChange={(e) => setValue(clampChange(e))}
    />
  );
}

/**
 * Simulate typing: write the DOM value through the prototype setter (React's
 * instance value tracker would otherwise swallow the change), place the caret,
 * then dispatch the change event React maps to onChange.
 */
function simulateInsert(
  input: HTMLInputElement,
  value: string,
  caret: number,
  setSelectionRange: MockInstance,
) {
  const setNativeValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
  setNativeValue.call(input, value);
  input.setSelectionRange(caret, caret);
  setSelectionRange.mockClear();
  fireEvent.change(input);
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("useTaskTitleSelectionRestore", () => {
  it("clamps the value to 60 code points on change", () => {
    render(<Harness initial={LONG.slice(0, 59)} />);
    const input = screen.getByTestId("title") as HTMLInputElement;
    input.focus();
    simulateInsert(input, LONG, 60, vi.spyOn(HTMLInputElement.prototype, "setSelectionRange"));
    expect(input.value).toHaveLength(60);
  });

  it("pins the caret after the inserted text when the clamp truncates", () => {
    const setSelectionRange = vi.spyOn(HTMLInputElement.prototype, "setSelectionRange");
    render(<Harness initial={LONG} />);
    const input = screen.getByTestId("title") as HTMLInputElement;
    input.focus();
    // Simulate the DOM right after typing "XY" at position 6: 62 code points,
    // caret at 8.
    simulateInsert(input, `${LONG.slice(0, 6)}XY${LONG.slice(6)}`, 8, setSelectionRange);

    expect(input.value).toHaveLength(60);
    expect(input.value.slice(6, 8)).toBe("XY");
    // The caret must be re-pinned after React rewrites the truncated value.
    expect(setSelectionRange).toHaveBeenCalledWith(8, 8);
  });

  it("leaves the caret alone when the clamp does not truncate", () => {
    const setSelectionRange = vi.spyOn(HTMLInputElement.prototype, "setSelectionRange");
    render(<Harness initial={LONG.slice(0, 30)} />);
    const input = screen.getByTestId("title") as HTMLInputElement;
    input.focus();
    simulateInsert(input, `${"T".repeat(30)}XY`, 32, setSelectionRange);
    expect(input.value).toHaveLength(32);
    expect(setSelectionRange).not.toHaveBeenCalled();
  });

  it("skips the restore when the input is not focused", () => {
    const setSelectionRange = vi.spyOn(HTMLInputElement.prototype, "setSelectionRange");
    render(<Harness initial={LONG} />);
    const input = screen.getByTestId("title") as HTMLInputElement;
    simulateInsert(input, `${LONG.slice(0, 6)}XY${LONG.slice(6)}`, 8, setSelectionRange);
    expect(setSelectionRange).not.toHaveBeenCalled();
  });

  it("does not replay a stale selection from an unfocused truncating change", () => {
    const setSelectionRange = vi.spyOn(HTMLInputElement.prototype, "setSelectionRange");
    render(<Harness initial={LONG} />);
    const input = screen.getByTestId("title") as HTMLInputElement;
    // Truncating change while the input is not focused: records then discards.
    simulateInsert(input, `${LONG.slice(0, 6)}XY${LONG.slice(6)}`, 8, setSelectionRange);
    // A later focused, non-truncating change must not restore the old caret.
    input.focus();
    simulateInsert(input, `${LONG.slice(0, 6)}XY${LONG.slice(6, 58)}`, 8, setSelectionRange);
    expect(setSelectionRange).not.toHaveBeenCalled();
  });
});
