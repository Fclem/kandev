package handlers

import (
	"context"
	"encoding/json"
	"expvar"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/user/store"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

const sessionUserActivityAcknowledgementEnabledKey = "enabled"
const (
	userActivityOutcomeRejected  = "rejected"
	userActivityOutcomeDisabled  = "disabled"
	userActivityOutcomeCoalesced = "coalesced"
	userActivityOutcomeRecorded  = "recorded"
)

var sessionUserActivityBeatsTotal = expvar.NewMap("session_user_activity_beats_total")

type sessionUserActivityBeatRequest struct {
	SessionID string `json:"session_id"`
}

func (h *TaskHandlers) wsSessionUserActivityBeat(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req sessionUserActivityBeatRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil || req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	identity, ok := authn.IdentityFromContext(ctx)
	actorID := store.DefaultUserID
	if ok && identity.UserID != "" {
		actorID = identity.UserID
	}
	outcome, err := h.service.RecordUserActivity(ctx, actorID, req.SessionID)
	if err != nil {
		h.logger.Warn("Failed to record session user activity", zap.String("session_id", req.SessionID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Unable to record session activity", nil)
	}
	sessionUserActivityBeatsTotal.Add(outcome, 1)
	h.logger.Debug("session user activity beat", zap.String("session_id", req.SessionID))
	if outcome == userActivityOutcomeRejected {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Task session not found", map[string]interface{}{"outcome": userActivityOutcomeRejected})
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]bool{sessionUserActivityAcknowledgementEnabledKey: outcome != userActivityOutcomeDisabled})
}
