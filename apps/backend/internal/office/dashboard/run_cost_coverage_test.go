package dashboard_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/models"
)

// @covers AC-OFFICE-COSTS-001.9
func TestRunCostCoverageSurvivesDetailProjection(t *testing.T) {
	cases := []struct {
		name                                      string
		known, gap, zero, missingOutput, unpriced bool
		want                                      map[string]any
	}{
		{name: "gap-only", gap: true, want: map[string]any{"input_tokens": nil, "output_tokens": nil, "cached_tokens": nil, "cost_subcents": nil}},
		{name: "mixed", known: true, gap: true, want: map[string]any{"input_tokens": nil, "output_tokens": nil, "cached_tokens": nil, "cost_subcents": float64(12500)}},
		{name: "measured-zero", known: true, zero: true, want: map[string]any{"input_tokens": float64(0), "output_tokens": float64(0), "cached_tokens": float64(0), "cost_subcents": float64(0)}},
		{name: "missing-output", known: true, missingOutput: true, want: map[string]any{"input_tokens": float64(7), "output_tokens": nil, "cached_tokens": float64(3), "cost_subcents": float64(12500)}},
		{name: "unpriced", known: true, unpriced: true, want: map[string]any{"input_tokens": float64(7), "output_tokens": float64(11), "cached_tokens": float64(3), "cost_subcents": nil}},
		{name: "unobserved", want: map[string]any{"input_tokens": nil, "output_tokens": nil, "cached_tokens": nil, "cost_subcents": nil}},
	}
	for _, owner := range []string{"task", "run-session"} {
		for _, tc := range cases {
			t.Run(owner+"/"+tc.name, func(t *testing.T) {
				deps := newRunDetailDeps(t)
				seedRunDetailAgent(t, deps, "agent-1", "agent-1")
				taskID, sessionID := "task-1", ""
				if owner == "run-session" {
					taskID, sessionID = "", "run-session"
				}
				at := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
				runID := seedRunDetailRun(t, deps, "agent-1", "finished", taskID, at)
				if sessionID != "" {
					_, err := deps.db.Exec(`INSERT INTO office_run_sessions
						(id, workspace_id, agent_profile_id, run_id, attempt, state, created_at)
						VALUES (?, 'ws-1', 'agent-1', ?, 1, 'finished', ?)`, sessionID, runID, at)
					if err != nil {
						t.Fatal(err)
					}
				}
				ctx := context.Background()
				priced, unpriced := models.CostSource("provider_reported"), models.CostSource("unpriced")
				foreignOutput := int64(999)
				if err := deps.repo.CreateCostEvent(ctx, &models.CostEvent{ID: "foreign", SessionID: "other-session", TaskID: "", TokensIn: 999, TokensOut: &foreignOutput, CostSubcents: 999, CostSource: &priced, OccurredAt: at}); err != nil {
					t.Fatal(err)
				}
				if tc.known {
					input, output, cached, cost := int64(7), int64(11), int64(3), int64(12500)
					if tc.zero {
						input, output, cached, cost = 0, 0, 0, 0
					}
					event := &models.CostEvent{ID: "known", SessionID: sessionID, TaskID: taskID, TokensIn: input, TokensOut: &output, TokensCachedIn: &cached, CostSubcents: cost, CostSource: &priced, OccurredAt: at}
					if tc.missingOutput {
						event.TokensOut = nil
					}
					if tc.unpriced {
						event.CostSource, event.CostSubcents = &unpriced, 0
					}
					if err := deps.repo.CreateCostEvent(ctx, event); err != nil {
						t.Fatal(err)
					}
				}
				if tc.gap {
					if err := deps.repo.CreateCostEvent(ctx, &models.CostEvent{ID: "gap", SessionID: sessionID, TaskID: taskID, UsageUnavailable: true, CostSource: &unpriced, OccurredAt: at}); err != nil {
						t.Fatal(err)
					}
				}
				_, rollup, err := deps.repo.GetRunWithCosts(ctx, runID)
				if err != nil {
					t.Fatal(err)
				}
				detail, err := dashboard.GetRunDetail(ctx, deps.repo, "agent-1", runID)
				if err != nil {
					t.Fatal(err)
				}
				for label, projection := range map[string]any{"rollup": rollup, "detail": detail.Costs} {
					encoded, err := json.Marshal(projection)
					if err != nil {
						t.Fatal(err)
					}
					var got map[string]any
					if err := json.Unmarshal(encoded, &got); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(got, tc.want) {
						t.Errorf("%s costs=%s, want %+v", label, encoded, tc.want)
					}
				}
			})
		}
	}
}
