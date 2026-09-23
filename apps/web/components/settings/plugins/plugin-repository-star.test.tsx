import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import type { GitHubStarConnectionStatus } from "@/lib/types/github";
import { PluginRepositoryStar } from "./plugin-repository-star";

const fetchStarConnection = vi.fn();
const fetchStarStates = vi.fn();
const starRepository = vi.fn();
const unstarRepository = vi.fn();

vi.mock("@/lib/api/domains/github-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/github-api")>();
  return {
    ...actual,
    fetchGitHubStarConnection: (...args: unknown[]) => fetchStarConnection(...args),
    fetchGitHubStarStates: (...args: unknown[]) => fetchStarStates(...args),
    starGitHubRepository: (...args: unknown[]) => starRepository(...args),
    unstarGitHubRepository: (...args: unknown[]) => unstarRepository(...args),
  };
});

const WORKSPACE_ID = "ws-1";
const REPOSITORY_URL = "https://github.com/kdlbs/kandev";
const CANONICAL = "kdlbs/kandev";
const TOGGLE_ID = "plugin-star-toggle";
const CTA_ID = "plugin-star-connect";
const PRESSED = "aria-pressed";

function connection(
  overrides: Partial<GitHubStarConnectionStatus> = {},
): GitHubStarConnectionStatus {
  return {
    // The backend resolves the authenticated identity to this id while
    // authentication is disabled, and the store's default auth state matches.
    user_id: "default-user",
    credential_generation: 1,
    active: true,
    capable: true,
    reconnect_required: false,
    available: true,
    ...overrides,
  };
}

async function renderControl(props: { repositoryUrl?: string | null } = {}) {
  render(
    <StateProvider>
      <PluginRepositoryStar
        workspaceId={WORKSPACE_ID}
        repositoryUrl={props.repositoryUrl ?? REPOSITORY_URL}
      />
    </StateProvider>,
  );
  // The connection read resolves before anything can be asserted.
  await waitFor(() =>
    expect(fetchStarConnection).toHaveBeenCalledWith(WORKSPACE_ID, { cache: "no-store" }),
  );
}

beforeEach(() => {
  fetchStarConnection.mockReset().mockResolvedValue(connection());
  fetchStarStates.mockReset().mockResolvedValue({ states: { [CANONICAL]: true } });
  starRepository.mockReset().mockResolvedValue({ starred: true });
  unstarRepository.mockReset().mockResolvedValue({ starred: false });
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("PluginRepositoryStar availability", () => {
  it("renders nothing at all for an unsupported deployment", async () => {
    fetchStarConnection.mockResolvedValue(
      connection({ available: false, active: false, capable: false }),
    );

    await renderControl();

    expect(screen.queryByTestId(TOGGLE_ID)).toBeNull();
    expect(screen.queryByTestId(CTA_ID)).toBeNull();
    expect(screen.queryByText("Connect GitHub")).toBeNull();
    expect(fetchStarStates).not.toHaveBeenCalled();
  });

  it("renders nothing for a repository that is not a public GitHub repository", async () => {
    // A catalog or manifest with a javascript: or non-GitHub URL gets no control.
    await renderControl({ repositoryUrl: "https://example.test/plugin" });

    expect(screen.queryByTestId(TOGGLE_ID)).toBeNull();
    expect(screen.queryByTestId(CTA_ID)).toBeNull();
    expect(fetchStarStates).not.toHaveBeenCalled();
  });

  it("offers Connect GitHub without asserting a star state", async () => {
    fetchStarConnection.mockResolvedValue(connection({ active: false, capable: false }));

    await renderControl();

    expect((await screen.findByTestId(CTA_ID)).textContent).toBe("Connect GitHub");
    expect(screen.queryByTestId(TOGGLE_ID)).toBeNull();
  });

  it("offers Reconnect GitHub when the grant is unusable", async () => {
    fetchStarConnection.mockResolvedValue(
      connection({ active: false, capable: false, reconnect_required: true }),
    );

    await renderControl();

    expect((await screen.findByTestId(CTA_ID)).textContent).toBe("Reconnect GitHub");
    expect(screen.queryByTestId(TOGGLE_ID)).toBeNull();
  });

  it("keeps star controls and adds the reconnect CTA for a capable replaceable grant", async () => {
    fetchStarConnection.mockResolvedValue(connection({ reconnect_required: true }));

    await renderControl();

    const toggle = await screen.findByTestId(TOGGLE_ID);
    await waitFor(() => expect(toggle.getAttribute(PRESSED)).toBe("true"));
    expect(screen.getByTestId(CTA_ID).textContent).toBe("Reconnect GitHub");
  });
});

describe("PluginRepositoryStar star state", () => {
  it("reflects a confirmed starred state through aria-pressed", async () => {
    await renderControl();

    const toggle = await screen.findByTestId(TOGGLE_ID);
    await waitFor(() => expect(toggle.getAttribute(PRESSED)).toBe("true"));
    expect(toggle.textContent).toBe("Unstar");
    expect(fetchStarStates).toHaveBeenCalledWith({
      workspace_id: WORKSPACE_ID,
      repository_urls: [REPOSITORY_URL],
    });
  });

  it("reflects a confirmed unstarred state through aria-pressed", async () => {
    fetchStarStates.mockResolvedValue({ states: { [CANONICAL]: false } });

    await renderControl();

    const toggle = await screen.findByTestId(TOGGLE_ID);
    await waitFor(() => expect(toggle.getAttribute(PRESSED)).toBe("false"));
    expect(toggle.textContent).toBe("Star");
  });

  it("does not assert unstarred when the provider result is unknown", async () => {
    fetchStarStates.mockResolvedValue({ states: { [CANONICAL]: null } });

    await renderControl();

    await waitFor(() => expect(fetchStarStates).toHaveBeenCalled());
    await waitFor(() => expect(document.querySelector(`[${PRESSED}]`)).toBeNull());
    expect(screen.queryByTestId(TOGGLE_ID)).toBeNull();
  });

  it("keeps a missed status read unknown rather than unstarred", async () => {
    fetchStarStates.mockRejectedValue(new Error("provider unavailable"));

    await renderControl();

    await waitFor(() => expect(fetchStarStates).toHaveBeenCalled());
    await waitFor(() => expect(screen.queryByTestId(TOGGLE_ID)).toBeNull());
    expect(document.querySelector(`[${PRESSED}]`)).toBeNull();
  });
});

describe("PluginRepositoryStar mutations", () => {
  it("announces the in-flight mutation without renaming the control", async () => {
    let confirmUnstar: (value: { starred: boolean }) => void = () => undefined;
    unstarRepository.mockReturnValue(
      new Promise<{ starred: boolean }>((resolve) => {
        confirmUnstar = resolve;
      }),
    );

    await renderControl();
    const toggle = await screen.findByTestId(TOGGLE_ID);
    await waitFor(() => expect(toggle.getAttribute(PRESSED)).toBe("true"));

    act(() => toggle.click());

    await waitFor(() => expect((toggle as HTMLButtonElement).disabled).toBe(true));
    expect(toggle.textContent).toBe("Unstar");
    expect(screen.getByRole("status").textContent).toBe("Updating star…");

    await act(async () => confirmUnstar({ starred: false }));

    await waitFor(() => expect(toggle.getAttribute(PRESSED)).toBe("false"));
    expect(screen.getByRole("status").textContent).toBe("");
  });

  it("keeps the previous state when the mutation fails", async () => {
    unstarRepository.mockRejectedValue(new Error("insufficient scope"));

    await renderControl();
    const toggle = await screen.findByTestId(TOGGLE_ID);
    await waitFor(() => expect(toggle.getAttribute(PRESSED)).toBe("true"));

    act(() => toggle.click());

    await waitFor(() => expect(toggle.getAttribute(PRESSED)).toBe("true"));
    expect((toggle as HTMLButtonElement).disabled).toBe(false);
  });
});
