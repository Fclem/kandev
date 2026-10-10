package orchestrator

import (
	"context"
	"errors"
	"expvar"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agent/runtime/providerlimit"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.1
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.2
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.8
func TestProviderLimitWaitArmsExactResetWithoutDispatching(t *testing.T) {
	ctx := context.Background()
	svc, profile, manager, repo, data := newProviderLimitFallbackFixture(t)
	beforeArmed := providerLimitWaitMetric(t, providerlimit.MetricContextKanban, providerlimit.MetricWaitArmed)
	profile.LimitFallback, profile.ResumeAfterReset = false, true
	reset := time.Now().UTC().Add(time.Hour).Truncate(time.Second).Add(123456789 * time.Nanosecond)
	data.ProviderError.ResetAt = &reset
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	if delta := providerLimitWaitMetric(t, providerlimit.MetricContextKanban, providerlimit.MetricWaitArmed) - beforeArmed; delta != 1 {
		t.Fatalf("durable wait arm metric delta = %d, want 1", delta)
	}
	record, _, err := repo.GetTaskDeferredLaunch(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	waits, err := models.ReadProviderLimitWaits(record)
	if err != nil {
		t.Fatal(err)
	}
	wait, exists := waits[models.ProviderLimitWaitIdentity("s1", "limited-turn")]
	if !exists || !wait.NotBefore.Equal(reset) || wait.Model != "anthropic/live-opus" || wait.Payload["prompt"] != "original user input" {
		t.Fatalf("exact wait or replay ownership was lost: %+v", waits)
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil || session.State != models.TaskSessionStateWaitingForInput || models.CeilingRecordInt(session.Metadata["provider_limit_waits"]) != 1 {
		t.Fatalf("waiting session still occupies admission or lost its wait budget: %+v, %v", session, err)
	}
	messages, err := repo.ListMessages(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Metadata["limit_wait"] != true || messages[0].Metadata["retry_at"] != reset.Format(time.RFC3339Nano) {
		t.Fatalf("exact waiting card was not durable: %+v", messages)
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if len(manager.capturedPrompts) != 0 || len(manager.setSessionModelCalls) != 0 {
		t.Fatal("wait dispatched or changed the limited model before reset")
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.1
func TestProviderLimitWaitReportedFailureKeepsActorOwnedTurn(t *testing.T) {
	ctx := context.Background()
	svc, profile, _, _, data := newProviderLimitFallbackFixture(t)
	profile.LimitFallback, profile.ResumeAfterReset = false, true
	reported := errors.Join(lifecycle.ErrAgentReported, errors.New(data.ProviderError.Message))
	_ = svc.handlePromptError(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, reported)
	turnID, err := svc.peekActiveTurnID(ctx, "s1")
	if err != nil || turnID != "limited-turn" {
		t.Fatalf("reported error cleanup settled the actor-owned wait turn: %q, %v", turnID, err)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.3
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.6
func TestProviderLimitWaitUntrustedAndExhaustedUseExistingRecovery(t *testing.T) {
	for _, boundary := range []string{"disabled", "unknown", "eight days", "fourth wait"} {
		t.Run(boundary, func(t *testing.T) {
			ctx := context.Background()
			svc, profile, manager, repo, data := newProviderLimitFallbackFixture(t)
			beforeExhausted := providerLimitWaitMetric(t, providerlimit.MetricContextKanban, providerlimit.MetricWaitExhausted)
			profile.LimitFallback, profile.ResumeAfterReset = false, true
			switch boundary {
			case "disabled":
				profile.ResumeAfterReset = false
			case "unknown":
				data.ProviderError.ResetAt = nil
			case "eight days":
				reset := time.Now().UTC().Add(8 * 24 * time.Hour)
				data.ProviderError.ResetAt = &reset
			case "fourth wait":
				if err := repo.SetSessionMetadataKey(ctx, "s1", "provider_limit_waits", 3); err != nil {
					t.Fatal(err)
				}
			}
			svc.handleAgentFailed(ctx, data)
			waitForFailureRecovery(t, svc)
			wantExhausted := int64(0)
			if boundary == "fourth wait" {
				wantExhausted = 1
			}
			if delta := providerLimitWaitMetric(t, providerlimit.MetricContextKanban, providerlimit.MetricWaitExhausted) - beforeExhausted; delta != wantExhausted {
				t.Fatalf("%s exhausted metric delta = %d, want %d", boundary, delta, wantExhausted)
			}
			record, _, err := repo.GetTaskDeferredLaunch(ctx, "t1")
			if err != nil {
				t.Fatal(err)
			}
			waits, err := models.ReadProviderLimitWaits(record)
			if err != nil || len(waits) != 0 {
				t.Fatalf("untrusted/exhausted failure armed a wait: %+v, %v", waits, err)
			}
			rows, err := repo.ListMessages(ctx, "s1")
			if err != nil || len(rows) != 1 || rows[0].Metadata["recovery_actions"] != true {
				t.Fatalf("existing manual recovery was lost: %+v, %v", rows, err)
			}
			manager.mu.Lock()
			defer manager.mu.Unlock()
			if len(manager.capturedPrompts) != 0 {
				t.Fatal("untrusted/exhausted wait resumed")
			}
		})
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.7
func TestProviderLimitWaitCancelRemovesOnlyItsExactOwnedWait(t *testing.T) {
	ctx := context.Background()
	svc, profile, manager, repo, data := newProviderLimitFallbackFixture(t)
	beforeCancelled := providerLimitWaitMetric(t, providerlimit.MetricContextKanban, providerlimit.MetricWaitCancelled)
	profile.LimitFallback, profile.ResumeAfterReset = false, true
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	record, prior, err := repo.GetTaskDeferredLaunch(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	waits, err := models.ReadProviderLimitWaits(record)
	if err != nil {
		t.Fatal(err)
	}
	sibling := waits[models.ProviderLimitWaitIdentity("s1", "limited-turn")]
	sibling.SessionID, sibling.TurnID = "sibling", "sibling-turn"
	record, err = models.PutProviderLimitWait(record, sibling)
	if err != nil {
		t.Fatal(err)
	}
	record["ceiling_deferred"] = true
	record["dependency_queued"] = true
	if changed, _, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "t1", prior, record); err != nil || !changed {
		t.Fatalf("seed sibling intent: %t, %v", changed, err)
	}
	if svc.CancelProviderLimitWait(ctx, "t1", "s1", models.ProviderLimitWaitIdentity("s1", "old-turn")) {
		t.Fatal("a stale card cancelled the current waiter")
	}
	if delta := providerLimitWaitMetric(t, providerlimit.MetricContextKanban, providerlimit.MetricWaitCancelled) - beforeCancelled; delta != 0 {
		t.Fatalf("stale cancellation changed metric by %d", delta)
	}
	if !svc.CancelProviderLimitWait(ctx, "t1", "s1", models.ProviderLimitWaitIdentity("s1", "limited-turn")) {
		t.Fatal("the owned wait was not cancelled")
	}
	if delta := providerLimitWaitMetric(t, providerlimit.MetricContextKanban, providerlimit.MetricWaitCancelled) - beforeCancelled; delta != 1 {
		t.Fatalf("owned cancellation metric delta = %d, want 1", delta)
	}
	record, _, err = repo.GetTaskDeferredLaunch(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	waits, err = models.ReadProviderLimitWaits(record)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := waits[models.ProviderLimitWaitIdentity("s1", "limited-turn")]; exists {
		t.Fatal("cancelled wait remained replayable")
	}
	if _, exists := waits[models.ProviderLimitWaitIdentity("sibling", "sibling-turn")]; !exists || record["ceiling_deferred"] != true || record["dependency_queued"] != true {
		t.Fatalf("cancellation deleted a sibling launch or wait: %+v", record)
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	failure, exists := models.LoadLastAgentError(session.Metadata)
	if !exists || failure.Code != "provider_limit_wait_cancelled" {
		t.Fatalf("cancellation did not preserve its localized recovery reason: %+v", failure)
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if len(manager.capturedPrompts) != 0 {
		t.Fatal("Cancel dispatched provider inference")
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.1
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.7
func TestProviderLimitWaitAtomicArmFailureKeepsSessionBudgetAndOtherIntents(t *testing.T) {
	ctx := context.Background()
	svc, profile, _, repo, data := newProviderLimitFallbackFixture(t)
	profile.LimitFallback, profile.ResumeAfterReset = false, true
	record, prior, err := repo.GetTaskDeferredLaunch(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if record == nil {
		record = make(map[string]interface{})
	}
	record["dependency_queued"] = true
	if changed, _, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "t1", prior, record); err != nil || !changed {
		t.Fatalf("seed independent intent: %t, %v", changed, err)
	}
	if _, err := repo.DB().Exec(`CREATE TRIGGER reject_wait_arm BEFORE UPDATE OF metadata ON tasks WHEN json_type(NEW.metadata, '$.deferred_launch.provider_limit_waits') = 'object' BEGIN SELECT RAISE(ABORT, 'wait refused'); END`); err != nil {
		t.Fatal(err)
	}
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if models.CeilingRecordInt(session.Metadata[providerLimitWaitCounterKey]) != 0 || session.Metadata[providerLimitWaitIdentityKey] != nil {
		t.Fatalf("failed task-intent write committed the session wait budget: %+v", session.Metadata)
	}
	record, _, err = repo.GetTaskDeferredLaunch(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	waits, err := models.ReadProviderLimitWaits(record)
	if err != nil || len(waits) != 0 || record["dependency_queued"] != true {
		t.Fatalf("arm rollback lost existing intent or published a waiter: %+v, %v", record, err)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.7
func TestProviderLimitWaitManualPromptSupersedesMatchingWait(t *testing.T) {
	ctx := context.Background()
	svc, profile, manager, repo, data := newProviderLimitFallbackFixture(t)
	profile.LimitFallback, profile.ResumeAfterReset = false, true
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	if _, err := svc.PromptTask(ctx, "t1", "s1", "explicit manual continuation", "anthropic/live-opus", false, nil, true); err != nil {
		t.Fatal(err)
	}
	record, _, err := repo.GetTaskDeferredLaunch(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	waits, err := models.ReadProviderLimitWaits(record)
	if err != nil || len(waits) != 0 {
		t.Fatalf("manual continuation left an automatic replay behind: %+v, %v", waits, err)
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if len(manager.capturedPrompts) != 1 || manager.capturedPrompts[0] != "explicit manual continuation" {
		t.Fatalf("manual override replayed the old automatic input: %+v", manager.capturedPrompts)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.7
func TestProviderLimitWakerArchiveRemovesWaitWithoutDispatch(t *testing.T) {
	ctx := context.Background()
	svc, profile, manager, repo, data := newProviderLimitFallbackFixture(t)
	profile.LimitFallback, profile.ResumeAfterReset = false, true
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	if err := repo.ArchiveTask(ctx, "t1"); err != nil {
		t.Fatal(err)
	}
	svc.reconcileProviderLimitWaits(ctx)
	waitForFailureRecovery(t, svc)
	record, _, err := repo.GetTaskDeferredLaunch(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	waits, err := models.ReadProviderLimitWaits(record)
	if err != nil || len(waits) != 0 {
		t.Fatalf("archive retained an automatic reset replay: %+v, %v", waits, err)
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if len(manager.capturedPrompts) != 0 {
		t.Fatal("archive dispatched an automatic provider probe")
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.7
func TestProviderLimitWaitStopAndDeleteInvalidateOwnedWait(t *testing.T) {
	for _, action := range []string{"stop", "delete"} {
		t.Run(action, func(t *testing.T) {
			ctx := context.Background()
			svc, profile, _, repo, data := newProviderLimitFallbackFixture(t)
			profile.LimitFallback, profile.ResumeAfterReset = false, true
			svc.handleAgentFailed(ctx, data)
			waitForFailureRecovery(t, svc)
			var err error
			if action == "stop" {
				err = svc.StopSession(ctx, "s1", "user stop", true)
			} else {
				err = svc.DeleteSession(ctx, "s1")
			}
			if err != nil {
				t.Fatal(err)
			}
			record, _, err := repo.GetTaskDeferredLaunch(ctx, "t1")
			if err != nil {
				t.Fatal(err)
			}
			waits, err := models.ReadProviderLimitWaits(record)
			if err != nil || len(waits) != 0 {
				t.Fatalf("%s left an automatic replay owned by the removed session: %+v, %v", action, waits, err)
			}
		})
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.6
func TestProviderLimitWaitManualSuccessfulTurnResetsConsecutiveBudget(t *testing.T) {
	ctx := context.Background()
	svc, profile, _, repo, data := newProviderLimitFallbackFixture(t)
	profile.LimitFallback, profile.ResumeAfterReset = false, true
	svc.handleAgentFailed(ctx, data)
	waitForFailureRecovery(t, svc)
	if _, err := svc.PromptTask(ctx, "t1", "s1", "manual successful continuation", "anthropic/live-opus", false, nil, true); err != nil {
		t.Fatal(err)
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	svc.clearProviderLimitSuccess(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "live-exec", PromptGeneration: 2}, session)
	session, err = repo.GetTaskSession(ctx, "s1")
	if err != nil || models.CeilingRecordInt(session.Metadata[models.ProviderLimitWaitsKey]) != 0 {
		t.Fatalf("successful manual turn retained consecutive wait exhaustion: %+v, %v", session, err)
	}
}
func providerLimitWaitMetric(t *testing.T, context, outcome string) int64 {
	t.Helper()
	value := expvar.Get("provider_limit_waits_total")
	if value == nil {
		t.Fatal("provider limit waits expvar map is not registered")
	}
	entry := value.(*expvar.Map).Get("context=" + context + ";outcome=" + outcome)
	if entry == nil {
		return 0
	}
	return entry.(*expvar.Int).Value()
}
