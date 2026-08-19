package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// promptKeyLayout is the exact byte form of the normalized-microsecond
// ordering key: `YYYY-MM-DD HH:MM:SS.ffffff` (space separator, zero-padded
// 6-digit fraction, no zone suffix, UTC). SQLite rows compare against this
// text; the same literal is cast to `timestamp` on PostgreSQL.
const promptKeyLayout = "2006-01-02 15:04:05.000000"

// promptIndexLeadWindow bounds how far ahead of the current wall clock a live
// user-message create may advance its ordering timestamp to keep the
// session's prompt ordering monotonic. A clock that is behind the session's
// newest user message by more than this window is treated as over-window
// skew: advancing would stamp the message far in the future, so the create
// fails with a retryable error instead.
const promptIndexLeadWindow = time.Minute

// ErrMessageClockSkew is returned by live user-message creates when the
// timestamp advance needed to stay after the session's newest user message
// would exceed promptIndexLeadWindow. Retryable: once the clock catches up
// (or the skew is repaired), the same create succeeds.
var ErrMessageClockSkew = errors.New("message clock skew exceeds prompt-ordering lead window")

// ErrMessageTimestampNotAfterNewest is returned when an explicit user-message
// timestamp is not strictly after the session's newest user message. Explicit
// imports preserve only valid strictly-after-max timestamps; reorderings of
// an existing session's history are rejected.
var ErrMessageTimestampNotAfterNewest = errors.New("explicit message timestamp is not strictly after the session's newest user message")

// messageBoundaryExecer is the subset of *sqlx.DB / *sqlx.Tx the per-session
// user-message create boundary needs: reads and the insert under one
// transaction.
type messageBoundaryExecer interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
	Rebind(query string) string
}

func formatPromptKey(t time.Time) string {
	return t.UTC().Format(promptKeyLayout)
}

func parsePromptKey(s string) (time.Time, error) {
	return time.Parse(promptKeyLayout, s)
}

// resolvePromptOrderingTime applies the atomic create-boundary timestamp
// rules for a user message. Live creates (zero timestamp) keep the session's
// prompt ordering monotonic: a colliding or backward key is advanced by one
// microsecond tick, bounded by promptIndexLeadWindow (an over-window skew
// returns a retryable error instead of a future timestamp). Explicit
// timestamps are preserved only when strictly after the session's newest user
// message. maxKey is the session's newest user-message normalized key (NULL
// when the session has no user rows).
func resolvePromptOrderingTime(live bool, created time.Time, maxKey sql.NullString, now time.Time) (time.Time, error) {
	if !maxKey.Valid || maxKey.String == "" {
		return created, nil
	}
	newKey := formatPromptKey(created)
	if live {
		if newKey > maxKey.String {
			return created, nil
		}
		advanced, err := parsePromptKey(maxKey.String)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse session max user-message key %q: %w", maxKey.String, err)
		}
		advanced = advanced.Add(time.Microsecond)
		if advanced.After(now.Add(promptIndexLeadWindow)) {
			return time.Time{}, fmt.Errorf(
				"%w: session's newest user message (%s) is %s ahead of now",
				ErrMessageClockSkew, maxKey.String, advanced.Sub(now).Round(time.Microsecond),
			)
		}
		return advanced, nil
	}
	if newKey <= maxKey.String {
		return time.Time{}, fmt.Errorf(
			"%w: %s is not strictly after %s",
			ErrMessageTimestampNotAfterNewest, formatPromptKey(created), maxKey.String,
		)
	}
	return created, nil
}

// createUserMessageWithBoundary persists a user message with its
// prompt-ordering timestamp and ordinal assigned inside the per-session write
// boundary (a transaction on both dialects; PostgreSQL additionally takes the
// session turn-write advisory lock, and SQLite's single-writer pool provides
// the same serialization). The caller-visible CreatedAt and PromptIndex fields
// are only committed on success, so a failed attempt (clock-skew rejection,
// busy retry, concurrent winner) leaves the retried model untouched.
func (r *Repository) createUserMessageWithBoundary(
	ctx context.Context,
	message *models.Message,
	requestsInput int,
	messageType, metadataJSON string,
) error {
	driver := r.db.DriverName()
	nm := dialect.NormalizedMicrosecond(driver, "created_at")

	origCreatedAt := message.CreatedAt
	origUpdatedAt := message.UpdatedAt
	origPromptIndex := message.PromptIndex
	restore := func() {
		message.CreatedAt = origCreatedAt
		message.UpdatedAt = origUpdatedAt
		message.PromptIndex = origPromptIndex
	}

	run := func(execer messageBoundaryExecer) error {
		now := r.nowUTC()
		created := message.CreatedAt
		if created.IsZero() {
			created = now
		}

		var maxKey sql.NullString
		if err := execer.QueryRowContext(ctx, execer.Rebind(fmt.Sprintf(
			"SELECT max(%s) FROM task_session_messages WHERE task_session_id = ? AND author_type = ?",
			nm,
		)), message.TaskSessionID, string(models.MessageAuthorUser)).Scan(&maxKey); err != nil {
			return fmt.Errorf("read session max user-message key: %w", err)
		}

		created, err := resolvePromptOrderingTime(message.CreatedAt.IsZero(), created, maxKey, now)
		if err != nil {
			return err
		}
		newKey := formatPromptKey(created)

		var ordinal int
		if err := execer.QueryRowContext(ctx, execer.Rebind(fmt.Sprintf(
			"SELECT COUNT(*) FROM task_session_messages WHERE task_session_id = ? AND author_type = ? AND (%s < ? OR (%s = ? AND id <= ?))",
			nm, nm,
		)), message.TaskSessionID, string(models.MessageAuthorUser), newKey, newKey, message.ID).Scan(&ordinal); err != nil {
			return fmt.Errorf("count session user messages: %w", err)
		}
		// The count covers the session's EXISTING user rows up to the new
		// row's ordering position (the row itself is not yet inserted, so the
		// id <= self disjunct cannot match it); the new row's ordinal is
		// therefore count + 1, matching GetMessageWithPromptIndex's
		// self-inclusive correlated count by construction.
		message.CreatedAt = created
		message.PromptIndex = ordinal + 1
		if message.UpdatedAt.IsZero() {
			message.UpdatedAt = created
		}
		if err := r.insertMessageRow(ctx, execer, message, requestsInput, messageType, metadataJSON); err != nil {
			restore()
			return err
		}
		return nil
	}

	// One per-session write boundary on both dialects. lockSessionTurnWrites
	// takes the advisory session lock on PostgreSQL and is a no-op on SQLite,
	// whose single-writer pool already serializes message insertion.
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin user message creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockSessionTurnWrites(ctx, tx, driver, message.TaskSessionID); err != nil {
		return err
	}
	if err := run(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		restore()
		return fmt.Errorf("commit user message creation: %w", err)
	}
	return nil
}

// GetMessageWithPromptIndex retrieves a message by ID with its prompt_index
// derived from a correlated COUNT over the session's user rows using the same
// normalized-microsecond predicate as the live-create transaction, so live,
// replay, and read ordinals agree by construction. Non-user rows return zero.
// This is the 13-column read used by the idempotent WS replay/response path
// and user update-event publication; hot-path GetMessage stays on the
// 12-column scan.
func (r *Repository) GetMessageWithPromptIndex(ctx context.Context, id string) (*models.Message, error) {
	drv := r.ro.DriverName()
	nmM := dialect.NormalizedMicrosecond(drv, "m.created_at")
	nmU := dialect.NormalizedMicrosecond(drv, "u.created_at")
	message := &models.Message{}
	var requestsInput int
	var messageType string
	var metadataJSON string
	query := fmt.Sprintf(`
		SELECT m.id, m.task_session_id, m.task_id, m.turn_id, m.author_type, m.author_id,
		       m.content, m.requests_input, m.type, m.metadata, m.created_at, m.updated_at,
		       CASE WHEN m.author_type = 'user' THEN (
		         SELECT COUNT(*) FROM task_session_messages u
		         WHERE u.task_session_id = m.task_session_id
		           AND u.author_type = 'user'
		           AND (%[1]s < %[2]s OR (%[1]s = %[2]s AND u.id <= m.id))
		       ) ELSE 0 END AS prompt_index
		FROM task_session_messages m
		WHERE m.id = ?
	`, nmU, nmM)
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(query), id).Scan(
		&message.ID, &message.TaskSessionID, &message.TaskID, &message.TurnID, &message.AuthorType, &message.AuthorID,
		&message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt, &message.UpdatedAt,
		&message.PromptIndex,
	)
	if err != nil {
		return nil, err
	}
	message.RequestsInput = requestsInput == 1
	message.Type = models.MessageType(messageType)
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &message.Metadata); err != nil {
			return nil, fmt.Errorf("failed to deserialize message metadata: %w", err)
		}
	}
	return message, nil
}
