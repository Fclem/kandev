package main

import (
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	transportacp "github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acp"
)

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.1
func TestProviderLimitScenarioEmitsRealClassifiableLimitForTheRequestedModel(t *testing.T) {
	a := newOverloadedTestAgent()
	_, err, handled := a.handleProviderLimit("s1", "/provider-limit mock-fast 41")
	request, ok := err.(*acp.RequestError)
	if !handled || !ok {
		t.Fatalf("limit result = handled %t, error %T", handled, err)
	}
	projected := transportacp.ProviderErrorFromError(request, "claude-acp", "mock-fast")
	if projected == nil || projected.RetryAfterMs == nil || *projected.RetryAfterMs != 41000 {
		t.Fatalf("projected limit=%+v", projected)
	}
	failure := routingerr.Classify(routingerr.Input{ProviderID: "claude-acp", Stderr: projected.Message, DiagnosticSource: projected.Source, OccurredAt: projected.OccurredAt, RetryAfter: 41 * time.Second})
	if failure.Code != routingerr.CodeRateLimited || failure.LimitScope == "account" || failure.ResetHint == nil || !failure.ResetHint.Equal(projected.OccurredAt.Add(41*time.Second)) {
		t.Fatalf("classified limit=%+v", failure)
	}
	// The same replay is not another failure once the actual session model changes.
	a.model = "mock-smart"
	_, err, handled = a.handleProviderLimit("s1", "/provider-limit mock-fast 41")
	if handled || err != nil {
		t.Fatalf("fallback model incorrectly failed: handled %t, error %v", handled, err)
	}
}

func TestProviderLimitScenarioRejectsMalformedDurationAndUnrelatedPrompts(t *testing.T) {
	a := newOverloadedTestAgent()
	for _, prompt := range []string{"hello", "/provider-limit", "/provider-limit mock-fast 0", "/provider-limit mock-fast -1", "/provider-limit mock-fast 1.5", "/provider-limit mock-fast 99999999999999999999", "/provider-limit mock-fast 9223372036855", "/provider-limit mock-fast 1 extra"} {
		_, err, handled := a.handleProviderLimit("s1", prompt)
		if handled || err != nil {
			t.Fatalf("invalid scenario %q was interpreted as a provider failure", prompt)
		}
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.4
func TestProviderLimitScenarioResetWindowExpiresExactly(t *testing.T) {
	a := newOverloadedTestAgent()
	now := time.Date(2026, 10, 4, 12, 0, 0, 123456789, time.UTC)
	a.providerLimitNow = func() time.Time { return now }
	_, err, handled := a.handleProviderLimit("s1", "/provider-limit mock-fast 5")
	if !handled || err == nil {
		t.Fatal("initial limit window was not observed")
	}
	now = now.Add(5*time.Second - time.Nanosecond)
	_, err, handled = a.handleProviderLimit("s1", "/provider-limit mock-fast 5")
	if !handled || err == nil {
		t.Fatal("provider window expired before its exact reset")
	}
	now = now.Add(time.Nanosecond)
	_, err, handled = a.handleProviderLimit("s1", "/provider-limit mock-fast 5")
	if handled || err != nil {
		t.Fatal("provider continued rejecting the owned replay after reset")
	}
	_, err, handled = a.handleProviderLimit("sibling", "/provider-limit mock-fast 5")
	if !handled || err == nil {
		t.Fatal("another conversation inherited the elapsed mock scenario")
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.1
func TestProviderLimitQuotaScenarioUsesTheDurableRecoveryPath(t *testing.T) {
	a := newOverloadedTestAgent()
	_, err, handled := a.handleProviderLimit("s1", "/provider-quota mock-fast 31")
	request, ok := err.(*acp.RequestError)
	if !handled || !ok {
		t.Fatalf("quota fixture was not a real provider RPC error: %t, %T", handled, err)
	}
	projected := transportacp.ProviderErrorFromError(request, "claude-acp", "mock-fast")
	failure := routingerr.Classify(routingerr.Input{ProviderID: "claude-acp", Stderr: projected.Message, DiagnosticSource: projected.Source, OccurredAt: projected.OccurredAt, RetryAfter: 31 * time.Second})
	if failure.Code != routingerr.CodeQuotaLimited || failure.LimitScope != "account" || failure.ResetHint == nil {
		t.Fatalf("quota fixture lost its scope or trusted reset: %+v", failure)
	}
	if routingerr.Decide(routingerr.ContextKanban, failure, projected.OccurredAt) == routingerr.DecisionShortRetry {
		t.Fatal("quota fixture was incorrectly absorbed by legacy short retries")
	}
}
