package codexappserver

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	protocol "github.com/kandev/kandev/pkg/codexappserver"
)

func TestNormalizeRawResponseUsagePreservesDisjointLedgerComponents(t *testing.T) {
	usage, observation, ok := normalizeRawResponseUsage(protocol.TokenUsageBreakdown{
		InputTokens:           1_000,
		CachedInputTokens:     600,
		CacheWriteInputTokens: 0,
		OutputTokens:          200,
		ReasoningOutputTokens: 80,
		TotalTokens:           1_200,
	}, "thread-1", "turn-1", "response-1", false, "gpt-5")
	if !ok {
		t.Fatal("normalizeRawResponseUsage rejected valid response usage")
	}
	if usage.InputTokens != 400 || usage.CachedReadTokens != 600 || usage.OutputTokens != 200 || usage.TotalTokens != 1_200 {
		t.Fatalf("normalized usage = %#v, want 400 uncached + 600 cached + 200 output = 1200", usage)
	}
	if usage.ThoughtTokens != 0 {
		t.Fatalf("ThoughtTokens = %d, native reasoning is an output subset", usage.ThoughtTokens)
	}
	if observation.ReasoningOutputTokens == nil || *observation.ReasoningOutputTokens != 80 {
		t.Fatalf("reasoning detail = %#v, want 80", observation.ReasoningOutputTokens)
	}
	if observation.PriceSuppressed {
		t.Fatal("zero native cache-write count should not suppress pricing")
	}
}

func TestNormalizeRawResponseUsageSuppressesUnknownCacheWritePricing(t *testing.T) {
	usage, observation, ok := normalizeRawResponseUsage(protocol.TokenUsageBreakdown{
		InputTokens:           100,
		CachedInputTokens:     25,
		CacheWriteInputTokens: 10,
		OutputTokens:          20,
		ReasoningOutputTokens: 5,
		TotalTokens:           120,
	}, "thread-1", "turn-1", "response-1", false, "gpt-5")
	if !ok || usage == nil {
		t.Fatal("normalizeRawResponseUsage rejected valid cache-write usage")
	}
	if !observation.PriceSuppressed || observation.ReportedCacheWriteTokens == nil || *observation.ReportedCacheWriteTokens != 10 {
		t.Fatalf("cache-write detail = %#v, pricing suppression = %t", observation.ReportedCacheWriteTokens, observation.PriceSuppressed)
	}
}

func TestNormalizeRawResponseUsageRejectsImpossibleSubsetAndNegativeCounts(t *testing.T) {
	cases := []protocol.TokenUsageBreakdown{
		{InputTokens: 10, CachedInputTokens: 11, OutputTokens: 2},
		{InputTokens: 10, CachedInputTokens: 2, OutputTokens: 2, ReasoningOutputTokens: 3},
		{InputTokens: -1, OutputTokens: 2},
	}
	for _, raw := range cases {
		if _, _, ok := normalizeRawResponseUsage(raw, "t", "u", "r", false, "m"); ok {
			t.Errorf("normalizeRawResponseUsage accepted invalid usage %#v", raw)
		}
	}
}

func TestRawResponsesRemainDistinctAfterCompletion(t *testing.T) {
	a := NewAdapter(nil, nil)
	t.Cleanup(func() { _ = a.Close() })
	a.threadID = "thread-1"
	a.modelID = "gpt-5"
	a.activeGeneration = 7
	a.beginProviderTurn("thread-1", "native-turn")
	params := func(responseID string) map[string]any {
		return map[string]any{
			"threadId": "thread-1", "turnId": "native-turn", "responseId": responseID,
			"usage": map[string]any{
				"totalTokens": 12, "inputTokens": 10, "cachedInputTokens": 4,
				"cacheWriteInputTokens": 0, "outputTokens": 2, "reasoningOutputTokens": 1,
			},
		}
	}
	a.handleRawResponseCompleted(params("response-a"))
	a.handleTurnCompleted(nil, "thread-1", "thread-1", "native-turn", true)
	a.handleRawResponseCompleted(params("response-b"))
	var responseIDs []string
	for len(responseIDs) < 2 {
		event := <-a.Updates()
		if event.Type != streams.EventTypeUsageObservation {
			continue
		}
		responseIDs = append(responseIDs, event.UsageObservation.ProviderResponseID)
	}
	if responseIDs[0] == responseIDs[1] {
		t.Fatalf("response observations share an ID: %v", responseIDs)
	}
	select {
	case event := <-a.Updates():
		if event.Type == streams.EventTypeUsageObservation {
			t.Fatalf("turn finalization added fallback after exact response events: %s", event.UsageObservation.Source)
		}
	default:
	}
}

func TestDelayedUsageRetainsTurnModelAndGeneration(t *testing.T) {
	a := NewAdapter(nil, nil)
	defer func() { _ = a.Close() }()
	a.threadID = "thread-1"
	a.modelID = "model-before-switch"
	a.activeGeneration = 11
	a.beginProviderTurn("thread-1", "turn-1")
	if err := a.SetModel(context.Background(), "model-after-switch"); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.activeGeneration = 12
	a.mu.Unlock()
	// A duplicate start notification must not replace the original attribution.
	a.beginProviderTurn("thread-1", "turn-1")

	a.handleRawResponseCompleted(map[string]any{
		"threadId": "thread-1", "turnId": "turn-1", "responseId": "response-1",
		"usage": map[string]any{
			"totalTokens": 12, "inputTokens": 10, "cachedInputTokens": 4,
			"cacheWriteInputTokens": 0, "outputTokens": 2, "reasoningOutputTokens": 1,
		},
	})
	event := <-a.Updates()
	if event.Type != streams.EventTypeUsageObservation {
		t.Fatalf("event type = %q, want usage observation", event.Type)
	}
	if event.UsageObservation.Model != "model-before-switch" || event.PromptGeneration != 11 {
		t.Fatalf("delayed usage attribution = model %q generation %d, want original turn model and generation", event.UsageObservation.Model, event.PromptGeneration)
	}
}

func TestDelayedChildUsageRetainsItsParentTurnDuringUnrelatedTurn(t *testing.T) {
	a := NewAdapter(nil, nil)
	defer func() { _ = a.Close() }()
	a.threadID = "root-thread"
	a.modelID = "root-model"
	a.activeGeneration = 42
	a.children["child-thread"] = childBinding{
		toolCallID: "spawn-1", parentThreadID: "root-thread", generation: 7, model: "child-model",
	}
	a.beginProviderTurn("child-thread", "child-turn")
	a.mu.Lock()
	a.activeGeneration = 43
	a.modelID = "unrelated-root-model"
	a.mu.Unlock()

	a.handleRawResponseCompleted(map[string]any{
		"threadId": "child-thread", "turnId": "child-turn", "responseId": "child-response",
		"usage": map[string]any{
			"totalTokens": 9, "inputTokens": 7, "cachedInputTokens": 2,
			"cacheWriteInputTokens": 0, "outputTokens": 2, "reasoningOutputTokens": 1,
		},
	})
	event := <-a.Updates()
	if event.SessionID != "root-thread" || event.ParentToolCallID != "spawn-1" || event.PromptGeneration != 7 {
		t.Fatalf("child usage correlation = session %q parent %q generation %d", event.SessionID, event.ParentToolCallID, event.PromptGeneration)
	}
	if event.UsageObservation.Model != "child-model" || event.UsageObservation.Scope != "child" {
		t.Fatalf("child usage attribution = model %q scope %q", event.UsageObservation.Model, event.UsageObservation.Scope)
	}
}

func TestRawResponseUsageNotificationDecodesOptionalUsage(t *testing.T) {
	var got protocol.RawResponseCompleted
	raw := json.RawMessage(`{"threadId":"thread","turnId":"turn","responseId":"response","usage":null,"usageMetadata":null}`)
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.ResponseID != "response" || got.Usage != nil {
		t.Fatalf("decoded raw response = %#v", got)
	}
}

func TestRawResponseMissingUsagePreservesPartialCoverage(t *testing.T) {
	counts := func(input any, present bool) map[string]any {
		raw := map[string]any{
			"cachedInputTokens": 0, "cacheWriteInputTokens": 0, "outputTokens": 2,
			"reasoningOutputTokens": 1, "totalTokens": 12,
		}
		if present {
			raw["inputTokens"] = input
		}
		return raw
	}
	for _, missing := range []any{nil, counts(-1, true), counts(0.5, true), counts(nil, true), counts(0, false)} {
		t.Run(fmt.Sprint(missing), func(t *testing.T) {
			a := NewAdapter(nil, nil)
			defer func() { _ = a.Close() }()
			a.threadID = "thread-1"
			a.beginProviderTurn("thread-1", "turn-1")
			a.handleRawResponseCompleted(map[string]any{
				"threadId": "thread-1", "turnId": "turn-1", "responseId": "known",
				"usage": counts(10, true),
			})
			a.handleRawResponseCompleted(map[string]any{
				"threadId": "thread-1", "turnId": "turn-1", "responseId": "missing", "usage": missing,
			})
			known := <-a.Updates()
			if known.Usage == nil || known.Usage.OutputTokens != 2 {
				t.Fatalf("known usage lost: %#v", known)
			}
			select {
			case gap := <-a.Updates():
				if gap.Usage != nil || gap.UsageObservation == nil || gap.UsageObservation.Completeness != "partial" ||
					gap.UsageObservation.ProviderResponseID != "missing" {
					t.Fatalf("missing response coverage = %#v", gap)
				}
			default:
				t.Fatal("missing response was discarded without partial coverage")
			}
			select {
			case extra := <-a.Updates():
				t.Fatalf("finalization invented an additional observation: %#v", extra)
			default:
			}
		})
	}
}

func TestRawResponseDefaultCacheWriteRemainsComplete(t *testing.T) {
	for _, input := range []int64{0, 10} {
		t.Run(fmt.Sprint(input), func(t *testing.T) {
			a := NewAdapter(nil, nil)
			defer func() { _ = a.Close() }()
			a.threadID = "thread"
			a.beginProviderTurn("thread", "turn")
			output := input / 5
			a.handleRawResponseCompleted(map[string]any{
				"threadId": "thread", "turnId": "turn", "responseId": "response",
				"usage": map[string]any{
					"inputTokens": input, "cachedInputTokens": 0, "outputTokens": output,
					"reasoningOutputTokens": input / 10, "totalTokens": input + output,
				},
			})
			got := <-a.Updates()
			if got.Usage == nil || got.Usage.InputTokens != input || got.Usage.OutputTokens != output ||
				got.Usage.PriceSuppressed || !got.Usage.CachedWriteTokensPresent ||
				got.UsageObservation.Completeness != "exact" {
				t.Fatalf("default cache-write corrupted measured usage: %#v", got)
			}
		})
	}
}

func TestRawResponseDispatchPreservesInt64Counts(t *testing.T) {
	for _, count := range []int64{9007199254740991, 9007199254740993, 9223372036854775807} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			a := NewAdapter(nil, nil)
			defer func() { _ = a.Close() }()
			a.threadID = "thread"
			a.beginProviderTurn("thread", "turn")
			raw := json.RawMessage(fmt.Sprintf(`{"threadId":"thread","turnId":"turn","responseId":"response","usage":{"inputTokens":%d,"cachedInputTokens":0,"outputTokens":0,"reasoningOutputTokens":0,"totalTokens":%d}}`, count, count))
			a.handleNotification(context.Background(), protocol.NotificationRawResponse, raw)
			got := <-a.Updates()
			if got.Usage == nil || got.Usage.InputTokens != count || got.Usage.TotalTokens != count {
				t.Fatalf("native count %d changed: %#v", count, got.Usage)
			}
		})
	}
}

func TestRawResponseDispatchRejectsInvalidInt64CountsAsCoverage(t *testing.T) {
	for _, count := range []string{"0.5", "-1", "9223372036854775808"} {
		t.Run(count, func(t *testing.T) {
			a := NewAdapter(nil, nil)
			defer func() { _ = a.Close() }()
			a.threadID = "thread"
			a.beginProviderTurn("thread", "turn")
			raw := json.RawMessage(fmt.Sprintf(`{"threadId":"thread","turnId":"turn","responseId":"response","usage":{"inputTokens":%s,"cachedInputTokens":0,"outputTokens":0,"reasoningOutputTokens":0,"totalTokens":0}}`, count))
			a.handleNotification(context.Background(), protocol.NotificationRawResponse, raw)
			got := <-a.Updates()
			if got.Usage != nil || got.UsageObservation == nil || got.UsageObservation.Completeness != streams.NativeUsageCompletenessPartial {
				t.Fatalf("invalid count became measured usage: %#v", got)
			}
		})
	}
}

func dispatchTokenSnapshot(t *testing.T, a *Adapter, turn string, counts map[string]any) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"threadId": "thread", "turnId": turn,
		"tokenUsage": map[string]any{"total": counts},
	})
	if err != nil {
		t.Fatal(err)
	}
	a.handleNotification(context.Background(), protocol.NotificationTokenUsage, raw)
}

func tokenSnapshot(output int64) map[string]any {
	return map[string]any{
		"inputTokens": 0, "cachedInputTokens": 0, "outputTokens": output,
		"reasoningOutputTokens": 0, "totalTokens": output,
	}
}

func TestCumulativeSnapshotsWithoutFinalityCannotChargeTurns(t *testing.T) {
	for _, scenario := range []string{"interim predecessor", "invalid gap and stale replay", "initial invalid then valid"} {
		t.Run(scenario, func(t *testing.T) {
			a := NewAdapter(nil, nil)
			defer func() { _ = a.Close() }()
			a.threadID = "thread"
			a.modelID = "captured-model"
			dispatchTokenSnapshot(t, a, "prior", tokenSnapshot(0))
			a.beginProviderTurn("thread", "first")
			switch scenario {
			case "interim predecessor":
				dispatchTokenSnapshot(t, a, "first", tokenSnapshot(110))
			case "invalid gap and stale replay":
				dispatchTokenSnapshot(t, a, "first", tokenSnapshot(20))
				invalid := tokenSnapshot(20)
				invalid["outputTokens"] = nil
				dispatchTokenSnapshot(t, a, "first", invalid)
				dispatchTokenSnapshot(t, a, "first", tokenSnapshot(10))
			case "initial invalid then valid":
				invalid := tokenSnapshot(0)
				delete(invalid, "inputTokens")
				dispatchTokenSnapshot(t, a, "first", invalid)
				dispatchTokenSnapshot(t, a, "first", tokenSnapshot(0))
			}
			a.handleTurnCompleted(nil, "thread", "thread", "first", true)
			a.beginProviderTurn("thread", "second")
			dispatchTokenSnapshot(t, a, "second", tokenSnapshot(130))
			a.handleTurnCompleted(nil, "thread", "thread", "second", true)
			dispatchTokenSnapshot(t, a, "first", tokenSnapshot(120))
			for len(a.updates) > 0 {
				event := <-a.Updates()
				if event.Type == streams.EventTypeUsageObservation {
					t.Fatalf("unproven final boundary charged %s: %+v", event.OperationID, event.Usage)
				}
			}
			for _, turn := range []string{"first", "second"} {
				output := int64(0)
				if turn == "second" {
					output = 20
				}
				a.handleRawResponseCompleted(map[string]any{
					"threadId": "thread", "turnId": turn, "responseId": turn + "-exact", "usage": tokenSnapshot(output),
				})
				var event streams.AgentEvent
				for {
					event = <-a.Updates()
					if event.Type == streams.EventTypeUsageObservation {
						break
					}
				}
				if event.Usage == nil || event.Usage.Estimated || !event.Usage.OutputTokensPresent ||
					event.Usage.OutputTokens != output || event.UsageObservation.ProviderResponseID != turn+"-exact" {
					t.Fatalf("cumulative ambiguity discarded known response: %+v", event)
				}
			}
		})
	}
}

func TestNativeAttributionRetentionRejectsExpiredResponsesAndPreservesRecentOwnership(t *testing.T) {
	a := NewAdapter(nil, nil)
	t.Cleanup(func() { _ = a.Close() })
	a.threadID = "thread"
	for generation := uint64(1); generation <= 256; generation++ {
		a.mu.Lock()
		a.modelID = fmt.Sprintf("captured-model-%d", generation)
		a.activeGeneration = generation
		a.mu.Unlock()
		start := json.RawMessage(fmt.Sprintf(`{"threadId":"thread","turn":{"id":"turn-%d","status":"inProgress"}}`, generation))
		a.handleNotification(context.Background(), "turn/started", start)
		<-a.Updates()
		complete := json.RawMessage(fmt.Sprintf(`{"threadId":"thread","turn":{"id":"turn-%d","status":"completed"}}`, generation))
		a.handleNotification(context.Background(), "turn/completed", complete)
		<-a.Updates()
	}
	a.mu.Lock()
	a.modelID, a.activeGeneration = "successor-model", 999
	a.mu.Unlock()
	a.handleNotification(context.Background(), "turn/started", json.RawMessage(`{"threadId":"thread","turn":{"id":"successor","status":"inProgress"}}`))
	<-a.Updates()
	response := func(turn, response string) {
		raw := json.RawMessage(fmt.Sprintf(`{"threadId":"thread","turnId":%q,"responseId":%q,"usage":{"inputTokens":10,"cachedInputTokens":4,"outputTokens":2,"reasoningOutputTokens":1,"totalTokens":12}}`, turn, response))
		a.handleNotification(context.Background(), protocol.NotificationRawResponse, raw)
	}
	response("turn-1", "expired")
	response("turn-255", "recent")
	var observed []streams.AgentEvent
	for len(a.Updates()) > 0 {
		observed = append(observed, <-a.Updates())
	}
	if len(observed) != 1 || observed[0].UsageObservation.ProviderResponseID != "recent" {
		t.Fatalf("expired native attribution remained eligible: %+v", observed)
	}
	if observed[0].UsageObservation.Model != "captured-model-255" || observed[0].PromptGeneration != 255 {
		t.Fatalf("retained response borrowed successor ownership: %+v", observed[0])
	}
	a.mu.RLock()
	active := a.turnID
	a.mu.RUnlock()
	if active != "successor" {
		t.Fatalf("delayed response replaced active turn with %q", active)
	}
}

func TestNewSessionCannotReusePreviousNativeAttribution(t *testing.T) {
	server := newProtocolServer(t, func(req map[string]json.RawMessage, write func(any) error) error {
		if readString(req, "method") == "thread/start" {
			return write(resultFrame(req["id"], map[string]any{
				"thread": map[string]any{"id": "thread"}, "model": "session-model",
			}))
		}
		return write(errorFrame(req["id"], -32601, "unsupported"))
	})
	t.Cleanup(server.close)
	a := NewAdapter(nil, logger.Default())
	t.Cleanup(func() { _ = a.Close() })
	if err := a.Connect(server.clientWriter, server.clientReader); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := a.NewSession(context.Background(), nil); err != nil {
			t.Fatal(err)
		}
		for len(a.Updates()) > 0 {
			<-a.Updates()
		}
		if i == 0 {
			a.activeGeneration = 1
			a.beginProviderTurn("thread", "previous")
		}
	}
	a.activeGeneration = 2
	a.beginProviderTurn("thread", "current")
	for _, turn := range []string{"previous", "current"} {
		a.handleRawResponseCompleted(map[string]any{
			"threadId": "thread", "turnId": turn, "responseId": turn,
			"usage": map[string]any{"inputTokens": 10, "cachedInputTokens": 0, "outputTokens": 2, "reasoningOutputTokens": 0, "totalTokens": 12},
		})
	}
	var observations []streams.AgentEvent
	for len(a.Updates()) > 0 {
		observations = append(observations, <-a.Updates())
	}
	if len(observations) != 1 || observations[0].UsageObservation.ProviderResponseID != "current" || observations[0].PromptGeneration != 2 {
		t.Fatalf("fresh session reused old captured ownership: %+v", observations)
	}
}

// The retention bound is resource-contract coverage; ownership is exercised above.
func TestNativeAttributionStateIsBoundedAcrossRootAndChildTurns(t *testing.T) {
	a := NewAdapter(nil, nil)
	t.Cleanup(func() { _ = a.Close() })
	a.threadID = "root"
	a.children["child"] = childBinding{toolCallID: "spawn", parentThreadID: "root", generation: 7, model: "child-model"}
	for i := 0; i < 1024; i++ {
		thread := "root"
		if i%2 != 0 {
			thread = "child"
		}
		a.beginProviderTurn(thread, fmt.Sprintf("turn-%d", i))
		if len(a.turnAttributions) > 128 {
			t.Fatalf("native attribution exceeded its resource bound: %d", len(a.turnAttributions))
		}
	}
	a.beginProviderTurn("child", "turn-1023")
	a.handleRawResponseCompleted(map[string]any{
		"threadId": "child", "turnId": "turn-1023", "responseId": "retained-child",
		"usage": map[string]any{"inputTokens": 10, "cachedInputTokens": 0, "outputTokens": 2, "reasoningOutputTokens": 0, "totalTokens": 12},
	})
	event := <-a.Updates()
	if event.UsageObservation.Model != "child-model" || event.PromptGeneration != 7 || event.UsageObservation.Scope != "child" {
		t.Fatalf("bounded child attribution lost its origin: %+v", event)
	}
}

func TestResumePreservesSameThreadAttributionAndReleasesDifferentThread(t *testing.T) {
	server := newProtocolServer(t, func(req map[string]json.RawMessage, write func(any) error) error {
		var params struct {
			ThreadID string `json:"threadId"`
		}
		if err := json.Unmarshal(req["params"], &params); err != nil {
			return err
		}
		switch readString(req, "method") {
		case "thread/resume", "thread/read":
			return write(resultFrame(req["id"], map[string]any{
				"thread": map[string]any{"id": params.ThreadID, "turns": []any{}}, "model": "resumed-model",
			}))
		case "thread/backgroundTerminals/list":
			return write(resultFrame(req["id"], map[string]any{"data": []any{}}))
		default:
			return write(errorFrame(req["id"], -32601, "unsupported"))
		}
	})
	t.Cleanup(server.close)
	a := NewAdapter(nil, logger.Default())
	t.Cleanup(func() { _ = a.Close() })
	if err := a.Connect(server.clientWriter, server.clientReader); err != nil {
		t.Fatal(err)
	}
	a.threadID, a.modelID, a.activeGeneration = "root", "captured-root", 22
	a.beginProviderTurn("root", "prior")
	if err := a.LoadSession(context.Background(), "root", nil); err != nil {
		t.Fatal(err)
	}
	for len(a.Updates()) > 0 {
		<-a.Updates()
	}
	a.handleNotification(context.Background(), protocol.NotificationRawResponse, json.RawMessage(`{"threadId":"root","turnId":"prior","responseId":"late","usage":{"inputTokens":10,"cachedInputTokens":0,"outputTokens":2,"reasoningOutputTokens":0,"totalTokens":12}}`))
	event := <-a.Updates()
	if event.PromptGeneration != 22 || event.UsageObservation.Model != "captured-root" {
		t.Fatalf("same-thread resume replaced captured attribution: %+v", event)
	}
	if err := a.LoadSession(context.Background(), "other", nil); err != nil {
		t.Fatal(err)
	}
	if len(a.turnAttributions) != 0 {
		t.Fatalf("different-thread resume retained attribution: %+v", a.turnAttributions)
	}
	for _, key := range a.turnAttributionKeys {
		if key != "" {
			t.Fatalf("different-thread resume retained old key %q", key)
		}
	}
}
