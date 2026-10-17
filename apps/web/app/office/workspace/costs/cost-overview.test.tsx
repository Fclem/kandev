import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import { CostOverview } from "./cost-overview";
import { t } from "@/lib/i18n";

// Hoisted mock so the getCostsBreakdown import is replaced before the
// component imports it. Tests configure the mock per-case.
const getCostsBreakdownMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/office-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-api")>(
    "@/lib/api/domains/office-api",
  );
  return {
    ...actual,
    getCostsBreakdown: getCostsBreakdownMock,
  };
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const EMPTY_BREAKDOWN = {
  total_subcents: 0,
  coverage_degraded: false,
  by_agent: [],
  by_project: [],
  by_model: [],
  by_provider: [],
};

// Renders as a StateProvider child purely to hand the real store instance
// back to the test, so it can bump office.refetchTriggers the same way the
// WS handler does and let real zustand reactivity re-render CostOverview.
function StoreCapture({ storeRef }: { storeRef: { current: StoreApi<AppState> | null } }) {
  storeRef.current = useAppStoreApi();
  return null;
}

describe("CostOverview", () => {
  it("discards a stale response when an older request resolves after a newer one", async () => {
    let resolveFirst: (value: typeof EMPTY_BREAKDOWN) => void;
    let resolveSecond: (value: typeof EMPTY_BREAKDOWN) => void;
    getCostsBreakdownMock
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveFirst = resolve;
          }),
      )
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveSecond = resolve;
          }),
      );
    const storeRef: { current: StoreApi<AppState> | null } = { current: null };

    render(
      <StateProvider initialState={{ workspaces: { activeId: "ws-1", items: [] } }}>
        <StoreCapture storeRef={storeRef} />
        <CostOverview workspaceId="ws-1" />
      </StateProvider>,
    );

    await waitFor(() => {
      expect(getCostsBreakdownMock).toHaveBeenCalledTimes(1);
    });

    act(() => {
      storeRef.current!.getState().setOfficeRefetchTrigger("costs");
    });

    await waitFor(() => {
      expect(getCostsBreakdownMock).toHaveBeenCalledTimes(2);
    });

    // Resolve the newer (second) request first with real data, then the
    // stale first request with different data. The stale response must not
    // overwrite the fresher state.
    await act(async () => {
      resolveSecond({ ...EMPTY_BREAKDOWN, total_subcents: 20000 });
    });
    await act(async () => {
      resolveFirst({ ...EMPTY_BREAKDOWN, total_subcents: 10000 });
    });

    // getByText throws if no element with this text exists.
    screen.getByText("$2.00");
  });
});

describe("CostOverview workspace isolation", () => {
  it("does not show the previous workspace after the next workspace fails to load", async () => {
    getCostsBreakdownMock
      .mockResolvedValueOnce({ ...EMPTY_BREAKDOWN, total_subcents: 10000 })
      .mockRejectedValueOnce(new Error("workspace unavailable"));

    const view = render(
      <StateProvider initialState={{ workspaces: { activeId: "ws-1", items: [] } }}>
        <CostOverview workspaceId="ws-1" />
      </StateProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText("$1.00")).toBeTruthy();
    });

    view.rerender(
      <StateProvider initialState={{ workspaces: { activeId: "ws-2", items: [] } }}>
        <CostOverview workspaceId="ws-2" />
      </StateProvider>,
    );

    await waitFor(() => {
      expect(getCostsBreakdownMock).toHaveBeenCalledTimes(2);
    });
    expect(screen.queryByText("$1.00")).toBeNull();
  });
});

describe("workspace monetary availability", () => {
  it.each([
    { name: "gap-only", amount: null, degraded: true },
    { name: "mixed recorded spend and gap", amount: 12500, degraded: true },
    { name: "measured zero", amount: 0, degraded: false },
  ])("preserves $name in the overview and grouped rows", async ({ amount, degraded }) => {
    const group = {
      group_key: "coverage-group",
      group_label: "Coverage group",
      total_subcents: amount,
      coverage_degraded: degraded,
      count: 1,
    };
    getCostsBreakdownMock.mockResolvedValue({
      total_subcents: amount,
      coverage_degraded: degraded,
      by_agent: [group],
      by_project: [group],
      by_model: [group],
      by_provider: [group],
    });
    render(
      <StateProvider initialState={{ workspaces: { activeId: "coverage-ws", items: [] } }}>
        <CostOverview workspaceId="coverage-ws" />
      </StateProvider>,
    );
    await waitFor(() => expect(screen.getAllByText("Coverage group")).toHaveLength(4));
    if (amount === null) {
      expect(screen.queryAllByText("$0.00")).toHaveLength(0);
      expect(screen.getAllByText("-")).toHaveLength(5);
    } else {
      expect(screen.getAllByText(amount === 0 ? "$0.00" : "$1.25")).toHaveLength(5);
    }
    if (degraded) {
      expect(screen.getAllByText(t("office:costCoverageIncomplete"))).toHaveLength(5);
    } else {
      expect(screen.queryByText(t("office:costCoverageIncomplete"))).toBeNull();
    }
  });
});
