package costs_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/costs"
	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/models"
)

func TestWorkspaceCostMonetaryCoverage(t *testing.T) {
	for _, owner := range []string{"task", "run"} {
		t.Run(owner, func(t *testing.T) {
			for _, tc := range []struct {
				name                         string
				known, gap, unpriced, legacy bool
				amount                       int64
				want                         string
				degraded                     bool
			}{
				{name: "no-observations", want: "null"},
				{name: "gap-only", gap: true, want: "null", degraded: true},
				{name: "unpriced-only", unpriced: true, want: "null", degraded: true},
				{name: "unpriced-nonzero-placeholder", unpriced: true, amount: 33000, want: "null", degraded: true},
				{name: "mixed-known-gap", known: true, gap: true, amount: 12500, want: "12500", degraded: true},
				{name: "measured-zero", known: true, want: "0"},
				{name: "legacy-amount", known: true, legacy: true, amount: 7500, want: "7500"},
				{name: "legacy-zero", known: true, legacy: true, want: "0"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					svc, repo, execSQL := newBudgetTestService(t)
					insertBudgetTestTask(t, execSQL, "coverage-task", "coverage-ws", "coverage-project")
					createBudgetTestAgent(t, repo, "coverage-ws", "coverage-agent")
					taskID := "coverage-task"
					if owner == "run" {
						taskID = ""
						execSQL(`INSERT INTO runs (id, agent_profile_id, reason, payload, status, requested_at)
					VALUES ('coverage-run', 'coverage-agent', 'routine_dispatch_cron', '{}', 'claimed', CURRENT_TIMESTAMP)`)
						execSQL(`INSERT INTO office_run_sessions (id, workspace_id, agent_profile_id, run_id, attempt, state, created_at)
					VALUES ('coverage-session', 'coverage-ws', 'coverage-agent', 'coverage-run', 1, 'running', CURRENT_TIMESTAMP)`)
					}
					insertBudgetTestTask(t, execSQL, "foreign-task", "foreign-ws", "coverage-project")
					foreignSource := new(models.CostSource("provider_reported"))
					if err := repo.CreateCostEvent(context.Background(), &models.CostEvent{
						ID: "foreign", TaskID: "foreign-task", AgentProfileID: "foreign-agent", CostSubcents: 9999, CostSource: foreignSource,
					}); err != nil {
						t.Fatal(err)
					}
					priced, unpriced := new(models.CostSource("provider_reported")), new(models.CostSource("unpriced"))
					if tc.known || tc.unpriced {
						event := &models.CostEvent{ID: "known", TaskID: taskID, SessionID: "coverage-session", AgentProfileID: "coverage-agent",
							Model: "coverage-model", Provider: "openai", CostSubcents: tc.amount, CostSource: priced}
						if tc.unpriced {
							event.CostSource = unpriced
						}
						if tc.legacy {
							event.CostSource = nil
						}
						if err := repo.CreateCostEvent(context.Background(), event); err != nil {
							t.Fatal(err)
						}
					}
					if tc.gap {
						event := &models.CostEvent{ID: "gap", TaskID: taskID, SessionID: "coverage-session", AgentProfileID: "coverage-agent",
							Model: "coverage-model", Provider: "openai", UsageUnavailable: true, CostSource: unpriced}
						if err := repo.CreateCostEvent(context.Background(), event); err != nil {
							t.Fatal(err)
						}
					}
					gin.SetMode(gin.TestMode)
					router := gin.New()
					costs.NewHandler(svc).RegisterRoutes(router.Group("/api/v1"))
					dashboardSvc := dashboard.NewDashboardService(repo, logger.Default(), &noopActivity{}, &repoAgents{repo: repo}, svc)
					dashboard.RegisterRoutes(router.Group("/api/v1/office"), dashboardSvc, repo, nil, nil, nil, logger.Default())
					dashboardRec := httptest.NewRecorder()
					router.ServeHTTP(dashboardRec, httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/coverage-ws/dashboard", nil))
					if dashboardRec.Code != http.StatusOK {
						t.Fatalf("dashboard: %d %s", dashboardRec.Code, dashboardRec.Body.String())
					}
					var dashboardResponse map[string]json.RawMessage
					if err := json.Unmarshal(dashboardRec.Body.Bytes(), &dashboardResponse); err != nil {
						t.Fatal(err)
					}
					assertWorkspaceCostAmount(t, "dashboard", map[string]json.RawMessage{
						"total_subcents":    dashboardResponse["month_spend_subcents"],
						"coverage_degraded": dashboardResponse["month_spend_coverage_degraded"],
					}, tc.want, tc.degraded)
					for _, endpoint := range []string{"summary", "breakdown", "by-agent", "by-project", "by-model", "by-provider"} {
						rec := httptest.NewRecorder()
						router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/coverage-ws/costs/"+endpoint, nil))
						if rec.Code != http.StatusOK {
							t.Fatalf("%s: %d %s", endpoint, rec.Code, rec.Body.String())
						}
						var response map[string]json.RawMessage
						if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
							t.Fatal(err)
						}
						if endpoint == "summary" || endpoint == "breakdown" {
							assertWorkspaceCostAmount(t, endpoint, response, tc.want, tc.degraded)
						}
						keys := []string{"breakdown"}
						if endpoint == "breakdown" {
							keys = []string{"by_agent", "by_project", "by_model", "by_provider"}
						}
						if endpoint == "summary" {
							continue
						}
						for _, key := range keys {
							var groups []map[string]json.RawMessage
							if err := json.Unmarshal(response[key], &groups); err != nil {
								t.Fatal(err)
							}
							if !tc.known && !tc.gap && !tc.unpriced {
								if len(groups) != 0 {
									t.Fatalf("unexpected observed group: %s", response[key])
								}
								continue
							}
							if len(groups) != 1 {
								t.Fatalf("%s/%s: want the fixture's one observed group, got %s", endpoint, key, response[key])
							}
							assertWorkspaceCostAmount(t, endpoint+"/"+key, groups[0], tc.want, tc.degraded)
						}
					}
				})
			}
		})
	}
}

func assertWorkspaceCostAmount(t *testing.T, label string, row map[string]json.RawMessage, want string, degraded bool) {
	t.Helper()
	if got := string(row["total_subcents"]); got != want {
		t.Errorf("%s money=%s, want %s", label, got, want)
	}
	var got bool
	if err := json.Unmarshal(row["coverage_degraded"], &got); err != nil {
		t.Errorf("%s missing monetary coverage: %v", label, err)
		return
	}
	if got != degraded {
		t.Errorf("%s degraded=%t, want %t", label, got, degraded)
	}
}
