package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestUsageTurnIndexMigrationPreservesCursorsAndCoverage(t *testing.T) {
	testUsageTurnIndexMigration(t, newUsageEventsTestRepo(t))
}

func TestPostgresUsageTurnIndexMigrationPreservesCursorsAndCoverage(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	testUsageTurnIndexMigration(t, repo)
}

func testUsageTurnIndexMigration(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	createUsageEventsTestTask(t, repo, "task")
	createUsageEventsTestSession(t, repo, "session", "task")
	for _, id := range []string{"old", "new", "gap", "absent"} {
		if err := repo.CreateTurn(ctx, &models.Turn{ID: id, TaskID: "task", TaskSessionID: "session"}); err != nil {
			t.Fatal(err)
		}
	}
	for i, turnID := range []string{"old", "old", "new"} {
		event := newTestUsageEvent([]string{"first", "second", "third"}[i], "task", "session")
		event.TurnID = turnID
		if err := repo.CreateTaskUsageEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	key := models.NativeUsageGapDirectPrefix + `"thread":"native-turn"`
	if _, _, err := repo.PatchTurnMetadata(ctx, "session", "gap", map[string]interface{}{key: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.PatchTurnMetadata(ctx, "session", "absent", map[string]interface{}{key: false}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.Exec(`DROP TABLE task_usage_turn_index`); err != nil {
		t.Fatal(err)
	}
	if err := repo.migrateUsageTurnIndex(); err != nil {
		t.Fatal(err)
	}
	page, err := repo.ListSessionUsageTurnCursors(ctx, "session", 0, 1)
	if err != nil || len(page) != 1 || page[0].TurnID != "gap" || page[0].Cursor <= 3 {
		t.Fatalf("coverage-only migration page=%#v err=%v", page, err)
	}
	gapCursor := page[0].Cursor
	page, err = repo.ListSessionUsageTurnCursors(ctx, "session", gapCursor, 1)
	if err != nil || len(page) != 1 || page[0].TurnID != "new" || page[0].Cursor != 3 {
		t.Fatalf("legacy newest cursor changed: %#v err=%v", page, err)
	}
	page, err = repo.ListSessionUsageTurnCursors(ctx, "session", 3, 1)
	if err != nil || len(page) != 1 || page[0].TurnID != "old" || page[0].Cursor != 1 {
		t.Fatalf("legacy older cursor changed: %#v err=%v", page, err)
	}
	late := newTestUsageEvent("late", "task", "session")
	late.TurnID = "old"
	if err := repo.CreateTaskUsageEvent(ctx, late); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.PatchTurnMetadata(ctx, "session", "gap", map[string]interface{}{key: true}); err != nil {
		t.Fatal(err)
	}
	if err := repo.migrateUsageTurnIndex(); err != nil {
		t.Fatal(err)
	}
	all, err := repo.ListSessionUsageTurnCursors(ctx, "session", 0, 10)
	if err != nil || len(all) != 3 || all[0].Cursor != gapCursor || all[2].Cursor != 1 {
		t.Fatalf("late rows/replay changed bounded membership: %#v err=%v", all, err)
	}
	if countTaskUsageEventRows(t, repo) != 4 {
		t.Fatal("coverage migration fabricated usage rows")
	}
}

func TestUsageTurnIndexFailureRollsBackCoverageAndAmounts(t *testing.T) {
	ctx := context.Background()
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task")
	createUsageEventsTestSession(t, repo, "session", "task")
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn", TaskID: "task", TaskSessionID: "session"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.Exec(`CREATE TRIGGER reject_usage_index BEFORE INSERT ON task_usage_turn_index BEGIN SELECT RAISE(ABORT, 'index unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	key := models.NativeUsageGapDirectPrefix + `"thread":"native-turn"`
	if _, _, err := repo.PatchTurnMetadata(ctx, "session", "turn", map[string]interface{}{key: true}); err == nil {
		t.Fatal("coverage commit succeeded without index membership")
	}
	turn, err := repo.GetTurn(ctx, "turn")
	if err != nil || turn.Metadata[key] != nil {
		t.Fatalf("failed membership leaked coverage: %#v err=%v", turn, err)
	}
	event := newTestUsageEvent("known", "task", "session")
	event.TurnID = "turn"
	if err := repo.CreateTaskUsageEvent(ctx, event); err == nil {
		t.Fatal("amount commit succeeded without index membership")
	}
	if countTaskUsageEventRows(t, repo) != 0 {
		t.Fatal("failed membership leaked usage amount")
	}
	input, cached, output, cost := readTaskSessionRollup(t, repo, "session")
	if input != 0 || cached != 0 || output != 0 || cost != 0 {
		t.Fatalf("failed index changed rollups: %d/%d/%d/%d", input, cached, output, cost)
	}
}

func TestUsageTurnIndexRejectsMismatchedCoverageOwner(t *testing.T) {
	ctx := context.Background()
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task")
	createUsageEventsTestTask(t, repo, "other-task")
	createUsageEventsTestSession(t, repo, "session", "task")
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "foreign-turn", TaskID: "other-task", TaskSessionID: "session"}); err != nil {
		t.Fatal(err)
	}
	key := models.NativeUsageGapChildPrefix + `"thread":"native-turn"`
	if _, _, err := repo.PatchTurnMetadata(ctx, "session", "foreign-turn", map[string]interface{}{key: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.Exec(`DROP TABLE task_usage_turn_index`); err != nil {
		t.Fatal(err)
	}
	if err := repo.migrateUsageTurnIndex(); err != nil {
		t.Fatal(err)
	}
	page, err := repo.ListSessionUsageTurnCursors(ctx, "session", 0, 10)
	if err != nil || len(page) != 0 {
		t.Fatalf("mismatched owner entered usage index: %#v err=%v", page, err)
	}
}

func TestTaskUsageCoverageSurvivesSessionPurge(t *testing.T) {
	for _, prefix := range []string{models.NativeUsageGapDirectPrefix, models.NativeUsageGapChildPrefix} {
		t.Run(prefix, func(t *testing.T) {
			ctx := context.Background()
			repo := newUsageEventsTestRepo(t)
			createUsageEventsTestTask(t, repo, "task")
			createUsageEventsTestTask(t, repo, "complete-task")
			createUsageEventsTestSession(t, repo, "session", "task")
			if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn", TaskID: "task", TaskSessionID: "session"}); err != nil {
				t.Fatal(err)
			}
			event := newTestUsageEvent("known", "task", "session")
			event.TurnID = "turn"
			if err := repo.CreateTaskUsageEvent(ctx, event); err != nil {
				t.Fatal(err)
			}
			if _, _, err := repo.PatchTurnMetadata(ctx, "session", "turn", map[string]interface{}{prefix + `"thread":"native-turn"`: true}); err != nil {
				t.Fatal(err)
			}
			session, err := repo.GetTaskSession(ctx, "session")
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.DeleteTaskSession(ctx, session); err != nil {
				t.Fatal(err)
			}
			totals, err := repo.GetTaskUsageTotals(ctx, "task")
			if err != nil {
				t.Fatal(err)
			}
			if totals.InputTokensComplete || totals.OutputTokensComplete || totals.TotalTokensComplete {
				t.Fatalf("purge promoted partial native work to complete: %+v", totals)
			}
			if totals.EventCount != 1 || totals.TokensTotal != 155 || totals.CostSubcents != 42 {
				t.Fatalf("purge changed independently measured work: %+v", totals)
			}
			if session, _ := repo.GetTaskSession(ctx, "session"); session != nil {
				t.Fatal("purged session remains accessible")
			}
			complete, err := repo.GetTaskUsageTotals(ctx, "complete-task")
			if err != nil || !complete.TotalTokensComplete {
				t.Fatalf("coverage leaked across task ownership: %+v err=%v", complete, err)
			}
		})
	}
}

func TestTaskUsageCoverageMigrationBackfillsOnlyOwnedGaps(t *testing.T) {
	ctx := context.Background()
	repo := newUsageEventsTestRepo(t)
	for _, taskID := range []string{"task", "foreign", "absent"} {
		createUsageEventsTestTask(t, repo, taskID)
	}
	createUsageEventsTestSession(t, repo, "session", "task")
	for _, taskID := range []string{"task", "foreign", "absent"} {
		if err := repo.CreateTurn(ctx, &models.Turn{ID: taskID, TaskID: taskID, TaskSessionID: "session"}); err != nil {
			t.Fatal(err)
		}
		key := models.NativeUsageGapDirectPrefix + `"thread":"native-turn"`
		if _, _, err := repo.PatchTurnMetadata(ctx, "session", taskID, map[string]interface{}{key: taskID != "absent"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.db.Exec(`DROP TABLE task_usage_coverage`); err != nil {
		t.Fatal(err)
	}
	if err := repo.migrateTaskUsageCoverage(); err != nil {
		t.Fatal(err)
	}
	if err := repo.migrateTaskUsageCoverage(); err != nil {
		t.Fatal(err)
	}
	for _, taskID := range []string{"task", "foreign", "absent"} {
		totals, err := repo.GetTaskUsageTotals(ctx, taskID)
		if err != nil || totals.TotalTokensComplete != (taskID != "task") || totals.EventCount != 0 {
			t.Fatalf("migration fabricated amounts or lost ownership for %s: %+v err=%v", taskID, totals, err)
		}
	}
}
