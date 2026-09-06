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

func TestDurableSessionTransferFencesConcurrentSourceInsert(t *testing.T) {
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
	prepared := make(chan struct{})
	release := make(chan struct{})
	transferDone := make(chan error, 1)
	go func() {
		transferDone <- service.TransferSessionWithDurableAttachmentPreparation(
			ctx,
			"task",
			"session-old",
			"session-new",
			func(context.Context, []string) error {
				close(prepared)
				<-release
				return nil
			},
			nil,
		)
	}()
	<-prepared

	err = repo.Insert(ctx, &QueuedMessage{
		ID: "concurrent", SessionID: "session-old", TaskID: "task",
		Content: "concurrent", QueuedBy: QueuedByUser,
	}, DefaultMaxPerSession)
	require.ErrorContains(t, err, "session transfer in progress")
	close(release)
	require.NoError(t, <-transferDone)
}

func TestAttachmentCleanupFollowsActiveSessionTransfer(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	persistence := repo.(interface {
		UpsertSessionTransferCompensation(context.Context, SessionTransferCompensation) error
		UpsertAttachmentCleanup(context.Context, AttachmentCleanup) error
		GetAttachmentCleanup(context.Context, string, string, string) (*AttachmentCleanup, error)
	})
	require.NoError(t, persistence.UpsertSessionTransferCompensation(ctx, SessionTransferCompensation{
		OperationID: "transfer-operation",
		TaskID:      "task", FromSessionID: "session-old", ToSessionID: "session-new",
	}))
	cleanup := AttachmentCleanup{
		SessionID: "session-old", CurrentSessionID: "session-old",
		EntryID: "entry", OperationID: "operation", TaskID: "task",
		Attachments: []MessageAttachment{{AttachmentID: "attachment"}},
	}

	require.NoError(t, persistence.UpsertAttachmentCleanup(ctx, cleanup))
	stored, err := persistence.GetAttachmentCleanup(
		ctx, cleanup.SessionID, cleanup.EntryID, cleanup.OperationID,
	)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, "session-new", stored.CurrentSessionID)
}

func TestAttachmentCleanupDeleteWaitsForActiveSessionTransfer(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	persistence := repo.(interface {
		UpsertSessionTransferCompensation(context.Context, SessionTransferCompensation) error
		UpsertAttachmentCleanup(context.Context, AttachmentCleanup) error
		DeleteAttachmentCleanup(context.Context, string, string, string) error
		GetAttachmentCleanup(context.Context, string, string, string) (*AttachmentCleanup, error)
	})
	cleanup := AttachmentCleanup{
		SessionID: "session-old", EntryID: "entry", OperationID: "operation", TaskID: "task",
		Attachments: []MessageAttachment{{AttachmentID: "attachment"}},
	}
	require.NoError(t, persistence.UpsertAttachmentCleanup(ctx, cleanup))
	require.NoError(t, persistence.UpsertSessionTransferCompensation(ctx, SessionTransferCompensation{
		OperationID: "transfer-operation",
		TaskID:      "task", FromSessionID: "session-old", ToSessionID: "session-new",
	}))

	err := persistence.DeleteAttachmentCleanup(
		ctx, cleanup.SessionID, cleanup.EntryID, cleanup.OperationID,
	)
	require.ErrorIs(t, err, ErrSessionTransferInProgress)
	stored, err := persistence.GetAttachmentCleanup(
		ctx, cleanup.SessionID, cleanup.EntryID, cleanup.OperationID,
	)
	require.NoError(t, err)
	require.NotNil(t, stored)
}

func TestSessionTransferCompensationRejectsActiveReplacement(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	persistence := repo.(interface {
		UpsertSessionTransferCompensation(context.Context, SessionTransferCompensation) error
		DeleteSessionTransferCompensation(context.Context, string, string, string, string) error
		ListSessionTransferCompensations(context.Context) ([]SessionTransferCompensation, error)
	})
	first := SessionTransferCompensation{
		OperationID: "transfer-first",
		TaskID:      "task", FromSessionID: "session-old", ToSessionID: "session-new",
		EntryIDs: []string{"entry-first"},
	}
	require.NoError(t, persistence.UpsertSessionTransferCompensation(ctx, first))

	replacement := first
	replacement.OperationID = "transfer-replacement"
	replacement.EntryIDs = []string{"entry-replacement"}
	err := persistence.UpsertSessionTransferCompensation(ctx, replacement)

	require.ErrorIs(t, err, ErrSessionTransferInProgress)
	stored, listErr := persistence.ListSessionTransferCompensations(ctx)
	require.NoError(t, listErr)
	require.Len(t, stored, 1)
	assert.Equal(t, first.EntryIDs, stored[0].EntryIDs)
	first.EntryIDs = []string{"entry-final"}
	require.NoError(t, persistence.UpsertSessionTransferCompensation(ctx, first))
	stored, listErr = persistence.ListSessionTransferCompensations(ctx)
	require.NoError(t, listErr)
	require.Len(t, stored, 1)
	assert.Equal(t, first.EntryIDs, stored[0].EntryIDs)
	err = persistence.DeleteSessionTransferCompensation(
		ctx, replacement.OperationID, first.TaskID, first.FromSessionID, first.ToSessionID,
	)
	require.ErrorIs(t, err, ErrSessionTransferOwnershipLost)
	stored, listErr = persistence.ListSessionTransferCompensations(ctx)
	require.NoError(t, listErr)
	require.Len(t, stored, 1)
	assert.Equal(t, first.OperationID, stored[0].OperationID)
	require.NoError(t, persistence.DeleteSessionTransferCompensation(
		ctx, first.OperationID, first.TaskID, first.FromSessionID, first.ToSessionID,
	))
	stored, listErr = persistence.ListSessionTransferCompensations(ctx)
	require.NoError(t, listErr)
	assert.Empty(t, stored)
}

func TestActiveSessionTransferFencesExistingQueueMutations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(context.Context, Repository, *QueuedMessage, *QueuedMessage) error
	}{
		{
			name: "edit",
			mutate: func(ctx context.Context, repo Repository, first, _ *QueuedMessage) error {
				return repo.UpdateContent(ctx, first.SessionID, first.ID, "edited", nil, QueuedByUser)
			},
		},
		{
			name: "purge",
			mutate: func(ctx context.Context, repo Repository, first, _ *QueuedMessage) error {
				_, err := repo.PurgeSession(ctx, first.SessionID)
				return err
			},
		},
		{
			name: "purge task",
			mutate: func(ctx context.Context, repo Repository, first, _ *QueuedMessage) error {
				_, err := repo.PurgeTask(ctx, first.TaskID)
				return err
			},
		},
		{
			name: "merge",
			mutate: func(ctx context.Context, repo Repository, _, second *QueuedMessage) error {
				_, err := repo.MergeIntoAbove(ctx, second.SessionID, second.ID, QueuedByUser)
				return err
			},
		},
		{
			name: "reorder",
			mutate: func(ctx context.Context, repo Repository, first, second *QueuedMessage) error {
				return repo.ReorderEntries(ctx, first.SessionID, []string{second.ID, first.ID})
			},
		},
		{
			name: "take pending move",
			mutate: func(ctx context.Context, repo Repository, first, _ *QueuedMessage) error {
				_, err := repo.TakePendingMove(ctx, first.SessionID)
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			repo := newTestSQLiteRepo(t)
			first := insertTestEntry(t, repo, "session-old", "task", "first", QueuedByUser, nil, nil)
			second := insertTestEntry(t, repo, "session-old", "task", "second", QueuedByUser, nil, nil)
			require.NoError(t, repo.SetPendingMove(ctx, first.SessionID, &PendingMove{
				MoveID: "move", TaskID: "task", WorkflowID: "workflow", WorkflowStepID: "step",
			}))
			persistence := repo.(interface {
				UpsertSessionTransferCompensation(context.Context, SessionTransferCompensation) error
			})
			require.NoError(t, persistence.UpsertSessionTransferCompensation(ctx, SessionTransferCompensation{
				OperationID: "transfer-operation",
				TaskID:      "task", FromSessionID: first.SessionID, ToSessionID: "session-new",
			}))

			err := test.mutate(ctx, repo, first, second)

			require.ErrorIs(t, err, ErrSessionTransferInProgress)
		})
	}
}
