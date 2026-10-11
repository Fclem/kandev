package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestStartTurnUsesReservedLaunchIdentityAtPersistenceBoundary(t *testing.T) {
	svc, _, repo := createTestService(t)
	sessionID := setupTestSession(t, repo)
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	require.NoError(t, err)
	identity := models.ReservedLaunchIdentity{TaskID: session.TaskID, SessionID: sessionID, TurnID: "reserved-retry-turn", SubmissionID: "reserved-submission"}
	ctx := models.WithReservedLaunchIdentity(context.Background(), identity)
	turn, err := svc.StartTurn(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, identity.TurnID, turn.ID)
	stored, err := repo.GetActiveTurn(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, identity.TurnID, stored.ID)
	identity.TaskID = "other-task"
	identity.TurnID = "conflicting-turn"
	_, err = svc.StartTurn(models.WithReservedLaunchIdentity(context.Background(), identity), sessionID)
	require.Error(t, err)
	stored, err = repo.GetActiveTurn(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, turn.ID, stored.ID)
}
