package service

import (
	"context"
	"encoding/json"
	"expvar"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/agents"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/providerlimit"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	runmodels "github.com/kandev/kandev/internal/runs/models"
)

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-006.6
func TestOfficeTrustedResetParksRunWithOpaqueWaitKey(t *testing.T) {
	profile := &models.AgentProfile{
		ID: "office-profile", AgentID: "agent-one", Name: "Worker", BillingType: "api_key",
		Model: "claude-primary", ResumeAfterReset: true,
	}
	agent := &models.Agent{ID: "agent-one", Name: "omp-acp"}
	profiles := providerLimitProfiles{profile: profile, agent: agent}
	limits := providerlimit.NewService(
		dynamic.NewCircuitRegistry(dynamic.WithCircuitPersistence(&officeCircuitPersistence{})),
		dynamic.NewCredentialBindingResolver([]byte("provider-limit-wait-installation")),
		profiles, providerLimitCatalog{},
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
	model := profile.Model
	run := &runmodels.Run{
		AgentProfileID: profile.ID, Reason: "task_assigned", Payload: `{}`,
		Status: runmodels.RunStatusQueued, CoalescedCount: 1,
		ResolvedProviderID: &provider, ResolvedModel: &model,
	}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create Office run: %v", err)
	}
	resetAt := time.Now().UTC().Add(time.Hour)
	waitBefore := officeProviderLimitMetric(t, "provider_limit_waits_total", "context=office;outcome=armed")
	handled := svc.tryProviderLimitRecovery(ctx, run, "You've hit your rate limit", &streams.ProviderError{
		Source: streams.ProviderErrorSourceACPPrompt, ProviderID: provider,
		Message: "You've hit your rate limit", OccurredAt: time.Now().UTC(), ResetAt: &resetAt,
	})
	if !handled {
		t.Fatal("trusted-reset recovery should park the run")
	}
	if delta := officeProviderLimitMetric(t, "provider_limit_waits_total", "context=office;outcome=armed") - waitBefore; delta != 1 {
		t.Fatalf("trusted wait arm metric delta = %d, want 1", delta)
	}
	got, err := repo.GetRunByID(ctx, run.ID)
	if err != nil {
		t.Fatalf("read parked run: %v", err)
	}
	if got.ProviderLimitWaitKey == nil || *got.ProviderLimitWaitKey == "" ||
		got.RoutingBlockedStatus == nil || *got.RoutingBlockedStatus != runmodels.RoutingBlockedWaitingForCapacity ||
		got.EarliestRetryAt == nil || !got.EarliestRetryAt.Equal(resetAt) {
		t.Fatalf("provider-limit wait state = %+v", got)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-006.7
func TestSuccessfulProviderLimitProbeReleasesLeaseAndWakesSiblings(t *testing.T) {
	profile := &models.AgentProfile{
		ID: "office-profile", AgentID: "agent-one", Name: "Worker", BillingType: "api_key",
		Model: "claude-primary", ResumeAfterReset: true,
	}
	profiles := providerLimitProfiles{profile: profile, agent: &models.Agent{ID: "agent-one", Name: "omp-acp"}}
	limits := providerlimit.NewService(
		dynamic.NewCircuitRegistry(dynamic.WithCircuitPersistence(&officeCircuitPersistence{})),
		dynamic.NewCredentialBindingResolver([]byte("provider-limit-owner-installation")),
		profiles, providerLimitCatalog{},
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
	now := time.Now().UTC()
	resetAt := now.Add(-time.Second)
	health := officemodels.ProviderHealth{
		WorkspaceID: "ws-office", ProviderID: "claude-acp",
		Scope: sqlite.HealthScopeProvider, State: sqlite.HealthStateDegraded,
		ErrorCode: string(routingerr.CodeRateLimited),
	}
	if err := repo.MarkProviderDegraded(ctx, health); err != nil {
		t.Fatalf("mark Office provider health: %v", err)
	}
	subject := limits.ForProfile(profile, agents.NewOmpACP())
	mark, err := limits.Record(ctx, subject, profile.Model,
		&routingerr.Error{Code: routingerr.CodeQuotaLimited, LimitScope: "model", ResetHint: &resetAt}, now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("record provider limit: %v", err)
	}
	lease, acquired, err := limits.AcquireProbe(ctx, mark.Key)
	if err != nil || !acquired {
		t.Fatalf("acquire probe: acquired=%t error=%v", acquired, err)
	}
	leaseJSON, err := json.Marshal(lease)
	if err != nil {
		t.Fatalf("encode probe lease: %v", err)
	}
	createRun := func() *runmodels.Run {
		t.Helper()
		run := &runmodels.Run{
			AgentProfileID: profile.ID, Reason: "task_assigned", Payload: `{}`,
			Status: runmodels.RunStatusQueued, CoalescedCount: 1,
		}
		if err := repo.CreateRun(ctx, run); err != nil {
			t.Fatalf("create run: %v", err)
		}
		return run
	}
	provider, model := "claude-acp", profile.Model
	owner := createRun()
	owner.ResolvedProviderID, owner.ResolvedModel = &provider, &model
	waitKey := mark.Key
	probe := string(leaseJSON)
	owner.ProviderLimitWaitKey = &waitKey
	owner.ProviderLimitProbe = probe
	if err := repo.SetRunResolvedRoute(ctx, owner.ID, profile.ID, provider, model); err != nil {
		t.Fatalf("persist resolved route: %v", err)
	}
	if err := repo.SetRunProviderLimitRecoveryState(ctx, owner.ID, nil, &waitKey, probe); err != nil {
		t.Fatalf("persist probe owner: %v", err)
	}
	if _, err := db.Exec(`UPDATE runs SET status = 'claimed' WHERE id = ?`, owner.ID); err != nil {
		t.Fatalf("claim probe owner: %v", err)
	}
	wrote, err := svc.FinishRun(ctx, owner.ID, RunOutcomeProcessed)
	if err != nil || !wrote {
		t.Fatalf("finish probe owner: wrote=%t error=%v", wrote, err)
	}
	svc.AppendRunEvent(ctx, owner.ID, "complete", "info", map[string]interface{}{})
	waiter := createRun()
	if err := repo.ParkRunForProviderLimit(ctx, waiter.ID, mark.Key, resetAt); err != nil {
		t.Fatalf("park sibling waiter: %v", err)
	}
	resumedBefore := officeProviderLimitMetric(t, "provider_limit_waits_total", "context=office;outcome=resumed")
	if err := svc.ReconcileProviderLimitProbeOwners(ctx); err != nil {
		t.Fatalf("reconcile successful probe owner: %v", err)
	}
	if delta := officeProviderLimitMetric(t, "provider_limit_waits_total", "context=office;outcome=resumed") - resumedBefore; delta != 1 {
		t.Fatalf("successful probe resume metric delta = %d, want 1", delta)
	}
	gotOwner, err := repo.GetRunByID(ctx, owner.ID)
	if err != nil {
		t.Fatalf("read successful owner: %v", err)
	}
	gotWaiter, err := repo.GetRunByID(ctx, waiter.ID)
	if err != nil {
		t.Fatalf("read sibling waiter: %v", err)
	}
	if gotOwner.ProviderLimitWaitKey != nil || gotOwner.ProviderLimitProbe != "" {
		t.Fatalf("successful owner retained probe state: %+v", gotOwner)
	}
	if gotWaiter.ProviderLimitWaitKey != nil || gotWaiter.RoutingBlockedStatus != nil {
		t.Fatalf("successful probe did not wake sibling: %+v", gotWaiter)
	}
	if remaining, exists := limits.Get(mark.Key); exists {
		t.Fatalf("successful probe retained its provider-limit mark and lease: %+v", remaining)
	}
	healthAfter, err := repo.GetProviderHealth(ctx, "ws-office", "claude-acp", sqlite.HealthScopeProvider, "")
	if err != nil || healthAfter == nil || healthAfter.State != sqlite.HealthStateDegraded {
		t.Fatalf("provider-limit clear changed independent Office health state: %+v error=%v", healthAfter, err)
	}
}

func TestSuccessfulProbeCleanupFailuresKeepOwnerAndWaitersRetryable(t *testing.T) {
	for _, failure := range []string{"mark clear", "probe release", "owner transaction", "stopped completion"} {
		t.Run(failure, func(t *testing.T) {
			ctx := context.Background()
			profile := &models.AgentProfile{
				ID: "office-profile", AgentID: "agent-one", Name: "Worker", BillingType: "api_key",
				Model: "claude-primary", ResumeAfterReset: true,
			}
			persistence := &officeCircuitPersistence{}
			limits := providerlimit.NewService(
				dynamic.NewCircuitRegistry(dynamic.WithCircuitPersistence(persistence)),
				dynamic.NewCredentialBindingResolver([]byte("provider-limit-cleanup-installation")),
				providerLimitProfiles{profile: profile, agent: &models.Agent{ID: "agent-one", Name: "omp-acp"}},
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
			now := time.Now().UTC()
			subject := limits.ForProfile(profile, agents.NewOmpACP())
			resetAt := now.Add(-time.Second)
			mark, err := limits.Record(ctx, subject, profile.Model,
				&routingerr.Error{Code: routingerr.CodeQuotaLimited, LimitScope: "model", ResetHint: &resetAt},
				now.Add(-time.Hour))
			if err != nil {
				t.Fatalf("record provider limit: %v", err)
			}
			lease, acquired, err := limits.AcquireProbe(ctx, mark.Key)
			if err != nil || !acquired {
				t.Fatalf("acquire probe: acquired=%t error=%v", acquired, err)
			}
			leaseJSON, err := json.Marshal(lease)
			if err != nil {
				t.Fatalf("encode probe lease: %v", err)
			}
			createRun := func() *runmodels.Run {
				t.Helper()
				run := &runmodels.Run{
					AgentProfileID: profile.ID, Reason: "task_assigned", Payload: `{}`,
					Status: runmodels.RunStatusQueued, CoalescedCount: 1,
				}
				if err := repo.CreateRun(ctx, run); err != nil {
					t.Fatalf("create run: %v", err)
				}
				return run
			}
			provider, model := "claude-acp", profile.Model
			owner := createRun()
			owner.ResolvedProviderID, owner.ResolvedModel = &provider, &model
			waitKey, probe := mark.Key, string(leaseJSON)
			if err := repo.SetRunResolvedRoute(ctx, owner.ID, profile.ID, provider, model); err != nil {
				t.Fatalf("persist owner execution route: %v", err)
			}
			if err := repo.SetRunProviderLimitRecoveryState(ctx, owner.ID, nil, &waitKey, probe); err != nil {
				t.Fatalf("persist probe owner: %v", err)
			}
			if _, err := db.Exec(`UPDATE runs SET status = 'claimed' WHERE id = ?`, owner.ID); err != nil {
				t.Fatalf("claim probe owner: %v", err)
			}
			wrote, err := svc.FinishRun(ctx, owner.ID, RunOutcomeProcessed)
			if err != nil || !wrote {
				t.Fatalf("finish probe owner: wrote=%t error=%v", wrote, err)
			}
			completion := map[string]interface{}{}
			if failure == "stopped completion" {
				completion["stopped"] = true
			}
			svc.AppendRunEvent(ctx, owner.ID, "complete", "info", completion)
			waiter := createRun()
			if err := repo.ParkRunForProviderLimit(ctx, waiter.ID, mark.Key, now.Add(time.Hour)); err != nil {
				t.Fatalf("park sibling waiter: %v", err)
			}

			switch failure {
			case "mark clear":
				persistence.setFailures(true, false)
			case "probe release":
				persistence.setFailures(false, true)
			case "owner transaction":
				if _, err := db.Exec(`
					CREATE TRIGGER fail_probe_owner_cleanup
					BEFORE UPDATE OF provider_limit_probe ON runs
					WHEN OLD.provider_limit_probe <> '' AND NEW.provider_limit_probe = ''
					BEGIN SELECT RAISE(ABORT, 'injected owner cleanup failure'); END
				`); err != nil {
					t.Fatalf("install cleanup failure trigger: %v", err)
				}
			}
			restartedCircuits := dynamic.NewCircuitRegistry(dynamic.WithCircuitPersistence(persistence))
			if err := restartedCircuits.Restore(ctx); err != nil {
				t.Fatalf("restore provider-limit circuits: %v", err)
			}
			limits = providerlimit.NewService(
				restartedCircuits,
				dynamic.NewCredentialBindingResolver([]byte("provider-limit-cleanup-installation")),
				providerLimitProfiles{profile: profile, agent: &models.Agent{ID: "agent-one", Name: "omp-acp"}},
				providerLimitCatalog{},
			)
			svc = NewService(ServiceOptions{Repo: repo, Logger: logger.Default(), ProviderLimits: limits})
			reconcileErr := svc.ReconcileProviderLimitProbeOwners(ctx)
			if failure == "stopped completion" {
				if reconcileErr != nil {
					t.Fatalf("stopped completion reconciliation: %v", reconcileErr)
				}
			} else if reconcileErr == nil {
				t.Fatal("injected cleanup failure was reported as successful")
			}
			assertRetained := func() {
				t.Helper()
				gotOwner, err := repo.GetRunByID(ctx, owner.ID)
				if err != nil {
					t.Fatalf("read owner: %v", err)
				}
				gotWaiter, err := repo.GetRunByID(ctx, waiter.ID)
				if err != nil {
					t.Fatalf("read waiter: %v", err)
				}
				if gotOwner.ProviderLimitWaitKey == nil || gotOwner.ProviderLimitProbe == "" {
					t.Fatalf("owner lost recovery ownership after failure: %+v", gotOwner)
				}
				if gotWaiter.ProviderLimitWaitKey == nil || gotWaiter.RoutingBlockedStatus == nil {
					t.Fatalf("waiter was released before owner cleanup: %+v", gotWaiter)
				}
			}
			assertRetained()
			if failure == "stopped completion" {
				state, exists := limits.Get(mark.Key)
				if !exists || state.Cleared || !state.ProbeUntil.Equal(lease.ExpiresAt) {
					t.Fatalf("stopped probe changed provider-limit mark or lease: %+v, present=%t", state, exists)
				}
				return
			}
			if failure == "mark clear" {
				state, exists := limits.Get(mark.Key)
				if !exists || state.Cleared || state.ProbeUntil.IsZero() {
					t.Fatalf("failed mark clear lost its active probe mark: %+v, present=%t", state, exists)
				}
			}
			persistence.setFailures(false, false)
			if failure == "owner transaction" {
				if _, err := db.Exec(`DROP TRIGGER fail_probe_owner_cleanup`); err != nil {
					t.Fatalf("remove cleanup failure trigger: %v", err)
				}
			}
			if err := svc.ReconcileProviderLimitProbeOwners(ctx); err != nil {
				t.Fatalf("retry successful cleanup: %v", err)
			}
			gotOwner, err := repo.GetRunByID(ctx, owner.ID)
			if err != nil {
				t.Fatalf("read cleaned owner: %v", err)
			}
			gotWaiter, err := repo.GetRunByID(ctx, waiter.ID)
			if err != nil {
				t.Fatalf("read awakened waiter: %v", err)
			}
			if gotOwner.ProviderLimitWaitKey != nil || gotOwner.ProviderLimitProbe != "" {
				t.Fatalf("cleanup retry retained owner: %+v", gotOwner)
			}
			if gotWaiter.ProviderLimitWaitKey != nil || gotWaiter.RoutingBlockedStatus != nil {
				t.Fatalf("cleanup retry did not wake waiter: %+v", gotWaiter)
			}
		})
	}
}

func TestProviderLimitResetTrustBoundariesDoNotPark(t *testing.T) {
	for _, test := range []struct {
		name        string
		resume      bool
		resetOffset time.Duration
		resetKnown  bool
		writeFail   bool
		wantMark    bool
	}{
		{name: "resume disabled", resetOffset: time.Hour, resetKnown: true, wantMark: true},
		{name: "unknown reset", resume: true, wantMark: true},
		{name: "beyond seven days", resume: true, resetOffset: 8 * 24 * time.Hour, resetKnown: true, wantMark: true},
		{name: "durable mark write failure", resume: true, resetOffset: time.Hour, resetKnown: true, writeFail: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			profile := &models.AgentProfile{
				ID: "office-profile", AgentID: "agent-one", Name: "Worker", BillingType: "api_key",
				Model: "claude-primary", ResumeAfterReset: test.resume, LimitFallback: true,
			}
			persistence := &officeCircuitPersistence{}
			limits := providerlimit.NewService(
				dynamic.NewCircuitRegistry(dynamic.WithCircuitPersistence(persistence)),
				dynamic.NewCredentialBindingResolver([]byte("provider-limit-reset-boundary-installation")),
				providerLimitProfiles{profile: profile, agent: &models.Agent{ID: "agent-one", Name: "omp-acp"}},
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
			if test.writeFail {
				persistence.setFailures(false, true)
			}
			provider, model := "claude-acp", profile.Model
			run := &runmodels.Run{
				AgentProfileID: profile.ID, Reason: "task_assigned", Payload: `{}`,
				Status: runmodels.RunStatusQueued, CoalescedCount: 1,
				ResolvedProviderID: &provider, ResolvedModel: &model,
			}
			if err := repo.CreateRun(context.Background(), run); err != nil {
				t.Fatalf("create run: %v", err)
			}
			if err := repo.SetRunResolvedRoute(context.Background(), run.ID, profile.ID, provider, model); err != nil {
				t.Fatalf("persist resolved route: %v", err)
			}
			var resetAt *time.Time
			if test.resetKnown {
				reset := time.Now().UTC().Add(test.resetOffset)
				resetAt = &reset
			}
			providerError := &streams.ProviderError{
				Source: streams.ProviderErrorSourceACPPrompt, ProviderID: provider,
				Message: "You've hit your rate limit", OccurredAt: time.Now().UTC(), ResetAt: resetAt,
			}
			if svc.tryProviderLimitRecovery(context.Background(), run, providerError.Message, providerError) {
				t.Fatal("untrusted or disabled reset parked the run")
			}
			got, err := repo.GetRunByID(context.Background(), run.ID)
			if err != nil {
				t.Fatalf("read run: %v", err)
			}
			if got.ProviderLimitWaitKey != nil || got.RoutingBlockedStatus != nil {
				t.Fatalf("run was parked without a trusted reset: %+v", got)
			}
			subject := limits.ForProfile(profile, agents.NewOmpACP())
			mark, limited := limits.Lookup(subject, model)
			if limited != test.wantMark {
				t.Fatalf("mark present = %t, want %t", limited, test.wantMark)
			}
			if limited && mark.ResetKnown != test.resetKnown {
				t.Fatalf("reset-known = %t, want %t", mark.ResetKnown, test.resetKnown)
			}
		})
	}
}

func TestNonLimitFailureKeepsProbeLeaseAndParksUntilExpiry(t *testing.T) {
	ctx := context.Background()
	profile := &models.AgentProfile{
		ID: "office-profile", AgentID: "agent-one", Name: "Worker", BillingType: "api_key",
		Model: "claude-primary", ResumeAfterReset: true,
	}
	limits := providerlimit.NewService(
		dynamic.NewCircuitRegistry(dynamic.WithCircuitPersistence(&officeCircuitPersistence{})),
		dynamic.NewCredentialBindingResolver([]byte("provider-limit-nonlimit-probe")),
		providerLimitProfiles{profile: profile, agent: &models.Agent{ID: "agent-one", Name: "omp-acp"}},
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
	probeFailedBefore := officeProviderLimitMetric(t, "provider_limit_waits_total", "context=office;outcome=probe_failed")
	now := time.Now().UTC()
	subject := limits.ForProfile(profile, agents.NewOmpACP())
	resetAt := now.Add(-time.Second)
	mark, err := limits.Record(ctx, subject, profile.Model,
		&routingerr.Error{Code: routingerr.CodeQuotaLimited, LimitScope: "model", ResetHint: &resetAt},
		now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("record provider limit: %v", err)
	}
	lease, acquired, err := limits.AcquireProbe(ctx, mark.Key)
	if err != nil || !acquired {
		t.Fatalf("acquire probe: acquired=%t error=%v", acquired, err)
	}
	provider, model := "claude-acp", profile.Model
	run := &runmodels.Run{
		AgentProfileID: profile.ID, Reason: "task_assigned", Payload: `{}`,
		Status: runmodels.RunStatusClaimed, CoalescedCount: 1,
		ResolvedProviderID: &provider, ResolvedModel: &model,
	}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := repo.SetRunResolvedRoute(ctx, run.ID, profile.ID, provider, model); err != nil {
		t.Fatalf("persist resolved route: %v", err)
	}
	leaseJSON, err := json.Marshal(lease)
	if err != nil {
		t.Fatalf("encode probe lease: %v", err)
	}
	waitKey, probe := mark.Key, string(leaseJSON)
	if err := repo.SetRunProviderLimitRecoveryState(ctx, run.ID, nil, &waitKey, probe); err != nil {
		t.Fatalf("persist probe owner: %v", err)
	}
	run.ProviderLimitWaitKey, run.ProviderLimitProbe = &waitKey, probe
	providerError := &streams.ProviderError{
		Source: streams.ProviderErrorSourceACPPrompt, ProviderID: provider,
		Message: "connection closed unexpectedly", OccurredAt: now,
	}
	if !svc.tryProviderLimitRecovery(ctx, run, providerError.Message, providerError) {
		t.Fatal("non-limit probe failure was not parked")
	}
	if delta := officeProviderLimitMetric(t, "provider_limit_waits_total", "context=office;outcome=probe_failed") - probeFailedBefore; delta != 1 {
		t.Fatalf("non-limit probe failure metric delta = %d, want 1", delta)
	}
	got, err := repo.GetRunByID(ctx, run.ID)
	if err != nil {
		t.Fatalf("read parked probe owner: %v", err)
	}
	if got.ProviderLimitWaitKey == nil || *got.ProviderLimitWaitKey != mark.Key ||
		got.ProviderLimitProbe != probe || got.EarliestRetryAt == nil ||
		!got.EarliestRetryAt.Equal(lease.ExpiresAt) {
		t.Fatalf("non-limit failure changed exact probe wait: %+v", got)
	}
	state, exists := limits.Get(mark.Key)
	if !exists || state.Cleared || !state.ProbeUntil.Equal(lease.ExpiresAt) {
		t.Fatalf("non-limit failure cleared mark or lease: %+v, present=%t", state, exists)
	}
	renewedReset := now.Add(2 * time.Hour)
	limitFailure := &streams.ProviderError{
		Source: streams.ProviderErrorSourceACPPrompt, ProviderID: provider,
		Message: "You've hit your rate limit", OccurredAt: now, ResetAt: &renewedReset,
	}
	if !svc.tryProviderLimitRecovery(ctx, run, limitFailure.Message, limitFailure) {
		t.Fatal("renewed provider-limit failure was not parked")
	}
	renewed, exists := limits.Get(mark.Key)
	if !exists || !renewed.Until.Equal(renewedReset) || !renewed.ProbeUntil.Equal(lease.ExpiresAt) {
		t.Fatalf("limit failure did not renew mark while retaining exact lease: %+v, present=%t", renewed, exists)
	}
}

func TestNonOptedOfficeProfileSkipsProviderLimitRecovery(t *testing.T) {
	profile := &models.AgentProfile{
		ID: "office-profile", AgentID: "agent-one", Name: "Worker", BillingType: "api_key",
		Model: "claude-primary",
	}
	limits := providerlimit.NewService(
		dynamic.NewCircuitRegistry(dynamic.WithCircuitPersistence(&officeCircuitPersistence{})),
		dynamic.NewCredentialBindingResolver([]byte("provider-limit-non-opted")),
		providerLimitProfiles{profile: profile, agent: &models.Agent{ID: "agent-one", Name: "omp-acp"}},
		providerLimitCatalog{},
	)
	svc := NewService(ServiceOptions{Logger: logger.Default(), ProviderLimits: limits})
	provider, model := "claude-acp", profile.Model
	run := &runmodels.Run{
		AgentProfileID: profile.ID, ResolvedProviderID: &provider, ResolvedModel: &model,
	}
	providerError := &streams.ProviderError{
		Source: streams.ProviderErrorSourceACPPrompt, ProviderID: provider,
		Message: "You've hit your rate limit", OccurredAt: time.Now().UTC(),
	}
	if svc.tryProviderLimitRecovery(context.Background(), run, providerError.Message, providerError) {
		t.Fatal("non-opted profile entered provider-limit recovery")
	}
	subject := limits.ForProfile(profile, agents.NewOmpACP())
	if mark, exists := limits.Lookup(subject, model); exists {
		t.Fatalf("non-opted profile wrote provider-limit mark: %+v", mark)
	}
}
func officeProviderLimitMetric(t *testing.T, name, label string) int64 {
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
