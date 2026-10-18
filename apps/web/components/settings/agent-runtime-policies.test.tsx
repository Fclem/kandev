import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentUpdateStatus } from "@/lib/api";
import { AgentRuntimePolicies } from "./agent-runtime-policies";
import { SettingsTargetProvider } from "./settings-target-provider";

const disclosureSelector = "#runtime-updates details";

const { status, snapshot, session } = vi.hoisted(() => ({
  status: {
    agent_name: "opencode-acp",
    display_name: "OpenCode",
    runtime_id: "native:opencode",
    management: "manual",
    managed_fallback: true,
    available: true,
    enabled: true,
    guidance_url: "https://opencode.ai/docs/cli/",
  } as AgentUpdateStatus,
  snapshot: { statuses: {} as Record<string, AgentUpdateStatus> },
  session: { canManage: true },
}));
vi.mock("@/hooks/domains/auth/use-is-admin", () => ({ useIsAdmin: () => session.canManage }));
vi.mock("@/hooks/domains/settings/use-agent-runtime-update-statuses", () => ({
  useAgentRuntimeUpdateStatuses: () => ({ statusByAgent: snapshot.statuses }),
}));
vi.mock("./use-runtime-auto-update-policy", () => ({
  useRuntimeAutoUpdatePolicy: (value: AgentUpdateStatus) => ({
    draft: value.auto_update ?? false,
    isDirty: false,
    setDraft: vi.fn(),
  }),
}));
beforeEach(() => {
  session.canManage = true;
  snapshot.statuses = { "opencode-acp": status };
});
afterEach(() => {
  cleanup();
  window.history.replaceState({}, "", "/");
});

function policies() {
  return (
    <SettingsTargetProvider>
      <AgentRuntimePolicies hasRuntimeControl={() => true} />
    </SettingsTargetProvider>
  );
}

function expand(view: ReturnType<typeof render>) {
  fireEvent.click(view.container.querySelector("#runtime-updates summary")!);
}

describe("runtime settings disclosure", () => {
  // @covers AC-AGENTS-RUNTIME-NOTIFY-001.5
  it("starts collapsed on ordinary entry with mounted runtime content", () => {
    const view = render(policies());
    expect(view.container.querySelector<HTMLDetailsElement>(disclosureSelector)?.open).toBe(false);
    expect(screen.getByTestId("runtime-policy-opencode-acp")).toBeTruthy();
  });

  // @covers AC-AGENTS-RUNTIME-NOTIFY-001.6
  it.each(["runtime-updates", "runtime-update-opencode-acp"])(
    "reveals an initial %s destination",
    async (target) => {
      window.history.replaceState({}, "", `/settings/agents#${target}`);
      const view = render(policies());
      await waitFor(() =>
        expect(view.container.querySelector<HTMLDetailsElement>(disclosureSelector)?.open).toBe(
          true,
        ),
      );
    },
  );

  it("reveals a same-page fragment after manual collapse", async () => {
    window.history.replaceState({}, "", "/settings/agents#runtime-updates");
    const view = render(policies());
    const details = view.container.querySelector<HTMLDetailsElement>(disclosureSelector)!;
    await waitFor(() => expect(details?.open).toBe(true));
    expand(view);
    await waitFor(() => expect(details.open).toBe(false));
    act(() => {
      window.history.replaceState({}, "", "/settings/agents#runtime-update-opencode-acp");
      window.dispatchEvent(new Event("hashchange"));
    });
    await waitFor(() => expect(details.open).toBe(true));
  });

  it("opens both disclosures when an inactive target arrives after navigation", async () => {
    snapshot.statuses = {};
    window.history.replaceState({}, "", "/settings/agents#runtime-update-opencode-acp");
    const view = render(policies());
    snapshot.statuses = { "opencode-acp": { ...status, available: false } };
    view.rerender(policies());
    await waitFor(() => {
      const disclosures = view.container.querySelectorAll<HTMLDetailsElement>(disclosureSelector);
      expect(disclosures).toHaveLength(2);
      expect([...disclosures].every((details) => details.open)).toBe(true);
    });
  });
});

describe("runtime control destinations", () => {
  it("retains vendor guidance when the separate card snapshot has no runtime control", () => {
    const view = render(<AgentRuntimePolicies hasRuntimeControl={() => false} />);
    expand(view);
    expect(screen.queryByRole("link", { name: "Manage fallback versions" })).toBeNull();
    expect(screen.getByRole("link", { name: "Manual update guidance" }).getAttribute("href")).toBe(
      "https://opencode.ai/docs/cli/",
    );
  });

  it("links to the installed runtime control after its snapshot becomes available", () => {
    const view = render(
      <AgentRuntimePolicies hasRuntimeControl={(name) => name === "opencode-acp"} />,
    );
    expand(view);
    expect(
      screen.getByRole("link", { name: "Manage fallback versions" }).getAttribute("href"),
    ).toBe("#installed-agent-opencode-acp");
  });
});

function ompStatus(overrides: Partial<AgentUpdateStatus> = {}): AgentUpdateStatus {
  return {
    ...status,
    agent_name: "omp-acp",
    display_name: "Oh My Pi",
    runtime_id: "omp-acp",
    update_mode: "self_update",
    managed_fallback: false,
    owner: "external",
    current_version: "1.2.0",
    effective_version: "1.2.0",
    latest_version: "1.1.0",
    auto_update_supported: true,
    auto_update: false,
    guidance_url: "https://github.com/can1357/oh-my-pi",
    ...overrides,
  };
}

describe("OMP automatic consent and retained evidence", () => {
  // @covers AC-AGENTS-RUNTIME-NOTIFY-004.1, AC-AGENTS-RUNTIME-NOTIFY-004.11
  it("exposes opt-in with its own cadence and risk description and both update destinations", () => {
    snapshot.statuses = { "omp-acp": ompStatus() };
    const view = render(policies());
    expand(view);
    const row = screen.getByTestId("runtime-policy-omp-acp");
    const control = within(row).getByRole("switch");
    expect(control.getAttribute("aria-checked")).toBe("false");
    const descriptionId = control.getAttribute("aria-describedby");
    expect(descriptionId).not.toBe("runtime-automatic-help");
    const description = document.getElementById(descriptionId!);
    expect(row.contains(description)).toBe(true);
    expect(description?.textContent).toMatch(/15/);
    expect(description?.textContent).toMatch(/channel/i);
    expect(description?.textContent).toMatch(/roll.?back/i);
    expect(row.querySelector('a[href="#installed-agent-omp-acp"]')).toBeTruthy();
    expect(row.querySelector('a[href="https://github.com/can1357/oh-my-pi"]')).toBeTruthy();
  });

  it("keeps saved unavailable consent withdrawable and respects operator permissions", () => {
    snapshot.statuses = {
      "omp-acp": ompStatus({ available: false, auto_update_supported: false, auto_update: true }),
    };
    const view = render(policies());
    expand(view);
    const control = screen
      .getByTestId("runtime-policy-omp-acp")
      .querySelector<HTMLButtonElement>('[role="switch"]');
    expect(control?.getAttribute("aria-checked")).toBe("true");
    expect(control?.disabled).toBe(false);
    session.canManage = false;
    view.rerender(policies());
    expect(control?.disabled).toBe(true);
  });

  it("does not grant unrelated external runtimes an automatic switch", () => {
    snapshot.statuses = {
      "opencode-acp": { ...status, auto_update_supported: false, auto_update: false },
    };
    const view = render(policies());
    expand(view);
    expect(
      screen.getByTestId("runtime-policy-opencode-acp").querySelector('[role="switch"]'),
    ).toBeNull();
  });
});

describe("OMP retained evidence and pause recovery", () => {
  it("retains the last change separately from a neutral no-change check and collapsed plain-text output", () => {
    snapshot.statuses = {
      "omp-acp": ompStatus({
        last_change: {
          id: "changed",
          previous_version: "1.0.0",
          observed_version: "1.2.0",
          finished_at: "2026-10-10T12:00:00Z",
        },
        last_outcome: {
          id: "same",
          status: "succeeded",
          update_mode: "self_update",
          operation: "up_to_date",
          previous_version: "1.2.0",
          target_version: "",
          observed_version: "1.2.0",
          output_excerpt: "channel declined\n<script>not markup</script>",
          finished_at: "2026-10-10T12:15:00Z",
        },
      }),
    };
    const view = render(policies());
    expand(view);
    const row = screen.getByTestId("runtime-policy-omp-acp");
    expect(
      within(row).getByText(
        (_, element) =>
          element?.tagName === "P" &&
          Boolean(element.textContent?.includes("1.0.0") && element.textContent?.includes("1.2.0")),
      ),
    ).toBeTruthy();
    const result = within(row).getByRole("status");
    expect(result.textContent).toContain("1.2.0");
    expect(result.textContent).not.toMatch(/up.?to.?date|updated successfully/i);
    const disclosure = row.querySelector<HTMLDetailsElement>("details");
    expect(disclosure?.open).toBe(false);
    expect(disclosure?.querySelector("pre")?.textContent).toBe(
      "channel declined\n<script>not markup</script>",
    );
    expect(disclosure?.querySelector("script")).toBeNull();
  });

  it("uses derived pause state and removes recovery guidance after saved recovery", () => {
    const retained = ompStatus({
      auto_update: true,
      auto_update_paused: true,
      last_outcome: {
        id: "failed",
        status: "failed",
        update_mode: "self_update",
        previous_version: "1.2.0",
        target_version: "",
        output_excerpt: "probe failed",
        finished_at: "2026-10-10T12:00:00Z",
      },
    });
    snapshot.statuses = { "omp-acp": retained };
    const view = render(policies());
    expand(view);
    const paused = screen.getByTestId("runtime-omp-paused-omp-acp");
    expect(paused.textContent).toMatch(/paused/i);
    expect(paused.textContent).toMatch(/off.*save.*on.*save/i);
    // The latest result stays visible next to the pause explanation.
    const statuses = screen.getAllByRole("status");
    expect(statuses).toHaveLength(2);
    expect(statuses[0].textContent).not.toMatch(/off.*save.*on.*save/i);
    expect(statuses[0].textContent).toMatch(/did not complete/i);
    expect(statuses[0].textContent).not.toMatch(/retry/i);
    snapshot.statuses = { "omp-acp": { ...retained, auto_update_paused: false } };
    view.rerender(policies());
    expect(screen.queryByTestId("runtime-omp-paused-omp-acp")).toBeNull();
    expect(screen.getByRole("status").textContent).not.toMatch(/paused|off.*save.*on.*save/i);
  });
});
