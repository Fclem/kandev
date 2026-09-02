package messagequeue

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type blockingPurgeRepository struct {
	Repository
	purgeStarted chan struct{}
	purgeRelease chan struct{}
	listStarted  chan struct{}
	listOnce     sync.Once
}

func (r *blockingPurgeRepository) PurgeTask(ctx context.Context, taskID string) (int, error) {
	close(r.purgeStarted)
	<-r.purgeRelease
	return r.Repository.PurgeTask(ctx, taskID)
}

func (r *blockingPurgeRepository) ListBySession(ctx context.Context, sessionID string) ([]QueuedMessage, error) {
	r.listOnce.Do(func() { close(r.listStarted) })
	return r.Repository.ListBySession(ctx, sessionID)
}

func TestPurgeTaskBlocksBeginEditUntilLeaseInvalidation(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	entry, err := svc.QueueMessage(ctx, "session-purge-race", "task-purge-race", "body", "", QueuedByUser, false, nil)
	require.NoError(t, err)

	blocking := &blockingPurgeRepository{
		Repository:   svc.repo,
		purgeStarted: make(chan struct{}),
		purgeRelease: make(chan struct{}),
		listStarted:  make(chan struct{}),
	}
	svc.repo = blocking

	purgeDone := make(chan error, 1)
	go func() {
		_, purgeErr := svc.PurgeTask(ctx, entry.TaskID)
		purgeDone <- purgeErr
	}()
	<-blocking.purgeStarted

	beginDone := make(chan error, 1)
	go func() {
		_, beginErr := svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
		beginDone <- beginErr
	}()

	select {
	case <-blocking.listStarted:
		t.Fatal("BeginEdit read the queue while PurgeTask was in progress")
	case <-time.After(100 * time.Millisecond):
	}

	close(blocking.purgeRelease)
	require.NoError(t, <-purgeDone)
	require.ErrorIs(t, <-beginDone, ErrEditLeaseNotFound)
}
