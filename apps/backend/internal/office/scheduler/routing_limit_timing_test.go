package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.1
func TestHandlePostStartFailureLimitDelay(t *testing.T) {
	repo := newTestRepoSched(t)
	ss := buildScheduler(t, repo, newFakeTaskStarter())
	run := seedRoutedRun(t, repo)
	observed := time.Date(2030, 10, 3, 12, 0, 0, 123000000, time.UTC)
	delay := int64(274579000)
	message := "You've hit your session limit"
	classified := routingerr.Classify(routingerr.Input{
		Phase: routingerr.PhaseStreaming, ProviderID: "claude-acp", Stderr: message,
		OccurredAt: observed, RetryAfter: time.Duration(delay) * time.Millisecond,
		DiagnosticSource: streams.ProviderErrorSourceACPPrompt,
	})
	want := observed.Add(time.Duration(delay) * time.Millisecond)
	if classified.ResetHint == nil || !classified.ResetHint.Equal(want) {
		t.Fatalf("classified reset hint = %v, want %v", classified.ResetHint, want)
	}
	handled, err := ss.HandlePostStartFailure(context.Background(), run, makeAgent(), "", &streams.ProviderError{
		Source: streams.ProviderErrorSourceACPPrompt, Message: message, OccurredAt: observed, RetryAfterMs: &delay,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("quota failure not handled")
	}
	attempts, err := repo.ListRouteAttempts(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].ResetHint == nil || !attempts[0].ResetHint.Equal(want) {
		t.Fatalf("attempt timing %v, want %v", attempts, want)
	}
}
