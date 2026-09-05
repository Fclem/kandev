package messagequeue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	internaldb "github.com/kandev/kandev/internal/db"
)

func (r *sqliteRepository) ensureAttachmentCleanupSchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS queue_attachment_cleanups (
			session_id       TEXT NOT NULL,
			entry_id         TEXT NOT NULL,
			operation_id     TEXT NOT NULL DEFAULT '',
			task_id          TEXT NOT NULL,
			owner_id        TEXT NOT NULL DEFAULT '',
			lease_id         TEXT NOT NULL DEFAULT '',
			attachments_json TEXT NOT NULL DEFAULT '[]',
			created_at       TIMESTAMP NOT NULL,
			PRIMARY KEY (session_id, entry_id, operation_id)
		)
	`); err != nil {
		return fmt.Errorf("ensure attachment cleanup schema: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `ALTER TABLE queue_attachment_cleanups ADD COLUMN owner_id TEXT NOT NULL DEFAULT ''`); err != nil && !internaldb.IsDuplicateColumnError(err) {
		return fmt.Errorf("add attachment cleanup owner: %w", err)
	}
	return nil
}

func (r *sqliteRepository) UpsertAttachmentCleanup(ctx context.Context, cleanup AttachmentCleanup) error {
	if err := r.ensureAttachmentCleanupSchema(ctx); err != nil {
		return err
	}
	attachmentsJSON, err := json.Marshal(cleanup.Attachments)
	if err != nil {
		return fmt.Errorf("marshal attachment cleanup: %w", err)
	}
	if cleanup.CreatedAt.IsZero() {
		cleanup.CreatedAt = time.Now().UTC()
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO queue_attachment_cleanups
			(session_id, entry_id, operation_id, task_id, owner_id, lease_id, attachments_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id, entry_id, operation_id) DO UPDATE SET
			task_id = excluded.task_id,
			owner_id = excluded.owner_id,
			lease_id = excluded.lease_id,
			attachments_json = excluded.attachments_json
	`), cleanup.SessionID, cleanup.EntryID, cleanup.OperationID, cleanup.TaskID,
		cleanup.OwnerID, cleanup.LeaseID, string(attachmentsJSON), cleanup.CreatedAt); err != nil {
		return fmt.Errorf("upsert attachment cleanup: %w", err)
	}
	return nil
}

func (r *sqliteRepository) DeleteAttachmentCleanup(
	ctx context.Context,
	sessionID, entryID, operationID string,
) error {
	if err := r.ensureAttachmentCleanupSchema(ctx); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM queue_attachment_cleanups
		WHERE session_id = ? AND entry_id = ? AND operation_id = ?
	`), sessionID, entryID, operationID); err != nil {
		return fmt.Errorf("delete attachment cleanup: %w", err)
	}
	return nil
}

func (r *sqliteRepository) ListAttachmentCleanups(ctx context.Context) ([]AttachmentCleanup, error) {
	if err := r.ensureAttachmentCleanupSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryxContext(ctx, `
		SELECT session_id, entry_id, operation_id, task_id, owner_id, lease_id, attachments_json, created_at
		FROM queue_attachment_cleanups
		ORDER BY created_at, session_id, entry_id, operation_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list attachment cleanups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var cleanups []AttachmentCleanup
	for rows.Next() {
		var cleanup AttachmentCleanup
		var attachmentsJSON string
		if err := rows.Scan(
			&cleanup.SessionID,
			&cleanup.EntryID,
			&cleanup.OperationID,
			&cleanup.TaskID,
			&cleanup.OwnerID,
			&cleanup.LeaseID,
			&attachmentsJSON,
			&cleanup.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan attachment cleanup: %w", err)
		}
		if err := json.Unmarshal([]byte(attachmentsJSON), &cleanup.Attachments); err != nil {
			return nil, fmt.Errorf("unmarshal attachment cleanup: %w", err)
		}
		cleanups = append(cleanups, cleanup)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate attachment cleanups: %w", err)
	}
	return cleanups, nil
}
