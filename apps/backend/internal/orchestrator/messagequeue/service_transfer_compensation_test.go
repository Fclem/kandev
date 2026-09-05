package messagequeue

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransferSessionWithPreparationRollsBackPreparationError(t *testing.T) {
	ctx := context.Background()
	service := setupService(t)
	queued, err := service.QueueMessage(ctx, "session-old", "task", "handoff", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	prepareErr := errors.New("preparation failed after side effect")
	rollbackCalls := 0

	err = service.TransferSessionWithPreparation(
		ctx,
		"session-old",
		"session-new",
		func(context.Context) error {
			return prepareErr
		},
		func(context.Context) error {
			rollbackCalls++
			return nil
		},
	)

	require.ErrorIs(t, err, prepareErr)
	assert.Equal(t, 1, rollbackCalls)
	status := service.GetStatus(ctx, "session-old")
	require.Len(t, status.Entries, 1)
	assert.Equal(t, queued.ID, status.Entries[0].ID)
	assert.Empty(t, service.GetStatus(ctx, "session-new").Entries)
}

func TestDurableSessionTransferPersistsBeforeExternalPreparation(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	service := newAutoMergeTestServiceWithRepository(t, repo, DefaultMaxPerSession)
	_, err := service.QueueMessage(
		ctx,
		"session-old",
		"task",
		"handoff",
		"",
		QueuedByUser,
		false,
		[]MessageAttachment{{AttachmentID: "attachment"}},
	)
	require.NoError(t, err)
	prepareErr := errors.New("external transfer failed")
	rollbackErr := errors.New("external rollback failed")

	err = service.TransferSessionWithDurablePreparation(
		ctx,
		"task",
		"session-old",
		"session-new",
		func(context.Context) error {
			compensations, listErr := service.ListSessionTransferCompensations(ctx)
			require.NoError(t, listErr)
			require.Len(t, compensations, 1)
			assert.Equal(t, "session-old", compensations[0].FromSessionID)
			return prepareErr
		},
		func(context.Context) error {
			return rollbackErr
		},
	)

	require.ErrorIs(t, err, prepareErr)
	require.ErrorIs(t, err, rollbackErr)
	compensations, listErr := service.ListSessionTransferCompensations(ctx)
	require.NoError(t, listErr)
	require.Len(t, compensations, 1)
	assert.Equal(t, "session-new", compensations[0].ToSessionID)
}
