package messagequeue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

func (r *sqliteRepository) ensureSessionTransferCompensationSchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS queue_session_transfer_compensations (
			task_id          TEXT NOT NULL,
			from_session_id  TEXT NOT NULL,
			to_session_id    TEXT NOT NULL,
			entry_ids_json   TEXT NOT NULL DEFAULT '[]',
			created_at       TIMESTAMP NOT NULL,
			PRIMARY KEY (task_id, from_session_id, to_session_id)
		)
	`); err != nil {
		return fmt.Errorf("ensure session transfer compensation schema: %w", err)
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
	if compensation.CreatedAt.IsZero() {
		compensation.CreatedAt = time.Now().UTC()
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO queue_session_transfer_compensations
			(task_id, from_session_id, to_session_id, entry_ids_json, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(task_id, from_session_id, to_session_id) DO UPDATE SET
			entry_ids_json = excluded.entry_ids_json
	`), compensation.TaskID, compensation.FromSessionID, compensation.ToSessionID,
		string(entryIDsJSON), compensation.CreatedAt); err != nil {
		return fmt.Errorf("upsert session transfer compensation: %w", err)
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
		SELECT task_id, from_session_id, to_session_id, entry_ids_json, created_at
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
		var entryIDsJSON string
		if err := rows.Scan(
			&compensation.TaskID,
			&compensation.FromSessionID,
			&compensation.ToSessionID,
			&entryIDsJSON,
			&compensation.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan session transfer compensation: %w", err)
		}
		if err := json.Unmarshal([]byte(entryIDsJSON), &compensation.EntryIDs); err != nil {
			return nil, fmt.Errorf("unmarshal session transfer compensation: %w", err)
		}
		compensations = append(compensations, compensation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session transfer compensations: %w", err)
	}
	return compensations, nil
}
