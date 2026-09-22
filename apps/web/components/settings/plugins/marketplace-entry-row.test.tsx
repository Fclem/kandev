import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { MarketplaceEntryRow } from "./marketplace-entry-row";
import type { MarketplaceEntry } from "@/lib/types/plugins";

const STAR_ROW_CANONICAL = "kdlbs/kandev-plugin-acme";

const fetchStarConnection = vi.fn();
const fetchStarStates = vi.fn();

vi.mock("@/lib/api/domains/github-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/github-api")>();
  return {
    ...actual,
    fetchGitHubStarConnection: (...args: unknown[]) => fetchStarConnection(...args),
    fetchGitHubStarStates: (...args: unknown[]) => fetchStarStates(...args),
  };
});

afterEach(() => {
  cleanup();
  fetchStarConnection.mockReset();
  fetchStarStates.mockReset();
});

function entry(overrides: Partial<MarketplaceEntry> = {}): MarketplaceEntry {
  return {
    id: "acme",
    name: "Acme",
    description: "desc",
    author: "acme",
    categories: [],
    icon_url: "",
    repo_url: "https://github.com/kdlbs/kandev-plugin-acme",
    version: "1.0.0",
    min_kandev_version: "",
    package_url: "https://ex/acme-1.0.0.tar.gz",
    package_sha256: "",
    stars: 3,
    updated_at: "",
    install_state: "available",
    source_id: "official",
    source_name: "Kandev Official",
    ...overrides,
  };
}

const noop = () => undefined;

describe("MarketplaceEntryRow repo link", () => {
  it("renders an http(s) repo link", () => {
    render(<MarketplaceEntryRow entry={entry()} busy={false} onInstall={noop} />);
    const link = screen.getByText("Repo").closest("a");
    expect(link?.getAttribute("href")).toBe("https://github.com/kdlbs/kandev-plugin-acme");
  });

  it("does NOT render a repo link for a javascript: (or other non-http) scheme", () => {
    // A malicious/compromised source could set repo_url to a javascript: URL.
    render(
      <MarketplaceEntryRow
        entry={entry({ repo_url: "javascript:alert(document.cookie)" })}
        busy={false}
        onInstall={noop}
      />,
    );
    expect(screen.queryByText("Repo")).toBeNull();
  });

  it("renders unknown (null) stars as a dash, not zero", () => {
    render(<MarketplaceEntryRow entry={entry({ stars: null })} busy={false} onInstall={noop} />);
    expect(screen.getByText("-")).toBeTruthy();
  });
});

describe("MarketplaceEntryRow attribution", () => {
  it("renders the curated catalog author with its canonical repository", () => {
    render(
      <MarketplaceEntryRow
        entry={entry({
          author: "acme",
          repo_url: "https://github.com/acme/kandev-plugin-acme",
        })}
        busy={false}
        onInstall={noop}
      />,
    );

    expect(screen.getByText("by acme")).toBeTruthy();
    const link = screen.getByText("Repo").closest("a");
    expect(link?.getAttribute("href")).toBe("https://github.com/acme/kandev-plugin-acme");
  });
});

describe("MarketplaceEntryRow repository star", () => {
  it("keeps the star toggle beside the repo link instead of inside it", async () => {
    fetchStarConnection.mockResolvedValue({
      user_id: "user-a",
      credential_generation: 1,
      active: true,
      capable: true,
      reconnect_required: false,
      available: true,
    });
    fetchStarStates.mockResolvedValue({ states: { [STAR_ROW_CANONICAL]: true } });

    render(
      <StateProvider>
        <MarketplaceEntryRow entry={entry()} busy={false} onInstall={noop} workspaceId="ws-1" />
      </StateProvider>,
    );

    const toggle = await screen.findByTestId("plugin-star-toggle");
    await waitFor(() => expect(toggle.getAttribute("aria-pressed")).toBe("true"));
    // The card's overlay/repo navigation must not own the star click.
    expect(toggle.closest("a")).toBeNull();
    expect(screen.getByTestId("plugin-repo-link").getAttribute("href")).toBe(
      "https://github.com/kdlbs/kandev-plugin-acme",
    );
  });

  it("renders no star control for a non-GitHub repository URL", async () => {
    fetchStarConnection.mockResolvedValue({
      user_id: "user-a",
      credential_generation: 1,
      active: true,
      capable: true,
      reconnect_required: false,
      available: true,
    });

    render(
      <StateProvider>
        <MarketplaceEntryRow
          entry={entry({ repo_url: "https://example.test/acme" })}
          busy={false}
          onInstall={noop}
          workspaceId="ws-1"
        />
      </StateProvider>,
    );

    await waitFor(() => expect(fetchStarConnection).toHaveBeenCalled());
    expect(screen.queryByTestId("plugin-star-toggle")).toBeNull();
    expect(fetchStarStates).not.toHaveBeenCalled();
  });
});

describe("MarketplaceEntryRow member view", () => {
  it("keeps catalog metadata visible without install or update actions", () => {
    const { rerender } = render(
      <MarketplaceEntryRow entry={entry()} busy={false} onInstall={noop} canManage={false} />,
    );
    expect(screen.getByText("Acme")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Install" })).toBeNull();

    rerender(
      <MarketplaceEntryRow
        entry={entry({ install_state: "update_available" })}
        busy={false}
        onInstall={noop}
        canManage={false}
      />,
    );
    expect(screen.queryByRole("button", { name: "Update" })).toBeNull();
  });
});
