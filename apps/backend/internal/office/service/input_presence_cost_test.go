package service_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

func TestPromptUsageInputPresencePricing(t *testing.T) {
	for _, measured := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent", true: "measured zero"}[measured], func(t *testing.T) {
			svc, eb := newTestServiceWithBus(t)
			svc.SetPricingLookup(&fakePricingLookup{hit: true, pricing: shared.ModelPricing{
				InputPerMillion: 1_000_000, OutputPerMillion: 2_000_000,
			}})
			createTestAgent(t, svc, "ws-1", "worker-input")
			insertTestTask(t, svc, "task-input", "ws-1")
			setTestTaskAssignee(t, svc, "task-input", "worker-input")
			usage := map[string]any{
				"input_tokens_present": measured, "output_tokens": 20, "output_tokens_present": true,
			}
			if measured {
				usage["input_tokens"] = 0
			}
			ctx := context.Background()
			event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]any{
				"task_id": "task-input", "session_id": "session-input", "agent_id": "claude-acp", "model": "sonnet", "usage": usage,
			})
			if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-input"), event); err != nil {
				t.Fatal(err)
			}
			rows, err := svc.ListCostEvents(ctx, "ws-1")
			if err != nil || len(rows) != 1 {
				t.Fatalf("cost rows = %v, error %v", rows, err)
			}
			row := rows[0]
			wantSource := models.CostSourceUnpriced
			if measured {
				wantSource = models.CostSourceModelsDevList
			}
			if row.CostSource == nil || *row.CostSource != wantSource || row.TokensOut == nil || *row.TokensOut != 20 {
				t.Fatalf("cost source/output = %v/%v, want %s/20", row.CostSource, row.TokensOut, wantSource)
			}
			if measured && row.CostSubcents != 40 {
				t.Fatalf("measured-zero input cost = %d, want 40", row.CostSubcents)
			}
			var wantInput *int64
			if measured {
				wantInput = new(int64(0))
			}
			wire, err := json.Marshal(row)
			if err != nil {
				t.Fatal(err)
			}
			var projection struct {
				Input  *int64 `json:"tokens_in"`
				Output *int64 `json:"tokens_out"`
			}
			if err := json.Unmarshal(wire, &projection); err != nil {
				t.Fatal(err)
			}
			assertOfficeInputOutput(t, "persisted event JSON", projection.Input, projection.Output, wantInput, 20)
			repo := svc.RepoForTest()
			now := time.Now().UTC()
			if _, err := repo.ExecRaw(ctx, `INSERT INTO runs
				(id, agent_profile_id, reason, payload, status, requested_at, claimed_at, finished_at)
				VALUES ('input-run', ?, 'test', '{"task_id":"task-input"}', 'finished', ?, ?, ?)`,
				row.AgentProfileID, now.Add(-time.Hour), now.Add(-time.Hour), now.Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			for _, mixed := range []bool{false, true} {
				output := int64(20)
				if mixed {
					priced := models.CostSourceProviderReported
					if err := repo.CreateCostEvent(ctx, &models.CostEvent{ID: "independent", TaskID: row.TaskID,
						AgentProfileID: row.AgentProfileID, TokensIn: 5, TokensOut: new(int64(7)),
						CostSubcents: 3, CostSource: &priced, OccurredAt: now}); err != nil {
						t.Fatal(err)
					}
					output = 27
					if measured {
						wantInput = new(int64(5))
					}
				}
				agent, err := repo.CostAggregateForAgent(ctx, row.AgentProfileID)
				if err != nil {
					t.Fatal(err)
				}
				assertOfficeInputOutput(t, "agent", agent.InputTokens, agent.OutputTokens, wantInput, output)
				recent, err := repo.RecentRunCostsForAgent(ctx, row.AgentProfileID, 10)
				if err != nil || len(recent) != 1 {
					t.Fatalf("recent=%v err=%v", recent, err)
				}
				assertOfficeInputOutput(t, "recent run", recent[0].InputTokens, recent[0].OutputTokens, wantInput, output)
				_, run, err := repo.GetRunWithCosts(ctx, "input-run")
				if err != nil {
					t.Fatal(err)
				}
				assertOfficeInputOutput(t, "run detail", run.InputTokens, run.OutputTokens, wantInput, output)
				tree, err := repo.GetSubtreeCostSummary(ctx, row.TaskID)
				if err != nil {
					t.Fatal(err)
				}
				assertOfficeInputOutput(t, "subtree", tree.TokensIn, tree.TokensOut, wantInput, output)
				if mixed {
					wantCost := int64(3)
					if measured {
						wantCost = 43
					}
					if tree.CostSubcents == nil || *tree.CostSubcents != wantCost {
						t.Fatalf("independent spend=%v, want %d", tree.CostSubcents, wantCost)
					}
				}
			}
		})
	}
}

func assertOfficeInputOutput(t *testing.T, name string, input, output, wantInput *int64, wantOutput int64) {
	t.Helper()
	if (input == nil) != (wantInput == nil) || (input != nil && *input != *wantInput) ||
		output == nil || *output != wantOutput {
		t.Errorf("%s input/output=%v/%v; want input=%v output=%d", name, input, output, wantInput, wantOutput)
	}
}

func TestPromptUsageRejectsNegativeMeasurements(t *testing.T) {
	for _, category := range []string{
		"input_tokens", "output_tokens", "cached_read_tokens", "cached_write_tokens",
		"thought_tokens", "total_tokens", "provider_reported_cost_subcents",
	} {
		t.Run(category, func(t *testing.T) {
			svc, eb := newTestServiceWithBus(t)
			svc.SetPricingLookup(&fakePricingLookup{hit: true, pricing: shared.ModelPricing{
				InputPerMillion: 1_000_000, OutputPerMillion: 2_000_000,
			}})
			createTestAgent(t, svc, "ws-1", "worker-negative")
			insertTestTask(t, svc, "task-negative", "ws-1")
			setTestTaskAssignee(t, svc, "task-negative", "worker-negative")
			ctx := context.Background()
			for _, invalid := range []bool{true, false} {
				counts := map[string]any{
					"input_tokens": 5, "input_tokens_present": true,
					"output_tokens": 20, "output_tokens_present": true,
				}
				if invalid {
					counts[category] = -1
					counts["provider_reported_cost_present"] = category == "provider_reported_cost_subcents"
				}
				event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]any{
					"task_id": "task-negative", "session_id": "session-negative",
					"agent_id": "claude-acp", "model": "sonnet", "usage": counts,
				})
				if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-negative"), event); err != nil {
					t.Fatal(err)
				}
				rows, err := svc.ListCostEvents(ctx, "ws-1")
				if err != nil {
					t.Fatal(err)
				}
				if invalid && len(rows) != 0 {
					t.Fatalf("negative measurement persisted: %+v", rows)
				}
				if !invalid && (len(rows) != 1 || rows[0].CostSubcents != 45) {
					t.Fatalf("valid sibling control failed: %+v", rows)
				}
			}
		})
	}
}
