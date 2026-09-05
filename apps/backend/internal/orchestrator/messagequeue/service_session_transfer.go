package messagequeue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
)

type durableSessionTransferState struct {
	compensation      *SessionTransferCompensation
	preparationCalled bool
	rollbackSucceeded bool
}

// TransferSessionWithDurablePreparation keeps attachment ownership aligned
// with every queue row, including lifecycle rows hidden from the UI status.
// Durable repositories record the external mutation before it starts so a
// failed rollback or process exit can be reconciled on startup.
func (s *Service) TransferSessionWithDurablePreparation(
	ctx context.Context,
	taskID, oldSessionID, newSessionID string,
	prepare func(context.Context) error,
	rollback func(context.Context) error,
) error {
	state := &durableSessionTransferState{}
	err := s.transferSession(
		ctx,
		oldSessionID,
		newSessionID,
		func(admittedCtx context.Context) error {
			return s.prepareDurableSessionTransfer(
				admittedCtx, taskID, oldSessionID, newSessionID, prepare, state,
			)
		},
		func(rollbackCtx context.Context) error {
			if !state.preparationCalled || rollback == nil {
				state.rollbackSucceeded = true
				return nil
			}
			rollbackErr := rollback(rollbackCtx)
			state.rollbackSucceeded = rollbackErr == nil
			return rollbackErr
		},
	)
	if err != nil {
		if state.compensation != nil && state.rollbackSucceeded {
			err = errors.Join(err, s.deleteSessionTransferCompensation(context.WithoutCancel(ctx), *state.compensation))
		}
		return err
	}
	if state.compensation != nil {
		if deleteErr := s.deleteSessionTransferCompensation(context.WithoutCancel(ctx), *state.compensation); deleteErr != nil {
			s.logger.Warn("session transfer committed with pending compensation record", zap.Error(deleteErr))
		}
	}
	return nil
}

func (s *Service) prepareDurableSessionTransfer(
	ctx context.Context,
	taskID, oldSessionID, newSessionID string,
	prepare func(context.Context) error,
	state *durableSessionTransferState,
) error {
	entries, err := s.repo.ListBySession(ctx, oldSessionID)
	if err != nil {
		return fmt.Errorf("snapshot session queue for transfer: %w", err)
	}
	entryIDs := make([]string, 0, len(entries))
	seenEntryIDs := make(map[string]struct{}, len(entries))
	hasAttachments := false
	for _, entry := range entries {
		entryIDs = append(entryIDs, entry.ID)
		seenEntryIDs[entry.ID] = struct{}{}
		hasAttachments = hasAttachments || len(entry.Attachments) > 0
	}
	if s.PendingQueueDispatchPersistenceAvailable() {
		pending, listErr := s.ListPendingQueueDispatches(ctx)
		if listErr != nil {
			return fmt.Errorf("snapshot in-flight queue dispatches for transfer: %w", listErr)
		}
		for _, dispatch := range pending {
			entry := dispatch.Message
			if entry.SessionID != oldSessionID {
				continue
			}
			if _, seen := seenEntryIDs[entry.ID]; !seen {
				entryIDs = append(entryIDs, entry.ID)
				seenEntryIDs[entry.ID] = struct{}{}
			}
			hasAttachments = hasAttachments || len(entry.Attachments) > 0
		}
	}
	if !hasAttachments || prepare == nil {
		return nil
	}
	if s.SessionTransferCompensationPersistenceAvailable() {
		state.compensation = &SessionTransferCompensation{
			TaskID: taskID, FromSessionID: oldSessionID, ToSessionID: newSessionID,
			EntryIDs: entryIDs, CreatedAt: time.Now().UTC(),
		}
		if err := s.UpsertSessionTransferCompensation(ctx, *state.compensation); err != nil {
			return fmt.Errorf("persist session transfer compensation: %w", err)
		}
	}
	state.preparationCalled = true
	if err := prepare(ctx); err != nil {
		return fmt.Errorf("prepare durable session transfer: %w", err)
	}
	return nil
}

func (s *Service) deleteSessionTransferCompensation(
	ctx context.Context,
	compensation SessionTransferCompensation,
) error {
	if err := s.DeleteSessionTransferCompensation(
		ctx,
		compensation.TaskID,
		compensation.FromSessionID,
		compensation.ToSessionID,
	); err != nil {
		return fmt.Errorf("delete session transfer compensation: %w", err)
	}
	return nil
}
