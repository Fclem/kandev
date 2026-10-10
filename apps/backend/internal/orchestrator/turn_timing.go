package orchestrator

import (
	"context"
	"time"

	"go.uber.org/zap"
)

type turnTimingRecord struct {
	turnID        string
	executionID   string
	startedAt     time.Time
	firstOutputAt time.Time
	completedAt   time.Time
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

func (s *Service) timingNow() time.Time {
	if s.timingClock != nil {
		return s.timingClock()
	}
	return time.Now()
}

// Timing records are correlated by session, execution and Kandev turn ID. The
// turn ID is unique per turn; prompt generations are not stamped on every
// stream event kind (turn_started carries none on ACP transports), so they
// cannot take part in the match.
func (s *Service) startTurnTiming(sessionID, executionID, turnID string, receivedAt time.Time) {
	if sessionID == "" || executionID == "" || receivedAt.IsZero() {
		return
	}
	s.turnTimingMu.Lock()
	if s.turnTimings == nil {
		s.turnTimings = make(map[string]turnTimingRecord)
	}
	s.turnTimings[sessionID] = turnTimingRecord{turnID: turnID, executionID: executionID, startedAt: receivedAt}
	s.turnTimingMu.Unlock()
}

func (s *Service) recordTurnFirstOutput(sessionID, executionID, turnID string, receivedAt time.Time) {
	if sessionID == "" || executionID == "" || receivedAt.IsZero() {
		return
	}
	s.turnTimingMu.Lock()
	record, ok := s.turnTimings[sessionID]
	if ok && record.executionID == executionID && record.firstOutputAt.IsZero() {
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

func (s *Service) closeTurnTiming(sessionID, executionID, turnID string, closedAt time.Time) {
	if sessionID == "" || executionID == "" || turnID == "" || closedAt.IsZero() {
		return
	}
	s.turnTimingMu.Lock()
	record, ok := s.turnTimings[sessionID]
	if ok && record.executionID == executionID && record.turnID == turnID && record.completedAt.IsZero() {
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
