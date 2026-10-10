package acp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const observedOMPSpendEnvelope = `429 {"type":"error","error":{"type":"rate_limit_error","message":"This request would exceed your account's monthly spend limit. Please try again later."},"request_id":"req_011CfL9vJs9DYV45eqL6jbh9"} retry-after-ms=274579000`
const ompTestModel = "anthropic/claude-opus-5-5"

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.5
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.6
func TestOMPPromptEndObservedFrame(t *testing.T) {
	before := time.Now().UTC()
	events := runOMPPromptEnd(t, "omp-acp", func(_ *Adapter, conn *sdk.AgentSideConnection) {
		sendOMPTextFrame(t, conn, "session-handoff", observedOMPSpendEnvelope)
	})
	after := time.Now().UTC()
	var failure *streams.ProviderError
	var errors, completes, chunks int
	for _, event := range events {
		switch event.Type {
		case streams.EventTypeError:
			errors++
			failure = event.ProviderError
			if event.PromptGeneration != 7 || event.SessionID != "session-handoff" {
				t.Fatalf("failure belongs to a different prompt: %+v", event)
			}
		case streams.EventTypeComplete:
			completes++
		case streams.EventTypeMessageChunk:
			chunks++
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "req_") {
			t.Fatalf("request identifier crossed the normalized event boundary: %s", encoded)
		}
	}
	if errors != 1 || completes != 0 || chunks != 0 {
		t.Fatalf("events = %+v; want one terminal failure, no completion or raw envelope", events)
	}
	if failure == nil || !failure.Valid() || failure.Source != "omp_acp" || failure.ProviderID != "omp-acp" || failure.ModelID != ompTestModel {
		t.Fatalf("provider diagnostic identity = %+v", failure)
	}
	if failure.Message != "This request would exceed your account's monthly spend limit. Please try again later" {
		t.Fatalf("sanitized error message = %q", failure.Message)
	}
	if failure.RetryAfterMs == nil || *failure.RetryAfterMs != 274579000 {
		t.Fatalf("retry delay = %v; want exact 274579000 milliseconds", failure.RetryAfterMs)
	}
	if failure.OccurredAt.Before(before) || failure.OccurredAt.After(after) {
		t.Fatalf("diagnostic observation time %v lies outside prompt execution", failure.OccurredAt)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.5
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.7
func TestOMPPromptEndEnvelopeBoundaries(t *testing.T) {
	rate := `429 {"type":"error","error":{"type":"rate_limit_error","message":"Too many requests"}}`
	for _, tc := range []struct {
		name    string
		agent   string
		chunks  []string
		failure bool
		delay   int64
	}{
		{name: "rate envelope without retry hint", chunks: []string{rate}, failure: true},
		{name: "trimmed envelope", chunks: []string{" \n" + rate + "\n "}, failure: true},
		{name: "eight day delay remains numeric", chunks: []string{rate + " retry-after-ms=691200000"}, failure: true, delay: 691200000},
		{name: "zero delay is not a reset", chunks: []string{rate + " retry-after-ms=0"}, failure: true},
		{name: "unrepresentable delay is not a reset", chunks: []string{rate + " retry-after-ms=99999999999999999999999"}, failure: true},
		{name: "repeated current envelope emits one failure", chunks: []string{rate, rate}, failure: true},
		{name: "later assistant progress clears candidate", chunks: []string{rate, "The retry succeeded"}},
		{name: "later empty assistant chunk clears candidate", chunks: []string{rate, ""}},
		{name: "quoted inside prose", chunks: []string{"The error was " + rate}},
		{name: "quoted JSON", chunks: []string{`"` + rate + `"`}},
		{name: "ordinary assistant text", chunks: []string{"The task is finished"}},
		{name: "other agent", agent: "claude-acp", chunks: []string{rate}},
		{name: "malformed JSON", chunks: []string{`429 {"type":"error","error":`}},
		{name: "wrong envelope type", chunks: []string{strings.Replace(rate, `"type":"error"`, `"type":"message"`, 1)}},
		{name: "other provider error type", chunks: []string{strings.Replace(rate, "rate_limit_error", "authentication_error", 1)}},
		{name: "missing error message", chunks: []string{`429 {"type":"error","error":{"type":"rate_limit_error"}}`}},
		{name: "arbitrary trailing text", chunks: []string{rate + " retrying now"}},
		{name: "malformed suffix", chunks: []string{rate + " retry-after-ms=oops"}},
		{name: "split envelope is not a final complete envelope", chunks: []string{rate[:20], rate[20:]}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agent := tc.agent
			if agent == "" {
				agent = "omp-acp"
			}
			events := runOMPPromptEnd(t, agent, func(_ *Adapter, conn *sdk.AgentSideConnection) {
				for _, chunk := range tc.chunks {
					sendOMPTextFrame(t, conn, "session-handoff", chunk)
				}
			})
			var failures, completes int
			for _, event := range events {
				if event.Type == streams.EventTypeComplete {
					completes++
				}
				if event.Type != streams.EventTypeError {
					continue
				}
				failures++
				if event.ProviderError == nil {
					t.Fatal("missing structured provider failure")
				}
				delay := event.ProviderError.RetryAfterMs
				if tc.delay == 0 && delay != nil || tc.delay != 0 && (delay == nil || *delay != tc.delay) {
					t.Fatalf("numeric retry delay = %v; want %d", delay, tc.delay)
				}
			}
			if tc.failure && (failures != 1 || completes != 0) || !tc.failure && (failures != 0 || completes != 1) {
				t.Fatalf("failures=%d completes=%d; expected failure=%t; events=%+v", failures, completes, tc.failure, events)
			}
		})
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.4
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.5
func TestOMPPromptEndRejectsStaleAndForeignEvidence(t *testing.T) {
	for _, tc := range []struct {
		name         string
		session      string
		generation   uint64
		currentFirst bool
	}{
		{name: "stale envelope does not fail current prompt", session: "session-handoff", generation: 6},
		{name: "foreign session does not fail current prompt", session: "different-session", generation: 7},
		{name: "stale progress does not clear current failure", session: "session-handoff", generation: 6, currentFirst: true},
		{name: "foreign progress does not clear current failure", session: "different-session", generation: 7, currentFirst: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := runOMPPromptEnd(t, "omp-acp", func(a *Adapter, _ *sdk.AgentSideConnection) {
				text := observedOMPSpendEnvelope
				if tc.currentFirst {
					a.handleACPUpdate(cursorMessageNotification("session-handoff", observedOMPSpendEnvelope), 7)
					text = "Unrelated older progress"
				}
				a.handleACPUpdate(cursorMessageNotification(tc.session, text), tc.generation)
			})
			var failures, completes int
			for _, event := range events {
				if event.Type == streams.EventTypeError {
					failures++
				}
				if event.Type == streams.EventTypeComplete {
					completes++
				}
			}
			if tc.currentFirst && (failures != 1 || completes != 0) || !tc.currentFirst && (failures != 0 || completes != 1) {
				t.Fatalf("failures=%d completes=%d; current candidate=%t", failures, completes, tc.currentFirst)
			}
		})
	}
}

func runOMPPromptEnd(t *testing.T, agentID string, inject func(*Adapter, *sdk.AgentSideConnection)) []AgentEvent {
	t.Helper()
	a, fake, conn := setupHandoffFakeAgent(t)
	a.agentID = agentID
	a.normalizer = NewNormalizer(agentID)
	a.dialect = newACPDialect(agentID)
	ctx := context.Background()
	if err := a.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewSession(ctx, nil); err != nil {
		t.Fatal(err)
	}
	_ = drainEvents(a)
	done := make(chan error, 1)
	go func() { done <- a.Prompt(ctx, "Try the selected model", nil, 7) }()
	select {
	case <-fake.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("prompt did not reach the ACP agent")
	}
	sendCapturedUpdate(t, conn, `{"sessionId":"session-handoff","update":{"sessionUpdate":"config_option_update","configOptions":[{"type":"select","id":"model","name":"Model","currentValue":"anthropic/claude-opus-5-5","options":[{"value":"anthropic/claude-opus-5-5","name":"Opus"}]}]}}`)
	a.syncNotifQueue()
	_ = drainEvents(a)
	inject(a, conn)
	fake.releasePrompts()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ACP prompt did not settle")
	}
	return drainEvents(a)
}

func sendOMPTextFrame(t *testing.T, conn *sdk.AgentSideConnection, session, text string) {
	t.Helper()
	encoded, err := json.Marshal(makeNotification(session, sdk.SessionUpdate{
		AgentMessageChunk: &sdk.SessionUpdateAgentMessageChunk{Content: sdk.TextBlock(text)},
	}))
	if err != nil {
		t.Fatal(err)
	}
	sendCapturedUpdate(t, conn, string(encoded))
}
