import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider, useAppStore } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { SettingsSaveProvider } from "./settings-save-provider";

const DATA_STATE_ATTRIBUTE = "data-state";
const CHECKED_STATE = "checked";
const UNCHECKED_STATE = "unchecked";
const updateUserSettings = vi.fn();
vi.mock("@/lib/api", () => ({
  updateUserSettings: (...args: unknown[]) => updateUserSettings(...args),
}));

import { MessageMetadataSettings } from "./message-metadata-settings";

function renderSettings() {
  return render(
    <StateProvider initialState={{ userSettings: { ...defaultState.userSettings } }}>
      <SettingsSaveProvider>
        <MessageMetadataSettings />
      </SettingsSaveProvider>
    </StateProvider>,
  );
}

beforeEach(() => {
  updateUserSettings.mockReset().mockResolvedValue({ settings: {} });
});
afterEach(cleanup);

describe("MessageMetadataSettings", () => {
  it("renders the saved selection and saves a changed selection", async () => {
    renderSettings();
    const timestamp = screen.getByRole("checkbox", { name: "Timestamp" });
    const ttft = screen.getByRole("checkbox", { name: "TTFT" });

    expect(timestamp.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(CHECKED_STATE);
    const options = screen.getByTestId("message-metadata-settings").querySelector(".grid-cols-1");
    expect(options?.className).toContain("grid-cols-1");
    for (const checkbox of screen.getAllByRole("checkbox")) {
      expect(checkbox.parentElement?.className).toContain("min-h-11");
      expect(checkbox.parentElement?.className).toContain("min-w-0");
    }
    expect(ttft.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(UNCHECKED_STATE);
    fireEvent.click(ttft);
    expect(updateUserSettings).not.toHaveBeenCalled();
    expect(ttft.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(CHECKED_STATE);
    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith({
        message_metadata_fields: ["session_config", "timestamp", "duration", "ttft"],
      }),
    );
    await waitFor(() => expect(ttft.getAttribute("data-settings-dirty")).toBe("false"));
  });

  it("keeps the stored selection unchanged and leaves a failed draft dirty", async () => {
    updateUserSettings.mockRejectedValueOnce(new Error("save failed"));
    function ReadStoredSelection() {
      const fields = useAppStore((state) => state.userSettings.messageMetadataFields);
      return <output data-testid="stored-selection">{fields.join(",")}</output>;
    }

    render(
      <StateProvider initialState={{ userSettings: { ...defaultState.userSettings } }}>
        <SettingsSaveProvider>
          <MessageMetadataSettings />
          <ReadStoredSelection />
        </SettingsSaveProvider>
      </StateProvider>,
    );
    const ttft = screen.getByRole("checkbox", { name: "TTFT" });
    fireEvent.click(ttft);
    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(screen.getByText("Couldn't save")).toBeTruthy());
    expect(screen.getByTestId("stored-selection").textContent).toBe(
      "session_config,timestamp,duration",
    );
    expect(ttft.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(CHECKED_STATE);
    expect(ttft.getAttribute("data-settings-dirty")).toBe("true");
  });
});
