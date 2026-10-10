package lifecycle

import (
	"context"
	"errors"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.2
func TestSendPromptAfterAdmissionOwnershipFailureNeverReachesProvider(t *testing.T) {
	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)
	sm := NewSessionManager(newSessionTestLogger(), newTestStopCh(t))
	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	ctx := context.Background()
	if err := client.StreamUpdates(ctx, func(event agentctl.AgentEvent) {}, nil, nil); err != nil {
		t.Fatal(err)
	}
	waitForWSConnected(t, mock)
	execution := &AgentExecution{
		ID: "owned-execution", TaskID: "owned-task", SessionID: "owned-session",
		Status: v1.AgentStatusRunning, agentctl: client, promptDoneCh: make(chan PromptCompletionSignal, 1),
	}
	refused := errors.New("durable probe owner write refused")
	var admittedGeneration uint64
	ctx = WithPromptAdmissionHook(ctx, func(executionID string, generation uint64) error {
		if executionID != execution.ID {
			t.Errorf("ownership targeted execution %q instead of %q", executionID, execution.ID)
		}
		admittedGeneration = generation
		return refused
	})
	_, err := sm.SendPrompt(ctx, execution, "original launch input", false, nil, true)
	if !errors.Is(err, refused) || admittedGeneration == 0 {
		t.Fatalf("durable ownership refusal was ignored: generation=%d error=%v", admittedGeneration, err)
	}
	for _, action := range mock.getActionLog() {
		if action == "agent.prompt" {
			t.Fatal("provider received input before durable probe ownership")
		}
	}
}
