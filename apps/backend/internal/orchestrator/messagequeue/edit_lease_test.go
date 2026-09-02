package messagequeue

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEditLeaseProtectsTargetWithoutPausingQueuePolicy(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	first, err := svc.QueueMessage(ctx, "session-lease", "task", "first", "", "user", false, nil)
	require.NoError(t, err)
	second, err := svc.QueueMessage(ctx, "session-lease", "task", "second", "", "user", false, nil)
	require.NoError(t, err)

	lease, err := svc.BeginEdit(ctx, second.SessionID, second.ID, "connection-a")
	require.NoError(t, err)
	require.Equal(t, int64(0), lease.TargetRevision)

	got, ok, autoRun := svc.ReserveQueuedWithAutoRun(ctx, first.SessionID)
	require.True(t, autoRun)
	require.True(t, ok)
	require.Equal(t, first.ID, got.ID)
	got, ok, autoRun = svc.ReserveQueuedWithAutoRun(ctx, second.SessionID)
	require.True(t, autoRun)
	require.False(t, ok)
	require.Nil(t, got)

	_, err = svc.BeginEdit(ctx, second.SessionID, second.ID, "connection-b")
	require.ErrorIs(t, err, ErrEditConflict)

	revision, err := svc.UpdateMessageWithLease(ctx, second.SessionID, second.ID, lease.LeaseID,
		"operation-1", "connection-a", lease.TargetRevision, "edited", nil, nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), revision)

	revision, err = svc.UpdateMessageWithLease(ctx, second.SessionID, second.ID, lease.LeaseID,
		"operation-1", "connection-a", lease.TargetRevision, "edited", nil, nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), revision)

	_, err = svc.UpdateMessageWithLease(ctx, second.SessionID, second.ID, lease.LeaseID,
		"operation-2", "connection-a", lease.TargetRevision, "stale", nil, nil)
	require.ErrorIs(t, err, ErrEditRevisionConflict)

	require.NoError(t, svc.EndEdit(ctx, second.SessionID, second.ID, lease.LeaseID, "connection-a"))
	got, ok, _ = svc.ReserveQueuedWithAutoRun(ctx, second.SessionID)
	require.True(t, ok)
	require.Equal(t, second.ID, got.ID)
}

func TestEditLeaseRejectsForeignReleaseAndUpdate(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	entry, err := svc.QueueMessage(ctx, "session-lease-foreign", "task", "body", "", "user", false, nil)
	require.NoError(t, err)
	lease, err := svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
	require.NoError(t, err)

	err = svc.EndEdit(ctx, entry.SessionID, entry.ID, lease.LeaseID, "connection-b")
	require.ErrorIs(t, err, ErrEditLeaseNotFound)
	_, err = svc.UpdateMessageWithLease(ctx, entry.SessionID, entry.ID, lease.LeaseID,
		"operation-1", "connection-b", 0, "tampered", nil, nil)
	require.ErrorIs(t, err, ErrEditLeaseNotFound)
	require.False(t, errors.Is(err, ErrEditRevisionConflict))
}

func TestEditLeaseBlocksTargetedDrains(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	entry, err := svc.QueueMessage(ctx, "session-lease-target", "task", "body", "", "user", false, nil)
	require.NoError(t, err)
	_, err = svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
	require.NoError(t, err)

	got, ok := svc.TakeQueued(ctx, entry.SessionID)
	require.False(t, ok)
	require.Nil(t, got)
	_, err = svc.ClaimSendNow(ctx, entry.SessionID, []QueuedMessage{*entry})
	require.ErrorIs(t, err, ErrEditConflict)
}
