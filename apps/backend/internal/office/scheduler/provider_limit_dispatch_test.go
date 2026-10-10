package scheduler_test

import (
	"context"
	"encoding/json"
	"expvar"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agent/runtime/providerlimit"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/office/routing"
	"github.com/kandev/kandev/internal/office/scheduler"
	runmodels "github.com/kandev/kandev/internal/runs/models"
)

type officeLimitProfileRepo struct {
	profile *settingsmodels.AgentProfile
	agent   *settingsmodels.Agent
}

func (r officeLimitProfileRepo) GetAgentProfile(context.Context, string) (*settingsmodels.AgentProfile, error) {
	return r.profile, nil
}

func (r officeLimitProfileRepo) GetAgent(context.Context, string) (*settingsmodels.Agent, error) {
	return r.agent, nil
}

func (r officeLimitProfileRepo) ListAgents(context.Context) ([]*settingsmodels.Agent, error) {
	return []*settingsmodels.Agent{r.agent}, nil
}

func (r officeLimitProfileRepo) ListAgentProfiles(context.Context, string) ([]*settingsmodels.AgentProfile, error) {
	return []*settingsmodels.AgentProfile{r.profile}, nil
}

type officeLimitAgentCatalog struct{}

func (officeLimitAgentCatalog) Get(string) (agents.Agent, bool) {
	return agents.NewOmpACP(), true
}

type officeLimitCircuitPersistence struct {
	mu   sync.Mutex
	rows map[string]dynamic.CircuitSnapshot
}

func (p *officeLimitCircuitPersistence) SaveCircuit(_ context.Context, row dynamic.CircuitSnapshot) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.rows == nil {
		p.rows = make(map[string]dynamic.CircuitSnapshot)
	}
	p.rows[row.Key] = row
	return nil
}

func (p *officeLimitCircuitPersistence) SaveCircuits(ctx context.Context, rows []dynamic.CircuitSnapshot) error {
	for _, row := range rows {
		if err := p.SaveCircuit(ctx, row); err != nil {
			return err
		}
	}
	return nil
}

func (p *officeLimitCircuitPersistence) LoadCircuits(context.Context) ([]dynamic.CircuitSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	rows := make([]dynamic.CircuitSnapshot, 0, len(p.rows))
	for _, row := range p.rows {
		rows = append(rows, row)
	}
	return rows, nil
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-006.4
func TestDispatchWithActiveLimitUsesProfileFallbackOnSameProvider(t *testing.T) {
	repo := newTestRepoSched(t)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp", "codex-acp"})
	starter := newFakeTaskStarter()
	ss := buildScheduler(t, repo, starter)
	profile := &settingsmodels.AgentProfile{
		ID: "claude-acp-profile", AgentID: "agent-one", BillingType: "api_key",
		Model: "claude-acp-bal", FallbackModel: "claude-limit-fallback", LimitFallback: true,
	}
	profileRepo := officeLimitProfileRepo{profile: profile, agent: &settingsmodels.Agent{ID: "agent-one", Name: "omp-acp"}}
	circuits := dynamic.NewCircuitRegistry(dynamic.WithCircuitPersistence(&officeLimitCircuitPersistence{}))
	limits := providerlimit.NewService(
		circuits, dynamic.NewCredentialBindingResolver([]byte("office-provider-limit-test")),
		profileRepo, officeLimitAgentCatalog{},
	)
	subject := limits.ForProfile(profile, agents.NewOmpACP())
	resetAt := time.Now().UTC().Add(time.Hour)
	if _, err := limits.Record(context.Background(), subject, profile.Model,
		&routingerr.Error{Code: routingerr.CodeRateLimited, LimitScope: "model", ResetHint: &resetAt}, time.Now().UTC()); err != nil {
		t.Fatalf("record provider limit: %v", err)
	}
	ss.SetProviderLimitService(limits)
	beforeSwitched := officeLimitMetric(t, "provider_limit_fallback_total", "context=office;outcome=switched")
	run := seedRun(t, repo, `{"task_id":"t-provider-limit-active"}`)
	launched, parked, err := ss.DispatchWithRouting(context.Background(), run, makeAgent(), scheduler.LaunchContext{})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !launched || parked {
		t.Fatalf("launched=%v parked=%v, want profile fallback launch", launched, parked)
	}
	if delta := officeLimitMetric(t, "provider_limit_fallback_total", "context=office;outcome=switched") - beforeSwitched; delta != 1 {
		t.Fatalf("successful fallback metric delta = %d, want 1", delta)
	}
	route := starter.lastCall().route
	if route.ProviderID != "claude-acp" || route.ExecutionProfileID != profile.ID || route.Model != profile.FallbackModel {
		t.Fatalf("active-limit fallback route = %+v", route)
	}
	attempts, err := repo.ListRouteAttempts(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("list route attempts: %v", err)
	}
	if len(attempts) != 1 || attempts[0].RequestedModel != profile.Model ||
		attempts[0].EffectiveModel != profile.FallbackModel || attempts[0].OverrideReason != "provider_limit" {
		t.Fatalf("active-limit route audit = %+v", attempts)
	}
	persisted, err := repo.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("read run: %v", err)
	}
	if persisted.ResolvedProviderID == nil || *persisted.ResolvedProviderID != "claude-acp" ||
		persisted.LimitFallbackModel == nil || *persisted.LimitFallbackModel != profile.FallbackModel {
		t.Fatalf("persisted active-limit route = %+v", persisted)
	}
}
func TestDispatchWithUnadvertisedProviderLimitFallbackCountsTerminalDecision(t *testing.T) {
	repo := newTestRepoSched(t)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp", "codex-acp"})
	starter := newFakeTaskStarter()
	starter.failFor["claude-acp"] = &lifecycle.BootstrapFailure{
		Reason: lifecycle.ModelSelectionReasonRequestedNotAdvertised,
	}
	ss := buildScheduler(t, repo, starter)
	profile := &settingsmodels.AgentProfile{
		ID: "claude-acp-profile", AgentID: "agent-one", BillingType: "api_key",
		Model: "claude-acp-bal", FallbackModel: "claude-limit-fallback", LimitFallback: true,
	}
	profileRepo := officeLimitProfileRepo{profile: profile, agent: &settingsmodels.Agent{ID: "agent-one", Name: "omp-acp"}}
	limits := providerlimit.NewService(
		dynamic.NewCircuitRegistry(dynamic.WithCircuitPersistence(&officeLimitCircuitPersistence{})),
		dynamic.NewCredentialBindingResolver([]byte("office-provider-limit-unadvertised-test")),
		profileRepo, officeLimitAgentCatalog{},
	)
	subject := limits.ForProfile(profile, agents.NewOmpACP())
	resetAt := time.Now().UTC().Add(time.Hour)
	if _, err := limits.Record(context.Background(), subject, profile.Model,
		&routingerr.Error{Code: routingerr.CodeRateLimited, LimitScope: "model", ResetHint: &resetAt}, time.Now().UTC()); err != nil {
		t.Fatalf("record provider limit: %v", err)
	}
	ss.SetProviderLimitService(limits)
	before := officeLimitMetric(t, "provider_limit_fallback_total", "context=office;outcome=not_advertised")
	run := seedRun(t, repo, `{"task_id":"t-provider-limit-unadvertised"}`)
	launched, parked, err := ss.DispatchWithRouting(context.Background(), run, makeAgent(), scheduler.LaunchContext{})
	if err == nil || launched || parked {
		t.Fatalf("unadvertised fallback launched=%t parked=%t error=%v", launched, parked, err)
	}
	if delta := officeLimitMetric(t, "provider_limit_fallback_total", "context=office;outcome=not_advertised") - before; delta != 1 {
		t.Fatalf("unadvertised fallback metric delta = %d, want 1", delta)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-006.5
func TestLiftParkedProviderLimitWaitClaimsExactProbeLease(t *testing.T) {
	repo := newTestRepoSched(t)
	ss := buildScheduler(t, repo, newFakeTaskStarter())
	clock := time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)
	profile := &settingsmodels.AgentProfile{
		ID: "office-profile", AgentID: "agent-one", BillingType: "api_key",
		Model: "claude-primary", ResumeAfterReset: true,
	}
	profileRepo := officeLimitProfileRepo{profile: profile, agent: &settingsmodels.Agent{ID: "agent-one", Name: "omp-acp"}}
	circuits := dynamic.NewCircuitRegistry(
		dynamic.WithCircuitPersistence(&officeLimitCircuitPersistence{}),
		dynamic.WithCircuitClock(func() time.Time { return clock }),
	)
	limits := providerlimit.NewService(
		circuits, dynamic.NewCredentialBindingResolver([]byte("office-provider-limit-test")),
		profileRepo, officeLimitAgentCatalog{},
		providerlimit.WithClock(func() time.Time { return clock }),
	)
	subject := limits.ForProfile(profile, agents.NewOmpACP())
	resetAt := clock.Add(time.Hour)
	mark, err := limits.Record(context.Background(), subject, profile.Model,
		&routingerr.Error{Code: routingerr.CodeQuotaLimited, LimitScope: "model", ResetHint: &resetAt}, clock)
	if err != nil {
		t.Fatalf("record provider limit: %v", err)
	}
	run := seedRun(t, repo, `{"task_id":"t-provider-limit-wait"}`)
	if err := repo.ParkRunForProviderLimit(context.Background(), run.ID, mark.Key, mark.Until); err != nil {
		t.Fatalf("park provider-limit run: %v", err)
	}
	ss.SetProviderLimitService(limits)
	clock = resetAt.Add(time.Second)
	lifted, err := ss.LiftParkedRuns(context.Background(), clock)
	if err != nil {
		t.Fatalf("lift parked provider-limit run: %v", err)
	}
	if lifted != 1 {
		t.Fatalf("lifted runs = %d, want 1", lifted)
	}
	got, err := repo.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("read lifted run: %v", err)
	}
	if got.ProviderLimitWaitKey == nil || *got.ProviderLimitWaitKey != mark.Key || got.ProviderLimitProbe == "" {
		t.Fatalf("lifted provider-limit ownership = %+v", got)
	}
	var lease dynamic.ProbeLease
	if err := json.Unmarshal([]byte(got.ProviderLimitProbe), &lease); err != nil {
		t.Fatalf("decode persisted probe lease: %v", err)
	}
	if lease.Key != mark.Key || !lease.ExpiresAt.Equal(clock.Add(providerlimit.ProbeLifetime)) {
		t.Fatalf("probe lease = %+v, want key %q expiring at %s", lease, mark.Key, clock.Add(providerlimit.ProbeLifetime))
	}
}

func TestExpiredFailedProbeLeaseGoesToDifferentDueWaiter(t *testing.T) {
	repo := newTestRepoSched(t)
	ss := buildScheduler(t, repo, newFakeTaskStarter())
	clock := time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)
	profile := &settingsmodels.AgentProfile{
		ID: "office-profile", AgentID: "agent-one", BillingType: "api_key",
		Model: "claude-primary", ResumeAfterReset: true,
	}
	profileRepo := officeLimitProfileRepo{profile: profile, agent: &settingsmodels.Agent{ID: "agent-one", Name: "omp-acp"}}
	limits := providerlimit.NewService(
		dynamic.NewCircuitRegistry(
			dynamic.WithCircuitPersistence(&officeLimitCircuitPersistence{}),
			dynamic.WithCircuitClock(func() time.Time { return clock }),
		),
		dynamic.NewCredentialBindingResolver([]byte("office-provider-limit-failed-probe")),
		profileRepo, officeLimitAgentCatalog{},
		providerlimit.WithClock(func() time.Time { return clock }),
	)
	subject := limits.ForProfile(profile, agents.NewOmpACP())
	resetAt := clock.Add(-time.Second)
	mark, err := limits.Record(context.Background(), subject, profile.Model,
		&routingerr.Error{Code: routingerr.CodeQuotaLimited, LimitScope: "model", ResetHint: &resetAt}, clock)
	if err != nil {
		t.Fatalf("record provider limit: %v", err)
	}
	lease, acquired, err := limits.AcquireProbe(context.Background(), mark.Key)
	if err != nil || !acquired {
		t.Fatalf("acquire failed-probe lease: acquired=%t error=%v", acquired, err)
	}
	createRun := func(id, taskID string) *runmodels.Run {
		t.Helper()
		run := &runmodels.Run{
			ID: id, AgentProfileID: "agent-1", Reason: "task_assigned",
			Payload: `{"task_id":"` + taskID + `"}`, Status: "queued",
			CoalescedCount: 1, RequestedAt: clock,
		}
		if err := repo.CreateRun(context.Background(), run); err != nil {
			t.Fatalf("create run: %v", err)
		}
		return run
	}
	owner := createRun("provider-limit-failed-owner", "t-provider-limit-failed-owner")
	waitKey, leaseJSON := mark.Key, mustMarshalProviderProbe(t, lease)
	if err := repo.SetRunProviderLimitRecoveryState(context.Background(), owner.ID, nil, &waitKey, leaseJSON); err != nil {
		t.Fatalf("persist failed probe owner: %v", err)
	}
	if err := repo.ParkRunForProviderLimit(context.Background(), owner.ID, waitKey, lease.ExpiresAt); err != nil {
		t.Fatalf("park failed probe owner: %v", err)
	}
	waiter := createRun("provider-limit-next-waiter", "t-provider-limit-next-waiter")
	if err := repo.ParkRunForProviderLimit(context.Background(), waiter.ID, waitKey, resetAt); err != nil {
		t.Fatalf("park next waiter: %v", err)
	}
	ss.SetProviderLimitService(limits)
	clock = lease.ExpiresAt
	lifted, err := ss.LiftParkedRuns(context.Background(), clock)
	if err != nil {
		t.Fatalf("lift expired failed-probe wait: %v", err)
	}
	if lifted != 1 {
		t.Fatalf("lifted runs = %d, want only the different waiter", lifted)
	}
	gotWaiter, err := repo.GetRun(context.Background(), waiter.ID)
	if err != nil {
		t.Fatalf("read next waiter: %v", err)
	}
	if gotWaiter.ProviderLimitProbe == "" || gotWaiter.ProviderLimitWaitKey == nil ||
		*gotWaiter.ProviderLimitWaitKey != mark.Key {
		t.Fatalf("different waiter did not acquire exact mark lease: %+v", gotWaiter)
	}
	gotOwner, err := repo.GetRun(context.Background(), owner.ID)
	if err != nil {
		t.Fatalf("read previous owner: %v", err)
	}
	if gotOwner.ProviderLimitProbe != leaseJSON || gotOwner.ProviderLimitWaitKey == nil ||
		*gotOwner.ProviderLimitWaitKey != mark.Key {
		t.Fatalf("previous lease owner was altered while successor owned mark: %+v", gotOwner)
	}
}

func mustMarshalProviderProbe(t *testing.T, lease dynamic.ProbeLease) string {
	t.Helper()
	encoded, err := json.Marshal(lease)
	if err != nil {
		t.Fatalf("encode provider-limit lease: %v", err)
	}
	return string(encoded)
}

func TestProviderLimitWaitersDoNotDispatchWithRoutingEnabledOrDisabled(t *testing.T) {
	for _, routingEnabled := range []bool{true, false} {
		name := "routing-disabled"
		if routingEnabled {
			name = "routing-enabled"
		}
		t.Run(name, func(t *testing.T) {
			repo := newTestRepoSched(t)
			starter := newFakeTaskStarter()
			ss := buildScheduler(t, repo, starter)
			if routingEnabled {
				seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp"})
			} else {
				ss.SetResolver(nil)
			}
			run := seedRun(t, repo, `{"task_id":"t-provider-limit-wait"}`)
			waitKey := "limit|binding|account"
			if err := repo.SetRunProviderLimitRecoveryState(context.Background(), run.ID, nil, &waitKey, ""); err != nil {
				t.Fatalf("persist wait key: %v", err)
			}
			run.ProviderLimitWaitKey = &waitKey
			launched, parked, err := ss.DispatchWithRouting(
				context.Background(), run, makeAgent(), scheduler.LaunchContext{},
			)
			if err != nil || launched || !parked {
				t.Fatalf("dispatch launched=%t parked=%t error=%v; want blocked without error", launched, parked, err)
			}
			if len(starter.calls) != 0 {
				t.Fatalf("provider-limit waiter launched %d sibling sessions", len(starter.calls))
			}
		})
	}
}
func officeLimitMetric(t *testing.T, name, label string) int64 {
	t.Helper()
	value := expvar.Get(name)
	if value == nil {
		t.Fatalf("expvar counter %q is not registered", name)
	}
	entry := value.(*expvar.Map).Get(label)
	if entry == nil {
		return 0
	}
	return entry.(*expvar.Int).Value()
}
