package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/improvekandev"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type improveKandevTestGitHubSecrets struct {
	values map[string]string
}

func (s *improveKandevTestGitHubSecrets) Reveal(_ context.Context, id string) (string, error) {
	value, ok := s.values[id]
	if !ok {
		return "", errors.New("secret not found")
	}
	return value, nil
}

func (s *improveKandevTestGitHubSecrets) Set(_ context.Context, id, _ string, value string) error {
	s.values[id] = value
	return nil
}

func (s *improveKandevTestGitHubSecrets) Delete(_ context.Context, id string) error {
	delete(s.values, id)
	return nil
}

func (s *improveKandevTestGitHubSecrets) Exists(_ context.Context, id string) (bool, error) {
	_, ok := s.values[id]
	return ok, nil
}

func (s *improveKandevTestGitHubSecrets) ListIDs(context.Context) ([]string, error) {
	ids := make([]string, 0, len(s.values))
	for id := range s.values {
		ids = append(ids, id)
	}
	return ids, nil
}

type improveKandevTestCloner struct {
	path string
}

// newImproveKandevTestRouter registers the improve-kandev routes with a
// synthetic identity, mirroring the production auth middleware (bootstrap
// requires an authenticated identity).
func newImproveKandevTestRouter(handler *improvekandev.Handler) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{UserID: "test-user"})
		c.Next()
	})
	improvekandev.RegisterRoutes(router, handler)
	return router
}

func (c improveKandevTestCloner) EnsureWorkspaceCloned(
	_ context.Context, _, _, _, _, _ string,
) (string, error) {
	return c.path, nil
}

func TestImproveKandevBootstrapCreatesBothHiddenWorkflowsIdempotently(t *testing.T) {
	ts := NewOrchestratorTestServer(t)

	repoPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", repoPath).Run())
	handler := improvekandev.NewHandler(ts.TaskSvc, improveKandevTestCloner{path: repoPath}, nil, nil, "test", ts.Logger)
	router := newImproveKandevTestRouter(handler)

	bootstrap := func() improvekandev.BootstrapResponse {
		t.Helper()
		body, err := json.Marshal(improvekandev.BootstrapRequest{CreateWorkspace: true})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/system/improve-kandev/bootstrap", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		require.Equal(t, http.StatusOK, res.Code, res.Body.String())
		var response improvekandev.BootstrapResponse
		require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))
		return response
	}

	first := bootstrap()
	second := bootstrap()
	require.NotEmpty(t, first.WorkspaceID, "bootstrap must return the dedicated workspace id")
	require.Equal(t, first.WorkspaceID, second.WorkspaceID)
	require.NotEmpty(t, first.WorkflowID)
	require.NotEmpty(t, first.IssueWorkflowID)
	require.NotEqual(t, first.WorkflowID, first.IssueWorkflowID)
	require.Equal(t, first.WorkflowID, second.WorkflowID)
	require.Equal(t, first.IssueWorkflowID, second.IssueWorkflowID)

	// The bootstrap auto-created a workspace named "Improve Kandev".
	workspace, err := ts.TaskRepo.GetWorkspace(context.Background(), first.WorkspaceID)
	require.NoError(t, err)
	require.Equal(t, "Improve Kandev", workspace.Name)

	workflows, err := ts.TaskRepo.ListWorkflows(context.Background(), first.WorkspaceID, true)
	require.NoError(t, err)
	byTemplate := make(map[string]*models.Workflow, len(workflows))
	for _, workflow := range workflows {
		if workflow.WorkflowTemplateID != nil {
			byTemplate[*workflow.WorkflowTemplateID] = workflow
		}
	}
	for _, templateID := range []string{"improve-kandev", "report-kandev-issue"} {
		workflow := byTemplate[templateID]
		require.NotNil(t, workflow, "workflow template %s", templateID)
		require.True(t, workflow.Hidden, "workflow template %s should stay hidden", templateID)
	}

	issueSteps, err := ts.WorkflowSvc.ListStepsByWorkflow(context.Background(), first.IssueWorkflowID)
	require.NoError(t, err)
	require.Len(t, issueSteps, 1)
	require.Equal(t, "Open issue", issueSteps[0].Name)
	require.True(t, issueSteps[0].IsStartStep)
}

func TestImproveKandevBootstrapReusesExistingImproveWorkspace(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	workspaceID := "123e4567-e89b-12d3-a456-426614174000"
	require.NoError(t, ts.TaskRepo.CreateWorkspace(context.Background(), &models.Workspace{
		ID:   workspaceID,
		Name: "Improve Kandev",
	}))

	repoPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", repoPath).Run())
	handler := improvekandev.NewHandler(ts.TaskSvc, improveKandevTestCloner{path: repoPath}, nil, nil, "test", ts.Logger)
	router := newImproveKandevTestRouter(handler)

	// The request may still carry a workspace_id from an older client; it must
	// be ignored in favor of the dedicated Improve Kandev workspace.
	body, err := json.Marshal(improvekandev.BootstrapRequest{WorkspaceID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/improve-kandev/bootstrap", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	var response improvekandev.BootstrapResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))

	require.Equal(t, workspaceID, response.WorkspaceID)
	workflows, err := ts.TaskRepo.ListWorkflows(context.Background(), workspaceID, true)
	require.NoError(t, err)
	require.NotEmpty(t, workflows, "hidden workflows must live in the pre-existing Improve Kandev workspace")
}

func TestImproveKandevBootstrapFallsBackToRequestedWorkspaceWhenCreationDeclined(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	fallbackID := "123e4567-e89b-12d3-a456-426614174000"
	require.NoError(t, ts.TaskRepo.CreateWorkspace(context.Background(), &models.Workspace{
		ID:   fallbackID,
		Name: "Active Workspace",
	}))

	repoPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", repoPath).Run())
	handler := improvekandev.NewHandler(ts.TaskSvc, improveKandevTestCloner{path: repoPath}, nil, nil, "test", ts.Logger)
	router := newImproveKandevTestRouter(handler)

	// No dedicated workspace exists and the caller declines creation
	// (create_workspace=false): bootstrap falls back to the requested
	// workspace and scopes the hidden workflows there.
	body, err := json.Marshal(improvekandev.BootstrapRequest{WorkspaceID: fallbackID})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/improve-kandev/bootstrap", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	var response improvekandev.BootstrapResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))
	require.Equal(t, fallbackID, response.WorkspaceID)
	require.NotEmpty(t, response.WorkflowID)
	require.NotEmpty(t, response.IssueWorkflowID)

	require.NotEmpty(t, response.RepositoryID)
	repository, err := ts.TaskSvc.GetRepository(context.Background(), response.RepositoryID)
	require.NoError(t, err)
	require.Equal(t, fallbackID, repository.WorkspaceID, "kandev repository must be target-local")

	workflows, err := ts.TaskRepo.ListWorkflows(context.Background(), fallbackID, true)
	require.NoError(t, err)
	byTemplate := make(map[string]*models.Workflow, len(workflows))
	for _, workflow := range workflows {
		if workflow.WorkflowTemplateID != nil {
			byTemplate[*workflow.WorkflowTemplateID] = workflow
		}
	}
	require.Equal(t, response.WorkflowID, byTemplate["improve-kandev"].ID)
	require.Equal(t, response.IssueWorkflowID, byTemplate["report-kandev-issue"].ID)
	require.True(t, byTemplate["improve-kandev"].Hidden)
	require.True(t, byTemplate["report-kandev-issue"].Hidden)

}

type fakeGitHubWorkspaceCopier struct {
	calls []struct{ src, dst string }
}

func (f *fakeGitHubWorkspaceCopier) CopyWorkspaceConnectionToWorkspace(_ context.Context, src, dst string) error {
	f.calls = append(f.calls, struct{ src, dst string }{src, dst})
	return nil
}

type improveKandevWorkspaceProbe struct {
	calls []string
}

func (p *improveKandevWorkspaceProbe) DescribeTaskGitCredentialPolicy(
	context.Context,
	string,
) (github.TaskGitCredentialPolicy, error) {
	return github.TaskGitCredentialPolicy{Mode: github.TaskGitCredentialsModeManaged}, nil
}

func (p *improveKandevWorkspaceProbe) ProbeContributionForkCapabilityForWorkspace(
	_ context.Context,
	workspaceID, _, _ string,
) (github.ContributionForkResolution, error) {
	p.calls = append(p.calls, workspaceID)
	switch workspaceID {
	case "ws-dedicated":
		return github.ContributionForkResolution{
			Status:     github.ContributionForkStatusReady,
			ActorLogin: "dedicated-actor",
		}, nil
	default:
		return github.ContributionForkResolution{
			Status:     github.ContributionForkStatusCreatable,
			ActorLogin: "active-actor",
		}, nil
	}
}

func TestImproveKandevBootstrapUsesResolvedWorkspaceGitHubAccess(t *testing.T) {
	t.Run("existing dedicated workspace wins over requested active workspace", func(t *testing.T) {
		ts := NewOrchestratorTestServer(t)
		for _, workspace := range []*models.Workspace{
			{ID: "ws-active", Name: "Active"},
			{ID: "ws-dedicated", Name: "Improve Kandev"},
		} {
			require.NoError(t, ts.TaskRepo.CreateWorkspace(context.Background(), workspace))
		}
		repoPath := t.TempDir()
		require.NoError(t, exec.Command("git", "init", repoPath).Run())
		probe := &improveKandevWorkspaceProbe{}
		handler := improvekandev.NewHandler(
			ts.TaskSvc,
			improveKandevTestCloner{path: repoPath},
			nil,
			nil,
			"test",
			ts.Logger,
		)
		handler.SetManagedGitHubForkProber(probe)
		router := newImproveKandevTestRouter(handler)
		response := postImproveKandevBootstrap(t, router, improvekandev.BootstrapRequest{
			WorkspaceID: "ws-active",
		})

		require.Equal(t, "ws-dedicated", response.WorkspaceID)
		require.Equal(t, "dedicated-actor", response.GitHubLogin)
		require.Equal(t, improvekandev.ForkStatusReady, response.ForkStatus)
		require.Equal(t, []string{"ws-dedicated"}, probe.calls)
	})

	t.Run("declined creation probes the requested active workspace", func(t *testing.T) {
		ts := NewOrchestratorTestServer(t)
		require.NoError(t, ts.TaskRepo.CreateWorkspace(context.Background(), &models.Workspace{
			ID: "ws-active", Name: "Active",
		}))
		repoPath := t.TempDir()
		require.NoError(t, exec.Command("git", "init", repoPath).Run())
		probe := &improveKandevWorkspaceProbe{}
		handler := improvekandev.NewHandler(
			ts.TaskSvc,
			improveKandevTestCloner{path: repoPath},
			nil,
			nil,
			"test",
			ts.Logger,
		)
		handler.SetManagedGitHubForkProber(probe)
		router := newImproveKandevTestRouter(handler)
		response := postImproveKandevBootstrap(t, router, improvekandev.BootstrapRequest{
			WorkspaceID: "ws-active",
		})

		require.Equal(t, "ws-active", response.WorkspaceID)
		require.Equal(t, "active-actor", response.GitHubLogin)
		require.Equal(t, improvekandev.ForkStatusCreatable, response.ForkStatus)
		require.Equal(t, []string{"ws-active"}, probe.calls)
	})
}

func postImproveKandevBootstrap(
	t *testing.T,
	router http.Handler,
	body improvekandev.BootstrapRequest,
) improvekandev.BootstrapResponse {
	t.Helper()
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/improve-kandev/bootstrap", bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	var response improvekandev.BootstrapResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))
	return response
}

func TestImproveKandevBootstrapCopiesOnlyGitHubConfiguration(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	ctx := context.Background()
	source, err := ts.TaskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{Name: "Source"})
	require.NoError(t, err)
	sourceWorkflow, err := ts.TaskSvc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{
		WorkspaceID: source.ID, Name: "User workflow",
	})
	require.NoError(t, err)
	sourceRepoPath := createTempRepoDir(t)
	sourceRepo, err := ts.TaskSvc.CreateRepository(ctx, &taskservice.CreateRepositoryRequest{
		WorkspaceID: source.ID, Name: "User repository", LocalPath: sourceRepoPath,
	})
	require.NoError(t, err)

	copier := &fakeGitHubWorkspaceCopier{}
	repoPath := createTempRepoDir(t)
	handler := improvekandev.NewHandler(
		ts.TaskSvc,
		improveKandevTestCloner{path: repoPath},
		copier,
		func(context.Context) (string, error) { return source.ID, nil },
		"test",
		ts.Logger,
	)
	response := postImproveKandevBootstrap(
		t,
		newImproveKandevTestRouter(handler),
		improvekandev.BootstrapRequest{CreateWorkspace: true},
	)

	workflows, err := ts.TaskRepo.ListWorkflows(ctx, response.WorkspaceID, true)
	require.NoError(t, err)
	require.Len(t, workflows, 2, "only the two bootstrap workflows should be created")
	repositories, err := ts.TaskSvc.ListRepositories(ctx, response.WorkspaceID)
	require.NoError(t, err)
	require.Len(t, repositories, 1, "only the kandev repository should be created")
	require.Equal(t, response.RepositoryID, repositories[0].ID)
	sourceWorkflows, err := ts.TaskRepo.ListWorkflows(ctx, source.ID, true)
	require.NoError(t, err)
	sourceWorkflowIDs := make([]string, len(sourceWorkflows))
	for i, workflow := range sourceWorkflows {
		sourceWorkflowIDs[i] = workflow.ID
	}
	require.Contains(t, sourceWorkflowIDs, sourceWorkflow.ID)
	sourceRepositories, err := ts.TaskSvc.ListRepositories(ctx, source.ID)
	require.NoError(t, err)
	require.Contains(t, repositoryIDs(sourceRepositories), sourceRepo.ID)
	require.Equal(t, []struct{ src, dst string }{{source.ID, response.WorkspaceID}}, copier.calls)
}

func TestImproveKandevBootstrapPreservesExistingWorkspaceConfiguration(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	ctx := context.Background()
	target, err := ts.TaskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{Name: "Improve Kandev"})
	require.NoError(t, err)
	userWorkflow, err := ts.TaskSvc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{
		WorkspaceID: target.ID, Name: "User workflow",
	})
	require.NoError(t, err)
	userRepoPath := createTempRepoDir(t)
	userRepo, err := ts.TaskSvc.CreateRepository(ctx, &taskservice.CreateRepositoryRequest{
		WorkspaceID: target.ID, Name: "User repository", LocalPath: userRepoPath,
	})
	require.NoError(t, err)

	copier := &fakeGitHubWorkspaceCopier{}
	handler := improvekandev.NewHandler(
		ts.TaskSvc,
		improveKandevTestCloner{path: createTempRepoDir(t)},
		copier,
		func(context.Context) (string, error) { return "ws-default", nil },
		"test",
		ts.Logger,
	)
	response := postImproveKandevBootstrap(
		t,
		newImproveKandevTestRouter(handler),
		improvekandev.BootstrapRequest{CreateWorkspace: true},
	)

	require.Equal(t, target.ID, response.WorkspaceID)
	workflows, err := ts.TaskRepo.ListWorkflows(ctx, target.ID, true)
	require.NoError(t, err)
	workflowIDs := make([]string, len(workflows))
	for i, workflow := range workflows {
		workflowIDs[i] = workflow.ID
	}
	require.Contains(t, workflowIDs, userWorkflow.ID)
	repositories, err := ts.TaskSvc.ListRepositories(ctx, target.ID)
	require.NoError(t, err)
	require.Contains(t, repositoryIDs(repositories), userRepo.ID)
	require.Len(t, copier.calls, 0, "reuse must not copy or synchronize workspace configuration")
}

func repositoryIDs(repositories []*models.Repository) []string {
	ids := make([]string, len(repositories))
	for i, repository := range repositories {
		ids[i] = repository.ID
	}
	return ids
}

func TestImproveKandevBootstrapCopiesGitHubConnectionOnWorkspaceCreation(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	copier := &fakeGitHubWorkspaceCopier{}

	repoPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", repoPath).Run())
	resolveDefault := func(context.Context) (string, error) {
		return "default-workspace-id", nil
	}
	handler := improvekandev.NewHandler(ts.TaskSvc, improveKandevTestCloner{path: repoPath}, copier, resolveDefault, "test", ts.Logger)
	router := newImproveKandevTestRouter(handler)

	body, err := json.Marshal(improvekandev.BootstrapRequest{CreateWorkspace: true})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/improve-kandev/bootstrap", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	var response improvekandev.BootstrapResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))

	require.Len(t, copier.calls, 1, "connection must be copied exactly once on workspace creation")
	require.Equal(t, "default-workspace-id", copier.calls[0].src)
	require.Equal(t, response.WorkspaceID, copier.calls[0].dst)

	// A second bootstrap (workspace now exists) must not copy again.
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/system/improve-kandev/bootstrap", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	res2 := httptest.NewRecorder()
	router.ServeHTTP(res2, req2)
	require.Equal(t, http.StatusOK, res2.Code, res2.Body.String())
	require.Len(t, copier.calls, 1, "existing workspace must not re-copy the connection")
}

func TestImproveKandevBootstrapCopiesConnectionFromResolvedWorkspace(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	ctx := context.Background()
	active, err := ts.TaskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{Name: "Active source"})
	require.NoError(t, err)
	earliest, err := ts.TaskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{Name: "Earliest source"})
	require.NoError(t, err)

	db := sqlx.NewDb(ts.TaskRepo.DB(), "sqlite3")
	ghStore, err := github.NewStore(db, db)
	require.NoError(t, err)
	secrets := &improveKandevTestGitHubSecrets{values: map[string]string{
		github.WorkspacePATSecretKey(active.ID):   "active-source-pat",
		github.WorkspacePATSecretKey(earliest.ID): "earliest-source-pat",
	}}
	for _, connection := range []*github.WorkspaceConnection{
		{WorkspaceID: active.ID, Source: github.ConnectionSourcePAT, GitHubHost: "github.com", Login: "active-user", Status: github.ConnectionStatusActive},
		{WorkspaceID: earliest.ID, Source: github.ConnectionSourcePAT, GitHubHost: "github.com", Login: "earliest-user", Status: github.ConnectionStatusActive},
	} {
		require.NoError(t, ghStore.UpsertWorkspaceConnection(ctx, connection))
	}
	githubService := github.NewService(github.NewMockClient(), github.AuthMethodNone, nil, ghStore, ts.EventBus, ts.Logger)
	githubService.SetConnectionSecretStore(secrets)

	repoPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", repoPath).Run())
	resolveDefault := func(context.Context) (string, error) {
		return active.ID, nil
	}
	handler := improvekandev.NewHandler(ts.TaskSvc, improveKandevTestCloner{path: repoPath}, githubService, resolveDefault, "test", ts.Logger)
	router := newImproveKandevTestRouter(handler)
	response := postImproveKandevBootstrap(t, router, improvekandev.BootstrapRequest{CreateWorkspace: true})

	target, err := ghStore.GetWorkspaceConnection(ctx, response.WorkspaceID)
	require.NoError(t, err)
	require.NotNil(t, target)
	require.Equal(t, github.ConnectionSourcePAT, target.Source)
	require.Equal(t, "active-user", target.Login)
	require.Equal(t, "active-source-pat", secrets.values[github.WorkspacePATSecretKey(response.WorkspaceID)])
}

func TestImproveKandevBootstrapDoesNotCopyWhenResolvedWorkspaceHasNoConnection(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	ctx := context.Background()
	source, err := ts.TaskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{Name: "Unconfigured source"})
	require.NoError(t, err)

	db := sqlx.NewDb(ts.TaskRepo.DB(), "sqlite3")
	ghStore, err := github.NewStore(db, db)
	require.NoError(t, err)
	githubService := github.NewService(github.NewMockClient(), github.AuthMethodNone, nil, ghStore, ts.EventBus, ts.Logger)
	secrets := &improveKandevTestGitHubSecrets{values: map[string]string{}}
	githubService.SetConnectionSecretStore(secrets)

	repoPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", repoPath).Run())
	resolveDefault := func(context.Context) (string, error) {
		return source.ID, nil
	}
	handler := improvekandev.NewHandler(ts.TaskSvc, improveKandevTestCloner{path: repoPath}, githubService, resolveDefault, "test", ts.Logger)
	response := postImproveKandevBootstrap(t, newImproveKandevTestRouter(handler), improvekandev.BootstrapRequest{
		CreateWorkspace: true,
	})

	connection, err := ghStore.GetWorkspaceConnection(ctx, response.WorkspaceID)
	require.NoError(t, err)
	require.Nil(t, connection)
	require.Empty(t, secrets.values)
}

func TestImproveKandevBootstrapRequiresWorkspaceIDWhenCreationDeclined(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	repoPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", repoPath).Run())
	handler := improvekandev.NewHandler(ts.TaskSvc, improveKandevTestCloner{path: repoPath}, nil, nil, "test", ts.Logger)
	router := newImproveKandevTestRouter(handler)

	body, err := json.Marshal(improvekandev.BootstrapRequest{})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/improve-kandev/bootstrap", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	require.Equal(t, http.StatusInternalServerError, res.Code, res.Body.String())
}

func TestImproveKandevBootstrapConvergesOnSingleWorkspace(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	// Simulate the outcome of two concurrent first-time bootstraps: two rows
	// with the same name (workspace names are not unique). Bootstrap must
	// converge on a single, deterministic workspace id across calls.
	for _, id := range []string{
		"aaaaaaaa-0000-0000-0000-000000000001",
		"aaaaaaaa-0000-0000-0000-000000000002",
	} {
		require.NoError(t, ts.TaskRepo.CreateWorkspace(context.Background(), &models.Workspace{
			ID:   id,
			Name: "Improve Kandev",
		}))
	}

	repoPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", repoPath).Run())
	handler := improvekandev.NewHandler(ts.TaskSvc, improveKandevTestCloner{path: repoPath}, nil, nil, "test", ts.Logger)
	router := newImproveKandevTestRouter(handler)

	bootstrap := func() improvekandev.BootstrapResponse {
		t.Helper()
		body, err := json.Marshal(improvekandev.BootstrapRequest{CreateWorkspace: true})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/system/improve-kandev/bootstrap", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		require.Equal(t, http.StatusOK, res.Code, res.Body.String())
		var response improvekandev.BootstrapResponse
		require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))
		return response
	}

	first := bootstrap()
	second := bootstrap()
	require.Equal(t, first.WorkspaceID, second.WorkspaceID, "both bootstraps must agree on one workspace")
}
