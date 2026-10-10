package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/providerlimit"
	"github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	runmodels "github.com/kandev/kandev/internal/runs/models"
)

type providerLimitProfiles struct {
	profile *models.AgentProfile
	agent   *models.Agent
}

func (r providerLimitProfiles) GetAgentProfile(context.Context, string) (*models.AgentProfile, error) {
	return r.profile, nil
}

func (r providerLimitProfiles) GetAgent(context.Context, string) (*models.Agent, error) {
	return r.agent, nil
}

func (r providerLimitProfiles) ListAgents(context.Context) ([]*models.Agent, error) {
	return []*models.Agent{r.agent}, nil
}

func (r providerLimitProfiles) ListAgentProfiles(context.Context, string) ([]*models.AgentProfile, error) {
	return []*models.AgentProfile{r.profile}, nil
}

type officeCircuitPersistence struct {
	mu        sync.Mutex
	rows      map[string]dynamic.CircuitSnapshot
	failBatch bool
	failWrite bool
}

func (p *officeCircuitPersistence) SaveCircuit(_ context.Context, row dynamic.CircuitSnapshot) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failWrite {
		return errors.New("injected circuit write failure")
	}
	if p.rows == nil {
		p.rows = make(map[string]dynamic.CircuitSnapshot)
	}
	p.rows[row.Key] = row
	return nil
}

func (p *officeCircuitPersistence) SaveCircuits(_ context.Context, rows []dynamic.CircuitSnapshot) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failBatch {
		return errors.New("injected circuit batch failure")
	}
	if p.rows == nil {
		p.rows = make(map[string]dynamic.CircuitSnapshot)
	}
	for _, row := range rows {
		p.rows[row.Key] = row
	}
	return nil
}

func (p *officeCircuitPersistence) setFailures(batch, write bool) {
	p.mu.Lock()
	p.failBatch = batch
	p.failWrite = write
	p.mu.Unlock()
}

func (p *officeCircuitPersistence) LoadCircuits(context.Context) ([]dynamic.CircuitSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	rows := make([]dynamic.CircuitSnapshot, 0, len(p.rows))
	for _, row := range p.rows {
		rows = append(rows, row)
	}
	return rows, nil
}

type providerLimitCatalog struct{}

func (providerLimitCatalog) Get(string) (agents.Agent, bool) {
	return agents.NewOmpACP(), true
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-006.2
func TestOfficeLimitFallbackPersistsMarkAndSameRunModelOverride(t *testing.T) {
	profile := &models.AgentProfile{
		ID: "office-profile", AgentID: "agent-one", Name: "Worker", BillingType: "api_key",
		Model: "claude-primary", FallbackModel: "claude-secondary", LimitFallback: true,
	}
	agent := &models.Agent{ID: "agent-one", Name: "omp-acp"}
	profiles := providerLimitProfiles{profile: profile, agent: agent}
	persistence := &officeCircuitPersistence{}
	limits := providerlimit.NewService(
		dynamic.NewCircuitRegistry(dynamic.WithCircuitPersistence(persistence)),
		dynamic.NewCredentialBindingResolver([]byte("provider-limit-test-installation")),
		profiles,
		providerLimitCatalog{},
	)
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize Office repository: %v", err)
	}
	svc := NewService(ServiceOptions{Repo: repo, Logger: logger.Default(), ProviderLimits: limits})
	ctx := context.Background()
	provider := "claude-acp"
	model := "claude-primary"
	run := &runmodels.Run{
		AgentProfileID:     profile.ID,
		Reason:             "task_assigned",
		Payload:            `{}`,
		Status:             runmodels.RunStatusQueued,
		CoalescedCount:     1,
		ResolvedProviderID: &provider,
		ResolvedModel:      &model,
	}
	if err := svc.repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create Office run: %v", err)
	}
	now := time.Now().UTC()
	handled := svc.tryProviderLimitRecovery(ctx, run, "You've hit your rate limit", &streams.ProviderError{
		Source: streams.ProviderErrorSourceACPPrompt, ProviderID: provider,
		Message: "You've hit your rate limit", OccurredAt: now,
	})
	if handled {
		t.Fatal("limit recovery without a routing dispatcher reported a requeue")
	}
	persisted, err := svc.repo.GetRunByID(ctx, run.ID)
	if err != nil {
		t.Fatalf("read Office run: %v", err)
	}
	if persisted.LimitFallbackModel == nil || *persisted.LimitFallbackModel != profile.FallbackModel {
		t.Fatalf("persisted fallback model = %v, want %q", persisted.LimitFallbackModel, profile.FallbackModel)
	}
	subject, err := limits.ResolveSubject(ctx, profile.ID)
	if err != nil {
		t.Fatalf("resolve limit subject: %v", err)
	}
	if mark, ok := limits.Lookup(subject, model); !ok || mark.Key == "" {
		t.Fatalf("provider limit mark = %+v, present=%t; want durable mark", mark, ok)
	}
}
