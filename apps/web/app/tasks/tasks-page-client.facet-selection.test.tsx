import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import { pluginRegistry } from "@/lib/plugins/registry";
import type { Task } from "@/lib/types/http";

const { replace, updateUserSettings } = vi.hoisted(() => ({
  replace: vi.fn(),
  updateUserSettings: vi.fn().mockResolvedValue({}),
}));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: vi.fn(), replace }),
  useSearchParams: () => new URLSearchParams(window.location.search),
}));
vi.mock("@/lib/api", () => ({
  archiveTask: vi.fn(),
  deleteTask: vi.fn(),
  unarchiveTask: vi.fn(),
  listTasksByWorkspace: vi.fn().mockResolvedValue({ tasks: [], total: 0 }),
  updateUserSettings,
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
vi.mock("@/hooks/use-kanban-display-settings", () => ({
  useKanbanDisplaySettings: () => ({
    activeWorkspaceId: "ws1",
    activeWorkflowId: null,
    repositories: [],
    selectedRepositoryId: null,
  }),
}));
vi.mock("@/hooks/use-debounce", () => ({ useDebounce: (value: unknown) => value }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));
vi.mock("@/hooks/use-task-listing-view", () => ({
  useTaskListingView: () => ({ effectiveView: "list", setView: vi.fn() }),
}));
vi.mock("@/hooks/use-foreground-refresh", () => ({ useForegroundRefresh: () => undefined }));
vi.mock("@/hooks/use-workflow-snapshot", () => ({ useWorkflowSnapshot: () => undefined }));
vi.mock("@/hooks/domains/gitlab/use-task-mr", () => ({ useWorkspaceMRs: () => undefined }));
vi.mock("@/hooks/domains/github/use-task-pr", () => ({ useWorkspacePRs: () => undefined }));
vi.mock("./tasks-page-content", () => ({
  TasksPageContent: ({
    tasks,
    sort,
    group,
    onSortChange,
  }: {
    tasks: Task[];
    sort: string;
    group: string;
    onSortChange: (value: string) => void;
  }) => (
    <div>
      <output data-testid="sort">{sort}</output>
      <output data-testid="group">{group}</output>
      <ol data-testid="rows">
        {tasks.map((task) => (
          <li key={task.id}>{task.id}</li>
        ))}
      </ol>
      <button onClick={() => onSortChange("facet:plugin:tags")}>Choose facet</button>
    </div>
  ),
}));
import { TasksPageClient } from "./tasks-page-client";

const key = "facet:plugin:tags";
const tasks = [
  {
    id: "alpha",
    title: "Alpha",
    updated_at: "2026-01-01T00:00:00Z",
    created_at: "2026-01-01T00:00:00Z",
  },
  {
    id: "beta",
    title: "Beta",
    updated_at: "2026-02-01T00:00:00Z",
    created_at: "2026-02-01T00:00:00Z",
  },
] as Task[];

function renderPage(
  initialSort: "title_asc" | typeof key = "title_asc",
  initialGroup: "state" | typeof key = "state",
) {
  return render(
    <StateProvider
      initialState={{
        workspaces: { items: [], activeId: "ws1" },
        userSettings: {
          ...defaultSettingsState.userSettings,
          tasksListSort: initialSort,
          tasksListGroup: initialGroup,
          loaded: true,
        },
      }}
    >
      <TasksPageClient
        workspaces={[]}
        initialWorkflows={[]}
        initialRepositories={[]}
        initialTasks={tasks}
        initialTotal={2}
        initialDataLoaded
        initialSort={initialSort as never}
        initialGroup={initialGroup as never}
      />
    </StateProvider>,
  );
}

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin("plugin");
  replace.mockReset();
  updateUserSettings.mockClear();
  window.history.replaceState({}, "", "/");
});

describe("TasksPageClient facet preference", () => {
  // AC-PLUGINS-TASKLIST-FACETS-002.6 and 003.1: preserve loaded ties on selection.
  it("persists a chosen facet without a built-in local re-sort", async () => {
    pluginRegistry.forPlugin("plugin").registerTaskListFacet({
      id: "tags",
      label: "Tag",
      getValues: () => [{ value: "same", label: "Same" }],
    });
    window.history.replaceState({}, "", "/tasks");
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Choose facet" }));
    expect(screen.getByTestId("rows").textContent).toBe("alphabeta");
    expect(screen.getByTestId("sort").textContent).toBe(key);
    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith(
        expect.objectContaining({ tasks_list_sort: key }),
        { cache: "no-store" },
      ),
    );
    expect(replace).toHaveBeenCalledWith(expect.stringContaining("sort=facet%3Aplugin%3Atags"), {
      scroll: false,
    });
  });

  // AC-PLUGINS-TASKLIST-FACETS-003.3: a link must retain the selection while unavailable.
  it("does not save the fallback when a facet deep link is temporarily unavailable", () => {
    window.history.replaceState({}, "", `/tasks?sort=${key}&group=${key}`);
    renderPage(key, key);
    expect(screen.getByTestId("sort").textContent).toBe("updated_desc");
    expect(screen.getByTestId("group").textContent).toBe("state");
    expect(updateUserSettings).not.toHaveBeenCalledWith(
      expect.objectContaining({ tasks_list_sort: "updated_desc", tasks_list_group: "state" }),
      expect.anything(),
    );
    expect(new URLSearchParams(window.location.search).get("sort")).toBe(key);
  });
});
