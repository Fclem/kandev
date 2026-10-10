package orchestrator

import (
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"testing"
	"time"
)

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.1
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.4
func TestClassifyKanbanFailureLimitDelayAndSource(t *testing.T) {
	observed := time.Date(2026, 10, 3, 12, 0, 0, 123000000, time.UTC)
	delay := int64(274579000)
	data := watcher.AgentEventData{AgentID: "omp-acp", ProviderError: &streams.ProviderError{
		Source: "omp_acp", ProviderID: "omp-acp", Message: "This request would exceed your account's monthly spend limit",
		OccurredAt: observed, RetryAfterMs: &delay,
	}}
	got := classifyKanbanFailure(data)
	if got.Code != routingerr.CodeQuotaLimited {
		t.Fatalf("converted OMP = %s", got.Code)
	}
	want := observed.Add(274579000 * time.Millisecond)
	if got.ResetHint == nil || !got.ResetHint.Equal(want) {
		t.Fatalf("reset %v, want %v", got.ResetHint, want)
	}
	data.ProviderError.Source = streams.ProviderErrorSourceACPPrompt
	if got := classifyKanbanFailure(data); got.Code != routingerr.CodeAgentRuntime {
		t.Fatalf("unattested OMP = %s", got.Code)
	}
}
