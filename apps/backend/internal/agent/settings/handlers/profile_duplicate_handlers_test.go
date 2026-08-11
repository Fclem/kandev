package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/httpmw"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// duplicateRepo is a minimal store.Repository stub: the duplicate path only
// reads one profile, inserts the copy, flips enabled, and reads/writes the
// MCP config row. Everything else is a no-op.
type duplicateRepo struct {
	profiles map[string]*models.AgentProfile
	created  []*models.AgentProfile
}

func newDuplicateRepo() *duplicateRepo {
	return &duplicateRepo{profiles: map[string]*models.AgentProfile{}}
}

func (r *duplicateRepo) GetAgentProfile(_ context.Context, id string) (*models.AgentProfile, error) {
	if p, ok := r.profiles[id]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("agent profile not found: %s", id)
}

func (r *duplicateRepo) GetAgentProfileIncludingDeleted(ctx context.Context, id string) (*models.AgentProfile, error) {
	return r.GetAgentProfile(ctx, id)
}

func (r *duplicateRepo) CreateAgentProfile(_ context.Context, p *models.AgentProfile) error {
	if p.ID == "" {
		p.ID = "duplicate-" + p.Name
	}
	r.profiles[p.ID] = p
	r.created = append(r.created, p)
	return nil
}

func (r *duplicateRepo) UpdateAgentProfileEnabled(_ context.Context, id string, enabled bool) (time.Time, error) {
	p, ok := r.profiles[id]
	if !ok {
		return time.Time{}, fmt.Errorf("agent profile not found: %s", id)
	}
	p.Enabled = enabled
	p.UpdatedAt = time.Now().UTC()
	return p.UpdatedAt, nil
}

func (r *duplicateRepo) GetAgentProfileMcpConfig(context.Context, string) (*models.AgentProfileMcpConfig, error) {
	return nil, nil
}

func (r *duplicateRepo) UpsertAgentProfileMcpConfig(context.Context, *models.AgentProfileMcpConfig) error {
	return nil
}

func (r *duplicateRepo) CreateAgent(context.Context, *models.Agent) error { return nil }
func (r *duplicateRepo) GetAgent(context.Context, string) (*models.Agent, error) {
	return nil, nil
}
func (r *duplicateRepo) GetAgentByName(context.Context, string) (*models.Agent, error) {
	return nil, nil
}
func (r *duplicateRepo) UpdateAgent(context.Context, *models.Agent) error { return nil }
func (r *duplicateRepo) DeleteAgent(context.Context, string) error        { return nil }
func (r *duplicateRepo) ListAgents(context.Context) ([]*models.Agent, error) {
	return nil, nil
}
func (r *duplicateRepo) ListTUIAgents(context.Context) ([]*models.Agent, error) {
	return nil, nil
}
func (r *duplicateRepo) UpdateAgentProfile(context.Context, *models.AgentProfile) error {
	return nil
}
func (r *duplicateRepo) DeleteAgentProfile(context.Context, string) error { return nil }
func (r *duplicateRepo) ListAgentProfiles(context.Context, string) ([]*models.AgentProfile, error) {
	return nil, nil
}
func (r *duplicateRepo) HasDeletedAgentProfiles(context.Context, string) (bool, error) {
	return false, nil
}
func (r *duplicateRepo) Close() error { return nil }

var _ store.Repository = (*duplicateRepo)(nil)

// duplicateHub captures every WS message so tests can assert the broadcast.
type duplicateHub struct {
	mu   sync.Mutex
	msgs []*ws.Message
}

func (h *duplicateHub) Broadcast(msg *ws.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.msgs = append(h.msgs, msg)
}

func (h *duplicateHub) actions() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, 0, len(h.msgs))
	for _, m := range h.msgs {
		out = append(out, m.Action)
	}
	return out
}

func newDuplicateRouter(t *testing.T, repo store.Repository, hub Broadcaster) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	ctrl := controller.NewController(repo, nil, nil, nil, log)
	router := gin.New()
	NewHandlers(ctrl, hub, log, "test-interlock").registerHTTP(router)
	return router
}

func TestDuplicateProfileEndpoint_CopiesAndBroadcasts(t *testing.T) {
	repo := newDuplicateRepo()
	repo.profiles["source-1"] = &models.AgentProfile{
		ID:               "source-1",
		AgentID:          "agent-1",
		Name:             "Default",
		AgentDisplayName: "Claude Code",
		Model:            "claude-sonnet",
		CLIFlags:         []models.CLIFlag{{Description: "Tools", Flag: "--allow-all-tools", Enabled: true}},
		Enabled:          true,
	}
	hub := &duplicateHub{}
	router := newDuplicateRouter(t, repo, hub)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-profiles/source-1/duplicate", nil)
	req.Header.Set(httpmw.InterimSettingsInterlockHeader, "test-interlock")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var created dto.AgentProfileDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.ID == "source-1" || created.ID == "" {
		t.Errorf("response profile ID = %q, want a fresh ID", created.ID)
	}
	if created.Name != "Default Copy" {
		t.Errorf("response name = %q, want %q", created.Name, "Default Copy")
	}
	if created.Model != "claude-sonnet" {
		t.Errorf("response model = %q, want claude-sonnet", created.Model)
	}
	if len(created.CLIFlags) != 1 || created.CLIFlags[0].Flag != "--allow-all-tools" {
		t.Errorf("response cli flags = %+v, want the source entry", created.CLIFlags)
	}

	found := false
	for _, action := range hub.actions() {
		if action == ws.ActionAgentProfileCreated {
			found = true
		}
	}
	if !found {
		t.Errorf("no %s broadcast, got %v", ws.ActionAgentProfileCreated, hub.actions())
	}
	if len(repo.created) != 1 {
		t.Fatalf("stored copies = %d, want 1", len(repo.created))
	}
}

func TestDuplicateProfileEndpoint_NotFound(t *testing.T) {
	router := newDuplicateRouter(t, newDuplicateRepo(), nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-profiles/missing/duplicate", nil)
	req.Header.Set(httpmw.InterimSettingsInterlockHeader, "test-interlock")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "agent profile not found") {
		t.Errorf("body = %q, want agent profile not found message", rec.Body.String())
	}
}

func TestDuplicateProfileEndpoint_RequiresInterlock(t *testing.T) {
	router := newDuplicateRouter(t, newDuplicateRepo(), nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-profiles/source-1/duplicate", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 without interlock token", rec.Code)
	}
}
