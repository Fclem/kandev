package messagequeue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestMaintainSessionTransferCompensationLeaseStopIsIdempotent(t *testing.T) {
	ctx := context.Background()
	service := setupService(t)
	compensation := SessionTransferCompensation{
		OperationID:   "transfer-operation",
		TaskID:        "task",
		FromSessionID: "session-old",
		ToSessionID:   "session-new",
	}
	stopCtx, stop := service.MaintainSessionTransferCompensationLease(
		ctx, compensation, compensation.OperationID,
	)
	require.NoError(t, stopCtx.Err())
	require.NoError(t, stop())
	require.NoError(t, stop())
}

func TestDurableSessionTransferRetainsCompensationAfterLeaseLoss(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	sqlRepo := repo.(*sqliteRepository)
	service := newAutoMergeTestServiceWithRepository(t, repo, DefaultMaxPerSession)
	service.sessionTransferCompensationLeaseRenewInterval = time.Millisecond
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
	preparationStarted := make(chan struct{})
	transferDone := make(chan error, 1)
	go func() {
		transferDone <- service.TransferSessionWithDurableAttachmentPreparation(
			ctx,
			"task",
			"session-old",
			"session-new",
			func(prepareCtx context.Context, _ []string) error {
				close(preparationStarted)
				<-prepareCtx.Done()
				return prepareCtx.Err()
			},
			func(context.Context, []string) error {
				return errors.New("rollback blocked")
			},
		)
	}()
	<-preparationStarted
	compensations, err := service.ListSessionTransferCompensations(ctx)
	require.NoError(t, err)
	require.Len(t, compensations, 1)
	_, err = sqlRepo.db.Exec(`
		UPDATE queue_session_transfer_compensations
		SET recovery_owner = 'another-owner'
		WHERE operation_id = ?
	`, compensations[0].OperationID)
	require.NoError(t, err)

	select {
	case err = <-transferDone:
		require.Error(t, err)
		assert.ErrorContains(t, err, "session transfer lease lost")
	case <-time.After(2 * time.Second):
		t.Fatal("transfer did not stop after lease loss")
	}
	compensations, err = service.ListSessionTransferCompensations(ctx)
	require.NoError(t, err)
	require.Len(t, compensations, 1)
}
