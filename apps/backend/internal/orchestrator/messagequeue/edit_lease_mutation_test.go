package messagequeue

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEditLeasePreventsAutomaticMergeIntoTarget(t *testing.T) {
	svc := setupService(t)
	svc.SetAutoMergeEnabled(true)
	ctx := context.Background()
	first, err := svc.QueueMessage(ctx, "session-lease-auto-merge", "task", "first", "", "user", false, nil)
	require.NoError(t, err)
	_, err = svc.BeginEdit(ctx, first.SessionID, first.ID, "connection-a")
	require.NoError(t, err)

	_, err = svc.QueueMessage(ctx, first.SessionID, "task", "second", "", "user", false, nil)
	require.NoError(t, err)

	entries := svc.GetStatus(ctx, first.SessionID).Entries
	require.Len(t, entries, 2)
	require.Equal(t, "first", entries[0].Content)
	require.Equal(t, "second", entries[1].Content)
}

func TestEditLeasePreventsManualMergeIntoTarget(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	first, err := svc.QueueMessage(ctx, "session-lease-manual-merge", "task", "first", "", "user", false, nil)
	require.NoError(t, err)
	second, err := svc.QueueMessage(ctx, first.SessionID, "task", "second", "", "user", false, nil)
	require.NoError(t, err)
	_, err = svc.BeginEdit(ctx, first.SessionID, first.ID, "connection-a")
	require.NoError(t, err)

	_, err = svc.MergeIntoAbove(ctx, first.SessionID, second.ID, "user")
	require.ErrorIs(t, err, ErrEditConflict)

	entries := svc.GetStatus(ctx, first.SessionID).Entries
	require.Len(t, entries, 2)
	require.Equal(t, "first", entries[0].Content)
	require.Equal(t, "second", entries[1].Content)
	require.False(t, errors.Is(err, ErrEntryNotFound))
}

func TestEditLeaseInvalidatedBySessionReplacement(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	entry, err := svc.QueueMessage(ctx, "session-lease-replace", "task", "original", "", "user", false, nil)
	require.NoError(t, err)
	lease, err := svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
	require.NoError(t, err)

	replacement := *entry
	replacement.Content = "restored"
	require.NoError(t, svc.RestoreSession(ctx, entry.SessionID, []QueuedMessage{replacement}, nil))

	_, err = svc.UpdateMessageWithLease(ctx, entry.SessionID, entry.ID, lease.LeaseID,
		"operation-1", "connection-a", lease.TargetRevision, "stale edit", nil, nil)
	require.ErrorIs(t, err, ErrEditLeaseNotFound)
	require.Equal(t, "restored", svc.GetStatus(ctx, entry.SessionID).Entries[0].Content)
}

func TestEditLeaseBlocksAppendIntoTarget(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	first, err := svc.QueueMessage(ctx, "session-lease-append", "task", "first", "", "user", false, nil)
	require.NoError(t, err)
	second, err := svc.QueueMessage(ctx, first.SessionID, "task", "second", "", "user", false, nil)
	require.NoError(t, err)
	_, err = svc.BeginEdit(ctx, first.SessionID, second.ID, "connection-a")
	require.NoError(t, err)

	_, _, err = svc.AppendContent(ctx, first.SessionID, "task", " appended", "", "user", false, nil)
	require.ErrorIs(t, err, ErrEditConflict)
	require.Equal(t, "second", svc.GetStatus(ctx, first.SessionID).Entries[1].Content)
}

func TestEditLeaseAllowsAppendToOtherEntry(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	first, err := svc.QueueMessage(ctx, "session-lease-append-other", "task", "first", "", "user", false, nil)
	require.NoError(t, err)
	_, err = svc.QueueMessage(ctx, first.SessionID, "task", "second", "", "user", false, nil)
	require.NoError(t, err)
	_, err = svc.BeginEdit(ctx, first.SessionID, first.ID, "connection-a")
	require.NoError(t, err)

	merged, appended, err := svc.AppendContent(ctx, first.SessionID, "task", " appended", "", "user", false, nil)
	require.NoError(t, err)
	require.True(t, appended)
	require.Equal(t, "second\n\n---\n\n appended", merged.Content)
}
