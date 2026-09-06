package messagequeue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	internaldb "github.com/kandev/kandev/internal/db"
)

func (r *sqliteRepository) ensureSessionTransferCompensationSchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS queue_session_transfer_compensations (
			task_id          TEXT NOT NULL,
			from_session_id  TEXT NOT NULL,
			to_session_id    TEXT NOT NULL,
			entry_ids_json        TEXT NOT NULL DEFAULT '[]',
			attachment_ids_json   TEXT NOT NULL DEFAULT '[]',
			cleanup_locators_json TEXT NOT NULL DEFAULT '[]',
			created_at            TIMESTAMP NOT NULL,
			PRIMARY KEY (task_id, from_session_id, to_session_id)
		)
	`); err != nil {
		return fmt.Errorf("ensure session transfer compensation schema: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `ALTER TABLE queue_session_transfer_compensations ADD COLUMN attachment_ids_json TEXT NOT NULL DEFAULT '[]'`); err != nil && !internaldb.IsDuplicateColumnError(err) {
		return fmt.Errorf("add session transfer attachment ids: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `ALTER TABLE queue_session_transfer_compensations ADD COLUMN cleanup_locators_json TEXT NOT NULL DEFAULT '[]'`); err != nil && !internaldb.IsDuplicateColumnError(err) {
		return fmt.Errorf("add session transfer cleanup locators: %w", err)
	}
	return nil
}

func (r *sqliteRepository) UpsertSessionTransferCompensation(
	ctx context.Context,
	compensation SessionTransferCompensation,
) error {
	if err := r.ensureSessionTransferCompensationSchema(ctx); err != nil {
		return err
	}
	entryIDsJSON, err := json.Marshal(compensation.EntryIDs)
	if err != nil {
		return fmt.Errorf("marshal session transfer compensation: %w", err)
	}
	attachmentIDsJSON, err := json.Marshal(compensation.AttachmentIDs)
	if err != nil {
		return fmt.Errorf("marshal session transfer attachment ids: %w", err)
	}
	cleanupLocatorsJSON, err := json.Marshal(compensation.CleanupLocators)
	if err != nil {
		return fmt.Errorf("marshal session transfer cleanup locators: %w", err)
	}
	if compensation.CreatedAt.IsZero() {
		compensation.CreatedAt = time.Now().UTC()
	}
	first, second := compensation.FromSessionID, compensation.ToSessionID
	if first > second {
		first, second = second, first
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session transfer compensation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockSessionTx(ctx, tx, first); err != nil {
		return err
	}
	if first != second {
		if err := r.lockSessionTx(ctx, tx, second); err != nil {
			return err
		}
	}
	var conflicts int
	if err := tx.GetContext(ctx, &conflicts, tx.Rebind(`
		SELECT COUNT(*) FROM queue_session_transfer_compensations
		WHERE (from_session_id IN (?, ?) OR to_session_id IN (?, ?))
		  AND NOT (task_id = ? AND from_session_id = ? AND to_session_id = ?)
	`), first, second, first, second, compensation.TaskID,
		compensation.FromSessionID, compensation.ToSessionID); err != nil {
		return fmt.Errorf("check active session transfer: %w", err)
	}
	if conflicts > 0 {
		return ErrSessionTransferInProgress
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO queue_session_transfer_compensations
			(task_id, from_session_id, to_session_id, entry_ids_json, attachment_ids_json,
			 cleanup_locators_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, from_session_id, to_session_id) DO UPDATE SET
			entry_ids_json = excluded.entry_ids_json,
			attachment_ids_json = excluded.attachment_ids_json,
			cleanup_locators_json = excluded.cleanup_locators_json
	`), compensation.TaskID, compensation.FromSessionID, compensation.ToSessionID,
		string(entryIDsJSON), string(attachmentIDsJSON), string(cleanupLocatorsJSON),
		compensation.CreatedAt); err != nil {
		return fmt.Errorf("upsert session transfer compensation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session transfer compensation: %w", err)
	}
	return nil
}

func (r *sqliteRepository) DeleteSessionTransferCompensation(
	ctx context.Context,
	taskID, fromSessionID, toSessionID string,
) error {
	if err := r.ensureSessionTransferCompensationSchema(ctx); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM queue_session_transfer_compensations
		WHERE task_id = ? AND from_session_id = ? AND to_session_id = ?
	`), taskID, fromSessionID, toSessionID); err != nil {
		return fmt.Errorf("delete session transfer compensation: %w", err)
	}
	return nil
}

func (r *sqliteRepository) ListSessionTransferCompensations(
	ctx context.Context,
) ([]SessionTransferCompensation, error) {
	if err := r.ensureSessionTransferCompensationSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryxContext(ctx, `
		SELECT task_id, from_session_id, to_session_id, entry_ids_json,
		       attachment_ids_json, cleanup_locators_json, created_at
		FROM queue_session_transfer_compensations
		ORDER BY created_at, task_id, from_session_id, to_session_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list session transfer compensations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var compensations []SessionTransferCompensation
	for rows.Next() {
		var compensation SessionTransferCompensation
		var entryIDsJSON, attachmentIDsJSON, cleanupLocatorsJSON string
		if err := rows.Scan(
			&compensation.TaskID,
			&compensation.FromSessionID,
			&compensation.ToSessionID,
			&entryIDsJSON,
			&attachmentIDsJSON,
			&cleanupLocatorsJSON,
			&compensation.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan session transfer compensation: %w", err)
		}
		if err := json.Unmarshal([]byte(entryIDsJSON), &compensation.EntryIDs); err != nil {
			return nil, fmt.Errorf("unmarshal session transfer compensation: %w", err)
		}
		if err := json.Unmarshal([]byte(attachmentIDsJSON), &compensation.AttachmentIDs); err != nil {
			return nil, fmt.Errorf("unmarshal session transfer attachment ids: %w", err)
		}
		if err := json.Unmarshal([]byte(cleanupLocatorsJSON), &compensation.CleanupLocators); err != nil {
			return nil, fmt.Errorf("unmarshal session transfer cleanup locators: %w", err)
		}
		compensations = append(compensations, compensation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session transfer compensations: %w", err)
	}
	return compensations, nil
}

func guardSessionTransferTx(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, sessionID string) error {
	var active bool
	if err := tx.GetContext(ctx, &active, db.Rebind(`
		SELECT EXISTS (
			SELECT 1 FROM queue_session_transfer_compensations
			WHERE from_session_id = ? OR to_session_id = ?
		)
	`), sessionID, sessionID); err != nil {
		return fmt.Errorf("guard active session transfer: %w", err)
	}
	if active {
		return ErrSessionTransferInProgress
	}
	return nil
}

// ResolveSessionTransferInTransaction maps a source session to the destination
// selected by the active durable transfer. Callers must first hold the
// queue_session_locks row for sessionID.
func ResolveSessionTransferInTransaction(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	sessionID string,
) (string, error) {
	var fromSessionID, toSessionID string
	err := tx.QueryRowxContext(ctx, db.Rebind(`
		SELECT from_session_id, to_session_id
		FROM queue_session_transfer_compensations
		WHERE from_session_id = ? OR to_session_id = ?
	`), sessionID, sessionID).Scan(&fromSessionID, &toSessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return sessionID, nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve active session transfer: %w", err)
	}
	if sessionID == fromSessionID {
		return toSessionID, nil
	}
	return sessionID, nil
}
