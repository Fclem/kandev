package dashboard_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// @covers AC-OFFICE-COSTS-001.9
func TestAgentSummaryMonetaryAvailability(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		known, gap, unpriced, legacy bool
		amount                       int64
		want                         string
	}{
		{name: "gap-only", gap: true, want: "null"},
		{name: "no-observations", want: "null"},
		{name: "unpriced-only", unpriced: true, want: "null"},
		{name: "mixed-known-gap", known: true, gap: true, amount: 12500, want: "12500"},
		{name: "measured-zero", known: true, want: "0"},
		{name: "legacy-amount", known: true, legacy: true, amount: 7500, want: "7500"},
		{name: "legacy-zero", known: true, legacy: true, want: "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := newTestDeps(t)
			ctx := context.Background()
			at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
			seedSummaryRun(t, deps.db, "run-money", summaryFixtureAgent, "finished", "task-money", at)
			if _, err := deps.db.Exec(`UPDATE runs SET claimed_at=?, finished_at=? WHERE id='run-money'`, at, at.Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			priced, unpriced := models.CostSource("provider_reported"), models.CostSource("unpriced")
			if tc.known || tc.unpriced {
				output := int64(20)
				event := &models.CostEvent{ID: "known", TaskID: "task-money", AgentProfileID: summaryFixtureAgent,
					TokensIn: 5, TokensOut: &output, CostSubcents: tc.amount, CostSource: &priced, OccurredAt: at.Add(time.Minute)}
				if tc.unpriced {
					event.CostSource = &unpriced
				}
				if tc.legacy {
					event.CostSource = nil
				}
				if err := deps.repo.CreateCostEvent(ctx, event); err != nil {
					t.Fatal(err)
				}
			}
			if tc.gap {
				event := &models.CostEvent{ID: "gap", TaskID: "task-money", AgentProfileID: summaryFixtureAgent,
					UsageUnavailable: true, CostSource: &unpriced, OccurredAt: at.Add(2 * time.Minute)}
				if err := deps.repo.CreateCostEvent(ctx, event); err != nil {
					t.Fatal(err)
				}
			}
			aggregate, err := deps.repo.CostAggregateForAgent(ctx, summaryFixtureAgent)
			if err != nil {
				t.Fatal(err)
			}
			assertMonetaryWire(t, "repository aggregate", aggregate.TotalCostSubcents, tc.want)
			recent, err := deps.repo.RecentRunCostsForAgent(ctx, summaryFixtureAgent, 10)
			if err != nil {
				t.Fatal(err)
			}
			observed := tc.known || tc.gap || tc.unpriced
			if observed {
				if len(recent) != 1 || recent[0].RunID != "run-money" {
					t.Fatalf("run coverage = %+v, want run-money", recent)
				}
				assertMonetaryWire(t, "repository recent run", recent[0].CostSubcents, tc.want)
			} else if len(recent) != 0 {
				t.Fatalf("unobserved run acquired cost coverage: %+v", recent)
			}
			w := httptest.NewRecorder()
			deps.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/office/agents/"+summaryFixtureAgent+"/summary", nil))
			if w.Code != http.StatusOK {
				t.Fatalf("summary status %d: %s", w.Code, w.Body.String())
			}
			var response struct {
				Aggregate map[string]json.RawMessage   `json:"cost_aggregate"`
				Recent    []map[string]json.RawMessage `json:"recent_run_costs"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if got := string(response.Aggregate["total_cost_subcents"]); got != tc.want {
				t.Errorf("dashboard aggregate money = %s, want %s", got, tc.want)
			}
			if observed && (len(response.Recent) != 1 || string(response.Recent[0]["cost_subcents"]) != tc.want) {
				t.Errorf("dashboard recent money = %s, want %s", w.Body.String(), tc.want)
			}
		})
	}
}

func assertMonetaryWire(t *testing.T, label string, value any, want string) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != want {
		t.Errorf("%s money = %s, want %s", label, encoded, want)
	}
}
