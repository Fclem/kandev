package sqlite_test

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSubtreeMonetaryCoverage(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		known, gap, unpriced, legacy bool
		amount                       interface{}
		want                         interface{}
		degraded                     bool
	}{
		{name: "no observations"},
		{name: "gap only", gap: true, degraded: true},
		{name: "unpriced only", unpriced: true, amount: 0, degraded: true},
		{name: "unpriced nonzero placeholder", unpriced: true, amount: 33000, degraded: true},
		{name: "mixed known and gap", known: true, gap: true, amount: 12500, want: float64(12500), degraded: true},
		{name: "measured zero", known: true, amount: 0, want: float64(0)},
		{name: "legacy amount", known: true, legacy: true, amount: 7500, want: float64(7500)},
		{name: "legacy zero", known: true, legacy: true, amount: 0, want: float64(0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newTreeHoldTestRepo(t)
			ctx := context.Background()
			insertTreeTask(t, repo, "root", "", "TODO")
			insertTreeTask(t, repo, "child", "root", "TODO")
			insertTreeTask(t, repo, "grandchild", "child", "TODO")
			insertTreeTask(t, repo, "archived", "root", "TODO")
			insertTreeTask(t, repo, "foreign", "", "TODO")
			if _, err := repo.ExecRaw(ctx, `UPDATE tasks SET archived_at=CURRENT_TIMESTAMP WHERE id='archived'`); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.ExecRaw(ctx, `INSERT INTO office_cost_events (id,task_id,cost_subcents,cost_source,usage_unavailable,occurred_at,created_at) VALUES ('archived','archived',999999,'provider_reported',0,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),('foreign','foreign',777777,'unpriced',1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err != nil {
				t.Fatal(err)
			}
			if tc.known || tc.unpriced {
				var source interface{} = "provider_reported"
				if tc.unpriced {
					source = "unpriced"
				}
				if tc.legacy {
					source = nil
				}
				if _, err := repo.ExecRaw(ctx, `INSERT INTO office_cost_events (id,task_id,cost_subcents,cost_source,tokens_in,tokens_out,occurred_at,created_at) VALUES ('known','child',?,?,10,20,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, tc.amount, source); err != nil {
					t.Fatal(err)
				}
			}
			if tc.gap {
				if _, err := repo.ExecRaw(ctx, `INSERT INTO office_cost_events (id,task_id,cost_subcents,cost_source,usage_unavailable,occurred_at,created_at) VALUES ('gap','grandchild',0,'unpriced',1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err != nil {
					t.Fatal(err)
				}
			}
			summary, err := repo.GetSubtreeCostSummary(ctx, "root")
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(summary)
			if err != nil {
				t.Fatal(err)
			}
			var wire map[string]interface{}
			if err := json.Unmarshal(encoded, &wire); err != nil {
				t.Fatal(err)
			}
			if wire["cost_subcents"] != tc.want || wire["coverage_degraded"] != tc.degraded || wire["task_count"] != float64(3) {
				t.Fatalf("summary=%s; want money=%v degraded=%t tasks=3", encoded, tc.want, tc.degraded)
			}
		})
	}
}
