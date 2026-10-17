package dto

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/mattn/go-sqlite3"
)

func newUsagePageFixture(t *testing.T) (*sqliterepo.Repository, *sqlx.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := sqlx.Open("sqlite3", filepath.Join(t.TempDir(), "page.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	repo, err := sqliterepo.NewWithDB(db, db, logger.Default())
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws", Name: "page"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "task", WorkspaceID: "ws", Title: "page", IsEphemeral: true}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "foreign", WorkspaceID: "ws", Title: "foreign", IsEphemeral: true}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task"}); err != nil {
		t.Fatal(err)
	}
	output := int64(20)
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("turn-%03d", i)
		if err := repo.CreateTurn(ctx, &models.Turn{ID: id, TaskID: "task", TaskSessionID: "session"}); err != nil {
			t.Fatal(err)
		}
		if i == 99 {
			if _, _, err := repo.PatchTurnMetadata(ctx, "session", id, map[string]interface{}{models.NativeUsageGapChildPrefix + `"child":"native"`: true}); err != nil {
				t.Fatal(err)
			}
			continue
		}
		for j := 0; j < 2; j++ {
			if err := repo.CreateTaskUsageEvent(ctx, &models.TaskUsageEvent{UsageEventID: fmt.Sprintf("%s-%d", id, j), TaskID: "task", SessionID: "session", TurnID: id, TokensIn: 0, TokensOut: &output, TokensTotal: 20, CostSource: "unpriced", ContractVersion: 1}); err != nil {
				t.Fatal(err)
			}
		}
	}
	return repo, db
}

func TestUsagePageBoundsDatabaseReadsAndPreservesCoverage(t *testing.T) {
	repo, db := newUsagePageFixture(t)
	ctx := context.Background()
	var reads atomic.Int64
	connection, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.Raw(func(driverConn any) error {
		driverConn.(*sqlite3.SQLiteConn).RegisterAuthorizer(func(op int, _, _, _ string) int {
			if op == sqlite3.SQLITE_SELECT {
				reads.Add(1)
			}
			return sqlite3.SQLITE_OK
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	svc := service.NewService(service.Repos{Workspaces: repo, Tasks: repo, Sessions: repo, Turns: repo, Usage: repo}, nil, logger.Default(), service.RepositoryDiscoveryConfig{})
	page, cursor, err := svc.ListTaskSessionUsageTurns(ctx, "task", "session", 0, 100)
	if err != nil || len(page) != 100 {
		t.Fatalf("page length=%d error=%v", len(page), err)
	}
	if count := reads.Load(); count > 8 {
		t.Fatalf("100-turn page executes %d SELECTs, want bounded batch reads", count)
	}
	for i, item := range page {
		want := fmt.Sprintf("turn-%03d", 99-i)
		if item.TurnID != want {
			t.Fatalf("page order/cursor changed at %d: %#v", i, item)
		}
		projection := ToUsageTurnDTO(item, true)
		if i == 0 {
			if projection.Completeness != "partial" || projection.Total.EventCount != 0 || !projection.Direct.OutputComplete || projection.Child.OutputComplete {
				t.Fatalf("gap-only scope lost: %#v", projection)
			}
		} else if projection.Total.OutputTokens != 40 || len(projection.Responses) != 2 {
			t.Fatalf("batch lost recorded responses: %#v", projection)
		}
	}
	next, unchanged, err := svc.ListTaskSessionUsageTurns(ctx, "task", "session", cursor, 100)
	if err != nil || len(next) != 0 || unchanged != cursor {
		t.Fatalf("exhaustion changed: next=%#v cursor=%d error=%v", next, unchanged, err)
	}
	if _, err := db.Exec(`UPDATE task_session_turns SET task_id='foreign' WHERE id='turn-099'`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.ListTaskSessionUsageTurns(ctx, "task", "session", 0, 100); !errors.Is(err, models.ErrTaskSessionNotFound) {
		t.Fatalf("foreign turn owner admitted: %v", err)
	}
}

func TestUsagePageLateHistoricalCoverageKeepsNewestTurn(t *testing.T) {
	repo, db := newUsagePageFixture(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	for i, id := range []string{"turn-000", "turn-098"} {
		if _, err := db.Exec(`UPDATE task_session_turns SET created_at = ? WHERE id = ?`, base.Add(time.Duration(i)*time.Hour), id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`DELETE FROM task_usage_turn_index WHERE turn_id NOT IN ('turn-000', 'turn-098')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM task_usage_turn_index WHERE turn_id = 'turn-000'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM task_usage_events WHERE turn_id = 'turn-000'`); err != nil {
		t.Fatal(err)
	}
	key := models.NativeUsageGapDirectPrefix + `"old-child":"old-native"`
	if _, _, err := repo.PatchTurnMetadata(ctx, "session", "turn-000", map[string]interface{}{key: true}); err != nil {
		t.Fatal(err)
	}
	svc := service.NewService(service.Repos{Workspaces: repo, Tasks: repo, Sessions: repo, Turns: repo, Usage: repo}, nil, logger.Default(), service.RepositoryDiscoveryConfig{})
	page, cursor, err := svc.ListTaskSessionUsageTurns(ctx, "task", "session", 0, 1)
	if err != nil || len(page) != 1 || page[0].TurnID != "turn-098" || len(page[0].Events) != 2 {
		t.Fatalf("late old coverage displaced newest turn: %#v error=%v", page, err)
	}
	next, nextCursor, err := svc.ListTaskSessionUsageTurns(ctx, "task", "session", cursor, 1)
	if err != nil || len(next) != 1 || next[0].TurnID != "turn-000" || !next[0].DirectIncomplete || len(next[0].Events) != 0 {
		t.Fatalf("historical coverage is not pageable: %#v error=%v", next, err)
	}
	end, unchanged, err := svc.ListTaskSessionUsageTurns(ctx, "task", "session", nextCursor, 1)
	if err != nil || len(end) != 0 || unchanged != nextCursor {
		t.Fatalf("chronological cursor did not exhaust: %#v cursor=%d error=%v", end, unchanged, err)
	}
}
