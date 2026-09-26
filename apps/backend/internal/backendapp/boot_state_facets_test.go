package backendapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userservice "github.com/kandev/kandev/internal/user/service"
)

type recordingBootTaskRepository struct {
	taskrepo.TaskRepository
	listedSort string
}

func (r *recordingBootTaskRepository) ListTasksByWorkspaceWithArchiveMode(
	ctx context.Context, workspaceID, workflowID, repositoryID, query string,
	page, pageSize int, sort string, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig, onlyArchived bool,
) ([]*models.Task, int, error) {
	r.listedSort = sort
	return r.TaskRepository.(interface {
		ListTasksByWorkspaceWithArchiveMode(context.Context, string, string, string, string, int, int, string, bool, bool, bool, bool, bool) ([]*models.Task, int, error)
	}).ListTasksByWorkspaceWithArchiveMode(ctx, workspaceID, workflowID, repositoryID, query,
		page, pageSize, sort, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig, onlyArchived)
}

// AC-PLUGINS-TASKLIST-FACETS-002.7, 003.1: requested sort survives boot while the query is built-in.
func TestTasksPageBootDataResolvesFacetQueryWithoutDiscardingSelection(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	workspaces, err := harness.taskSvc.ListWorkspaces(ctx)
	if err != nil || len(workspaces) == 0 {
		t.Fatalf("ListWorkspaces: %v, %d rows", err, len(workspaces))
	}
	recording := &recordingBootTaskRepository{TaskRepository: harness.taskRepo}
	repo := harness.taskRepo
	log := newTestLogger()
	svc := taskservice.NewService(taskservice.Repos{
		Workspaces: repo, Tasks: recording, TaskRepos: repo, Workflows: repo,
		Messages: repo, Turns: repo, Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		RepositorySets: repo, Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo, StatusSummaries: repo, WorkspaceFolders: repo,
	}, nil, log, taskservice.RepositoryDiscoveryConfig{})
	builder := bootStateBuilder{p: routeParams{taskSvc: svc, userCtrl: harness.userCtrl,
		services: &Services{Workflow: harness.workflowSvc}}}
	stored := "facet:my_plugin:tags"
	if _, err := harness.userSvc.UpdateUserSettings(ctx, &userservice.UpdateUserSettingsRequest{
		WorkspaceID: &workspaces[0].ID, TasksListSort: &stored, TasksListGroup: &stored,
	}); err != nil {
		t.Fatalf("save facet preference: %v", err)
	}
	for _, tc := range []struct{ name, path, wantSort, wantGroup string }{
		{"stored facet", "/tasks", stored, stored},
		{"query facet overrides stored", "/tasks?sort=facet:plugin:tags&group=facet:plugin:tags", "facet:plugin:tags", "facet:plugin:tags"},
		{"padded query facet", "/tasks?sort=%20facet:plugin:tags%20&group=%20facet:plugin:tags%20", "facet:plugin:tags", "facet:plugin:tags"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tc.path, nil)
			_, routeData := builder.tasksPageBootData(ctx, request)
			if routeData == nil || routeData["tasksListSort"] != tc.wantSort || routeData["tasksListGroup"] != tc.wantGroup {
				t.Fatalf("boot selections = %v, want sort %q group %q", routeData, tc.wantSort, tc.wantGroup)
			}
			if recording.listedSort != "updated_desc" {
				t.Fatalf("repository sort = %q, want updated_desc", recording.listedSort)
			}
		})
	}
}
