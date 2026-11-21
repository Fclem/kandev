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

// @covers AC-AGENTS-SESSION-CATALOG-001.7
// @covers AC-AGENTS-SESSION-CATALOG-001.8
func TestSessionCatalogRefreshRetiresLegacyCandidates(t *testing.T) {
	exerciseLegacyCatalogRetirement(t, newRepoForSessionTests(t))
}

// @covers AC-AGENTS-SESSION-CATALOG-001.7
func TestSessionCatalogRefreshRetiresLegacyCandidatesPostgres(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(database, database, nil)
	require.NoError(t, err)
	exerciseLegacyCatalogRetirement(t, repo)
}

func exerciseLegacyCatalogRetirement(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	owner := sessioncatalog.Owner{ExecutionID: "execution", AttemptID: "attempt", SourceGeneration: 1}
	probe := &sessioncatalog.Discovery{Outcome: sessioncatalog.OutcomeComplete, Models: sessioncatalog.Surface{Present: true, Supported: true, Choices: []sessioncatalog.Choice{{ID: "task-owned"}}}}
	records := map[string]sessioncatalog.Observation{
		"legacy-generic": {Version: 1, Revision: 3, ProfileID: "profile", Digest: "generic", Owner: owner, Disposition: sessioncatalog.DispositionHostCandidates, Reason: sessioncatalog.ReasonNewCandidates, Models: []sessioncatalog.Choice{{ID: "generic-utility"}}, Modes: []sessioncatalog.Choice{{ID: "generic-mode"}}},
		"task-owned":     {Version: 1, Revision: 2, ProfileID: "profile", Digest: "owned", Owner: owner, Disposition: sessioncatalog.DispositionHostCandidates, Reason: sessioncatalog.ReasonNewCandidates, Source: sessioncatalog.SourceTaskProbeV1, Probe: probe, Models: probe.Models.Choices},
		"notice":         {Version: 1, Revision: 1, ProfileID: "profile", Owner: owner, Disposition: sessioncatalog.DispositionRediscoveryRequired, Reason: sessioncatalog.ReasonExecutorScoped},
		"cleared":        {Version: 1, Revision: 7, ProfileID: "profile", Owner: owner, Disposition: sessioncatalog.DispositionCleared, Reason: sessioncatalog.ReasonDiscoveryComplete},
	}
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "legacy-task", Title: "Legacy"}))
	provider := map[string]any{"catalog_discovery": map[string]any{"revision": 9}, "current_model_id": "selected"}
	for id, record := range records {
		require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: id, TaskID: "legacy-task", State: models.TaskSessionStateIdle, Metadata: map[string]any{
			sessioncatalog.MetadataKey: record, models.SessionMetaKeyACPModelState: provider, "runtime_config": map[string]any{"model": "selected"},
		}}))
	}
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "without-record", TaskID: "legacy-task", State: models.TaskSessionStateIdle}))

	retired, err := repo.RetireLegacySessionCatalogCandidates(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, retired)
	for id, before := range records {
		session, err := repo.GetTaskSession(ctx, id)
		require.NoError(t, err)
		payload, err := json.Marshal(session.Metadata[sessioncatalog.MetadataKey])
		require.NoError(t, err)
		var after sessioncatalog.Observation
		require.NoError(t, json.Unmarshal(payload, &after))
		require.Equal(t, map[string]any{"model": "selected"}, session.Metadata["runtime_config"], id)
		providerJSON, err := json.Marshal(session.Metadata[models.SessionMetaKeyACPModelState])
		require.NoError(t, err)
		require.JSONEq(t, `{"catalog_discovery":{"revision":9},"current_model_id":"selected"}`, string(providerJSON), "provider choices are not cleared")
		if id != "legacy-generic" {
			require.Equal(t, before, after, id)
			continue
		}
		require.Equal(t, before.Revision+1, after.Revision, "a monotonic notice replaces the legacy candidates")
		require.Equal(t, sessioncatalog.DispositionRediscoveryRequired, after.Disposition)
		require.Equal(t, sessioncatalog.ReasonContextUnverified, after.Reason)
		require.Empty(t, after.Models)
		require.Empty(t, after.Modes)
		require.Empty(t, after.Source)
		require.Nil(t, after.Result, "retirement does not pretend that a restart settled the notice")
		require.Equal(t, owner, after.Owner)
	}
	again, err := repo.RetireLegacySessionCatalogCandidates(ctx)
	require.NoError(t, err)
	require.Zero(t, again)
}
