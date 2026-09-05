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

func TestDurableSessionTransferIncludesInFlightOrdinaryAttachment(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	service := newAutoMergeTestServiceWithRepository(t, repo, DefaultMaxPerSession)
	queued, err := service.QueueMessage(
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
	reserved, ok := service.ReserveQueued(ctx, queued.SessionID)
	require.True(t, ok)
	require.Equal(t, queued.ID, reserved.ID)
	assert.Empty(t, service.GetStatus(ctx, queued.SessionID).Entries)
	prepareCalls := 0

	err = service.TransferSessionWithDurablePreparation(
		ctx,
		queued.TaskID,
		queued.SessionID,
		"session-new",
		func(context.Context) error {
			prepareCalls++
			compensations, listErr := service.ListSessionTransferCompensations(ctx)
			require.NoError(t, listErr)
			require.Len(t, compensations, 1)
			assert.Equal(t, []string{queued.ID}, compensations[0].EntryIDs)
			return nil
		},
		nil,
	)

	require.NoError(t, err)
	assert.Equal(t, 1, prepareCalls)
	pending, err := service.ListPendingQueueDispatches(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, "session-new", pending[0].Message.SessionID)
	assert.Equal(t, queued.ID, pending[0].Message.ID)
}

func TestDurableSessionTransferIncludesCleanupOnlyAttachmentClaim(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	service := newAutoMergeTestServiceWithRepository(t, repo, DefaultMaxPerSession)
	cleanup := AttachmentCleanup{
		SessionID: "session-cleanup-only-old", EntryID: "entry-cleanup-only",
		OperationID: "operation-cleanup-only", TaskID: "task-cleanup-only",
		Attachments: []MessageAttachment{{AttachmentID: "attachment-cleanup-only"}},
	}
	require.NoError(t, service.UpsertAttachmentCleanup(ctx, cleanup))
	prepareCalls := 0

	err := service.TransferSessionWithDurablePreparation(
		ctx,
		cleanup.TaskID,
		cleanup.SessionID,
		"session-cleanup-only-new",
		func(context.Context) error {
			prepareCalls++
			compensations, listErr := service.ListSessionTransferCompensations(ctx)
			require.NoError(t, listErr)
			require.Len(t, compensations, 1)
			assert.Equal(t, []string{cleanup.EntryID}, compensations[0].EntryIDs)
			return nil
		},
		nil,
	)

	require.NoError(t, err)
	assert.Equal(t, 1, prepareCalls)
	stored, err := service.GetAttachmentCleanup(
		ctx, cleanup.SessionID, cleanup.EntryID, cleanup.OperationID,
	)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, "session-cleanup-only-new", stored.CurrentSessionID)
}
