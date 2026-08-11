import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import type { UserSettingsState } from "@/lib/state/slices/settings/types";
import { SettingsSaveProvider } from "./settings-save-provider";

const updateUserSettings = vi.fn();
const TODO_LIST_PANEL_LABEL = "Show agent todo list panel";
const ONLY_PIN_WHEN_NOT_EMPTY_LABEL = "Only pin when todo list is not empty";
const DATA_STATE_ATTR = "data-state";
const DATA_SETTINGS_DIRTY_ATTR = "data-settings-dirty";

vi.mock("@/lib/api", () => ({
  updateUserSettings: (...args: unknown[]) => updateUserSettings(...args),
}));

import { TodoListPanelSettings } from "./todo-list-panel-settings";

beforeEach(() => {
  updateUserSettings.mockReset().mockResolvedValue({ settings: {} });
});

afterEach(cleanup);

function renderTodoListPanelSettings(userSettingsOverrides: Partial<UserSettingsState> = {}): void {
  render(
    <StateProvider
      initialState={{
        userSettings: { ...defaultState.userSettings, ...userSettingsOverrides },
      }}
    >
      <SettingsSaveProvider>
        <TodoListPanelSettings />
      </SettingsSaveProvider>
    </StateProvider>,
  );
}

describe("TodoListPanelSettings", () => {
  it("keeps an enabled preference local until Save changes persists it", async () => {
    renderTodoListPanelSettings({ showTodoListPanel: false });
    const toggle = screen.getByRole("switch", { name: TODO_LIST_PANEL_LABEL });

    expect(toggle.getAttribute(DATA_STATE_ATTR)).toBe("unchecked");
    fireEvent.click(toggle);

    expect(updateUserSettings).not.toHaveBeenCalled();
    expect(toggle.getAttribute(DATA_STATE_ATTR)).toBe("checked");
    expect(
      screen.getByTestId("todo-list-panel-settings-card").getAttribute(DATA_SETTINGS_DIRTY_ATTR),
    ).toBe("true");

    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith({
        show_todo_list_panel: true,
        show_todo_list_panel_only_when_not_empty: false,
      }),
    );
    await waitFor(() => expect(toggle.getAttribute(DATA_SETTINGS_DIRTY_ATTR)).toBe("false"));
  });

  it("inhibits the sub-option while the main toggle is off and saves both fields", async () => {
    renderTodoListPanelSettings({
      showTodoListPanel: false,
      showTodoListPanelOnlyWhenNotEmpty: true,
    });

    // Inhibited (hidden entirely), not disabled, while the main preference is off.
    expect(screen.queryByRole("switch", { name: ONLY_PIN_WHEN_NOT_EMPTY_LABEL })).toBeNull();

    const toggle = screen.getByRole("switch", { name: TODO_LIST_PANEL_LABEL });
    fireEvent.click(toggle);

    // Appears once the main preference is on, in its preserved (on) state.
    const subToggle = screen.getByRole("switch", { name: ONLY_PIN_WHEN_NOT_EMPTY_LABEL });
    expect(subToggle.getAttribute(DATA_STATE_ATTR)).toBe("checked");

    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith({
        show_todo_list_panel: true,
        show_todo_list_panel_only_when_not_empty: true,
      }),
    );
    await waitFor(() => expect(toggle.getAttribute(DATA_SETTINGS_DIRTY_ATTR)).toBe("false"));
  });

  it("preserves the sub-option state across a main-toggle off/on cycle without saving", () => {
    renderTodoListPanelSettings({
      showTodoListPanel: true,
      showTodoListPanelOnlyWhenNotEmpty: true,
    });

    const toggle = screen.getByRole("switch", { name: TODO_LIST_PANEL_LABEL });
    expect(screen.getByRole("switch", { name: ONLY_PIN_WHEN_NOT_EMPTY_LABEL })).toBeTruthy();

    fireEvent.click(toggle);
    expect(screen.queryByRole("switch", { name: ONLY_PIN_WHEN_NOT_EMPTY_LABEL })).toBeNull();

    fireEvent.click(toggle);
    const subToggle = screen.getByRole("switch", { name: ONLY_PIN_WHEN_NOT_EMPTY_LABEL });
    expect(subToggle.getAttribute(DATA_STATE_ATTR)).toBe("checked");
    expect(updateUserSettings).not.toHaveBeenCalled();
  });

  it("marks each toggle dirty only for its own unsaved change", () => {
    renderTodoListPanelSettings({
      showTodoListPanel: true,
      showTodoListPanelOnlyWhenNotEmpty: false,
    });

    const toggle = screen.getByRole("switch", { name: TODO_LIST_PANEL_LABEL });
    const subToggle = screen.getByRole("switch", { name: ONLY_PIN_WHEN_NOT_EMPTY_LABEL });
    expect(toggle.getAttribute(DATA_SETTINGS_DIRTY_ATTR)).toBe("false");
    expect(subToggle.getAttribute(DATA_SETTINGS_DIRTY_ATTR)).toBe("false");

    // Changing only the sub-option marks only the sub-option dirty.
    fireEvent.click(subToggle);
    expect(subToggle.getAttribute(DATA_SETTINGS_DIRTY_ATTR)).toBe("true");
    expect(toggle.getAttribute(DATA_SETTINGS_DIRTY_ATTR)).toBe("false");

    // Reverting it clears the sub-option's dirty flag.
    fireEvent.click(subToggle);
    expect(subToggle.getAttribute(DATA_SETTINGS_DIRTY_ATTR)).toBe("false");
    expect(toggle.getAttribute(DATA_SETTINGS_DIRTY_ATTR)).toBe("false");

    // Changing only the main toggle marks only the main toggle dirty (the
    // sub-option unmounts with the main toggle off).
    fireEvent.click(toggle);
    expect(toggle.getAttribute(DATA_SETTINGS_DIRTY_ATTR)).toBe("true");
    expect(updateUserSettings).not.toHaveBeenCalled();
  });
});
