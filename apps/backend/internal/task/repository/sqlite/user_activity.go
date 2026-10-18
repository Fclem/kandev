package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const (
	maxUserActivitySessionBatch  = 400
	userActivityCaptureMetaKey   = "user_activity_capture"
	userActivityOutcomeRecorded  = "recorded"
	userActivityOutcomeCoalesced = "coalesced"
	userActivityOutcomeRejected  = "rejected"
	userActivityOutcomeDisabled  = "disabled"
)

type userActivityCaptureState struct {
	Enabled bool                     `json:"enabled"`
	Gaps    []userActivityCaptureGap `json:"gaps"`
}

type userActivityCaptureGap struct {
	Off time.Time  `json:"off"`
	On  *time.Time `json:"on,omitempty"`
}

// SetUserActivityCapture installs the startup-resolved capture state.
func (r *Repository) SetUserActivityCapture(enabled bool) {
	r.userActivityCapture = enabled
}

// RecordUserActivity stores a server-timestamped beat when capture is enabled.
// Outcome is one of recorded, coalesced, rejected, or disabled.
func (r *Repository) RecordUserActivity(ctx context.Context, actorID, sessionID string, now time.Time) (string, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	taskID, outcome, err := r.userActivityTaskForBeat(ctx, tx, sessionID)
	if err != nil || outcome != "" {
		return outcome, err
	}
	outcome, err = r.recordUserActivityInterval(ctx, tx, taskID, actorID, sessionID, now)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return outcome, nil
}

func (r *Repository) userActivityTaskForBeat(ctx context.Context, tx *sqlx.Tx, sessionID string) (string, string, error) {
	var taskID string
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT task_id FROM task_sessions WHERE id = ?`), sessionID).Scan(&taskID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", userActivityOutcomeRejected, nil
		}
		return "", "", err
	}
	if err := r.taskCleanupBarrierLocked(ctx, tx, taskID); err != nil {
		if errors.Is(err, repoerrors.ErrTaskCleanupInProgress) || errors.Is(err, sql.ErrNoRows) {
			return "", userActivityOutcomeRejected, nil
		}
		return "", "", err
	}
	var archived sql.NullTime
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT archived_at FROM tasks WHERE id = ?`), taskID).Scan(&archived); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", userActivityOutcomeRejected, nil
		}
		return "", "", err
	}
	if archived.Valid {
		return "", userActivityOutcomeRejected, nil
	}
	if !r.userActivityCapture {
		return "", userActivityOutcomeDisabled, nil
	}
	return taskID, "", nil
}

func (r *Repository) recordUserActivityInterval(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, actorID, sessionID string,
	now time.Time,
) (string, error) {
	var ended time.Time
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT ended_at FROM task_session_user_activity
		WHERE session_id = ? AND actor_id = ? ORDER BY ended_at DESC, id DESC LIMIT 1
	`), sessionID, actorID).Scan(&ended)
	if errors.Is(err, sql.ErrNoRows) {
		return r.insertUserActivityInterval(ctx, tx, taskID, actorID, sessionID, now)
	}
	if err != nil {
		return "", err
	}
	outcome, merged, err := r.mergeUserActivityInterval(ctx, tx, ended, actorID, sessionID, now)
	if err != nil {
		return "", err
	}
	if merged {
		return outcome, nil
	}
	return r.insertUserActivityInterval(ctx, tx, taskID, actorID, sessionID, now)
}

func (r *Repository) mergeUserActivityInterval(
	ctx context.Context,
	tx *sqlx.Tx,
	ended time.Time,
	actorID, sessionID string,
	now time.Time,
) (string, bool, error) {
	gap := now.Sub(ended)
	if gap < 5*time.Second {
		return userActivityOutcomeCoalesced, true, nil
	}
	if gap >= 45*time.Second {
		return "", false, nil
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE task_session_user_activity SET ended_at = ? WHERE session_id = ? AND actor_id = ? AND ended_at = ?`), now.UTC(), sessionID, actorID, ended); err != nil {
		return "", false, err
	}
	return userActivityOutcomeRecorded, true, nil
}

func (r *Repository) insertUserActivityInterval(ctx context.Context, tx *sqlx.Tx, taskID, actorID, sessionID string, now time.Time) (string, error) {
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO task_session_user_activity (session_id, task_id, actor_id, started_at, ended_at) VALUES (?, ?, ?, ?, ?)`), sessionID, taskID, actorID, now.UTC(), now.UTC()); err != nil {
		return "", err
	}
	return userActivityOutcomeRecorded, nil
}

// SumUserActivity returns durable aggregate person-time for the requested sessions.
func (r *Repository) SumUserActivity(ctx context.Context, sessionIDs []string) ([]models.UserActivitySummary, error) {
	if len(sessionIDs) == 0 {
		return nil, nil
	}
	state, err := r.readUserActivityCaptureState(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	duration := userActivityDurationExpression(r.db.DriverName())
	summaries := make([]models.UserActivitySummary, 0, len(sessionIDs))
	for offset := 0; offset < len(sessionIDs); offset += maxUserActivitySessionBatch {
		end := min(offset+maxUserActivitySessionBatch, len(sessionIDs))
		batch, err := r.sumUserActivityBatch(ctx, sessionIDs[offset:end], state, duration)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, batch...)
	}
	return summaries, nil
}

func userActivityDurationExpression(driverName string) string {
	if dialect.IsPostgres(driverName) {
		return `CAST(SUM(EXTRACT(EPOCH FROM (a.ended_at - a.started_at)) * 1000) AS BIGINT)`
	}
	return `CAST(ROUND(SUM((julianday(a.ended_at) - julianday(a.started_at)) * 86400000)) AS BIGINT)`
}

func (r *Repository) sumUserActivityBatch(
	ctx context.Context,
	sessionIDs []string,
	state userActivityCaptureState,
	duration string,
) ([]models.UserActivitySummary, error) {
	query, args, err := sqlx.In(`SELECT s.id, COALESCE(`+duration+`, 0), COUNT(a.id), MIN(a.started_at), MAX(a.ended_at), s.started_at, s.completed_at, s.metadata FROM task_sessions s LEFT JOIN task_session_user_activity a ON a.session_id = s.id WHERE s.id IN (?) GROUP BY s.id`, sessionIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	summaries := make([]models.UserActivitySummary, 0, len(sessionIDs))
	for rows.Next() {
		item, err := scanUserActivitySummary(rows, state)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return summaries, nil
}

func scanUserActivitySummary(rows *sqlx.Rows, state userActivityCaptureState) (models.UserActivitySummary, error) {
	var item models.UserActivitySummary
	var first, last interface{}
	var started time.Time
	var completed sql.NullTime
	var metadata string
	if err := rows.Scan(&item.SessionID, &item.ActiveMS, &item.IntervalCount, &first, &last, &started, &completed, &metadata); err != nil {
		return models.UserActivitySummary{}, err
	}
	if first != nil {
		parsed, err := parseTaskActivityTime(first)
		if err != nil {
			return models.UserActivitySummary{}, fmt.Errorf("parse first activity timestamp: %w", err)
		}
		item.FirstAt = parsed
	}
	if last != nil {
		parsed, err := parseTaskActivityTime(last)
		if err != nil {
			return models.UserActivitySummary{}, fmt.Errorf("parse last activity timestamp: %w", err)
		}
		item.LastAt = parsed
	}
	var values map[string]interface{}
	_ = json.Unmarshal([]byte(metadata), &values)
	stamped, _ := values["user_activity_capture"].(bool)
	item.ActivityMeasured = stamped && !userActivityGapOverlaps(state, started, completed)
	return item, nil
}

func userActivityGapOverlaps(state userActivityCaptureState, started time.Time, completed sql.NullTime) bool {
	for _, gap := range state.Gaps {
		if gap.On != nil && !gap.On.After(started) {
			continue
		}
		if completed.Valid && completed.Time.Before(gap.Off) {
			continue
		}
		return true
	}
	return false
}

// RecordCaptureTransition atomically persists startup capture state and gaps.
func (r *Repository) RecordCaptureTransition(ctx context.Context, effective bool, now time.Time) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var raw string
	err = tx.QueryRowContext(ctx, r.db.Rebind(`SELECT value FROM kandev_meta WHERE key = ?`), userActivityCaptureMetaKey).Scan(&raw)
	first := errors.Is(err, sql.ErrNoRows)
	if err != nil && !first {
		return err
	}
	state := userActivityCaptureState{}
	if !first {
		if err := json.Unmarshal([]byte(raw), &state); err != nil {
			return fmt.Errorf("decode user activity capture state: %w", err)
		}
	}
	if first {
		state.Enabled = effective
	} else if state.Enabled != effective {
		if state.Enabled {
			state.Gaps = append(state.Gaps, userActivityCaptureGap{Off: now.UTC()})
		} else if len(state.Gaps) > 0 && state.Gaps[len(state.Gaps)-1].On == nil {
			closed := now.UTC()
			state.Gaps[len(state.Gaps)-1].On = &closed
		}
		state.Enabled = effective
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO kandev_meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`), userActivityCaptureMetaKey, string(encoded))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) readUserActivityCaptureState(ctx context.Context) (userActivityCaptureState, error) {
	var raw string
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT value FROM kandev_meta WHERE key = ?`), userActivityCaptureMetaKey).Scan(&raw)
	if err != nil {
		return userActivityCaptureState{}, err
	}
	var state userActivityCaptureState
	err = json.Unmarshal([]byte(raw), &state)
	return state, err
}

func (r *Repository) initUserActivitySchema(ctx context.Context) error {
	idColumn := dialect.AutoIncrementIDColumn(r.db.DriverName())
	if _, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS kandev_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`); err != nil {
		return fmt.Errorf("ensure kandev_meta: %w", err)
	}
	_, err := r.db.ExecContext(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS task_session_user_activity (
			%s,
			session_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			actor_id TEXT NOT NULL,
			started_at TIMESTAMP NOT NULL,
			ended_at TIMESTAMP NOT NULL,
			FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE,
			FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_session_user_activity_session_actor_ended
			ON task_session_user_activity(session_id, actor_id, ended_at DESC)
	`, idColumn))
	if err != nil {
		return fmt.Errorf("init task session user activity schema: %w", err)
	}
	return nil
}
