package handlers

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestSessionActivityBeatLogsOnlySessionID(t *testing.T) {
	core, entries := observer.New(zap.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	repo := &userActivityHandlerRepo{outcome: userActivityOutcomeRecorded}
	svc := service.NewService(service.Repos{UserActivity: repo}, nil, log, service.RepositoryDiscoveryConfig{})
	h := &TaskHandlers{service: svc, logger: log}
	msg, err := ws.NewRequest("beat", ws.ActionSessionUserActivityBeat, map[string]string{"session_id": "private-session"})
	require.NoError(t, err)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "private-actor", Synthetic: true})
	response, err := h.wsSessionUserActivityBeat(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	beats := entries.FilterMessage("session user activity beat").All()
	require.Len(t, beats, 1, "privacy checks must observe an actual beat diagnostic")
	require.Equal(t, map[string]interface{}{"session_id": "private-session"}, beats[0].ContextMap())
}
