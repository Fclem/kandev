import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import type { SystemErrorNotification } from "@/lib/state/slices/ui/types";

const ERROR_TEXT = "Database is locked";

let mockNotification: SystemErrorNotification | null = null;
const mockClearNotification = vi.fn();
const mockToast = vi.fn();

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      systemErrorNotification: mockNotification,
      setSystemErrorNotification: mockClearNotification,
    }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

import { useSystemErrorToast } from "./use-system-error-toast";

describe("useSystemErrorToast", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockNotification = null;
  });

  it("shows an error toast naming the code when one is present", () => {
    mockNotification = { message: ERROR_TEXT, code: "db_locked" };
    renderHook(() => useSystemErrorToast());

    expect(mockToast).toHaveBeenCalledWith({
      title: "System error (db_locked)",
      description: ERROR_TEXT,
      variant: "error",
    });
    expect(mockClearNotification).toHaveBeenCalledWith(null);
  });

  it("falls back to a bare title when there is no code", () => {
    mockNotification = { message: "The backend reported an error." };
    renderHook(() => useSystemErrorToast());

    expect(mockToast).toHaveBeenCalledWith({
      title: "System error",
      description: "The backend reported an error.",
      variant: "error",
    });
  });

  it("does not toast when there is no notification", () => {
    renderHook(() => useSystemErrorToast());

    expect(mockToast).not.toHaveBeenCalled();
    expect(mockClearNotification).not.toHaveBeenCalled();
  });

  it("surfaces every frame — repeated errors are not deduplicated", () => {
    mockNotification = { message: ERROR_TEXT };
    const { rerender } = renderHook(() => useSystemErrorToast());
    expect(mockToast).toHaveBeenCalledTimes(1);

    mockNotification = { message: ERROR_TEXT };
    rerender();

    expect(mockToast).toHaveBeenCalledTimes(2);
  });
});
