package service_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/office/shared"
)

func nativeRunUsageFixture(t *testing.T) (*service.Service, bus.EventBus, lifecycle.AgentStreamEventPayload) {
	t.Helper()
	svc, eb := newTestServiceWithBus(t)
	svc.SetRunSessionLauncher(&tasklessTestLauncher{svc: svc})
	agent := &models.AgentInstance{ID: "native-usage-agent", WorkspaceID: "ws-1", Name: "native", Role: models.AgentRoleCEO, Status: models.AgentStatusIdle, ExecutorPreference: `{"type":"local_pc"}`}
	if err := svc.CreateAgentInstance(context.Background(), agent); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.QueueRun(context.Background(), agent.ID, service.RunReasonRoutineTrigger, `{}`, "usage"); err != nil {
		t.Fatal(err)
	}
	service.RunSchedulerTick(svc, context.Background())
	return svc, eb, lifecycle.AgentStreamEventPayload{
		ExecutionID: "execution-run-session-1", OwnerKind: lifecycle.ExecutionOwnerRun, WorkspaceID: "ws-1", RunSessionID: "run-session-1", RunAttempt: 1, AgentProfileID: agent.ID, AgentType: "codex",
		Data: &lifecycle.AgentStreamEventData{Type: streams.EventTypeUsageObservation, PromptGeneration: 1, TurnID: "turn-1", Usage: &streams.PromptUsage{InputTokens: 5, InputTokensPresent: true, OutputTokens: 20, OutputTokensPresent: true, TotalTokens: 25, ProviderReportedCostSubcents: 123, ProviderReportedCostPresent: true}, UsageObservation: &streams.NativeUsageObservation{Source: "response", ProviderThreadID: "thread", ProviderTurnID: "native-turn", ProviderResponseID: "response-1", Scope: "direct", Model: "captured-model", Completeness: "exact"}},
	}
}

func TestTasklessNativeUsageDistinctResponsesAndReplay(t *testing.T) {
	svc, eb, payload := nativeRunUsageFixture(t)
	for _, id := range []string{"response-1", "response-2", "response-1"} {
		payload.Data.UsageObservation.ProviderResponseID = id
		if err := eb.Publish(context.Background(), events.BuildAgentStreamSubject(payload.RunSessionID), bus.NewEvent(events.AgentStream, "test", payload)); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := svc.ListCostEvents(context.Background(), "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("native responses collapsed or replay duplicated: %+v", rows)
	}
	total, err := svc.RepoForTest().SumCosts(context.Background(), "ws-1")
	if err != nil || total != 246 {
		t.Fatalf("response spend=%d err=%v, want 246", total, err)
	}
}

func TestTasklessNativeUsageCapturedModelPricing(t *testing.T) {
	for _, tc := range []struct {
		name, scope, model, current            string
		usageSuppressed, observationSuppressed bool
		priced                                 bool
	}{
		{name: "captured direct", scope: "direct", model: "captured-model", priced: true},
		{name: "captured child", scope: "child", model: "child-model", current: "parent-model", priced: true},
		{name: "direct fallback", scope: "direct", current: "parent-model", priced: true},
		{name: "child missing model", scope: "child", current: "parent-model"},
		{name: "usage suppressed", scope: "direct", model: "captured-model", usageSuppressed: true},
		{name: "reported cache write suppressed", scope: "direct", model: "captured-model", observationSuppressed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, eb, payload := nativeRunUsageFixture(t)
			svc.SetPricingLookup(&fakePricingLookup{hit: true, pricing: shared.ModelPricing{InputPerMillion: 1_000_000, OutputPerMillion: 2_000_000}})
			payload.Data.Usage.ProviderReportedCostPresent = false
			payload.Data.Usage.ProviderReportedCostSubcents = 0
			payload.Data.Usage.PriceSuppressed = tc.usageSuppressed
			payload.Data.UsageObservation.PriceSuppressed = tc.observationSuppressed
			if tc.observationSuppressed {
				v := int64(10)
				payload.Data.UsageObservation.ReportedCacheWriteTokens = &v
			}
			payload.Data.UsageObservation.Scope = tc.scope
			payload.Data.UsageObservation.Model = tc.model
			payload.Data.CurrentModelID = tc.current
			if err := eb.Publish(context.Background(), events.BuildAgentStreamSubject(payload.RunSessionID), bus.NewEvent(events.AgentStream, "test", payload)); err != nil {
				t.Fatal(err)
			}
			rows, err := svc.ListCostEvents(context.Background(), "ws-1")
			if err != nil || len(rows) != 1 {
				t.Fatalf("rows=%v err=%v", rows, err)
			}
			wantModel := tc.model
			if wantModel == "" && tc.scope == "direct" {
				wantModel = tc.current
			}
			if rows[0].Model != wantModel {
				t.Fatalf("model=%q want %q", rows[0].Model, wantModel)
			}
			wantSource := models.CostSourceUnpriced
			wantCost := int64(0)
			if tc.priced {
				wantSource = models.CostSourceModelsDevList
				wantCost = 45
			}
			if rows[0].CostSource == nil || *rows[0].CostSource != wantSource || rows[0].CostSubcents != wantCost {
				t.Fatalf("source=%v cost=%d want %s/%d", rows[0].CostSource, rows[0].CostSubcents, wantSource, wantCost)
			}
		})
	}
}

func TestTasklessNativeUnavailableUsageSurvivesReplayAndBudgetRead(t *testing.T) {
	ctx := context.Background()
	svc, eb, payload := nativeRunUsageFixture(t)
	svc.SetPricingLookup(&fakePricingLookup{hit: true, pricing: shared.ModelPricing{InputPerMillion: 1_000_000, OutputPerMillion: 2_000_000}})
	measured := payload.Data.Usage
	payload.Data.Usage = nil
	payload.Data.UsageObservation.Completeness = "partial"
	for i := 0; i < 2; i++ {
		if err := eb.Publish(ctx, events.BuildAgentStreamSubject(payload.RunSessionID), bus.NewEvent(events.AgentStream, "test", payload)); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil || len(rows) != 1 {
		t.Fatalf("identified amount-free response disappeared or duplicated: %#v error=%v", rows, err)
	}
	encoded, err := json.Marshal(rows[0])
	if err != nil {
		t.Fatal(err)
	}
	var projected map[string]interface{}
	if err := json.Unmarshal(encoded, &projected); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"tokens_in", "tokens_cached_in", "tokens_out", "cost_subcents"} {
		if value, exists := projected[key]; !exists || value != nil {
			t.Fatalf("unavailable coverage fabricated %s=%v in %s", key, value, encoded)
		}
	}
	if projected["usage_unavailable"] != true || projected["cost_source"] != "unpriced" {
		t.Fatalf("coverage/provenance missing after durable read: %s", encoded)
	}
	payload.Data.Usage = measured
	payload.Data.UsageObservation.Completeness = "exact"
	if err := eb.Publish(ctx, events.BuildAgentStreamSubject(payload.RunSessionID), bus.NewEvent(events.AgentStream, "test", payload)); err != nil {
		t.Fatal(err)
	}
	window, err := svc.RepoForTest().SpendWindowForAgent(ctx, payload.AgentProfileID, time.Time{}, false, time.Now().Add(time.Hour))
	if err != nil || !window.Degraded || window.PricedSubcents != 123 {
		t.Fatalf("amount-free coverage did not degrade independently priced spend: %#v error=%v", window, err)
	}
	rows, err = svc.ListCostEvents(ctx, "ws-1")
	if err != nil || len(rows) != 2 {
		t.Fatalf("known response control lost: %#v error=%v", rows, err)
	}
}

func TestTasklessNativeMeasuredZeroRemainsMeasured(t *testing.T) {
	ctx := context.Background()
	svc, eb, payload := nativeRunUsageFixture(t)
	payload.Data.Usage = &streams.PromptUsage{
		InputTokensPresent: true, OutputTokensPresent: true, ProviderReportedCostPresent: true,
	}
	if err := eb.Publish(ctx, events.BuildAgentStreamSubject(payload.RunSessionID), bus.NewEvent(events.AgentStream, "test", payload)); err != nil {
		t.Fatal(err)
	}
	rows, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil || len(rows) != 1 {
		t.Fatalf("measured-zero response missing: %#v error=%v", rows, err)
	}
	encoded, err := json.Marshal(rows[0])
	if err != nil {
		t.Fatal(err)
	}
	var projected map[string]interface{}
	if err := json.Unmarshal(encoded, &projected); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"tokens_in", "tokens_cached_in", "tokens_out", "cost_subcents"} {
		if projected[key] != float64(0) {
			t.Fatalf("measured zero became unavailable %s=%v in %s", key, projected[key], encoded)
		}
	}
}

func TestTaskOwnedNativeUnavailableUsageRetainsPricedSpend(t *testing.T) {
	ctx := context.Background()
	svc, eb := newTestServiceWithBus(t)
	createTestAgent(t, svc, "ws-1", "worker")
	insertTestTask(t, svc, "task", "ws-1")
	setTestTaskAssignee(t, svc, "task", "worker")
	payload := lifecycle.SessionPromptUsageEventPayload{
		TaskID: "task", SessionID: "session", AgentProfileID: "worker", Model: "captured-model",
		TurnID: "turn", UsageEventID: "unavailable-response",
		UsageObservation: &streams.NativeUsageObservation{
			SchemaVersion: 1, Source: "response", Scope: "direct", Completeness: "partial",
			ProviderThreadID: "thread", ProviderTurnID: "native-turn", ProviderResponseID: "response",
		},
	}
	for i := 0; i < 2; i++ {
		if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session"), bus.NewEvent(events.SessionPromptUsageUpdated, "orchestrator", payload)); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil || len(rows) != 1 {
		t.Fatalf("task-owned coverage disappeared or duplicated: %+v err=%v", rows, err)
	}
	if !rows[0].UsageUnavailable || rows[0].CostSource == nil || *rows[0].CostSource != models.CostSourceUnpriced {
		t.Fatalf("task-owned gap became a measured amount: %+v", rows[0])
	}
	payload.UsageEventID = "response"
	payload.UsageObservation.Completeness = "exact"
	payload.Usage = &streams.PromptUsage{
		InputTokens: 5, InputTokensPresent: true, OutputTokens: 20, OutputTokensPresent: true,
		ProviderReportedCostSubcents: 123, ProviderReportedCostPresent: true,
	}
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session"), bus.NewEvent(events.SessionPromptUsageUpdated, "orchestrator", payload)); err != nil {
		t.Fatal(err)
	}
	window, err := svc.RepoForTest().SpendWindowForAgent(ctx, "worker", time.Time{}, false, time.Now().Add(time.Hour))
	if err != nil || !window.Degraded || window.PricedSubcents != 123 {
		t.Fatalf("task-owned coverage failed budget projection: %+v err=%v", window, err)
	}
	rows, err = svc.ListCostEvents(ctx, "ws-1")
	if err != nil || len(rows) != 2 {
		t.Fatalf("coverage suppressed independently priced work: %+v err=%v", rows, err)
	}
}
