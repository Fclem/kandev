package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLUGINS-EFFORT-SIGNALS-003.1 AC-PLUGINS-EFFORT-SIGNALS-003.2
func TestSessionCodeStatsFileCounts(t *testing.T) {
	db, tasks, analytics := newRealSchemaTestDB(t)
	assertSessionCodeStatsFileCounts(t, db, tasks, analytics)
}

func TestPostgresSessionCodeStatsFileCounts(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	tasks, err := tasksqlite.NewWithDB(db, db, nil)
	require.NoError(t, err)
	analytics, err := NewWithDB(db, db)
	require.NoError(t, err)
	assertSessionCodeStatsFileCounts(t, db, tasks, analytics)
}

func assertSessionCodeStatsFileCounts(t *testing.T, db *sqlx.DB, tasks *tasksqlite.Repository, analytics *Repository) {
	t.Helper()
	ctx := context.Background()
	base := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	_, err := db.ExecContext(ctx, db.Rebind(`UPDATE kandev_meta SET value = ? WHERE key = ?`), base.Format(time.RFC3339Nano), commitCaptureActivatedAtMetaKey)
	require.NoError(t, err)
	require.NoError(t, tasks.CreateTask(ctx, &taskmodels.Task{ID: "files-task", Title: "files"}))
	require.NoError(t, tasks.CreateTaskEnvironment(ctx, &taskmodels.TaskEnvironment{ID: "files-env", TaskID: "files-task", ExecutorType: "local", Status: taskmodels.TaskEnvironmentStatusReady}))
	for _, id := range []string{"legacy", "observed", "zero", "reused"} {
		start := base.Add(time.Hour)
		if id == "legacy" {
			start = base.Add(-time.Hour)
		}
		require.NoError(t, tasks.CreateTaskSession(ctx, &taskmodels.TaskSession{ID: id, TaskID: "files-task", TaskEnvironmentID: "files-env", StartedAt: start}))
	}
	for i, count := range []int{3, 2} {
		_, err := tasks.CreateSessionCommit(ctx, &taskmodels.SessionCommit{SessionID: "observed", CommitSHA: []string{"sha-one", "sha-two"}[i], CommittedAt: base, FilesChanged: count})
		require.NoError(t, err)
	}
	_, err = tasks.CreateSessionCommit(ctx, &taskmodels.SessionCommit{SessionID: "reused", CommitSHA: "sha-one", CommittedAt: base, FilesChanged: 3})
	require.NoError(t, err)
	for i, files := range []string{`{"a.go":{"additions":5},"b.go":{"deletions":1},"c.go":{}}`, `{"a.go":{"additions":99}}`, `{}`} {
		var values map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(files), &values))
		require.NoError(t, tasks.CreateGitSnapshot(ctx, &taskmodels.GitSnapshot{
			ID: []string{"snap-one", "snap-two", "snap-clean"}[i], SessionID: "observed", TaskEnvironmentID: "files-env",
			SnapshotType: taskmodels.SnapshotTypeStatusUpdate, Files: values, CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}))
	}
	rows, err := analytics.ListSessionCodeStats(ctx, models.SessionCodeStatsFilter{})
	require.NoError(t, err)
	byID := make(map[string]*models.SessionCodeStats)
	for _, row := range rows {
		byID[row.SessionID] = row
	}
	require.EqualValues(t, 5, *byID["observed"].FilesChangedCommitted, "committed count must sum file churn across commits")
	require.EqualValues(t, 3, byID["observed"].FilesChangedPeakPending, "peak file count must not follow peak lines or latest clean snapshot")
	require.Nil(t, byID["legacy"].FilesChangedCommitted, "pre-activation committed file count is unavailable")
	require.NotNil(t, byID["zero"].FilesChangedCommitted)
	require.EqualValues(t, 0, *byID["zero"].FilesChangedCommitted, "post-activation zero is measured")
	require.Zero(t, byID["zero"].FilesChangedPeakPending)
	require.EqualValues(t, 3, *byID["reused"].FilesChangedCommitted, "the same commit may be attributed independently to sessions sharing an environment")
}
