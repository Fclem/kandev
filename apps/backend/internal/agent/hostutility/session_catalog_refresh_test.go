package hostutility

import (
	"context"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
)

type catalogProbeCommitter interface {
	CommitIfCurrent(context.Context, func() error) error
}

// @covers AC-AGENTS-SESSION-CATALOG-001.3
func TestSavedProfileCatalogRefreshRuntimeCurrency(t *testing.T) {
	manager, server := newProfileProbeTestManager(t, agents.NewClaudeACP(), nil, func(w http.ResponseWriter, _ agentctlutil.ProbeRequest) {
		_, _ = w.Write([]byte(`{"success":true,"models":[{"id":"model-a","name":"A"}]}`))
	})
	defer server.Close()
	ctx := context.Background()
	request := ProfileCapabilityRequest{Context: ProfileProbeContext{Scope: "user:profile"}, Refresh: true}
	probe, err := manager.ProbeProfileCapabilities(ctx, "claude-acp", request)
	if err != nil {
		t.Fatal(err)
	}
	guard, ok := any(probe).(catalogProbeCommitter)
	if !ok {
		t.Fatal("completed probe cannot guard publication against runtime activation")
	}
	writes := 0
	if err := guard.CommitIfCurrent(ctx, func() error { writes++; return nil }); err != nil {
		t.Fatal(err)
	}
	manager.InvalidateCapabilities("claude-acp")
	if err := guard.CommitIfCurrent(ctx, func() error { writes++; return nil }); err == nil {
		t.Fatal("old runtime result published after invalidation")
	}
	fresh, err := manager.ProbeProfileCapabilities(ctx, "claude-acp", request)
	if err != nil {
		t.Fatal(err)
	}
	freshGuard := any(fresh).(catalogProbeCommitter)
	if err := freshGuard.CommitIfCurrent(ctx, func() error { writes++; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ProbeProfileCapabilities(ctx, "claude-acp", request); err != nil {
		t.Fatal(err)
	}
	if err := freshGuard.CommitIfCurrent(ctx, func() error { writes++; return nil }); err == nil {
		t.Fatal("older refresh published after a new probe generation")
	}
	if writes != 2 {
		t.Fatalf("committed writes = %d, want only current probes", writes)
	}
}
