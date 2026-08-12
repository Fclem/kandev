import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { SecretListItem } from "@/lib/types/http-secrets";
import {
  buildDefaultTargetName,
  CopyMoveSecretDialog,
  truncateUtf8Bytes,
} from "./copy-move-secret-dialog";

const { mockCopySecret, mockMoveSecret, mockUseDestinationNames, storeState } = vi.hoisted(() => {
  const storeState = {
    workspaces: {
      items: [
        { id: "ws-1", name: "Alpha" },
        { id: "ws-2", name: "Beta" },
      ],
      activeId: null,
    },
    setWorkspaces: vi.fn(),
  };
  return {
    mockCopySecret: vi.fn(),
    mockMoveSecret: vi.fn(),
    mockUseDestinationNames: vi.fn(() => ({ names: [], loaded: true, conflict: () => false })),
    storeState,
  };
});

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof storeState) => unknown) => selector(storeState),
}));
vi.mock("@/hooks/domains/settings/use-workspace-destinations", () => ({
  useWorkspaceDestinations: () => ({ loading: false, error: null, retry: vi.fn() }),
}));
vi.mock("@/hooks/domains/settings/use-secret-destination-names", () => ({
  useSecretDestinationNames: (scope: string, workspaceId?: string) =>
    mockUseDestinationNames(scope, workspaceId),
}));
vi.mock("@/lib/api/domains/secrets-api", () => ({
  copySecret: (...args: unknown[]) => mockCopySecret(...args),
  moveSecret: (...args: unknown[]) => mockMoveSecret(...args),
}));

const DEFAULT_GLOBAL_NAME = "API Key (from general)";

const globalSecret: SecretListItem = {
  id: "secret-1",
  name: "API Key",
  scope: "global",
  has_value: true,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const workspaceSecret: SecretListItem = {
  ...globalSecret,
  id: "secret-2",
  name: "WS Key",
  scope: "workspace",
  workspace_id: "ws-1",
};

function renderDialog(overrides: Partial<Parameters<typeof CopyMoveSecretDialog>[0]> = {}) {
  const onCompleted = vi.fn();
  const onClose = vi.fn();
  const onStaleSource = vi.fn();
  const view = render(
    <CopyMoveSecretDialog
      secret={globalSecret}
      originToken="general"
      onClose={onClose}
      onCompleted={onCompleted}
      onStaleSource={onStaleSource}
      {...overrides}
    />,
  );
  return { onCompleted, onClose, onStaleSource, view };
}

afterEach(async () => {
  cleanup();
  mockCopySecret.mockReset();
  mockMoveSecret.mockReset();
  mockUseDestinationNames.mockReset();
  mockUseDestinationNames.mockReturnValue({ names: [], loaded: true, conflict: () => false });
});

describe("CopyMoveSecretDialog", () => {
  it("pre-fills the name with the origin suffix and defaults to Copy", () => {
    renderDialog();
    expect(screen.getByRole("heading", { name: `Copy or move ${globalSecret.name}` })).toBeTruthy();
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe(DEFAULT_GLOBAL_NAME);
    expect(screen.getByRole("radio", { name: /^Copy/ }).getAttribute("aria-checked")).toBe("true");
    expect(screen.getByRole("button", { name: "Copy" })).toBeTruthy();
  });

  it("shows General as the default destination for a workspace source", () => {
    renderDialog({ secret: workspaceSecret, originToken: "Alpha" });
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("WS Key (from Alpha)");
    expect(screen.getByText("General")).toBeTruthy();
  });

  it("switches the submit label to Move and shows the warning", () => {
    renderDialog();
    fireEvent.click(screen.getByRole("radio", { name: /^Move/ }));
    expect(screen.getByRole("button", { name: "Move" })).toBeTruthy();
    expect(screen.getByText("The original secret will be removed from general.")).toBeTruthy();
  });

  it("blocks submit on a conflicting target name with an invalid field", () => {
    mockUseDestinationNames.mockReturnValue({
      names: ["Taken"],
      loaded: true,
      conflict: (name: string) => name.trim() === "Taken",
    });
    renderDialog();
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Taken" } });
    const input = screen.getByLabelText("Name") as HTMLInputElement;
    expect(input.getAttribute("aria-invalid")).toBe("true");
    expect(
      screen.getByText("A secret named Taken already exists in this destination."),
    ).toBeTruthy();
    expect((screen.getByRole("button", { name: "Copy" }) as HTMLButtonElement).disabled).toBe(true);
  });
});

describe("CopyMoveSecretDialog submit", () => {
  it("submits a copy with the destination workspace payload and calls onCompleted", async () => {
    mockCopySecret.mockResolvedValue({ id: "new-1", name: DEFAULT_GLOBAL_NAME });
    const { onCompleted } = renderDialog();
    fireEvent.click(screen.getByRole("button", { name: "Copy" }));

    await waitFor(() => expect(mockCopySecret).toHaveBeenCalledTimes(1));
    expect(mockCopySecret).toHaveBeenCalledWith(
      "secret-1",
      {
        target_scope: "workspace",
        target_workspace_id: "ws-1",
        name: DEFAULT_GLOBAL_NAME,
      },
      undefined,
    );
    await waitFor(() => expect(onCompleted).toHaveBeenCalled());
  });

  it("clears the workspace id when the destination switches to General", async () => {
    mockMoveSecret.mockResolvedValue({ id: "new-1", name: "moved" });
    renderDialog({ secret: workspaceSecret, originToken: "Alpha" });

    fireEvent.click(screen.getByRole("radio", { name: /^Move/ }));
    fireEvent.click(screen.getByRole("button", { name: "Move" }));

    await waitFor(() => expect(mockMoveSecret).toHaveBeenCalledTimes(1));
    const payload = mockMoveSecret.mock.calls[0][1] as Record<string, unknown>;
    expect(payload).toEqual({ target_scope: "global", name: "WS Key (from Alpha)" });
    expect(JSON.stringify(payload)).not.toContain('"name":null');
    expect(JSON.stringify(payload)).not.toContain("target_workspace_id");
    // A workspace-scoped source must carry its workspace id in the query.
    expect(mockMoveSecret.mock.calls[0][2]).toEqual({ workspaceId: "ws-1" });
  });

  it("surfaces a 409 conflict on the name field", async () => {
    mockCopySecret.mockRejectedValue(new ApiError("conflict", 409, {}));
    const { onCompleted } = renderDialog();
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: DEFAULT_GLOBAL_NAME } });
    fireEvent.click(screen.getByRole("button", { name: "Copy" }));

    await waitFor(() =>
      expect(
        screen.getByText(
          "A secret named API Key (from general) already exists in this destination.",
        ),
      ).toBeTruthy(),
    );
    expect(onCompleted).not.toHaveBeenCalled();
  });

  it("reports a stale source on 404 without adding a destination item", async () => {
    mockMoveSecret.mockRejectedValue(new ApiError("not found", 404, {}));
    const { onCompleted, onStaleSource } = renderDialog({
      secret: workspaceSecret,
      originToken: "Alpha",
    });
    fireEvent.click(screen.getByRole("radio", { name: /^Move/ }));
    fireEvent.click(screen.getByRole("button", { name: "Move" }));

    await waitFor(() => expect(onStaleSource).toHaveBeenCalledTimes(1));
    expect(onCompleted).not.toHaveBeenCalled();
  });

  it("ignores duplicate clicks while a transfer is in flight", async () => {
    const { promise, resolve } = Promise.withResolvers<unknown>();
    mockCopySecret.mockReturnValue(promise);
    renderDialog();
    const submit = screen.getByRole("button", { name: "Copy" });
    fireEvent.click(submit);
    fireEvent.click(submit);

    expect(mockCopySecret).toHaveBeenCalledTimes(1);
    await act(async () => {
      resolve({ id: "new-1", name: DEFAULT_GLOBAL_NAME });
    });
  });
});

describe("truncateUtf8Bytes", () => {
  it("keeps ASCII under the limit unchanged", () => {
    expect(truncateUtf8Bytes("short", 100)).toBe("short");
  });

  it("truncates multibyte input to the byte limit", () => {
    const long = "é".repeat(60); // 120 bytes
    const truncated = truncateUtf8Bytes(long, 100);
    expect(new TextEncoder().encode(truncated).length).toBe(100);
    expect(truncated).toBe("é".repeat(50));
  });

  it("never splits astral-plane surrogate pairs", () => {
    const value = "🔐".repeat(40); // 160 bytes
    const truncated = truncateUtf8Bytes(value, 100);
    expect(truncated).toBe("🔐".repeat(25));
    expect(new TextEncoder().encode(truncated).length).toBe(100);
    expect(truncated.includes("\uFFFD")).toBe(false);
  });
});

describe("buildDefaultTargetName", () => {
  it("builds the locale-independent origin suffix within the byte limit", () => {
    const name = buildDefaultTargetName("API Key", "general");
    expect(name).toBe(DEFAULT_GLOBAL_NAME);
    expect(new TextEncoder().encode(name).length).toBeLessThanOrEqual(100);
  });

  it("truncates an over-long source name so the copy can never fail validation", () => {
    const name = buildDefaultTargetName("K".repeat(120), "general");
    expect(new TextEncoder().encode(name).length).toBeLessThanOrEqual(100);
  });
});
