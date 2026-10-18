package handlers

import (
	"bytes"
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type userActivityHandlerRepo struct {
	outcome   string
	actorID   string
	sessionID string
}

func (r *userActivityHandlerRepo) RecordUserActivity(_ context.Context, actorID, sessionID string, _ time.Time) (string, error) {
	r.actorID = actorID
	r.sessionID = sessionID
	return r.outcome, nil
}

func (*userActivityHandlerRepo) SumUserActivity(context.Context, []string) ([]models.UserActivitySummary, error) {
	return nil, nil
}

func TestSessionUserActivityBeatAcknowledgements(t *testing.T) {
	for _, outcome := range []string{userActivityOutcomeRejected, userActivityOutcomeDisabled, userActivityOutcomeCoalesced, userActivityOutcomeRecorded} {
		t.Run(outcome, func(t *testing.T) {
			repo := &userActivityHandlerRepo{outcome: outcome}
			log := newTestLogger(t)
			svc := service.NewService(service.Repos{UserActivity: repo}, nil, log, service.RepositoryDiscoveryConfig{})
			h := &TaskHandlers{service: svc, logger: log}
			request, err := ws.NewRequest("beat-1", ws.ActionSessionUserActivityBeat, map[string]string{"session_id": "session-1"})
			if err != nil {
				t.Fatal(err)
			}
			response, err := h.wsSessionUserActivityBeat(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			var want *ws.Message
			if outcome == userActivityOutcomeRejected {
				want, err = ws.NewError(request.ID, request.Action, ws.ErrorCodeNotFound, "Task session not found", map[string]interface{}{"outcome": userActivityOutcomeRejected})
			} else {
				want, err = ws.NewResponse(request.ID, request.Action, map[string]bool{sessionUserActivityAcknowledgementEnabledKey: outcome != userActivityOutcomeDisabled})
			}
			if err != nil {
				t.Fatal(err)
			}
			if response.ID != want.ID || response.Type != want.Type || response.Action != want.Action ||
				!bytes.Equal(response.Payload, want.Payload) || !reflect.DeepEqual(response.Metadata, want.Metadata) {
				t.Fatalf("response = %+v, want protocol payload %+v", response, want)
			}
			if repo.actorID != "default-user" || repo.sessionID != "session-1" {
				t.Fatalf("record call actor/session = %q/%q", repo.actorID, repo.sessionID)
			}
		})
	}
}
