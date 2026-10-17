package sqlite_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

func newTreeHoldTestRepo(t *testing.T) *sqlite.Repository {
	t.Helper()
	repo := newTestRepo(t)
	_, err := repo.ExecRaw(context.Background(), `
		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			title TEXT DEFAULT '',
			state TEXT NOT NULL DEFAULT 'TODO',
			parent_id TEXT DEFAULT '',
			is_ephemeral INTEGER DEFAULT 0,
			origin TEXT DEFAULT 'manual',
			checkout_agent_id TEXT,
			checkout_at DATETIME,
			checkout_run_id TEXT,
			archived_at DATETIME,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	return repo
}

func insertTreeTask(t *testing.T, repo *sqlite.Repository, id, parentID, state string) {
	t.Helper()
	_, err := repo.ExecRaw(context.Background(), `
		INSERT INTO tasks (id, workspace_id, title, state, parent_id)
		VALUES (?, 'ws-1', ?, ?, ?)
	`, id, id, state, parentID)
	if err != nil {
		t.Fatalf("insert task %s: %v", id, err)
	}
}

func TestFindSubtreeDeep(t *testing.T) {
	repo := newTreeHoldTestRepo(t)
	ctx := context.Background()
	insertTreeTask(t, repo, "root", "", "TODO")
	insertTreeTask(t, repo, "child", "root", "IN_PROGRESS")
	insertTreeTask(t, repo, "grandchild", "child", "TODO")

	members, err := repo.FindSubtree(ctx, "root")
	if err != nil {
		t.Fatalf("FindSubtree: %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("member count = %d, want 3", len(members))
	}
	depthByID := map[string]int{}
	for _, member := range members {
		depthByID[member.TaskID] = member.Depth
	}
	if depthByID["root"] != 0 || depthByID["child"] != 1 || depthByID["grandchild"] != 2 {
		t.Fatalf("unexpected depths: %+v", depthByID)
	}
}

func TestCreateAndReleaseTreeHold(t *testing.T) {
	repo := newTreeHoldTestRepo(t)
	ctx := context.Background()
	insertTreeTask(t, repo, "root", "", "TODO")
	insertTreeTask(t, repo, "child", "root", "TODO")

	hold := &models.TreeHold{
		ID:          "hold-1",
		WorkspaceID: "ws-1",
		RootTaskID:  "root",
		Mode:        models.TreeHoldModePause,
	}
	if err := repo.CreateTreeHold(ctx, hold); err != nil {
		t.Fatalf("CreateTreeHold: %v", err)
	}
	members := []models.TreeHoldMember{
		{HoldID: hold.ID, TaskID: "root", Depth: 0, TaskStatus: "TODO"},
		{HoldID: hold.ID, TaskID: "child", Depth: 1, TaskStatus: "TODO"},
	}
	if err := repo.CreateTreeHoldMembers(ctx, hold.ID, members); err != nil {
		t.Fatalf("CreateTreeHoldMembers: %v", err)
	}

	active, err := repo.GetActiveHoldForMember(ctx, "child")
	if err != nil {
		t.Fatalf("GetActiveHoldForMember: %v", err)
	}
	if active == nil || active.ID != hold.ID {
		t.Fatalf("active hold = %+v, want %s", active, hold.ID)
	}
	if err := repo.ReleaseHold(ctx, hold.ID, "user:test", "resumed"); err != nil {
		t.Fatalf("ReleaseHold: %v", err)
	}
	active, err = repo.GetActiveHold(ctx, "root", models.TreeHoldModePause)
	if err != nil {
		t.Fatalf("GetActiveHold: %v", err)
	}
	if active != nil {
		t.Fatalf("active hold after release = %+v, want nil", active)
	}
}

func TestGetSubtreeCostSummary(t *testing.T) {
	repo := newTreeHoldTestRepo(t)
	ctx := context.Background()
	insertTreeTask(t, repo, "root", "", "TODO")
	insertTreeTask(t, repo, "child-a", "root", "TODO")
	insertTreeTask(t, repo, "child-b", "root", "TODO")

	for _, taskID := range []string{"root", "child-a", "child-b"} {
		_, err := repo.ExecRaw(ctx, `
			INSERT INTO office_cost_events (
				id, task_id, cost_subcents, tokens_in, tokens_cached_in, tokens_out, occurred_at, created_at
			) VALUES (?, ?, 10, 100, 20, 5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, "cost-"+taskID, taskID)
		if err != nil {
			t.Fatalf("insert cost %s: %v", taskID, err)
		}
	}

	_, err := repo.ExecRaw(ctx, `
		INSERT INTO office_cost_events (
			id, task_id, cost_subcents, tokens_in, tokens_cached_in, tokens_out, occurred_at, created_at
		) VALUES (?, ?, 10, 100, 20, NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, "cost-root-unmeasured-out", "root")
	if err != nil {
		t.Fatalf("insert unmeasured-out cost: %v", err)
	}

	summary, err := repo.GetSubtreeCostSummary(ctx, "root")
	if err != nil {
		t.Fatalf("GetSubtreeCostSummary: %v", err)
	}
	if summary.TaskCount != 3 || summary.CostSubcents == nil || *summary.CostSubcents != 40 {
		t.Fatalf("summary = %+v, want 3 distinct tasks (extra row is a 2nd event on root) and 40 subcents", summary)
	}
	if summary.TokensIn == nil || *summary.TokensIn != 400 ||
		summary.TokensCachedIn == nil || *summary.TokensCachedIn != 80 {
		t.Fatalf("unexpected input/cache totals: %+v", summary)
	}
	if summary.TokensOut != nil {
		t.Fatalf("unmeasured output must withhold the aggregate, got %d", *summary.TokensOut)
	}
}

func TestSubtreeTokenCompleteness(t *testing.T) {
	for _, test := range []struct {
		name    string
		gap     bool
		known   bool
		missing bool
		zero    bool
		wantIn  interface{}
		wantOut interface{}
	}{
		{name: "gap only", gap: true},
		{name: "mixed measured and gap", gap: true, known: true},
		{name: "missing output", known: true, missing: true, wantIn: float64(10)},
		{name: "measured zero", known: true, zero: true, wantIn: float64(0), wantOut: float64(0)},
		{name: "measured", known: true, wantIn: float64(10), wantOut: float64(20)},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newTreeHoldTestRepo(t)
			ctx := context.Background()
			insertTreeTask(t, repo, "root", "", "TODO")
			insertTreeTask(t, repo, "child", "root", "TODO")
			var wantCost interface{}
			if test.known {
				input, output := int64(10), interface{}(int64(20))
				if test.zero {
					input, output = 0, int64(0)
				}
				if test.missing {
					output = nil
				}
				_, err := repo.ExecRaw(ctx, `INSERT INTO office_cost_events
					(id, task_id, tokens_in, tokens_out, cost_subcents, occurred_at, created_at)
					VALUES ('measured', 'root', ?, ?, 12500, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, input, output)
				if err != nil {
					t.Fatal(err)
				}
				wantCost = float64(12500)
			}
			if test.gap {
				_, err := repo.ExecRaw(ctx, `INSERT INTO office_cost_events
					(id, task_id, tokens_in, tokens_out, tokens_cached_in, cost_subcents, usage_unavailable, occurred_at, created_at)
					VALUES ('gap', 'child', 0, ?, ?, 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			summary, err := repo.GetSubtreeCostSummary(ctx, "root")
			if err != nil {
				t.Fatal(err)
			}
			wire, err := json.Marshal(summary)
			if err != nil {
				t.Fatal(err)
			}
			var projection map[string]interface{}
			if err := json.Unmarshal(wire, &projection); err != nil {
				t.Fatal(err)
			}
			if projection["tokens_in"] != test.wantIn || projection["tokens_out"] != test.wantOut ||
				projection["cost_subcents"] != wantCost || projection["task_count"] != float64(2) {
				t.Fatalf("subtree projection = %s; want input=%v output=%v cost=%v", wire, test.wantIn, test.wantOut, wantCost)
			}
		})
	}
}
