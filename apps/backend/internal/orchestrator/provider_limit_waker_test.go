package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/providerlimit"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.4
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.5
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.9
func TestProviderLimitWakerNeverDispatchesEarlyAndPersistsExactProbe(t *testing.T) {
	ctx := context.Background()
	svc, profile, manager, repo, data := newProviderLimitFallbackFixture(t)
	profile.LimitFallback, profile.ResumeAfterReset = false, true
	clock := &providerLimitTestClock{now: time.Now().UTC()}
	registry := dynamic.NewCircuitRegistry(dynamic.WithCircuitClock(clock.Now), dynamic.WithCircuitPersistence(repo))
	svc.providerLimits = providerlimit.NewService(registry, dynamic.NewCredentialBindingResolver([]byte("installation-limit-test")), &limitMarkProfiles{profile: profile}, limitMarkCatalog{}, providerlimit.WithClock(clock.Now))
	svc.providerLimitClock = clock
	reset := clock.now.Add(time.Hour).Add(123456789 * time.Nanosecond)
	data.ProviderError.ResetAt = &reset
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	clock.now = reset.Add(-time.Nanosecond)
	svc.reconcileProviderLimitWaits(ctx)
	waitForFailureRecovery(t, svc)
	manager.mu.Lock()
	early := len(manager.capturedPrompts)
	manager.mu.Unlock()
	if early != 0 {
		t.Fatal("provider reset was rounded down or timer dispatched early")
	}
	clock.now = reset
	svc.reconcileProviderLimitWaits(ctx)
	waitForFailureRecovery(t, svc)
	record, _, err := repo.GetTaskDeferredLaunch(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	waits, err := models.ReadProviderLimitWaits(record)
	if err != nil {
		t.Fatal(err)
	}
	wait := waits[models.ProviderLimitWaitIdentity("s1", "limited-turn")]
	if wait.ProbeLease == nil || !wait.ProbeLease.Until.Equal(reset.Add(10*time.Minute)) || wait.ProbeLease.Key != wait.MarkKey || wait.ProbeTurnID == "" || wait.ProbeExecutionID != "live-exec" || wait.ProbeGeneration != 2 {
		t.Fatalf("exact lease/session/turn/generation was not owned durably before inference: %+v", wait)
	}
	if svc.CancelProviderLimitWait(ctx, "t1", "s1", models.ProviderLimitWaitIdentity("s1", "limited-turn")) {
		t.Fatal("a stale waiting-card action abandoned an already admitted provider probe")
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if len(manager.capturedPrompts) != 1 || manager.capturedPrompts[0] != "original user input" {
		t.Fatalf("reset did not replay the same owned input once: %+v", manager.capturedPrompts)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.5
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.9
func TestProviderLimitWakerRestoresUndispatchedExactLeaseWithoutAnotherAcquisition(t *testing.T) {
	ctx := context.Background()
	svc, profile, manager, repo, data := newProviderLimitFallbackFixture(t)
	profile.LimitFallback, profile.ResumeAfterReset = false, true
	clock := &providerLimitTestClock{now: time.Now().UTC()}
	registry := dynamic.NewCircuitRegistry(dynamic.WithCircuitClock(clock.Now), dynamic.WithCircuitPersistence(repo))
	svc.providerLimits = providerlimit.NewService(registry, dynamic.NewCredentialBindingResolver([]byte("installation-limit-test")), &limitMarkProfiles{profile: profile}, limitMarkCatalog{}, providerlimit.WithClock(clock.Now))
	svc.providerLimitClock = clock
	reset := clock.now.Add(time.Hour)
	data.ProviderError.ResetAt = &reset
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	clock.now = reset
	record, _, err := repo.GetTaskDeferredLaunch(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	waits, err := models.ReadProviderLimitWaits(record)
	if err != nil {
		t.Fatal(err)
	}
	identity := models.ProviderLimitWaitIdentity("s1", "limited-turn")
	wait := waits[identity]
	token, acquired, err := svc.providerLimits.AcquireProbe(ctx, wait.MarkKey)
	if err != nil || !acquired {
		t.Fatalf("acquire initial durable probe: %t, %v", acquired, err)
	}
	changed, err := svc.updateProviderLimitWait(ctx, "t1", identity, func(current models.ProviderLimitWait) (models.ProviderLimitWait, bool) {
		current.ProbeLease = &models.ProviderLimitProbeLease{Key: token.Key, Until: token.ExpiresAt}
		return current, true
	})
	if err != nil || !changed {
		t.Fatalf("persist pre-crash token: %t, %v", changed, err)
	}
	restored := dynamic.NewCircuitRegistry(dynamic.WithCircuitClock(clock.Now), dynamic.WithCircuitPersistence(repo))
	if err := restored.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	svc.providerLimits = providerlimit.NewService(restored, dynamic.NewCredentialBindingResolver([]byte("installation-limit-test")), &limitMarkProfiles{profile: profile}, limitMarkCatalog{}, providerlimit.WithClock(clock.Now))
	svc.reconcileProviderLimitWaits(ctx)
	waitForFailureRecovery(t, svc)
	svc.reconcileProviderLimitWaits(ctx)
	waitForFailureRecovery(t, svc)
	record, _, err = repo.GetTaskDeferredLaunch(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	waits, err = models.ReadProviderLimitWaits(record)
	if err != nil {
		t.Fatal(err)
	}
	wait = waits[identity]
	if wait.ProbeLease == nil || !wait.ProbeLease.Until.Equal(token.ExpiresAt) || wait.ProbeTurnID == "" {
		t.Fatalf("restart lost the original probe token or dispatch ownership: %+v", wait)
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if len(manager.capturedPrompts) != 1 {
		t.Fatalf("restart suppressed or duplicated owned inference: %+v", manager.capturedPrompts)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.6
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.6
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.9
func TestProviderLimitProbeLeaseSuccessfulCleanupFailureRetainsOwnerBarrier(t *testing.T) {
	ctx := context.Background()
	svc, profile, _, repo, data := newProviderLimitFallbackFixture(t)
	profile.LimitFallback, profile.ResumeAfterReset = false, true
	clock := &providerLimitTestClock{now: time.Now().UTC()}
	registry := dynamic.NewCircuitRegistry(dynamic.WithCircuitClock(clock.Now), dynamic.WithCircuitPersistence(repo))
	svc.providerLimits = providerlimit.NewService(registry, dynamic.NewCredentialBindingResolver([]byte("installation-limit-test")), &limitMarkProfiles{profile: profile}, limitMarkCatalog{}, providerlimit.WithClock(clock.Now))
	svc.providerLimitClock = clock
	reset := clock.now.Add(time.Hour)
	data.ProviderError.ResetAt = &reset
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	clock.now = reset
	svc.reconcileProviderLimitWaits(ctx)
	waitForFailureRecovery(t, svc)
	if _, err := repo.DB().Exec(`CREATE TRIGGER reject_success_cleanup BEFORE UPDATE OF metadata ON tasks WHEN json_type(OLD.metadata, '$.deferred_launch.provider_limit_waits') = 'object' AND json_type(NEW.metadata, '$.deferred_launch.provider_limit_waits') IS NULL BEGIN SELECT RAISE(ABORT, 'owner cleanup refused'); END`); err != nil {
		t.Fatal(err)
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	svc.clearProviderLimitSuccess(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "live-exec", PromptGeneration: 2}, session)
	record, _, err := repo.GetTaskDeferredLaunch(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	waits, err := models.ReadProviderLimitWaits(record)
	if err != nil {
		t.Fatal(err)
	}
	identity := models.ProviderLimitWaitIdentity("s1", "limited-turn")
	if wait, exists := waits[identity]; !exists || wait.ProbeLease == nil || !wait.ProbeSucceeded {
		t.Fatalf("successful cleanup failure lost its durable barrier owner: %+v", waits)
	}
	session, err = repo.GetTaskSession(ctx, "s1")
	if err != nil || models.CeilingRecordInt(session.Metadata[providerLimitWaitCounterKey]) != 1 {
		t.Fatalf("cleanup failure reset consecutive budget: %+v, %v", session, err)
	}
	if _, err := repo.DB().Exec(`DROP TRIGGER reject_success_cleanup`); err != nil {
		t.Fatal(err)
	}
	svc.reconcileProviderLimitWaits(ctx)
	waitForFailureRecovery(t, svc)
	record, _, err = repo.GetTaskDeferredLaunch(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	waits, err = models.ReadProviderLimitWaits(record)
	if err != nil || len(waits) != 0 {
		t.Fatalf("safety sweep did not complete durable successful ownership: %+v, %v", waits, err)
	}
	session, err = repo.GetTaskSession(ctx, "s1")
	if err != nil || models.CeilingRecordInt(session.Metadata[providerLimitWaitCounterKey]) != 0 {
		t.Fatalf("completed cleanup did not reset budget: %+v, %v", session, err)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.5
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.9
func TestProviderLimitWakerThreeSessionsShareOneFIFOProbe(t *testing.T) {
	ctx := context.Background()
	svc, profile, manager, repo, data := newProviderLimitFallbackFixture(t)
	profile.LimitFallback, profile.ResumeAfterReset = false, true
	clock := &providerLimitTestClock{now: time.Now().UTC()}
	registry := dynamic.NewCircuitRegistry(dynamic.WithCircuitClock(clock.Now), dynamic.WithCircuitPersistence(repo))
	svc.providerLimits = providerlimit.NewService(registry, dynamic.NewCredentialBindingResolver([]byte("installation-limit-test")), &limitMarkProfiles{profile: profile}, limitMarkCatalog{}, providerlimit.WithClock(clock.Now))
	svc.providerLimitClock = clock
	reset := clock.now.Add(time.Hour)
	data.ProviderError.ResetAt = &reset
	template, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	for _, id := range []string{"s2", "s3"} {
		session := *template
		session.ID, session.QueueIncarnationID, session.AgentExecutionID = id, "", "exec-"+id
		if err := repo.CreateTaskSession(ctx, &session); err != nil {
			t.Fatal(err)
		}
		seedExecutorRunning(t, repo, id, "t1", session.AgentExecutionID)
		turnID := "failed-" + id
		if err := repo.CreateTurn(ctx, &models.Turn{ID: turnID, TaskSessionID: id, TaskID: "t1", StartedAt: clock.now, CreatedAt: clock.now, UpdatedAt: clock.now}); err != nil {
			t.Fatal(err)
		}
		svc.activeTurns.Store(id, turnID)
		svc.lastTurnPrompt.Store(id, capturedPrompt{text: "input " + id, model: "anthropic/live-opus"})
		svc.beginPromptAttempt(id, session.AgentExecutionID, 1, false)
		failure := data
		failure.SessionID, failure.AgentExecutionID = id, session.AgentExecutionID
		svc.handleAgentFailed(ctx, failure)
		waitForFailureRecovery(t, svc)
	}
	clock.now = reset
	svc.reconcileProviderLimitWaits(ctx)
	waitForFailureRecovery(t, svc)
	svc.reconcileProviderLimitWaits(ctx)
	waitForFailureRecovery(t, svc)
	record, _, err := repo.GetTaskDeferredLaunch(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	waits, err := models.ReadProviderLimitWaits(record)
	if err != nil || len(waits) != 3 {
		t.Fatalf("shared mark lost independent session waits: %+v, %v", waits, err)
	}
	owner := waits[models.ProviderLimitWaitIdentity("s1", "limited-turn")]
	if owner.ProbeLease == nil || owner.ProbeTurnID == "" {
		t.Fatalf("oldest waiting session did not own the probe: %+v", owner)
	}
	for identity, wait := range waits {
		if wait.MarkKey != owner.MarkKey || (wait.SessionID != "s1" && (wait.ProbeLease != nil || wait.ProbeTurnID != "")) {
			t.Fatalf("shared mark admitted a competing probe for %s: %+v", identity, wait)
		}
	}
	manager.mu.Lock()
	probes := append([]string(nil), manager.capturedPrompts...)
	manager.mu.Unlock()
	if len(probes) != 1 || probes[0] != "original user input" {
		t.Fatalf("FIFO probe dispatched the wrong continuation or duplicated it: %+v", probes)
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	svc.clearProviderLimitSuccess(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "live-exec", PromptGeneration: 2}, session)
	svc.reconcileProviderLimitWaits(ctx)
	waitForFailureRecovery(t, svc)
	manager.mu.Lock()
	defer manager.mu.Unlock()
	counts := make(map[string]int)
	for _, prompt := range manager.capturedPrompts {
		counts[prompt]++
	}
	if len(manager.capturedPrompts) != 3 || counts["original user input"] != 1 || counts["input s2"] != 1 || counts["input s3"] != 1 {
		t.Fatalf("successful owner did not wake each remaining continuation exactly once: %+v", manager.capturedPrompts)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.5
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.9
func TestProviderLimitProbeLeaseFailedProbeRenewsOwnedWait(t *testing.T) {
	ctx := context.Background()
	svc, profile, manager, repo, data := newProviderLimitFallbackFixture(t)
	profile.LimitFallback, profile.ResumeAfterReset = false, true
	clock := &providerLimitTestClock{now: time.Now().UTC()}
	registry := dynamic.NewCircuitRegistry(dynamic.WithCircuitClock(clock.Now), dynamic.WithCircuitPersistence(repo))
	svc.providerLimits = providerlimit.NewService(registry, dynamic.NewCredentialBindingResolver([]byte("installation-limit-test")), &limitMarkProfiles{profile: profile}, limitMarkCatalog{}, providerlimit.WithClock(clock.Now))
	svc.providerLimitClock = clock
	reset := clock.now.Add(time.Hour)
	data.ProviderError.ResetAt = &reset
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	clock.now = reset
	svc.reconcileProviderLimitWaits(ctx)
	waitForFailureRecovery(t, svc)
	nextReset := reset.Add(time.Hour)
	data.ProviderError.ResetAt = &nextReset
	data.PromptGeneration = 2
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	svc.reconcileProviderLimitWaits(ctx)
	waitForFailureRecovery(t, svc)
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	svc.clearProviderLimitSuccess(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "live-exec", PromptGeneration: 2}, session)
	record, _, err := repo.GetTaskDeferredLaunch(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	waits, err := models.ReadProviderLimitWaits(record)
	if err != nil || len(waits) != 1 {
		t.Fatalf("failed probe lost or duplicated its renewed wait: %+v, %v", waits, err)
	}
	identity, _ := session.Metadata[providerLimitWaitIdentityKey].(string)
	wait := waits[identity]
	if session.State != models.TaskSessionStateWaitingForInput || models.CeilingRecordInt(session.Metadata[providerLimitWaitCounterKey]) != 2 || !wait.NotBefore.Equal(nextReset) || wait.ProbeLease != nil || wait.ProbeTurnID != "" {
		t.Fatalf("failed probe did not renew exact reset and consecutive budget: session=%+v wait=%+v", session, wait)
	}
	mark, exists := svc.providerLimits.Get(wait.MarkKey)
	if !exists || !mark.Until.Equal(nextReset) {
		t.Fatalf("late success from the failed probe cleared its successor mark: %+v", mark)
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if len(manager.capturedPrompts) != 1 {
		t.Fatalf("failed probe replayed again before its new reset: %+v", manager.capturedPrompts)
	}
}

type providerLimitTestClock struct{ now time.Time }

func (c *providerLimitTestClock) Now() time.Time { return c.now }
func (c *providerLimitTestClock) AfterFunc(_ time.Duration, callback func()) providerLimitTimer {
	return &providerLimitTestTimer{callback: callback}
}

type providerLimitTestTimer struct {
	callback func()
	stopped  bool
}

func (t *providerLimitTestTimer) Stop() bool {
	wasActive := !t.stopped
	t.stopped = true
	return wasActive
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.5
func TestProviderLimitProbeLeaseOwnerWriteFailureNeverDispatches(t *testing.T) {
	ctx := context.Background()
	svc, profile, manager, repo, data := newProviderLimitFallbackFixture(t)
	profile.LimitFallback, profile.ResumeAfterReset = false, true
	clock := &providerLimitTestClock{now: time.Now().UTC()}
	registry := dynamic.NewCircuitRegistry(dynamic.WithCircuitClock(clock.Now), dynamic.WithCircuitPersistence(repo))
	svc.providerLimits = providerlimit.NewService(registry, dynamic.NewCredentialBindingResolver([]byte("installation-limit-test")), &limitMarkProfiles{profile: profile}, limitMarkCatalog{}, providerlimit.WithClock(clock.Now))
	svc.providerLimitClock = clock
	reset := clock.now.Add(time.Hour)
	data.ProviderError.ResetAt = &reset
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	if _, err := repo.DB().Exec(`CREATE TRIGGER reject_probe_owner BEFORE UPDATE OF metadata ON tasks WHEN EXISTS (SELECT 1 FROM json_each(NEW.metadata, '$.deferred_launch.provider_limit_waits') WHERE json_type(value, '$.probe_lease') = 'object') BEGIN SELECT RAISE(ABORT, 'probe owner refused'); END`); err != nil {
		t.Fatal(err)
	}
	clock.now = reset
	svc.reconcileProviderLimitWaits(ctx)
	waitForFailureRecovery(t, svc)
	svc.reconcileProviderLimitWaits(ctx)
	waitForFailureRecovery(t, svc)
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil || session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("failed durable probe owner changed session state: %v, %v", session, err)
	}
	record, _, err := repo.GetTaskDeferredLaunch(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	waits, err := models.ReadProviderLimitWaits(record)
	if err != nil {
		t.Fatal(err)
	}
	wait := waits[models.ProviderLimitWaitIdentity("s1", "limited-turn")]
	if wait.ProbeLease != nil || wait.ProbeTurnID != "" || !wait.NotBefore.Equal(reset) {
		t.Fatalf("failed owner write corrupted its pending continuation: %+v", wait)
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if len(manager.capturedPrompts) != 0 {
		t.Fatalf("provider inference ran without a durable acquired-token owner: %+v", manager.capturedPrompts)
	}
}
