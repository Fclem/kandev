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
	_, ok = record.measurement("turn-a", time.Time{}, false)
	require.False(t, ok)
}

func TestTurnTimingBindsSyntheticTurnAtFirstOutputOnce(t *testing.T) {
	svc := &Service{}
	started := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	firstOutput := started.Add(125 * time.Millisecond)

	svc.startTurnTiming("session", "", started)
	svc.recordTurnFirstOutput("session", "turn-a", firstOutput)
	svc.recordTurnFirstOutput("session", "turn-b", firstOutput.Add(time.Second))

	svc.turnTimingMu.Lock()
	got := svc.turnTimings["session"]
	svc.turnTimingMu.Unlock()
	require.Equal(t, "turn-a", got.turnID)
	require.Equal(t, firstOutput, got.firstOutputAt)
}

func TestTurnTimingRepeatedStartReplacesPriorTurn(t *testing.T) {
	svc := &Service{}
	started := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	svc.startTurnTiming("session", "turn-a", started)
	svc.recordTurnFirstOutput("session", "turn-a", started.Add(time.Second))

	nextStart := started.Add(2 * time.Second)
	svc.startTurnTiming("session", "turn-b", nextStart)
	svc.closeTurnTiming("session", "turn-a", nextStart.Add(time.Second))

	svc.turnTimingMu.Lock()
	got := svc.turnTimings["session"]
	svc.turnTimingMu.Unlock()
	require.Equal(t, "turn-b", got.turnID)
	require.True(t, got.firstOutputAt.IsZero())
	require.True(t, got.completedAt.IsZero())
}
