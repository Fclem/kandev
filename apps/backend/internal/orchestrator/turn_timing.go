package orchestrator

import (
	"context"
	"time"

	"go.uber.org/zap"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
)

// turnTimingRetention bounds how long an unconsumed record may stay in
// turnTimings. Records are normally deleted when the turn's complete-stream is
// admitted; a session that never receives one (deleted task, crashed
// execution) is pruned the next time any session starts a turn.
const turnTimingRetention = 24 * time.Hour

type turnTimingRecord struct {
	turnID           string
	executionID      string
	promptGeneration uint64
	startedAt        time.Time
	firstOutputAt    time.Time
	completedAt      time.Time
	// trackedAt is the wall-clock creation time used only for retention; the
	// measurement marks above come from the injectable timing clock.
	trackedAt time.Time
}

// measurement returns ttft_ms whenever start and first output exist, and adds
// generation_ms only when a valid close mark follows the first output.
func (r turnTimingRecord) measurement(turnID string, completedAt time.Time, cancelled bool) (map[string]interface{}, bool) {
	if cancelled || r.turnID == "" || r.turnID != turnID || r.startedAt.IsZero() ||
		r.firstOutputAt.IsZero() || r.firstOutputAt.Before(r.startedAt) {
		return nil, false
	}
	out := map[string]interface{}{"ttft_ms": r.firstOutputAt.Sub(r.startedAt).Milliseconds()}
	if !completedAt.IsZero() && !completedAt.Before(r.firstOutputAt) {
		out["generation_ms"] = completedAt.Sub(r.firstOutputAt).Milliseconds()
	}
	return out, true
}

func (r *turnTimingRecord) matchesCompletion(executionID, turnID string, generation uint64) bool {
	return r.executionID == executionID && r.turnID == turnID &&
		(r.promptGeneration == 0 || generation == 0 || r.promptGeneration == generation)
}

func (s *Service) timingNow() time.Time {
	if s.timingClock != nil {
		return s.timingClock()
	}
	return time.Now()
}

// Records correlate session, execution, Kandev turn ID and available prompt
// generation. Generation-zero frames retain execution/turn correlation.
func (s *Service) startTurnTiming(sessionID, executionID, turnID string, promptGeneration uint64, receivedAt time.Time) {
	if sessionID == "" || executionID == "" || receivedAt.IsZero() {
		return
	}
	s.turnTimingMu.Lock()
	if s.turnTimings == nil {
		s.turnTimings = make(map[string]turnTimingRecord)
	}
	now := time.Now()
	s.pruneExpiredTurnTimingsLocked(now)
	s.turnTimings[sessionID] = turnTimingRecord{
		turnID: turnID, executionID: executionID, promptGeneration: promptGeneration,
		startedAt: receivedAt, trackedAt: now,
	}
	s.turnTimingMu.Unlock()
}

func (s *Service) pruneExpiredTurnTimingsLocked(now time.Time) {
	for sessionID, record := range s.turnTimings {
		if now.Sub(record.trackedAt) > turnTimingRetention {
			delete(s.turnTimings, sessionID)
		}
	}
}

func (s *Service) recordTurnFirstOutput(sessionID, executionID, turnID string, promptGeneration uint64, receivedAt time.Time) {
	if sessionID == "" || executionID == "" || receivedAt.IsZero() {
		return
	}
	s.turnTimingMu.Lock()
	record, ok := s.turnTimings[sessionID]
	if ok && record.executionID == executionID && record.firstOutputAt.IsZero() &&
		(record.promptGeneration == 0 || promptGeneration == 0 || record.promptGeneration == promptGeneration) {
		if record.turnID == "" {
			record.turnID = turnID
		}
		if record.turnID != "" && record.turnID == turnID {
			if record.promptGeneration == 0 {
				record.promptGeneration = promptGeneration
			}
			record.firstOutputAt = receivedAt
			s.turnTimings[sessionID] = record
		}
	}
	s.turnTimingMu.Unlock()
}

func (s *Service) closeTurnTiming(sessionID, executionID, turnID string, promptGeneration uint64, closedAt time.Time) {
	if sessionID == "" || executionID == "" || turnID == "" || closedAt.IsZero() {
		return
	}
	s.turnTimingMu.Lock()
	record, ok := s.turnTimings[sessionID]
	if ok && record.completedAt.IsZero() && record.matchesCompletion(executionID, turnID, promptGeneration) {
		if record.promptGeneration == 0 {
			record.promptGeneration = promptGeneration
		}
		record.completedAt = closedAt
		s.turnTimings[sessionID] = record
	}
	s.turnTimingMu.Unlock()
}

func (s *Service) discardTurnTiming(sessionID, executionID, turnID string) {
	if sessionID == "" || executionID == "" {
		return
	}
	s.turnTimingMu.Lock()
	if record, ok := s.turnTimings[sessionID]; ok && record.executionID == executionID &&
		(turnID == "" || record.turnID == turnID) {
		delete(s.turnTimings, sessionID)
	}
	s.turnTimingMu.Unlock()
}

func (s *Service) persistTurnTiming(ctx context.Context, payload *agentruntime.AgentStreamEventPayload, turnID string) {
	if payload == nil || payload.Data == nil || turnID == "" || s.turnService == nil {
		return
	}
	sessionID, executionID := payload.SessionID, payload.ExecutionID
	if executionID == "" {
		executionID = payload.AgentID
	}
	if sessionID == "" || executionID == "" {
		return
	}
	s.turnTimingMu.Lock()
	record, ok := s.turnTimings[sessionID]
	owned := ok && record.matchesCompletion(executionID, turnID, payload.Data.PromptGeneration)
	if owned {
		delete(s.turnTimings, sessionID)
	}
	s.turnTimingMu.Unlock()
	if !owned {
		return
	}
	metadata, ok := record.measurement(turnID, record.completedAt, extractStopReason(payload) == stopReasonCancelled)
	if !ok {
		return
	}
	if err := s.turnService.PatchTurnMetadata(ctx, sessionID, turnID, map[string]interface{}{"timing": metadata}); err != nil {
		s.logger.Warn("failed to persist turn timing", zap.String("session_id", sessionID), zap.String("turn_id", turnID), zap.Error(err))
	}
}
