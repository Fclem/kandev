package messagequeue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// PendingQueueDispatch records an ordinary queue row removed for delivery.
// Accepted selects acknowledgement rather than restoration during startup.
type PendingQueueDispatch struct {
	Message  QueuedMessage
	Accepted bool
}

func (r *sqliteRepository) ensureQueueDispatchRecoverySchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS queue_dispatch_claims (
			entry_id     TEXT PRIMARY KEY,
			session_id   TEXT NOT NULL,
			message_json TEXT NOT NULL,
			accepted     INTEGER NOT NULL DEFAULT 0,
			created_at   TIMESTAMP NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("ensure queue dispatch recovery schema: %w", err)
	}
	return nil
}

func (r *sqliteRepository) persistQueueDispatchClaimTx(
	ctx context.Context,
	tx *sqlx.Tx,
	msg *QueuedMessage,
) error {
	messageJSON, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal queue dispatch claim: %w", err)
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO queue_dispatch_claims (entry_id, session_id, message_json, created_at)
		VALUES (?, ?, ?, ?)
	`), msg.ID, msg.SessionID, string(messageJSON), time.Now().UTC()); err != nil {
		return fmt.Errorf("persist queue dispatch claim: %w", err)
	}
	return nil
}

func listPendingRecoveryRecords[T any, R any](
	ctx context.Context,
	db *sqlx.DB,
	query, label string,
	wrap func(T, bool) R,
) ([]R, error) {
	rows, err := db.QueryxContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list pending %s: %w", label, err)
	}
	defer func() { _ = rows.Close() }()
	var pending []R
	for rows.Next() {
		var payloadJSON string
		var accepted int
		if err := rows.Scan(&payloadJSON, &accepted); err != nil {
			return nil, fmt.Errorf("scan pending %s: %w", label, err)
		}
		var payload T
		if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
			return nil, fmt.Errorf("unmarshal pending %s: %w", label, err)
		}
		pending = append(pending, wrap(payload, accepted != 0))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending %s: %w", label, err)
	}
	return pending, nil
}

func (r *sqliteRepository) ListPendingQueueDispatches(ctx context.Context) ([]PendingQueueDispatch, error) {
	if err := r.ensureQueueDispatchRecoverySchema(ctx); err != nil {
		return nil, err
	}
	return listPendingRecoveryRecords(
		ctx,
		r.db,
		`SELECT message_json, accepted FROM queue_dispatch_claims ORDER BY created_at, entry_id`,
		"queue dispatch",
		func(msg QueuedMessage, accepted bool) PendingQueueDispatch {
			return PendingQueueDispatch{Message: msg, Accepted: accepted}
		},
	)
}

func (r *sqliteRepository) deletePendingQueueDispatchTx(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID, entryID string,
) error {
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM queue_dispatch_claims WHERE session_id = ? AND entry_id = ?
	`), sessionID, entryID); err != nil {
		return fmt.Errorf("delete queue dispatch claim: %w", err)
	}
	return nil
}

func (r *sqliteRepository) deletePendingQueueDispatchesBySessionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID string,
) error {
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM queue_dispatch_claims WHERE session_id = ?
	`), sessionID); err != nil {
		return fmt.Errorf("delete session queue dispatch claims: %w", err)
	}
	return nil
}

func (r *sqliteRepository) MarkPendingQueueDispatchAccepted(
	ctx context.Context,
	sessionID, entryID string,
) error {
	if err := r.ensureQueueDispatchRecoverySchema(ctx); err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE queue_dispatch_claims SET accepted = 1 WHERE entry_id = ? AND session_id = ?
	`), entryID, sessionID)
	if err != nil {
		return fmt.Errorf("mark queue dispatch accepted: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark queue dispatch accepted rows affected: %w", err)
	}
	if affected != 1 {
		return ErrEntryNotFound
	}
	return nil
}

func (r *sqliteRepository) DeletePendingQueueDispatch(
	ctx context.Context,
	sessionID, entryID string,
) error {
	if err := r.ensureQueueDispatchRecoverySchema(ctx); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM queue_dispatch_claims WHERE entry_id = ? AND session_id = ?
	`), entryID, sessionID); err != nil {
		return fmt.Errorf("delete pending queue dispatch: %w", err)
	}
	return nil
}
