package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Real-repository coverage for the human Web transfer adapter
// (docs/specs/tasks/system-design/cross-workspace-task-transfer.md#human-http-boundary).

const (
	transferHTTPTaskID   = "transfer-http-task"
	transferHTTPOwner    = "user-ana"
	transferHTTPOther    = "user-bruno"
	transferHTTPForeign  = "user-zed"
	transferHTTPSession  = "transfer-http-session"
	transferHTTPTurn     = "transfer-http-turn"
	transferHTTPKey      = "transfer-http-key-1"
	transferHTTPConflict = "task transfer conflict"
)

// transferReplayGate pauses the first repository transfer for one
// idempotency key, modelling a same-key request that already passed service
// authorization and the unserialized replay pre-check.
type transferReplayGate struct {
	*sqliterepo.Repository
	mu       sync.Mutex
	key      string
	admitted chan struct{}
	release  chan struct{}
}

func (g *transferReplayGate) TransferTask(
	ctx context.Context,
	command models.TaskTransferCommand,
) (*models.TaskTransferReceipt, error) {
	g.mu.Lock()
	gated := g.key != "" && command.IdempotencyKey == g.key
	if gated {
		g.key = ""
	}
	g.mu.Unlock()
	if gated {
		close(g.admitted)
		<-g.release
	}
	return g.Repository.TransferTask(ctx, command)
}

func (g *transferReplayGate) arm(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.key = key
	g.admitted = make(chan struct{})
	g.release = make(chan struct{})
}

func (g *transferReplayGate) waitAdmitted(t *testing.T) {
	t.Helper()
	select {
	case <-g.admitted:
	case <-time.After(10 * time.Second):
		t.Fatal("same-key request never reached the repository transfer")
	}
}

type httpTransferFixture struct {
	repo   *sqliterepo.Repository
	gate   *transferReplayGate
	router *gin.Engine
	task   *models.Task
}

func newHTTPTransferFixture(t *testing.T) *httpTransferFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, _ := workflowHandlerTemplate.Open(t)
	repo := sqliterepo.NewWithInitializedDB(db, db, nil)
	log := newTestLogger(t)
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	gate := &transferReplayGate{Repository: repo}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: gate, TaskRepos: repo, Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo, RepositorySets: repo, BranchPolicies: repo,
		RepositoryCleanup: repo, Executors: repo, Environments: repo, TaskEnvironments: repo, Reviews: repo,
		ResourceCleanups: repo, Usage: repo, BackgroundWork: repo,
	}, eventBus, log, service.RepositoryDiscoveryConfig{})
	router := gin.New()
	NewTaskHandlers(svc, nil, repo, nil, log).registerHTTP(router)
	f := &httpTransferFixture{repo: repo, gate: gate, router: router}
	f.seed(t)
	return f
}

func (f *httpTransferFixture) seed(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	for _, workspace := range []*models.Workspace{
		{ID: "ws-a", Name: "Personal", OwnerID: transferHTTPOwner},
		{ID: "ws-b", Name: "Product", OwnerID: transferHTTPOwner},
		{ID: "ws-c", Name: "Foreign", OwnerID: transferHTTPForeign},
	} {
		require.NoError(t, f.repo.CreateWorkspace(ctx, workspace))
	}
	for _, workflow := range []*models.Workflow{
		{ID: "wf-a", WorkspaceID: "ws-a", Name: "Kanban"},
		{ID: "wf-b", WorkspaceID: "ws-b", Name: "Delivery"},
		{ID: "wf-c", WorkspaceID: "ws-c", Name: "Foreign"},
	} {
		require.NoError(t, f.repo.CreateWorkflow(ctx, workflow))
	}
	for _, step := range []struct{ id, workflow, name string }{
		{"step-a-work", "wf-a", "Work"}, {"step-a-blocked", "wf-a", "Blocked"},
		{"step-b-work", "wf-b", "Work"}, {"step-b-review", "wf-b", "Review"},
		{"step-c-work", "wf-c", "Work"},
	} {
		f.exec(t, `INSERT INTO workflow_steps (id, workflow_id, name, position) VALUES (?, ?, ?, 0)`,
			step.id, step.workflow, step.name)
	}
	require.NoError(t, f.repo.CreateTask(ctx, &models.Task{
		ID: transferHTTPTaskID, WorkspaceID: "ws-a", WorkflowID: "wf-a", WorkflowStepID: "step-a-work",
		Title: "Move me", Description: "keep description", State: v1.TaskStateInProgress, Priority: "medium",
	}))
	require.NoError(t, f.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: transferHTTPSession, TaskID: transferHTTPTaskID, State: models.TaskSessionStateRunning, IsPrimary: true,
	}))
	now := time.Now().UTC()
	f.exec(t, `INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`, transferHTTPTurn, transferHTTPSession, transferHTTPTaskID, now, now, now)
	f.exec(t, `INSERT INTO task_plans (id, task_id, title, content, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, "transfer-http-plan", transferHTTPTaskID, "Plan", "plan body", "tester", now, now)
	f.refreshTask(t)
}

func (f *httpTransferFixture) exec(t *testing.T, query string, args ...interface{}) {
	t.Helper()
	_, err := f.repo.DB().Exec(query, args...)
	require.NoError(t, err)
}

func (f *httpTransferFixture) refreshTask(t *testing.T) {
	t.Helper()
	task, err := f.repo.GetTask(context.Background(), transferHTTPTaskID)
	require.NoError(t, err)
	f.task = task
}

func (f *httpTransferFixture) setMember(t *testing.T, workspaceID, userID, role string) {
	t.Helper()
	if role == "" {
		require.NoError(t, f.repo.DeleteWorkspaceMember(context.Background(), workspaceID, userID))
		return
	}
	require.NoError(t, f.repo.UpsertWorkspaceMember(context.Background(), &models.WorkspaceMember{
		WorkspaceID: workspaceID, UserID: userID, Role: role, AddedBy: transferHTTPOwner,
	}))
}

func (f *httpTransferFixture) body(destinationStepID, key string) map[string]any {
	return map[string]any{
		"expected_source_workspace_id":     "ws-a",
		"expected_source_workflow_id":      "wf-a",
		"expected_source_workflow_step_id": "step-a-work",
		"expected_task_updated_at":         f.task.UpdatedAt.Format(time.RFC3339Nano),
		"destination_workspace_id":         "ws-b",
		"destination_workflow_id":          "wf-b",
		"destination_workflow_step_id":     destinationStepID,
		"idempotency_key":                  key,
	}
}

type transferHTTPResponse struct {
	status  int
	headers http.Header
	body    map[string]json.RawMessage
}

func (r transferHTTPResponse) code(t *testing.T) string {
	t.Helper()
	var code string
	if raw, ok := r.body["code"]; ok {
		require.NoError(t, json.Unmarshal(raw, &code))
	}
	return code
}

func (r transferHTTPResponse) receipt(t *testing.T) models.TaskTransferReceipt {
	t.Helper()
	var receipt models.TaskTransferReceipt
	raw, ok := r.body["receipt"]
	require.True(t, ok, "response has no receipt: %v", r.body)
	require.NoError(t, json.Unmarshal(raw, &receipt))
	return receipt
}

func (f *httpTransferFixture) postRaw(t *testing.T, identity *authn.Identity, payload []byte) transferHTTPResponse {
	t.Helper()
	ctx := context.Background()
	if identity != nil {
		ctx = authn.WithIdentity(ctx, *identity)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+transferHTTPTaskID+"/transfer",
		bytes.NewReader(payload)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, request)
	response := transferHTTPResponse{status: recorder.Code, headers: recorder.Header()}
	_ = json.Unmarshal(recorder.Body.Bytes(), &response.body)
	return response
}

func (f *httpTransferFixture) post(t *testing.T, identity authn.Identity, body map[string]any) transferHTTPResponse {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	return f.postRaw(t, &identity, payload)
}

func (f *httpTransferFixture) requireSourcePlacement(t *testing.T, want *models.Task) {
	t.Helper()
	stored, err := f.repo.GetTask(context.Background(), transferHTTPTaskID)
	require.NoError(t, err)
	require.Equal(t, "ws-a", stored.WorkspaceID)
	require.Equal(t, "wf-a", stored.WorkflowID)
	require.Equal(t, "step-a-work", stored.WorkflowStepID)
	require.True(t, stored.UpdatedAt.Equal(want.UpdatedAt), "updated_at changed: %v -> %v", want.UpdatedAt, stored.UpdatedAt)
	sessions, err := f.repo.ListTaskSessions(context.Background(), transferHTTPTaskID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, models.TaskSessionStateRunning, sessions[0].State)
}

func (f *httpTransferFixture) failedAuditRows(t *testing.T, key string) int {
	t.Helper()
	var count int
	require.NoError(t, f.repo.DB().QueryRow(
		`SELECT COUNT(*) FROM task_transfer_audit WHERE idempotency_key = ? AND result = 'failed'`, key,
	).Scan(&count))
	return count
}

func identityFor(userID string) authn.Identity {
	return authn.Identity{UserID: userID, Role: authn.RoleMember}
}

// @covers AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.1 AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.3
func TestHTTPTransferTaskPreservesIdentityAndRunningSession(t *testing.T) {
	f := newHTTPTransferFixture(t)

	response := f.post(t, identityFor(transferHTTPOwner), f.body("step-b-work", transferHTTPKey))

	require.Equal(t, http.StatusOK, response.status, "body: %v", response.body)
	require.Equal(t, "no-store", response.headers.Get("Cache-Control"))
	receipt := response.receipt(t)
	require.Equal(t, transferHTTPTaskID, receipt.TaskID)
	require.Equal(t, "ws-b", receipt.DestinationWorkspaceID)
	require.Equal(t, "step-b-work", receipt.DestinationStepID)
	require.Len(t, receipt.Sessions, 1)
	require.Equal(t, transferHTTPSession, receipt.Sessions[0].ID)
	require.Equal(t, transferHTTPTurn, receipt.Sessions[0].TurnID)

	stored, err := f.repo.GetTask(context.Background(), transferHTTPTaskID)
	require.NoError(t, err)
	require.Equal(t, "ws-b", stored.WorkspaceID)
	require.Equal(t, "wf-b", stored.WorkflowID)
	require.Equal(t, "step-b-work", stored.WorkflowStepID)
	require.Equal(t, "Move me", stored.Title)
	sessions, err := f.repo.ListTaskSessions(context.Background(), transferHTTPTaskID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, transferHTTPSession, sessions[0].ID)
	require.Equal(t, models.TaskSessionStateRunning, sessions[0].State)
	var openTurns, plans int
	require.NoError(t, f.repo.DB().QueryRow(
		`SELECT COUNT(*) FROM task_session_turns WHERE id = ? AND completed_at IS NULL`, transferHTTPTurn).Scan(&openTurns))
	require.NoError(t, f.repo.DB().QueryRow(
		`SELECT COUNT(*) FROM task_plans WHERE task_id = ?`, transferHTTPTaskID).Scan(&plans))
	require.Equal(t, 1, openTurns)
	require.Equal(t, 1, plans)
}

// @covers AC-TASKS-CROSS-WORKSPACE-TRANSFER-002.1 AC-TASKS-CROSS-WORKSPACE-TRANSFER-005.1
func TestHTTPTransferTaskRejectsActorFields(t *testing.T) {
	f := newHTTPTransferFixture(t)
	valid := func() map[string]any { return f.body("step-b-work", transferHTTPKey) }
	tests := []struct {
		name    string
		payload func(t *testing.T) []byte
	}{
		{name: "actor", payload: withField(valid, "actor", map[string]string{"kind": "coordinator", "id": "ceo"})},
		{name: "actor id", payload: withField(valid, "actor_id", transferHTTPOther)},
		{name: "session", payload: withField(valid, "actor_session_id", "session-1")},
		{name: "owner", payload: withField(valid, "authorized_owner_id", transferHTTPOwner)},
		{name: "policy", payload: withField(valid, "preservation_policy", models.TaskTransferPreservationPolicyV1)},
		{name: "audit attempt", payload: withField(valid, "audit_attempt_id", "attempt-1")},
		{name: "entry options", payload: withField(valid, "entry_options", map[string]any{})},
		{name: "agent overrides", payload: withField(valid, "agent_overrides", map[string]any{})},
		{name: "route task", payload: withField(valid, "task_id", "another-task")},
		{name: "step name", payload: withField(valid, "destination_workflow_step_name", "Work")},
		{name: "trailing value", payload: func(t *testing.T) []byte {
			encoded, err := json.Marshal(valid())
			require.NoError(t, err)
			return append(encoded, []byte(` {}`)...)
		}},
		{name: "rounded generation", payload: func(t *testing.T) []byte {
			body := valid()
			body["expected_task_updated_at"] = "not-a-time"
			encoded, err := json.Marshal(body)
			require.NoError(t, err)
			return encoded
		}},
		{name: "missing step", payload: func(t *testing.T) []byte {
			body := valid()
			delete(body, "destination_workflow_step_id")
			encoded, err := json.Marshal(body)
			require.NoError(t, err)
			return encoded
		}},
	}
	identity := identityFor(transferHTTPOwner)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := f.postRaw(t, &identity, tt.payload(t))
			require.Equal(t, http.StatusBadRequest, response.status, "body: %v", response.body)
			require.Empty(t, response.code(t))
			f.requireSourcePlacement(t, f.task)
		})
	}

	t.Run("missing identity is never internal authority", func(t *testing.T) {
		payload, err := json.Marshal(valid())
		require.NoError(t, err)
		response := f.postRaw(t, nil, payload)
		require.Equal(t, http.StatusNotFound, response.status, "body: %v", response.body)
		f.requireSourcePlacement(t, f.task)
	})

	// The rejected bodies never claim the key: the reviewed request can still commit.
	response := f.post(t, identity, valid())
	require.Equal(t, http.StatusOK, response.status, "body: %v", response.body)
}

func withField(valid func() map[string]any, field string, value any) func(t *testing.T) []byte {
	return func(t *testing.T) []byte {
		body := valid()
		body[field] = value
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		return encoded
	}
}

// @covers AC-TASKS-CROSS-WORKSPACE-TRANSFER-005.1 AC-TASKS-CROSS-WORKSPACE-TRANSFER-002.2
func TestHTTPTransferTaskRequiresWriteInBothWorkspaces(t *testing.T) {
	tests := []struct {
		name        string
		actor       string
		source      string
		destination string
		destStep    func(map[string]any)
		wantStatus  int
	}{
		{name: "read-only source", actor: transferHTTPOther, source: "viewer", destination: "collaborator", wantStatus: http.StatusForbidden},
		{name: "read-only destination", actor: transferHTTPOther, source: "collaborator", destination: "viewer", wantStatus: http.StatusForbidden},
		{name: "unreachable destination", actor: transferHTTPOther, source: "collaborator", wantStatus: http.StatusNotFound},
		{name: "unreachable source", actor: transferHTTPOther, destination: "collaborator", wantStatus: http.StatusNotFound},
		{name: "foreign owner destination", actor: transferHTTPOwner, destStep: func(body map[string]any) {
			body["destination_workspace_id"] = "ws-c"
			body["destination_workflow_id"] = "wf-c"
			body["destination_workflow_step_id"] = "step-c-work"
		}, wantStatus: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newHTTPTransferFixture(t)
			if tt.source != "" {
				f.setMember(t, "ws-a", tt.actor, tt.source)
			}
			if tt.destination != "" {
				f.setMember(t, "ws-b", tt.actor, tt.destination)
			}
			// The owner can read the foreign destination, so the boundary
			// result must still be the same-owner non-disclosing unavailable.
			f.setMember(t, "ws-c", transferHTTPOwner, "collaborator")
			body := f.body("step-b-work", transferHTTPKey)
			if tt.destStep != nil {
				tt.destStep(body)
			}
			response := f.post(t, identityFor(tt.actor), body)
			require.Equal(t, tt.wantStatus, response.status, "body: %v", response.body)
			require.Empty(t, response.code(t), "denied caller received a diagnostic reason")
			_, hasReceipt := response.body["receipt"]
			require.False(t, hasReceipt)
			f.requireSourcePlacement(t, f.task)
		})
	}

	f := newHTTPTransferFixture(t)
	f.setMember(t, "ws-a", transferHTTPOther, "collaborator")
	f.setMember(t, "ws-b", transferHTTPOther, "collaborator")
	response := f.post(t, identityFor(transferHTTPOther), f.body("step-b-work", transferHTTPKey))
	require.Equal(t, http.StatusOK, response.status, "body: %v", response.body)
}

// @covers AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.2 AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.4 AC-TASKS-CROSS-WORKSPACE-TRANSFER-005.2
func TestHTTPTransferTaskReportsSafeConflictReasons(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*testing.T, *httpTransferFixture)
		mutate func(map[string]any)
		want   string
	}{
		{name: "stale source generation", mutate: func(body map[string]any) {
			body["expected_task_updated_at"] = "2001-01-01T00:00:00.123456789Z"
		}, want: "source_changed"},
		{name: "stale source step", mutate: func(body map[string]any) {
			body["expected_source_workflow_step_id"] = "step-a-blocked"
		}, want: "source_changed"},
		{name: "missing destination step", mutate: func(body map[string]any) {
			body["destination_workflow_step_id"] = "step-b-missing"
		}, want: "destination_unavailable"},
		{name: "different step name", mutate: func(body map[string]any) {
			body["destination_workflow_step_id"] = "step-b-review"
		}, want: "step_incompatible"},
		{name: "different saved prompt", setup: func(t *testing.T, f *httpTransferFixture) {
			f.exec(t, `UPDATE workflow_steps SET prompt = ? WHERE id = ?`, "different instructions", "step-b-work")
		}, want: "step_incompatible"},
		{name: "full destination", setup: func(t *testing.T, f *httpTransferFixture) {
			f.exec(t, `UPDATE tasks SET wip_admitted = 1, queued_for_step_id = '' WHERE id = ?`, transferHTTPTaskID)
			f.exec(t, `UPDATE workflow_steps SET wip_limit = 1 WHERE id IN (?, ?)`, "step-a-work", "step-b-work")
			require.NoError(t, f.repo.CreateTask(context.Background(), &models.Task{
				ID: "transfer-http-occupant", WorkspaceID: "ws-b", WorkflowID: "wf-b", WorkflowStepID: "step-b-work",
				Title: "Occupant",
			}))
		}, want: "destination_full"},
		{name: "project binding", setup: func(t *testing.T, f *httpTransferFixture) {
			f.exec(t, `UPDATE tasks SET project_id = ? WHERE id = ?`, "project-a", transferHTTPTaskID)
		}, want: "workspace_relation_unmapped"},
		{name: "workspace label", setup: func(t *testing.T, f *httpTransferFixture) {
			f.exec(t, `CREATE TABLE office_labels (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, name TEXT NOT NULL)`)
			f.exec(t, `CREATE TABLE office_task_labels (task_id TEXT NOT NULL, label_id TEXT NOT NULL)`)
			f.exec(t, `INSERT INTO office_labels (id, workspace_id, name) VALUES (?, ?, ?)`, "label-a", "ws-a", "urgent")
			f.exec(t, `INSERT INTO office_task_labels (task_id, label_id) VALUES (?, ?)`, transferHTTPTaskID, "label-a")
		}, want: "workspace_relation_unmapped"},
		{name: "workspace group", setup: func(t *testing.T, f *httpTransferFixture) {
			f.exec(t, `CREATE TABLE task_workspace_groups (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, owner_task_id TEXT NOT NULL)`)
			f.exec(t, `CREATE TABLE task_workspace_group_members (workspace_group_id TEXT NOT NULL, task_id TEXT NOT NULL)`)
			f.exec(t, `INSERT INTO task_workspace_groups (id, workspace_id, owner_task_id) VALUES (?, ?, ?)`,
				"group-a", "ws-a", transferHTTPTaskID)
		}, want: "workspace_relation_unmapped"},
		{name: "workspace profile", setup: func(t *testing.T, f *httpTransferFixture) {
			f.exec(t, `CREATE TABLE agent_profiles (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '',
				role TEXT NOT NULL DEFAULT '', deleted_at TIMESTAMP, enabled INTEGER NOT NULL DEFAULT 1,
				status TEXT NOT NULL DEFAULT 'idle')`)
			f.exec(t, `INSERT INTO agent_profiles (id, workspace_id) VALUES (?, ?)`, "runner-a", "ws-a")
			f.exec(t, `INSERT INTO workflow_step_participants (id, step_id, task_id, role, agent_profile_id,
				decision_required, position) VALUES (?, ?, ?, 'runner', ?, 0, 0)`,
				"runner-seat", "step-a-work", transferHTTPTaskID, "runner-a")
		}, want: "profile_mapping_required"},
		{name: "queued step without destination", setup: func(t *testing.T, f *httpTransferFixture) {
			f.exec(t, `UPDATE tasks SET queued_for_step_id = ? WHERE id = ?`, "step-a-blocked", transferHTTPTaskID)
		}, want: "pending_step_unmapped"},
		{name: "cleanup running", setup: func(t *testing.T, f *httpTransferFixture) {
			now := time.Now().UTC()
			f.exec(t, `INSERT INTO task_resource_cleanup_jobs (id, operation_id, task_id, trigger, state, created_at,
				updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, "cleanup-1", "cleanup-op", transferHTTPTaskID, "archive",
				"running", now, now)
		}, want: "cleanup_active"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newHTTPTransferFixture(t)
			if tt.setup != nil {
				tt.setup(t, f)
				f.refreshTask(t)
			}
			body := f.body("step-b-work", transferHTTPKey)
			if tt.mutate != nil {
				tt.mutate(body)
			}

			response := f.post(t, identityFor(transferHTTPOwner), body)

			require.Equal(t, http.StatusConflict, response.status, "body: %v", response.body)
			require.Len(t, response.body, 2, "conflict body exposes extra fields: %v", response.body)
			require.JSONEq(t, `"`+transferHTTPConflict+`"`, string(response.body["error"]))
			require.Equal(t, tt.want, response.code(t))
			f.requireSourcePlacement(t, f.task)
		})
	}
}

// @covers AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.1 AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.2
func TestHTTPTransferTaskReplaysExactRequest(t *testing.T) {
	f := newHTTPTransferFixture(t)
	owner := identityFor(transferHTTPOwner)
	body := f.body("step-b-work", transferHTTPKey)
	first := f.post(t, owner, body)
	require.Equal(t, http.StatusOK, first.status, "body: %v", first.body)
	operationID := first.receipt(t).OperationID

	// Replay does not depend on mutable lane configuration.
	f.exec(t, `UPDATE workflow_steps SET prompt = ? WHERE id = ?`, "changed after commit", "step-b-work")
	replay := f.post(t, owner, body)
	require.Equal(t, http.StatusOK, replay.status, "body: %v", replay.body)
	require.Equal(t, operationID, replay.receipt(t).OperationID)

	changed := f.body("step-b-review", transferHTTPKey)
	conflict := f.post(t, owner, changed)
	require.Equal(t, http.StatusConflict, conflict.status, "body: %v", conflict.body)
	require.Equal(t, "idempotency_conflict", conflict.code(t))

	f.setMember(t, "ws-a", transferHTTPOther, "collaborator")
	f.setMember(t, "ws-b", transferHTTPOther, "collaborator")
	otherActor := f.post(t, identityFor(transferHTTPOther), body)
	require.Equal(t, http.StatusConflict, otherActor.status, "body: %v", otherActor.body)
	require.Equal(t, "idempotency_conflict", otherActor.code(t))

	again := f.post(t, owner, body)
	require.Equal(t, http.StatusOK, again.status, "body: %v", again.body)
	require.Equal(t, operationID, again.receipt(t).OperationID)
	stored, err := f.repo.GetTask(context.Background(), transferHTTPTaskID)
	require.NoError(t, err)
	require.Equal(t, "step-b-work", stored.WorkflowStepID)

	t.Run("synthetic local identity is stable across retries", func(t *testing.T) {
		local := newHTTPTransferFixture(t)
		synthetic := authn.Identity{UserID: "local-user", Role: authn.RoleAdmin, Synthetic: true}
		localBody := local.body("step-b-work", "synthetic-key")
		committed := local.post(t, synthetic, localBody)
		require.Equal(t, http.StatusOK, committed.status, "body: %v", committed.body)
		retried := local.post(t, synthetic, localBody)
		require.Equal(t, http.StatusOK, retried.status, "body: %v", retried.body)
		require.Equal(t, committed.receipt(t).OperationID, retried.receipt(t).OperationID)
	})
}

// @covers AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.5 AC-TASKS-CROSS-WORKSPACE-TRANSFER-005.3
func TestHTTPTransferTaskCheckAfterPermissionRevocation(t *testing.T) {
	f := newHTTPTransferFixture(t)
	f.setMember(t, "ws-a", transferHTTPOther, "collaborator")
	f.setMember(t, "ws-b", transferHTTPOther, "collaborator")
	bruno := identityFor(transferHTTPOther)
	body := f.body("step-b-work", transferHTTPKey)
	committed := f.post(t, bruno, body)
	require.Equal(t, http.StatusOK, committed.status, "body: %v", committed.body)
	operationID := committed.receipt(t).OperationID

	f.setMember(t, "ws-b", transferHTTPOther, "viewer")
	forbidden := f.post(t, bruno, body)
	require.Equal(t, http.StatusForbidden, forbidden.status, "body: %v", forbidden.body)
	require.Empty(t, forbidden.code(t))

	f.setMember(t, "ws-b", transferHTTPOther, "")
	unavailable := f.post(t, bruno, body)
	require.Equal(t, http.StatusNotFound, unavailable.status, "body: %v", unavailable.body)
	require.Empty(t, unavailable.code(t))

	f.setMember(t, "ws-b", transferHTTPOther, "collaborator")
	restored := f.post(t, bruno, body)
	require.Equal(t, http.StatusOK, restored.status, "body: %v", restored.body)
	require.Equal(t, operationID, restored.receipt(t).OperationID)
}

// @covers AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.5 AC-TASKS-CROSS-WORKSPACE-TRANSFER-005.3
func TestHTTPTransferTaskTerminalRejectedKey(t *testing.T) {
	t.Run("rejection bars every later same-key commit", func(t *testing.T) {
		f := newHTTPTransferFixture(t)
		owner := identityFor(transferHTTPOwner)
		f.exec(t, `UPDATE workflow_steps SET prompt = ? WHERE id = ?`, "different instructions", "step-b-work")
		body := f.body("step-b-work", transferHTTPKey)
		rejected := f.post(t, owner, body)
		require.Equal(t, http.StatusConflict, rejected.status, "body: %v", rejected.body)
		require.Equal(t, "step_incompatible", rejected.code(t))
		require.Equal(t, 1, f.failedAuditRows(t, transferHTTPKey), "rejection is one durable barrier row")

		// The destination becomes compatible, but the rejected operation
		// must never commit afterwards.
		f.exec(t, `UPDATE workflow_steps SET prompt = '' WHERE id = ?`, "step-b-work")
		retried := f.post(t, owner, body)
		require.Equal(t, http.StatusConflict, retried.status, "body: %v", retried.body)
		require.Equal(t, "operation_rejected", retried.code(t))
		f.requireSourcePlacement(t, f.task)

		// A changed body cannot replace or poison the original terminal result.
		changed := f.post(t, owner, f.body("step-b-review", transferHTTPKey))
		require.Equal(t, http.StatusConflict, changed.status, "body: %v", changed.body)
		require.Equal(t, "idempotency_conflict", changed.code(t))
		require.Equal(t, 1, f.failedAuditRows(t, transferHTTPKey))
		original := f.post(t, owner, body)
		require.Equal(t, "operation_rejected", original.code(t))

		fresh := f.post(t, owner, f.body("step-b-work", "transfer-http-key-reviewed"))
		require.Equal(t, http.StatusOK, fresh.status, "body: %v", fresh.body)
	})

	t.Run("lookup failure stays uncertain", func(t *testing.T) {
		f := newHTTPTransferFixture(t)
		owner := identityFor(transferHTTPOwner)
		body := f.body("step-b-work", transferHTTPKey)
		body["expected_source_workflow_step_id"] = "step-a-blocked"
		rejected := f.post(t, owner, body)
		require.Equal(t, "source_changed", rejected.code(t))

		f.exec(t, `ALTER TABLE task_transfer_audit RENAME TO task_transfer_audit_unavailable`)
		uncertain := f.post(t, owner, body)
		require.Equal(t, http.StatusInternalServerError, uncertain.status, "body: %v", uncertain.body)
		require.Empty(t, uncertain.code(t))
		f.exec(t, `ALTER TABLE task_transfer_audit_unavailable RENAME TO task_transfer_audit`)

		settled := f.post(t, owner, body)
		require.Equal(t, http.StatusConflict, settled.status, "body: %v", settled.body)
		require.Equal(t, "operation_rejected", settled.code(t))
	})
}

// @covers AC-TASKS-CROSS-WORKSPACE-TRANSFER-005.3
func TestHTTPTransferTaskLostDomainConflictResponse(t *testing.T) {
	for _, tt := range []struct {
		name   string
		setup  func(*testing.T, *httpTransferFixture, map[string]any)
		repair func(*testing.T, *httpTransferFixture, map[string]any)
		code   string
	}{
		{name: "source changed", setup: func(_ *testing.T, _ *httpTransferFixture, body map[string]any) {
			body["expected_task_updated_at"] = "2001-01-01T00:00:00.123456789Z"
		}, repair: func(_ *testing.T, f *httpTransferFixture, body map[string]any) {
			body["expected_task_updated_at"] = f.task.UpdatedAt.Format(time.RFC3339Nano)
		}, code: "source_changed"},
		{name: "step incompatible", setup: func(t *testing.T, f *httpTransferFixture, _ map[string]any) {
			f.exec(t, `UPDATE workflow_steps SET prompt = ? WHERE id = ?`, "different instructions", "step-b-work")
		}, repair: func(t *testing.T, f *httpTransferFixture, _ map[string]any) {
			f.exec(t, `UPDATE workflow_steps SET prompt = '' WHERE id = ?`, "step-b-work")
		}, code: "step_incompatible"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newHTTPTransferFixture(t)
			owner := identityFor(transferHTTPOwner)
			body := f.body("step-b-work", transferHTTPKey)
			tt.setup(t, f, body)
			lost := f.post(t, owner, body)
			require.Equal(t, tt.code, lost.code(t), "body: %v", lost.body)

			check := f.post(t, owner, body)
			require.Equal(t, http.StatusConflict, check.status, "body: %v", check.body)
			require.Equal(t, "operation_rejected", check.code(t))
			f.requireSourcePlacement(t, f.task)

			tt.repair(t, f, body)
			body["idempotency_key"] = "transfer-http-key-reviewed"
			reviewed := f.post(t, owner, body)
			require.Equal(t, http.StatusOK, reviewed.status, "body: %v", reviewed.body)
			sessions, err := f.repo.ListTaskSessions(context.Background(), transferHTTPTaskID)
			require.NoError(t, err)
			require.Len(t, sessions, 1)
			require.Equal(t, models.TaskSessionStateRunning, sessions[0].State)
		})
	}

	t.Run("writer committed first wins over a later rejection", func(t *testing.T) {
		f := newHTTPTransferFixture(t)
		owner := identityFor(transferHTTPOwner)
		body := f.body("step-b-work", transferHTTPKey)
		f.gate.arm(transferHTTPKey)
		admitted := make(chan transferHTTPResponse, 1)
		go func() { admitted <- f.post(t, owner, body) }()
		f.gate.waitAdmitted(t)

		writer := f.post(t, owner, body)
		require.Equal(t, http.StatusOK, writer.status, "body: %v", writer.body)
		// The placement no longer matches the paused attempt; its committed
		// receipt must still win over a source_changed rejection.
		close(f.gate.release)
		late := <-admitted
		require.Equal(t, http.StatusOK, late.status, "body: %v", late.body)
		require.Equal(t, writer.receipt(t).OperationID, late.receipt(t).OperationID)
		require.Zero(t, f.failedAuditRows(t, transferHTTPKey))
	})

	t.Run("rejection first prevents an admitted writer from committing", func(t *testing.T) {
		f := newHTTPTransferFixture(t)
		owner := identityFor(transferHTTPOwner)
		f.exec(t, `UPDATE workflow_steps SET prompt = ? WHERE id = ?`, "different instructions", "step-b-work")
		body := f.body("step-b-work", transferHTTPKey)
		f.gate.arm(transferHTTPKey)
		admitted := make(chan transferHTTPResponse, 1)
		go func() { admitted <- f.post(t, owner, body) }()
		f.gate.waitAdmitted(t)

		rejection := f.post(t, owner, body)
		require.Equal(t, "step_incompatible", rejection.code(t), "body: %v", rejection.body)
		f.exec(t, `UPDATE workflow_steps SET prompt = '' WHERE id = ?`, "step-b-work")
		close(f.gate.release)
		late := <-admitted
		require.Equal(t, http.StatusConflict, late.status, "body: %v", late.body)
		require.Equal(t, "operation_rejected", late.code(t))
		f.requireSourcePlacement(t, f.task)
	})
}

// @covers AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.3
func TestHTTPTransferTaskCancelledBeforeCommitLeavesTaskUnchanged(t *testing.T) {
	f := newHTTPTransferFixture(t)
	owner := identityFor(transferHTTPOwner)
	body := f.body("step-b-work", transferHTTPKey)
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(authn.WithIdentity(context.Background(), owner))
	cancel()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+transferHTTPTaskID+"/transfer",
		bytes.NewReader(payload)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, request)
	require.NotEqual(t, http.StatusOK, recorder.Code, "cancelled request committed: %s", recorder.Body.String())
	f.requireSourcePlacement(t, f.task)

	// The cancelled attempt never commits later; checking it settles the
	// operation instead of moving the task.
	check := f.post(t, owner, body)
	require.Equal(t, http.StatusConflict, check.status, "body: %v", check.body)
	require.Equal(t, "operation_rejected", check.code(t))
	f.requireSourcePlacement(t, f.task)
}
