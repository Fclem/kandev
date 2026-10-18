package backendapp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/task/models"
)

func TestFlagOverrideStampsNewActivitySession(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		HomeDir:  t.TempDir(),
		Database: config.DatabaseConfig{Driver: "sqlite"},
	}
	log := newTestLogger()
	_, repos, cleanups, err := provideRepositories(ctx, cfg, log, "test-session-user-activity")
	if err != nil {
		t.Fatalf("provideRepositories: %v", err)
	}
	t.Cleanup(func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			if cleanups[i] != nil {
				_ = cleanups[i]()
			}
		}
	})
	if err := repos.RuntimeFlags.SetOverride(ctx, "features.sessionUserActivity", true); err != nil {
		t.Fatalf("set SQLite override: %v", err)
	}
	if !applyStartupRuntimeFlags(ctx, cfg, repos, log) {
		t.Fatal("applyStartupRuntimeFlags returned false")
	}
	if !cfg.Features.SessionUserActivity {
		t.Fatal("SQLite override did not enable session user-activity capture")
	}
	repos.Task.SetUserActivityCapture(cfg.Features.SessionUserActivity)
	if err := repos.Task.RecordCaptureTransition(ctx, cfg.Features.SessionUserActivity, time.Now().UTC()); err != nil {
		t.Fatalf("record capture transition: %v", err)
	}
	if err := repos.Task.CreateTask(ctx, &models.Task{ID: "activity-override-task", Title: "activity"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repos.Task.CreateTaskSession(ctx, &models.TaskSession{ID: "activity-override-session", TaskID: "activity-override-task"}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	var metadata string
	if err := repos.Task.DB().QueryRowContext(ctx, `SELECT metadata FROM task_sessions WHERE id = ?`, "activity-override-session").Scan(&metadata); err != nil {
		t.Fatalf("read session metadata: %v", err)
	}
	var values map[string]interface{}
	if err := json.Unmarshal([]byte(metadata), &values); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if stamped, _ := values["user_activity_capture"].(bool); !stamped {
		t.Fatalf("session metadata = %s, want user_activity_capture=true", metadata)
	}
}
