package sqlite

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

type catalogConcurrentWrite func() (*sessioncatalog.Observation, error)

func concurrentCatalogWinner(t *testing.T, writes ...catalogConcurrentWrite) *sessioncatalog.Observation {
	t.Helper()
	type result struct {
		record *sessioncatalog.Observation
		err    error
	}
	start := make(chan struct{})
	completed := make(chan result, len(writes))
	for _, write := range writes {
		go func(write catalogConcurrentWrite) { <-start; record, err := write(); completed <- result{record, err} }(write)
	}
	close(start)
	var winner *sessioncatalog.Observation
	for range writes {
		outcome := <-completed
		require.NoError(t, outcome.err)
		if outcome.record != nil {
			require.Nil(t, winner, "one expected prior record can allocate only one new revision")
			winner = outcome.record
		}
	}
	require.NotNil(t, winner)
	return winner
}

func exerciseConcurrentCatalogCAS(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	owner := sessioncatalog.Owner{ExecutionID: "concurrent-execution", AttemptID: "concurrent-attempt", SourceGeneration: 1}
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "concurrent-task", Title: "Concurrent publication"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "concurrent-session", TaskID: "concurrent-task", AgentProfileID: "profile", ExecutionProfileID: "profile", State: models.TaskSessionStateIdle, Metadata: map[string]any{"runtime_config": map[string]any{"model": "selected"}, "composer_draft": "retained"}}))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "concurrent-runtime", TaskID: "concurrent-task", SessionID: "concurrent-session", AgentExecutionID: owner.ExecutionID, ExecutionProfileID: "profile", Status: models.ExecutorRunningStatusReady}))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "concurrent-session", models.SessionMetaKeyACPModelState, map[string]any{"settings_source_execution_id": owner.ExecutionID, "settings_attempt_id": owner.AttemptID, "settings_source_generation": owner.SourceGeneration, "current_model_id": "selected"}))
	candidate := sessioncatalog.Observation{Owner: owner, ProfileID: "profile", Disposition: "host_candidates", Models: []sessioncatalog.Choice{{ID: "candidate", Name: "Candidate"}}}
	publish := func(expected any, digest string) catalogConcurrentWrite {
		next := candidate
		next.Digest = digest
		return func() (*sessioncatalog.Observation, error) {
			return repo.PublishSessionCatalog(ctx, "concurrent-session", owner.ExecutionID, "profile", expected, next)
		}
	}
	first := concurrentCatalogWinner(t, publish(nil, "first-a"), publish(nil, "first-b"))
	require.Equal(t, uint64(1), first.Revision)
	session, err := repo.GetTaskSession(ctx, "concurrent-session")
	require.NoError(t, err)
	prior := session.Metadata[sessioncatalog.MetadataKey]
	launch, err := repo.BeginSessionCatalogDiscovery(ctx, session.ID, "profile", owner, "strict")
	require.NoError(t, err)
	require.Equal(t, first.Revision, launch.NoticeRevision)
	terminal := *first
	terminal.Disposition = "cleared"
	terminal.Reason = "discovery_complete"
	terminal.Models = nil
	terminal.Result = &sessioncatalog.Discovery{Outcome: "complete", Models: sessioncatalog.Surface{Present: true, Supported: true, Choices: []sessioncatalog.Choice{}}}
	second := concurrentCatalogWinner(t, publish(prior, "newer-host"), func() (*sessioncatalog.Observation, error) {
		return repo.SettleSessionCatalogDiscovery(ctx, session.ID, owner.ExecutionID, "profile", prior, terminal)
	})
	require.Equal(t, uint64(2), second.Revision)
	current, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	encoded, err := json.Marshal(current.Metadata[sessioncatalog.MetadataKey])
	require.NoError(t, err)
	var persisted sessioncatalog.Observation
	require.NoError(t, json.Unmarshal(encoded, &persisted))
	require.Equal(t, *second, persisted)
	require.Equal(t, "retained", current.Metadata["composer_draft"])
	require.Equal(t, map[string]any{"model": "selected"}, current.Metadata["runtime_config"])
	if second.Digest == "newer-host" {
		late, err := repo.SettleSessionCatalogDiscovery(ctx, session.ID, owner.ExecutionID, "profile", current.Metadata[sessioncatalog.MetadataKey], terminal)
		require.NoError(t, err)
		require.Nil(t, late, "a delayed result cannot clear the concurrently published newer notice")
	}
}

func TestSessionCatalogRefreshConcurrentCAS(t *testing.T) {
	exerciseConcurrentCatalogCAS(t, newRepoForSessionTests(t))
}

func TestSessionCatalogRefreshConcurrentCASPostgres(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(database, database, nil)
	require.NoError(t, err)
	exerciseConcurrentCatalogCAS(t, repo)
}
