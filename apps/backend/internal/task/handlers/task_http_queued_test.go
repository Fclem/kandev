package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
)

type fakeQueuedPromptCounter struct {
	byTask map[string]int
	err    error
}

func (f fakeQueuedPromptCounter) CountPendingByTaskIDs(_ context.Context, taskIDs []string) (map[string]int, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]int, len(taskIDs))
	for _, id := range taskIDs {
		out[id] = f.byTask[id]
	}
	return out, nil
}

func newQueuedTaskDTOBuilder(t *testing.T) (*service.Service, *TaskHandlers, *taskrepo.Repository) {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = cleanup()
		_ = sqlxDB.Close()
	})
	require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "ws-1", Name: "Workspace"}))
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo, StatusSummaries: repo,
	}, bus.NewMemoryEventBus(log), log, service.RepositoryDiscoveryConfig{})
	h := &TaskHandlers{service: svc, logger: log}
	return svc, h, repo
}

func createQueuedTestTask(
	t *testing.T,
	svc *service.Service,
	repo *taskrepo.Repository,
	counter fakeQueuedPromptCounter,
) *models.Task {
	t.Helper()
	ctx := context.Background()
	svc.SetQueuedPromptCounter(counter)
	task := &models.Task{ID: "task-1", WorkspaceID: "ws-1", Title: "Queued badge task"}
	require.NoError(t, repo.CreateTask(ctx, task))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "s1", TaskID: "task-1", State: models.TaskSessionStateIdle, IsPrimary: true,
	}))
	return task
}

func TestTaskDTOBuilderStampsQueuedPromptCount(t *testing.T) {
	svc, h, repo := newQueuedTaskDTOBuilder(t)
	ctx := context.Background()
	task := createQueuedTestTask(t, svc, repo, fakeQueuedPromptCounter{byTask: map[string]int{"task-1": 3}})

	result, err := h.toTaskDTOsWithSessionInfo(ctx, []*models.Task{task})
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.NotNil(t, result[0].StatusSummary, "expected a status summary on the DTO")
	assert.Equal(t, 3, result[0].StatusSummary.QueuedPromptCount)

	payload, err := json.Marshal(result[0])
	require.NoError(t, err)
	assert.Contains(t, string(payload), `"queued_prompt_count":3`)
}

func TestTaskDTOBuilderOmitsZeroQueuedPromptCount(t *testing.T) {
	svc, h, repo := newQueuedTaskDTOBuilder(t)
	ctx := context.Background()
	task := createQueuedTestTask(t, svc, repo, fakeQueuedPromptCounter{byTask: map[string]int{"task-1": 0}})

	result, err := h.toTaskDTOsWithSessionInfo(ctx, []*models.Task{task})
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.NotNil(t, result[0].StatusSummary)
	assert.Zero(t, result[0].StatusSummary.QueuedPromptCount)

	payload, err := json.Marshal(result[0])
	require.NoError(t, err)
	assert.NotContains(t, string(payload), "queued_prompt_count")
}

func TestTaskDTOBuilderDegradesWhenQueuedCounterFails(t *testing.T) {
	svc, h, repo := newQueuedTaskDTOBuilder(t)
	ctx := context.Background()
	task := createQueuedTestTask(t, svc, repo, fakeQueuedPromptCounter{err: errors.New("queue store down")})

	result, err := h.toTaskDTOsWithSessionInfo(ctx, []*models.Task{task})
	require.NoError(t, err, "a failed queued counter must not fail the task list")
	require.Len(t, result, 1)
	if result[0].StatusSummary != nil {
		assert.Zero(t, result[0].StatusSummary.QueuedPromptCount)
	}
}
