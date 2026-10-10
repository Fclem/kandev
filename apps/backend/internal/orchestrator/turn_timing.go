package orchestrator

import (
	"context"
	"time"

	"go.uber.org/zap"
)

type turnTimingRecord struct {
	turnID           string
	executionID      string
	promptGeneration uint64
	startedAt        time.Time
	firstOutputAt    time.Time
	completedAt      time.Time
}

func (r turnTimingRecord) measurement(turnID string, completedAt time.Time, cancelled bool) (map[string]interface{}, bool) {
	if cancelled || r.turnID == "" || r.turnID != turnID || r.startedAt.IsZero() ||
		r.firstOutputAt.IsZero() || completedAt.IsZero() || completedAt.Before(r.firstOutputAt) {
		return nil, false
	}
	return map[string]interface{}{
		"ttft_ms":       r.firstOutputAt.Sub(r.startedAt).Milliseconds(),
		"generation_ms": completedAt.Sub(r.firstOutputAt).Milliseconds(),
	}, true
}

func (s *Service) startTurnTiming(sessionID, executionID string, promptGeneration uint64, turnID string, receivedAt time.Time) {
	if sessionID == "" || executionID == "" || receivedAt.IsZero() {
		return
	}
	s.turnTimingMu.Lock()
	if s.turnTimings == nil {
		s.turnTimings = make(map[string]turnTimingRecord)
	}
	s.turnTimings[sessionID] = turnTimingRecord{
		turnID: turnID, executionID: executionID, promptGeneration: promptGeneration, startedAt: receivedAt,
	}
	s.turnTimingMu.Unlock()
}

func (s *Service) recordTurnFirstOutput(sessionID, executionID string, promptGeneration uint64, turnID string, receivedAt time.Time) {
	if sessionID == "" || executionID == "" || receivedAt.IsZero() {
		return
	}
	s.turnTimingMu.Lock()
	record, ok := s.turnTimings[sessionID]
	if ok && record.executionID == executionID && record.promptGeneration == promptGeneration &&
		record.firstOutputAt.IsZero() {
		if record.turnID == "" {
			record.turnID = turnID
		}
		if record.turnID != "" && record.turnID == turnID {
			record.firstOutputAt = receivedAt
			s.turnTimings[sessionID] = record
		}
	}
	s.turnTimingMu.Unlock()
}

func (s *Service) closeTurnTiming(sessionID, executionID string, promptGeneration uint64, turnID string, closedAt time.Time) {
	if sessionID == "" || executionID == "" || turnID == "" || closedAt.IsZero() {
		return
	}
	s.turnTimingMu.Lock()
	record, ok := s.turnTimings[sessionID]
	if ok && record.executionID == executionID && record.promptGeneration == promptGeneration &&
		record.turnID == turnID && record.completedAt.IsZero() {
		record.completedAt = closedAt
		s.turnTimings[sessionID] = record
	}
	s.turnTimingMu.Unlock()
}

func (s *Service) discardTurnTiming(sessionID string) {
	if sessionID == "" {

		return
	}
	s.turnTimingMu.Lock()
	delete(s.turnTimings, sessionID)
	s.turnTimingMu.Unlock()
}

func (s *Service) persistTurnTiming(ctx context.Context, sessionID, turnID string, cancelled bool) {
	if sessionID == "" || turnID == "" || s.turnService == nil {
		return
	}
	s.turnTimingMu.Lock()
	record, ok := s.turnTimings[sessionID]
	if ok && record.turnID == turnID {
		delete(s.turnTimings, sessionID)
	}
	s.turnTimingMu.Unlock()
	if !ok || record.turnID != turnID {
		return
	}
	metadata, ok := record.measurement(turnID, record.completedAt, cancelled)
	if !ok {
		return
	}
	if err := s.turnService.PatchTurnMetadata(ctx, sessionID, turnID, map[string]interface{}{"timing": metadata}); err != nil {
		s.logger.Warn("failed to persist turn timing", zap.String("session_id", sessionID), zap.String("turn_id", turnID), zap.Error(err))
	}
}
