package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	_ "github.com/mattn/go-sqlite3"
)

func TestHTTPGetCoverageOnlyUsageTurn(t *testing.T) {
	ctx := context.Background()
	db, err := sqlx.Open("sqlite3", filepath.Join(t.TempDir(), "coverage.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	log := logger.Default()
	repo, err := sqliterepo.NewWithDB(db, db, log)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws", Name: "coverage"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "task", WorkspaceID: "ws", Title: "coverage", IsEphemeral: true}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn", TaskID: "task", TaskSessionID: "session"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.PatchTurnMetadata(ctx, "session", "turn", map[string]interface{}{models.NativeUsageGapChildPrefix + `"child":"native"`: true}); err != nil {
		t.Fatal(err)
	}
	svc := service.NewService(service.Repos{Workspaces: repo, Tasks: repo, Sessions: repo, Turns: repo, Usage: repo}, nil, log, service.RepositoryDiscoveryConfig{})
	h := &TaskHandlers{service: svc, logger: log}
	params := gin.Params{{Key: "id", Value: "task"}, {Key: "sessionId", Value: "session"}, {Key: "turnId", Value: "turn"}}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/usage/turns/turn", nil)
	c.Params = params
	h.httpGetTaskSessionUsageTurn(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("coverage detail status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var detail dto.UsageTurnDTO
	if err := json.Unmarshal(recorder.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Completeness != "partial" || detail.Total.EventCount != 0 || detail.Total.CostSubcents != nil || len(detail.Responses) != 0 || !detail.Direct.OutputComplete || detail.Child.OutputComplete {
		t.Fatalf("coverage detail fabricated or lost scope: %#v", detail)
	}
	recorder = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/usage/turns?limit=1", nil)
	c.Params = params
	h.httpGetTaskSessionUsageTurns(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("coverage list status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var page dto.UsageTurnsPageDTO
	if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Turns) != 1 || page.Turns[0].TurnID != "turn" || page.Turns[0].Completeness != "partial" || page.NextCursor == "" {
		t.Fatalf("coverage-only bounded list=%#v", page)
	}
}
