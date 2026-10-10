package orchestrator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTurnTimingConsumesOnlyMatchingCompleteMeasurement(t *testing.T) {
	started := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	firstOutput := started.Add(125 * time.Millisecond)
	closed := started.Add(2 * time.Second)
	record := turnTimingRecord{turnID: "turn-a", startedAt: started, firstOutputAt: firstOutput}

	got, ok := record.measurement("turn-a", closed, false)
	require.True(t, ok)
	require.Equal(t, int64(125), got["ttft_ms"])
	require.Equal(t, int64(1875), got["generation_ms"])
}

func TestTurnTimingRejectsCancelledAndMismatchedCompletions(t *testing.T) {
	started := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	record := turnTimingRecord{turnID: "turn-a", startedAt: started, firstOutputAt: started.Add(time.Second)}

	_, ok := record.measurement("turn-b", started.Add(2*time.Second), false)
	require.False(t, ok)
	_, ok = record.measurement("turn-a", started.Add(2*time.Second), true)
	require.False(t, ok)
	got, ok := record.measurement("turn-a", time.Time{}, false)
	require.True(t, ok, "a missing close mark keeps TTFT")
	require.Equal(t, int64(1000), got["ttft_ms"])
	require.NotContains(t, got, "generation_ms")
}

func TestTurnTimingBindsSyntheticTurnAtFirstOutputOnce(t *testing.T) {
	svc := &Service{}
	started := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	firstOutput := started.Add(125 * time.Millisecond)

	svc.startTurnTiming("session", "execution", "", started)
	svc.recordTurnFirstOutput("session", "execution", "turn-a", firstOutput)
	svc.recordTurnFirstOutput("session", "execution", "turn-b", firstOutput.Add(time.Second))

	svc.turnTimingMu.Lock()
	got := svc.turnTimings["session"]
	svc.turnTimingMu.Unlock()
	require.Equal(t, "turn-a", got.turnID)
	require.Equal(t, firstOutput, got.firstOutputAt)
}

func TestTurnTimingRepeatedStartReplacesPriorTurn(t *testing.T) {
	svc := &Service{}
	started := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	svc.startTurnTiming("session", "execution", "turn-a", started)
	svc.recordTurnFirstOutput("session", "execution", "turn-a", started.Add(time.Second))

	nextStart := started.Add(2 * time.Second)
	svc.startTurnTiming("session", "execution", "turn-b", nextStart)
	svc.closeTurnTiming("session", "execution", "turn-a", nextStart.Add(time.Second))

	svc.turnTimingMu.Lock()
	got := svc.turnTimings["session"]
	svc.turnTimingMu.Unlock()
	require.Equal(t, "turn-b", got.turnID)
	require.True(t, got.firstOutputAt.IsZero())
	require.True(t, got.completedAt.IsZero())
}
func TestTurnTimingRejectsMismatchedCloseMark(t *testing.T) {
	started := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		sessionID   string
		executionID string
		turnID      string
	}{
		{name: "session", sessionID: "other-session", executionID: "execution", turnID: "turn"},
		{name: "execution", sessionID: "session", executionID: "other-execution", turnID: "turn"},
		{name: "turn", sessionID: "session", executionID: "execution", turnID: "other-turn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &Service{}
			svc.startTurnTiming("session", "execution", "turn", started)
			svc.recordTurnFirstOutput("session", "execution", "turn", started.Add(time.Second))

			svc.closeTurnTiming(tt.sessionID, tt.executionID, tt.turnID, started.Add(2*time.Second))

			svc.turnTimingMu.Lock()
			got := svc.turnTimings["session"]
			svc.turnTimingMu.Unlock()
			require.True(t, got.completedAt.IsZero())
		})
	}
}

func TestTurnTimingUsesTerminalCloseBoundary(t *testing.T) {
	svc := &Service{}
	started := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	firstOutput := started.Add(time.Second)
	terminalClose := started.Add(4 * time.Second)
	svc.startTurnTiming("session", "execution", "turn", started)
	svc.recordTurnFirstOutput("session", "execution", "turn", firstOutput)

	svc.closeTurnTiming("session", "execution", "turn", terminalClose)

	svc.turnTimingMu.Lock()
	got := svc.turnTimings["session"]
	svc.turnTimingMu.Unlock()
	measurement, ok := got.measurement("turn", got.completedAt, false)
	require.True(t, ok)
	require.Equal(t, int64(3000), measurement["generation_ms"])
}

func TestTurnTimingPreservesReadyCloseMark(t *testing.T) {
	svc := &Service{}
	started := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	readyClosed := started.Add(3 * time.Second)
	svc.startTurnTiming("session", "execution", "turn", started)
	svc.recordTurnFirstOutput("session", "execution", "turn", started.Add(time.Second))
	svc.closeTurnTiming("session", "execution", "turn", readyClosed)
	svc.closeTurnTiming("session", "execution", "turn", started.Add(10*time.Second))

	svc.turnTimingMu.Lock()
	got := svc.turnTimings["session"]
	svc.turnTimingMu.Unlock()
	require.Equal(t, readyClosed, got.completedAt)
}
