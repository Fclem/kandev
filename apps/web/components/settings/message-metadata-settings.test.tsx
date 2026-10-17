import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { SettingsSaveProvider } from "./settings-save-provider";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { UserSettingsResponse } from "@/lib/types/http";
import { registerUsersHandlers } from "@/lib/ws/handlers/users";
import { deferred } from "@/hooks/domains/session/session-read-test-helpers";
import Link from "@/components/routing/app-link";
import { clearNavigationBlockerForTests } from "@/lib/routing/navigation-guard";

const DATA_STATE_ATTRIBUTE = "data-state";
const CHECKED_STATE = "checked";
const UNCHECKED_STATE = "unchecked";
const SAVE_CHANGES_LABEL = "Save changes";
const DIRTY_ATTRIBUTE = "data-settings-dirty";
const updateUserSettings = vi.fn();
vi.mock("@/lib/api", () => ({
  updateUserSettings: (...args: unknown[]) => updateUserSettings(...args),
}));

import { MessageMetadataSettings } from "./message-metadata-settings";

function renderSettings() {
  let store!: StoreApi<AppState>;
  function ReadStoredSelection() {
    store = useAppStoreApi();
    const fields = useAppStore((state) => state.userSettings.messageMetadataFields);
    return <output data-testid="stored-selection">{fields.join(",")}</output>;
  }
  render(
    <StateProvider initialState={{ userSettings: { ...defaultState.userSettings } }}>
      <SettingsSaveProvider>
        <MessageMetadataSettings />
        <ReadStoredSelection />
        <Link href="/settings/general/terminal">Terminal</Link>
      </SettingsSaveProvider>
    </StateProvider>,
  );
  return store;
}

function savedSelectionResponse(revision: number): UserSettingsResponse {
  return {
    settings: {
      user_id: "default",
      workspace_id: "workspace" as UserSettingsResponse["settings"]["workspace_id"],
      repository_ids: [],
      message_metadata_fields: ["session_config", "timestamp", "duration", "ttft"],
      revision,
      updated_at: "2026-10-13T12:00:00Z",
    },
  };
}

beforeEach(() => {
  updateUserSettings.mockReset().mockResolvedValue(savedSelectionResponse(1));
});
afterEach(() => {
  cleanup();
  clearNavigationBlockerForTests();
});

describe("MessageMetadataSettings", () => {
  it("renders the saved selection and saves a changed selection", async () => {
    renderSettings();
    const timestamp = screen.getByRole("checkbox", { name: "Timestamp" });
    const ttft = screen.getByRole("checkbox", { name: "TTFT" });

    expect(timestamp.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(CHECKED_STATE);
    expect(ttft.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(UNCHECKED_STATE);
    fireEvent.click(ttft);
    expect(updateUserSettings).not.toHaveBeenCalled();
    expect(ttft.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(CHECKED_STATE);
    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));

    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith({
        message_metadata_fields: ["session_config", "timestamp", "duration", "ttft"],
      }),
    );
    await waitFor(() => expect(ttft.getAttribute(DIRTY_ATTRIBUTE)).toBe("false"));
  });

  it("keeps the stored selection unchanged and leaves a failed draft dirty", async () => {
    updateUserSettings.mockRejectedValueOnce(new Error("save failed"));
    renderSettings();
    const ttft = screen.getByRole("checkbox", { name: "TTFT" });
    fireEvent.click(ttft);
    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));

    await waitFor(() => expect(screen.getByText("Couldn't save")).toBeTruthy());
    expect(screen.getByTestId("stored-selection").textContent).toBe(
      "session_config,timestamp,duration",
    );
    expect(ttft.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(CHECKED_STATE);
    expect(ttft.getAttribute(DIRTY_ATTRIBUTE)).toBe("true");
  });
});

describe("MessageMetadataSettings delayed save", () => {
  it.each([
    { responseRevision: 1, expectedRevision: 2, expectedFields: "tps", dirty: "true" },
    {
      responseRevision: 3,
      expectedRevision: 3,
      expectedFields: "session_config,timestamp,duration,ttft",
      dirty: "false",
    },
  ])(
    "applies response revision $responseRevision after a newer WS update",
    async ({ responseRevision, expectedRevision, expectedFields, dirty }) => {
      const pending = deferred<UserSettingsResponse>();
      updateUserSettings.mockReturnValueOnce(pending.promise);
      const store = renderSettings();
      const ttft = screen.getByRole("checkbox", { name: "TTFT" });
      fireEvent.click(ttft);
      fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));
      const handler = registerUsersHandlers(store)["user.settings.updated"]!;
      const newerEvent = {
        type: "notification" as const,
        action: "user.settings.updated" as const,
        payload: {
          user_id: "default",
          workspace_id: "workspace",
          repository_ids: [],
          revision: 2,
          message_metadata_fields: ["tps"],
        },
      };
      await act(async () => handler(newerEvent));
      expect(store.getState().userSettings.messageMetadataFields).toEqual(["tps"]);
      await act(async () => pending.resolve(savedSelectionResponse(responseRevision)));
      await act(async () => handler(newerEvent));
      expect(store.getState().userSettings.revision).toBe(expectedRevision);
      expect(screen.getByTestId("stored-selection").textContent).toBe(expectedFields);
      expect(ttft.getAttribute(DIRTY_ATTRIBUTE)).toBe(dirty);
    },
  );
});

describe("MessageMetadataSettings pending navigation", () => {
  it.each([
    { responseRevision: 1, canLeave: false },
    { responseRevision: 3, canLeave: true },
  ])(
    "admits navigation only after an accepted response (revision $responseRevision)",
    async ({ responseRevision, canLeave }) => {
      const appearancePath = "/settings/general/appearance";
      const terminalPath = "/settings/general/terminal";
      window.history.replaceState({}, "", appearancePath);
      const pending = deferred<UserSettingsResponse>();
      updateUserSettings.mockReturnValueOnce(pending.promise);
      const store = renderSettings();
      fireEvent.click(screen.getByRole("checkbox", { name: "TTFT" }));
      fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));
      await act(async () =>
        registerUsersHandlers(store)["user.settings.updated"]!({
          type: "notification",
          action: "user.settings.updated",
          payload: {
            user_id: "default",
            workspace_id: "workspace",
            repository_ids: [],
            revision: 2,
            message_metadata_fields: ["tps"],
          },
        }),
      );
      fireEvent.click(screen.getByRole("link", { name: "Terminal" }));
      expect(window.location.pathname).toBe(appearancePath);
      await act(async () => pending.resolve(savedSelectionResponse(responseRevision)));
      expect(window.location.pathname).toBe(canLeave ? terminalPath : appearancePath);
      const ttft = screen.getByRole("checkbox", { name: "TTFT", hidden: true });
      expect(ttft.getAttribute(DIRTY_ATTRIBUTE)).toBe(canLeave ? "false" : "true");
      if (!canLeave) {
        expect(store.getState().userSettings.messageMetadataFields).toEqual(["tps"]);
        expect(screen.getByRole("button", { name: "Save and leave" })).toBeTruthy();
      }
    },
  );
});
