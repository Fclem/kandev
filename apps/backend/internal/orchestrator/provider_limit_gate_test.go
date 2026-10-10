package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.1
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.2
func TestProviderLimitGateAutomaticLaunchWaitsBeforeCeilingAdmission(t *testing.T) {
	ctx := context.Background()
	svc, limits, profile := newProviderLimitMarkService(t)
	profile.ResumeAfterReset = true
	now := time.Now().UTC()
	reset := now.Add(time.Hour)
	if _, err := limits.Record(ctx, limits.ForProfile(profile, agents.NewOmpACP()), profile.Model, &routingerr.Error{Code: routingerr.CodeQuotaLimited, LimitScope: "account", ResetHint: &reset}, now); err != nil {
		t.Fatal(err)
	}
	payload := map[string]interface{}{metaKeyAgentProfileID: profile.ID, metaKeyPrompt: "automatic input", metaKeyWorkflowStepID: "step1"}
	reservation, deferred, err := svc.admitOrDeferSeam1(ctx, "t1", launchOriginAutomatic, payload)
	if reservation != nil {
		defer reservation.releaseIfNotConsumed()
	}
	if err != nil || !deferred || reservation != nil {
		t.Fatalf("marked automatic launch reached ceiling admission: deferred=%t reservation=%v error=%v", deferred, reservation, err)
	}
	record, _, err := svc.repo.GetTaskDeferredLaunch(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := record[models.CeilingLaunchPayloadKey].(map[string]interface{})
	if stored[metaKeyPrompt] != "automatic input" || stored[metaKeyAgentProfileID] != profile.ID {
		t.Fatalf("limit deferral lost its original launch payload: %+v", record)
	}
	deferredTask, err := svc.repo.GetTask(ctx, "t1")
	if err != nil || string(deferredTask.State) != "IN_PROGRESS" {
		t.Fatalf("deferring the automatic launch disturbed the active sibling session: task=%+v error=%v", deferredTask, err)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.1
func TestProviderLimitGateExistingSessionLaunchesWaitBeforeCeiling(t *testing.T) {
	for _, kind := range []models.CeilingLaunchKind{models.CeilingLaunchStartCreated, models.CeilingLaunchResume} {
		t.Run(string(kind), func(t *testing.T) {
			ctx := context.Background()
			svc, limits, profile := newProviderLimitMarkService(t)
			profile.ResumeAfterReset = true
			now := time.Now().UTC()
			reset := now.Add(time.Hour)
			if _, err := limits.Record(ctx, limits.ForProfile(profile, agents.NewOmpACP()), profile.Model, &routingerr.Error{Code: routingerr.CodeQuotaLimited, LimitScope: "account", ResetHint: &reset}, now); err != nil {
				t.Fatal(err)
			}
			payload := map[string]interface{}{metaKeySessionID: "s1", metaKeyPrompt: "preserved existing input"}
			reservation, deferred, err := svc.admitOrDeferSessionKeyedLaunch(ctx, "t1", "s1", launchOriginAutomatic, string(kind), kind, payload, "test launch")
			if reservation != nil {
				defer reservation.releaseIfNotConsumed()
			}
			if err != nil || !deferred || reservation != nil {
				t.Fatalf("marked existing session reached ceiling admission: deferred=%t reservation=%v error=%v", deferred, reservation, err)
			}
			record, _, err := svc.repo.GetTaskDeferredLaunch(ctx, "t1")
			if err != nil {
				t.Fatal(err)
			}
			launch, err := models.ReadProviderLimitLaunch(record)
			if err != nil || launch == nil || launch.Kind != kind || launch.Payload[metaKeyPrompt] != "preserved existing input" {
				t.Fatalf("existing-session launch lost its replay identity: %+v error=%v", launch, err)
			}
		})
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.1
func TestProviderLimitGateColdResumeAndWorkflowEnsureRefuseMarkedModel(t *testing.T) {
	ctx := context.Background()
	svc, limits, profile := newProviderLimitMarkService(t)
	profile.ResumeAfterReset = true
	now := time.Now().UTC()
	reset := now.Add(time.Hour)
	if _, err := limits.Record(ctx, limits.ForProfile(profile, agents.NewOmpACP()), profile.Model, &routingerr.Error{Code: routingerr.CodeQuotaLimited, LimitScope: "account", ResetHint: &reset}, now); err != nil {
		t.Fatal(err)
	}
	reservation, refusal := svc.admitSeam3(ctx, "t1", "s1", launchOriginAutomatic)
	if reservation != nil {
		reservation.releaseIfNotConsumed()
	}
	if reservation != nil || refusal == nil {
		t.Fatal("marked cold resume reached ceiling admission")
	}
	reservation, deferred, err := svc.admitOrDeferWorkflowStepEnsureWithBinding(ctx, "t1", "s1", "step1", nil)
	if reservation != nil {
		defer reservation.releaseIfNotConsumed()
	}
	if err != nil || !deferred || reservation != nil {
		t.Fatalf("marked workflow ensure reached ceiling admission: deferred=%t reservation=%v error=%v", deferred, reservation, err)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.3
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.4
func TestProviderLimitGateManualAndUntrustedWaitsKeepExistingAdmission(t *testing.T) {
	for _, name := range []string{"manual", "opted out", "unknown reset", "eight days", "trusted reset"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			svc, limits, profile := newProviderLimitMarkService(t)
			profile.ResumeAfterReset = name != "opted out"
			now := time.Now().UTC()
			reset := now.Add(time.Hour)
			failure := &routingerr.Error{Code: routingerr.CodeQuotaLimited, LimitScope: "account", ResetHint: &reset}
			switch name {
			case "unknown reset":
				failure.ResetHint = nil
			case "eight days":
				reset = now.Add(8 * 24 * time.Hour)
			}
			if _, err := limits.Record(ctx, limits.ForProfile(profile, agents.NewOmpACP()), profile.Model, failure, now); err != nil {
				t.Fatal(err)
			}
			origin := launchOriginAutomatic
			if name == "manual" {
				origin = launchOriginManual
			}
			reservation, deferred, err := svc.admitOrDeferSeam1(ctx, "t1", origin, map[string]interface{}{metaKeyAgentProfileID: profile.ID, metaKeyPrompt: "requested input"})
			if reservation != nil {
				defer reservation.releaseIfNotConsumed()
			}
			if err != nil || deferred != (name == "trusted reset") {
				t.Fatalf("admission changed outside an authorized trusted wait: deferred=%t error=%v", deferred, err)
			}
			if _, limited := limits.Lookup(limits.ForProfile(profile, agents.NewOmpACP()), profile.Model); !limited {
				t.Fatal("admission removed the provider limit mark")
			}
		})
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.2
func TestProviderLimitGateCeilingConflictPreservesEarlierLaunch(t *testing.T) {
	ctx := context.Background()
	svc, limits, profile := newProviderLimitMarkService(t)
	profile.ResumeAfterReset = true
	now := time.Now().UTC()
	reset := now.Add(time.Hour)
	if _, err := limits.Record(ctx, limits.ForProfile(profile, agents.NewOmpACP()), profile.Model, &routingerr.Error{Code: routingerr.CodeQuotaLimited, LimitScope: "account", ResetHint: &reset}, now); err != nil {
		t.Fatal(err)
	}
	original := map[string]interface{}{metaKeyAgentProfileID: profile.ID, metaKeyPrompt: "first automatic input"}
	if _, deferred, err := svc.admitOrDeferSeam1(ctx, "t1", launchOriginAutomatic, original); err != nil || !deferred {
		t.Fatalf("initial limit deferral: deferred=%t error=%v", deferred, err)
	}
	second := map[string]interface{}{metaKeyAgentProfileID: profile.ID, metaKeyPrompt: "different automatic input"}
	err := svc.deferCeilingRefusal(ctx, "t1", "", models.CeilingLaunchStart, second, "ceiling_full", 1, true, 1)
	if !errors.Is(err, ErrCeilingLaunchConflict) {
		t.Fatalf("ceiling replaced the first automatic launch instead of returning conflict: %v", err)
	}
	record, _, err := svc.repo.GetTaskDeferredLaunch(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	launch, err := models.ReadProviderLimitLaunch(record)
	if err != nil || launch == nil || launch.Payload[metaKeyPrompt] != "first automatic input" {
		t.Fatalf("first launch ownership was lost: %+v error=%v", launch, err)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.4
func TestManualProviderLimitNoticeIsOncePerSessionAndMarkWithoutChangingModel(t *testing.T) {
	ctx := context.Background()
	svc, profile, _, repo, _ := newProviderLimitFallbackFixture(t)
	limits := svc.providerLimits
	now := time.Now().UTC()
	reset := now.Add(time.Hour).Truncate(time.Second).Add(123456789 * time.Nanosecond)
	if _, err := limits.Record(ctx, limits.ForProfile(profile, agents.NewOmpACP()), profile.Model, &routingerr.Error{Code: routingerr.CodeQuotaLimited, LimitScope: "account", ResetHint: &reset}, now); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := svc.recordManualProviderLimitNotice(ctx, "t1", "s1", launchOriginManual, profile.Model); err != nil {
			t.Fatal(err)
		}
	}
	messages, err := repo.ListMessages(ctx, "s1")
	if err != nil || len(messages) != 1 {
		t.Fatalf("manual retry duplicated its provider limit notice: %+v error=%v", messages, err)
	}
	notice := messages[0]
	if notice.Metadata["kind"] != "provider_limit_notice" || notice.Metadata["model_id"] != profile.Model || notice.Metadata["retry_at"] != reset.Format(time.RFC3339Nano) || notice.RequestsInput {
		t.Fatalf("manual notice lost its requested model or trusted deadline: %+v", notice)
	}
	if _, leaked := notice.Metadata["mark_key"]; leaked {
		t.Fatal("manual notice exposed a credential binding")
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	config, _ := models.LoadEffectiveSessionRuntimeConfig(session)
	if session.State != models.TaskSessionStateRunning || config.Model != "anthropic/live-opus" || profile.RequireExactModel {
		t.Fatalf("manual notice changed requested launch behavior: state=%s model=%q exact=%t", session.State, config.Model, profile.RequireExactModel)
	}
}
