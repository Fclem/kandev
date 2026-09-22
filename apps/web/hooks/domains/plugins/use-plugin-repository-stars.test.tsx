import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import type { GitHubStarConnectionStatus } from "@/lib/types/github";
import { usePluginRepositoryStar } from "./use-plugin-repository-stars";

const fetchStarConnection = vi.fn();
const fetchStarStates = vi.fn();
const startStarConnection = vi.fn();
const starRepository = vi.fn();
const unstarRepository = vi.fn();

vi.mock("@/lib/api/domains/github-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/github-api")>();
  return {
    ...actual,
    fetchGitHubStarConnection: (...args: unknown[]) => fetchStarConnection(...args),
    fetchGitHubStarStates: (...args: unknown[]) => fetchStarStates(...args),
    startGitHubStarConnection: (...args: unknown[]) => startStarConnection(...args),
    starGitHubRepository: (...args: unknown[]) => starRepository(...args),
    unstarGitHubRepository: (...args: unknown[]) => unstarRepository(...args),
  };
});

const WORKSPACE_ID = "ws-1";
const OTHER_WORKSPACE_ID = "ws-2";
const REPOSITORY_URL = "https://github.com/kdlbs/kandev";
const CANONICAL = "kdlbs/kandev";

function connection(
  overrides: Partial<GitHubStarConnectionStatus> = {},
): GitHubStarConnectionStatus {
  return {
    user_id: "user-a",
    credential_generation: 1,
    active: true,
    capable: true,
    reconnect_required: false,
    available: true,
    ...overrides,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

let store: StoreApi<AppState>;

function Harness({
  workspaceId = WORKSPACE_ID,
  repositoryUrl = REPOSITORY_URL,
}: {
  workspaceId?: string;
  repositoryUrl?: string;
}) {
  const star = usePluginRepositoryStar({ workspaceId, repositoryUrl });
  store = useAppStoreApi();
  return (
    <div>
      <span data-testid={`mode-${workspaceId}`}>{star.mode}</span>
      <span data-testid={`starred-${workspaceId}`}>{String(star.starred)}</span>
      <span data-testid={`busy-${workspaceId}`}>{String(star.busy)}</span>
      <button type="button" onClick={star.toggle}>
        {`toggle-${workspaceId}`}
      </button>
      <button type="button" onClick={star.connect}>
        {`connect-${workspaceId}`}
      </button>
    </div>
  );
}

function renderHarness(children: ReactNode = <Harness />) {
  return render(<StateProvider>{children}</StateProvider>);
}

/** One provider (one store) whose child can be unmounted and mounted again. */
function RemountHarness({ mounted }: { mounted: boolean }) {
  return <StateProvider>{mounted ? <Harness /> : null}</StateProvider>;
}

beforeEach(() => {
  fetchStarConnection.mockReset().mockResolvedValue(connection());
  fetchStarStates.mockReset().mockResolvedValue({ states: { [CANONICAL]: true } });
  startStarConnection.mockReset().mockResolvedValue({
    authorization_url: "https://github.com/login/oauth/authorize?state=s",
    state_expires_at: "2026-09-22T10:00:00Z",
  });
  starRepository.mockReset().mockResolvedValue({ starred: true });
  unstarRepository.mockReset().mockResolvedValue({ starred: false });
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("usePluginRepositoryStar hydration", () => {
  it("reads the connection status before asking for repository states", async () => {
    const pending = deferred<GitHubStarConnectionStatus>();
    fetchStarConnection.mockReturnValue(pending.promise);

    renderHarness();

    await waitFor(() =>
      expect(fetchStarConnection).toHaveBeenCalledWith(WORKSPACE_ID, { cache: "no-store" }),
    );
    expect(fetchStarStates).not.toHaveBeenCalled();
    expect(screen.getByTestId(`mode-${WORKSPACE_ID}`).textContent).toBe("omit");

    await act(async () => {
      pending.resolve(connection());
      await pending.promise;
    });

    await waitFor(() => expect(fetchStarStates).toHaveBeenCalledTimes(1));
    expect(fetchStarStates).toHaveBeenCalledWith({
      workspace_id: WORKSPACE_ID,
      repository_urls: [REPOSITORY_URL],
    });
    await waitFor(() =>
      expect(screen.getByTestId(`starred-${WORKSPACE_ID}`).textContent).toBe("true"),
    );
    expect(screen.getByTestId(`mode-${WORKSPACE_ID}`).textContent).toBe("stars");
  });

  it("omits the control and issues no request for an unsupported deployment", async () => {
    fetchStarConnection.mockResolvedValue(
      connection({ available: false, active: false, capable: false }),
    );

    renderHarness();

    await waitFor(() =>
      expect(screen.getByTestId(`mode-${WORKSPACE_ID}`).textContent).toBe("omit"),
    );
    expect(fetchStarStates).not.toHaveBeenCalled();

    act(() => screen.getByText(`toggle-${WORKSPACE_ID}`).click());
    act(() => screen.getByText(`connect-${WORKSPACE_ID}`).click());
    expect(starRepository).not.toHaveBeenCalled();
    expect(unstarRepository).not.toHaveBeenCalled();
    expect(startStarConnection).not.toHaveBeenCalled();
  });

  it("offers reconnect without asserting a star state", async () => {
    fetchStarConnection.mockResolvedValue(
      connection({ active: false, capable: false, reconnect_required: true }),
    );

    renderHarness();

    await waitFor(() =>
      expect(screen.getByTestId(`mode-${WORKSPACE_ID}`).textContent).toBe("reconnect"),
    );
    expect(fetchStarStates).not.toHaveBeenCalled();
    expect(screen.getByTestId(`starred-${WORKSPACE_ID}`).textContent).toBe("null");
  });

  it("reuses a confirmed state instead of re-requesting it on remount", async () => {
    const view = render(<RemountHarness mounted />);
    await waitFor(() =>
      expect(screen.getByTestId(`starred-${WORKSPACE_ID}`).textContent).toBe("true"),
    );

    view.rerender(<RemountHarness mounted={false} />);
    view.rerender(<RemountHarness mounted />);

    await waitFor(() =>
      expect(screen.getByTestId(`starred-${WORKSPACE_ID}`).textContent).toBe("true"),
    );
    expect(fetchStarStates).toHaveBeenCalledTimes(1);
  });
});

describe("usePluginRepositoryStar scoping", () => {
  it("requests a repository state once for sibling rows sharing the key", async () => {
    renderHarness(
      <>
        <Harness workspaceId={WORKSPACE_ID} />
        <Harness workspaceId={WORKSPACE_ID} />
      </>,
    );

    await waitFor(() => expect(fetchStarStates).toHaveBeenCalledTimes(1));
  });

  it("keeps two workspaces' states apart", async () => {
    fetchStarStates.mockImplementation(async (payload: { workspace_id: string }) => ({
      states: { [CANONICAL]: payload.workspace_id === OTHER_WORKSPACE_ID },
    }));

    renderHarness(
      <>
        <Harness workspaceId={WORKSPACE_ID} />
        <Harness workspaceId={OTHER_WORKSPACE_ID} />
      </>,
    );

    await waitFor(() =>
      expect(screen.getByTestId(`starred-${WORKSPACE_ID}`).textContent).toBe("false"),
    );
    await waitFor(() =>
      expect(screen.getByTestId(`starred-${OTHER_WORKSPACE_ID}`).textContent).toBe("true"),
    );
    expect(fetchStarConnection).toHaveBeenCalledWith(WORKSPACE_ID, { cache: "no-store" });
    expect(fetchStarConnection).toHaveBeenCalledWith(OTHER_WORKSPACE_ID, { cache: "no-store" });
  });

  it("never asserts a star state for a provider-unknown result", async () => {
    fetchStarStates.mockResolvedValue({ states: { [CANONICAL]: null } });

    renderHarness();

    await waitFor(() => expect(fetchStarStates).toHaveBeenCalledTimes(1));
    await waitFor(() =>
      expect(screen.getByTestId(`mode-${WORKSPACE_ID}`).textContent).toBe("stars"),
    );
    expect(screen.getByTestId(`starred-${WORKSPACE_ID}`).textContent).toBe("null");
  });

  it("treats an omitted status key as unknown", async () => {
    fetchStarStates.mockResolvedValue({ states: {} });

    renderHarness();

    await waitFor(() => expect(fetchStarStates).toHaveBeenCalledTimes(1));
    expect(screen.getByTestId(`starred-${WORKSPACE_ID}`).textContent).toBe("null");
  });

  it("leaves the state unknown when the status read fails", async () => {
    fetchStarStates.mockRejectedValue(new Error("provider down"));

    renderHarness();

    await waitFor(() => expect(fetchStarStates).toHaveBeenCalledTimes(1));
    await waitFor(() =>
      expect(screen.getByTestId(`mode-${WORKSPACE_ID}`).textContent).toBe("stars"),
    );
    expect(screen.getByTestId(`starred-${WORKSPACE_ID}`).textContent).toBe("null");
    expect(screen.getByTestId(`busy-${WORKSPACE_ID}`).textContent).toBe("false");
  });

  it("invalidates the previous scope when the connection tuple changes", async () => {
    renderHarness();
    await waitFor(() =>
      expect(screen.getByTestId(`starred-${WORKSPACE_ID}`).textContent).toBe("true"),
    );

    act(() => {
      const epoch = store.getState().beginPluginStarConnectionRequest(WORKSPACE_ID);
      store
        .getState()
        .applyPluginStarConnection(WORKSPACE_ID, epoch, connection({ user_id: "user-b" }));
    });

    expect(screen.getByTestId(`starred-${WORKSPACE_ID}`).textContent).toBe("null");
  });
});

describe("usePluginRepositoryStar mutations", () => {
  it("sends the opposite mutation and commits only the confirmed response", async () => {
    const pending = deferred<{ starred: boolean }>();
    unstarRepository.mockReturnValue(pending.promise);
    renderHarness();
    await waitFor(() =>
      expect(screen.getByTestId(`starred-${WORKSPACE_ID}`).textContent).toBe("true"),
    );

    act(() => screen.getByText(`toggle-${WORKSPACE_ID}`).click());

    expect(unstarRepository).toHaveBeenCalledWith({
      workspace_id: WORKSPACE_ID,
      repository_url: REPOSITORY_URL,
    });
    expect(screen.getByTestId(`busy-${WORKSPACE_ID}`).textContent).toBe("true");
    // No optimistic flip while GitHub has not confirmed.
    expect(screen.getByTestId(`starred-${WORKSPACE_ID}`).textContent).toBe("true");

    await act(async () => {
      pending.resolve({ starred: false });
      await pending.promise;
    });

    expect(screen.getByTestId(`starred-${WORKSPACE_ID}`).textContent).toBe("false");
    expect(screen.getByTestId(`busy-${WORKSPACE_ID}`).textContent).toBe("false");
  });

  it("keeps the previous state and clears busy when a mutation fails", async () => {
    unstarRepository.mockRejectedValue(new Error("insufficient scope"));
    renderHarness();
    await waitFor(() =>
      expect(screen.getByTestId(`starred-${WORKSPACE_ID}`).textContent).toBe("true"),
    );

    act(() => screen.getByText(`toggle-${WORKSPACE_ID}`).click());

    await waitFor(() =>
      expect(screen.getByTestId(`busy-${WORKSPACE_ID}`).textContent).toBe("false"),
    );
    expect(screen.getByTestId(`starred-${WORKSPACE_ID}`).textContent).toBe("true");
  });

  it("discards a late status response once a mutation has been started", async () => {
    const statusResponse = deferred<{ states: Record<string, boolean | null> }>();
    fetchStarStates.mockReturnValue(statusResponse.promise);
    const mutation = deferred<{ starred: boolean }>();
    unstarRepository.mockReturnValue(mutation.promise);

    renderHarness();
    await waitFor(() => expect(fetchStarStates).toHaveBeenCalledTimes(1));
    await waitFor(() =>
      expect(screen.getByTestId(`mode-${WORKSPACE_ID}`).textContent).toBe("stars"),
    );

    // The control cannot toggle while the state is unknown, so seed the
    // confirmed state the same way a sibling row's mutation would.
    act(() => {
      const scopeKey = Object.keys(store.getState().pluginStars.epochs)[0];
      const epoch = store.getState().beginPluginStarRequest(scopeKey, "mutation");
      if (epoch === null) {
        throw new Error("expected a mutation epoch for the seeded key");
      }
      store.getState().applyPluginStarResult(scopeKey, epoch, true);
    });
    await waitFor(() =>
      expect(screen.getByTestId(`starred-${WORKSPACE_ID}`).textContent).toBe("true"),
    );

    act(() => screen.getByText(`toggle-${WORKSPACE_ID}`).click());
    await act(async () => {
      statusResponse.resolve({ states: { [CANONICAL]: false } });
      await statusResponse.promise;
    });

    // The pre-mutation status read is fenced out; the mutation still owns the key.
    expect(screen.getByTestId(`starred-${WORKSPACE_ID}`).textContent).toBe("true");

    await act(async () => {
      mutation.resolve({ starred: false });
      await mutation.promise;
    });
    expect(screen.getByTestId(`starred-${WORKSPACE_ID}`).textContent).toBe("false");
  });
});

describe("usePluginRepositoryStar connect flow", () => {
  it("starts the dedicated flow with a Settings > Plugins return path", async () => {
    const assign = vi.fn();
    vi.stubGlobal("location", {
      ...window.location,
      pathname: "/settings/plugins",
      search: "?tab=browse",
      hash: "",
      assign,
    });
    fetchStarConnection.mockResolvedValue(connection({ active: false, capable: false }));

    renderHarness();
    await waitFor(() =>
      expect(screen.getByTestId(`mode-${WORKSPACE_ID}`).textContent).toBe("connect"),
    );

    act(() => screen.getByText(`connect-${WORKSPACE_ID}`).click());

    await waitFor(() =>
      expect(startStarConnection).toHaveBeenCalledWith({
        workspace_id: WORKSPACE_ID,
        return_path: "/settings/plugins?tab=browse",
      }),
    );
    await waitFor(() =>
      expect(assign).toHaveBeenCalledWith("https://github.com/login/oauth/authorize?state=s"),
    );
  });
});
