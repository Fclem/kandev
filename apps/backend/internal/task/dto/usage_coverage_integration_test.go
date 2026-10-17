package dto

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	_ "github.com/mattn/go-sqlite3"
)

func TestNativeCoverageGapDurableProjection(t *testing.T) {
	for _, scope := range []string{"direct", "child"} {
		for _, known := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/known=%t", scope, known), func(t *testing.T) {
				ctx := context.Background()
				db, err := sqlx.Open("sqlite3", filepath.Join(t.TempDir(), "usage.db"))
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := db.Close(); err != nil {
						t.Error(err)
					}
				}()
				repo, err := sqliterepo.NewWithDB(db, db, logger.Default())
				if err != nil {
					t.Fatal(err)
				}
				if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws", Name: "usage"}); err != nil {
					t.Fatal(err)
				}
				if err := repo.CreateTask(ctx, &models.Task{ID: "task", WorkspaceID: "ws", Title: "usage", IsEphemeral: true}); err != nil {
					t.Fatal(err)
				}
				if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task"}); err != nil {
					t.Fatal(err)
				}
				if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn", TaskID: "task", TaskSessionID: "session"}); err != nil {
					t.Fatal(err)
				}
				output := int64(2)
				if known {
					if err := repo.CreateTaskUsageEvent(ctx, &models.TaskUsageEvent{
						UsageEventID: "known", TaskID: "task", SessionID: "session", TurnID: "turn",
						NativeScope: "direct", ProviderThreadID: "thread", ProviderTurnID: "native-turn", ProviderResponseID: "known",
						MeasurementSource: "response", UsageCompleteness: "exact", TokensIn: 10, TokensOut: &output, TokensTotal: 12,
						CostSource: "provider_reported", CostSubcents: 45, ContractVersion: 1,
					}); err != nil {
						t.Fatal(err)
					}
				}
				key := `native_usage_gap_direct:"thread":"native-turn"`
				if scope == "child" {
					key = `native_usage_gap_child:"child-thread":"child-turn"`
				}
				updated, _, err := repo.PatchTurnMetadata(ctx, "session", "turn", map[string]interface{}{key: true})
				if err != nil || !updated {
					t.Fatalf("patch=%t, err=%v", updated, err)
				}
				svc := service.NewService(service.Repos{Workspaces: repo, Tasks: repo, Sessions: repo, Turns: repo, Usage: repo}, nil, logger.Default(), service.RepositoryDiscoveryConfig{})
				page, _, err := svc.ListTaskSessionUsageTurns(ctx, "task", "session", 0, 10)
				if err != nil || len(page) != 1 {
					t.Fatalf("page=%#v err=%v", page, err)
				}
				projection := ToUsageTurnDTO(page[0], true)
				if projection.Completeness != "partial" || projection.Direct.OutputComplete != (scope == "child") || projection.Total.OutputComplete || projection.Total.CostSubcents != nil {
					t.Fatalf("missing response coverage advertised complete: %#v", projection)
				}
				expectedRows := 0
				if known {
					expectedRows = 1
				}
				if projection.Total.EventCount != expectedRows || len(projection.Responses) != expectedRows {
					t.Fatalf("coverage changed recorded amounts: %#v", projection)
				}
				if projection.LastResponse != nil {
					t.Fatalf("older measured response labeled latest despite a coverage gap: %#v", projection.LastResponse)
				}
				if known && (projection.Responses[0].OutputTokens == nil || *projection.Responses[0].OutputTokens != 2) {
					t.Fatalf("known amounts changed: %#v", projection)
				}
				detail, err := svc.GetTaskSessionUsageTurn(ctx, "task", "session", "turn")
				if err != nil {
					t.Fatal(err)
				}
				detailProjection := ToUsageTurnDTO(detail, true)
				if detailProjection.Completeness != "partial" || detailProjection.Direct.OutputComplete != (scope == "child") {
					t.Fatalf("detail lost scoped coverage: %#v", detailProjection)
				}
				if detailProjection.LastResponse != nil {
					t.Fatalf("detail labels an older measured response latest: %#v", detailProjection.LastResponse)
				}
				totals, err := svc.GetTaskSessionUsageTotals(ctx, "task", "session")
				if err != nil {
					t.Fatal(err)
				}
				if totals.InputTokensComplete || totals.OutputTokensComplete || totals.TotalTokensComplete {
					t.Fatalf("session totals advertise complete counts despite an amount-free response gap: %#v", totals)
				}
			})
		}
	}
}
