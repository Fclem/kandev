package sqlite

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

func hasNativeUsageGap(metadata map[string]interface{}) bool {
	for key, value := range metadata {
		if value == true && (strings.HasPrefix(key, models.NativeUsageGapDirectPrefix) || strings.HasPrefix(key, models.NativeUsageGapChildPrefix)) {
			return true
		}
	}
	return false
}

func (r *Repository) indexUsageTurnTx(ctx context.Context, tx *sqlx.Tx, sessionID, turnID string) error {
	if sessionID == "" || turnID == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_usage_turn_index (session_id, turn_id) VALUES (?, ?)
		ON CONFLICT (session_id, turn_id) DO NOTHING
	`), sessionID, turnID)
	return err
}

func (r *Repository) indexCoverageTurnTx(ctx context.Context, tx *sqlx.Tx, sessionID, turnID string) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_usage_turn_index (session_id, turn_id)
		SELECT t.task_session_id, t.id FROM task_session_turns t
		JOIN task_sessions s ON s.id = t.task_session_id AND s.task_id = t.task_id
		WHERE t.task_session_id = ? AND t.id = ?
		ON CONFLICT (session_id, turn_id) DO NOTHING
	`), sessionID, turnID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_usage_coverage (task_id)
		SELECT t.task_id FROM task_session_turns t
		JOIN task_sessions s ON s.id = t.task_session_id AND s.task_id = t.task_id
		WHERE t.task_session_id = ? AND t.id = ?
		ON CONFLICT (task_id) DO NOTHING
	`), sessionID, turnID)
	return err
}

func (r *Repository) migrateUsageTurnIndex() error {
	exists, err := r.tableExists("task_usage_turn_index")
	if err != nil || exists {
		return err
	}
	ctx := r.migrationContext()
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `CREATE TABLE task_usage_turn_index (
		`+dialect.AutoIncrementIDColumn(r.db.DriverName())+`,
		session_id TEXT NOT NULL REFERENCES task_sessions(id) ON DELETE CASCADE,
		turn_id TEXT NOT NULL,
		UNIQUE (session_id, turn_id)
	)`)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX idx_usage_turn_index_session_cursor ON task_usage_turn_index(session_id, id)`); err != nil {
		return err
	}
	// Existing opaque cursors retain their first-ledger-row identities.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO task_usage_turn_index (id, session_id, turn_id)
		SELECT MIN(u.id), u.session_id, u.turn_id FROM task_usage_events u
		JOIN task_sessions s ON s.id = u.session_id AND s.task_id = u.task_id
		WHERE u.turn_id IS NOT NULL AND u.turn_id <> ''
		GROUP BY u.session_id, u.turn_id
	`); err != nil {
		return err
	}
	if dialect.IsPostgres(r.db.DriverName()) {
		if _, err := tx.ExecContext(ctx, `SELECT setval(pg_get_serial_sequence('task_usage_turn_index', 'id'), COALESCE(MAX(id), 1), COUNT(*) > 0) FROM task_usage_turn_index`); err != nil {
			return err
		}
	}
	if err := r.backfillCoverageTurnIndex(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) backfillCoverageTurnIndex(ctx context.Context, tx *sqlx.Tx) error {
	rows, err := tx.QueryxContext(ctx, `
		SELECT t.task_session_id, t.id, t.metadata FROM task_session_turns t
		JOIN task_sessions s ON s.id = t.task_session_id AND s.task_id = t.task_id
		WHERE t.metadata LIKE '%native_usage_gap_%'
		ORDER BY t.created_at, t.id
	`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	type coverageTurn struct{ sessionID, turnID string }
	turns := make([]coverageTurn, 0)
	for rows.Next() {
		var turn coverageTurn
		var encoded string
		if err := rows.Scan(&turn.sessionID, &turn.turnID, &encoded); err != nil {
			return err
		}
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(encoded), &metadata); err != nil {
			return err
		}
		if hasNativeUsageGap(metadata) {
			turns = append(turns, turn)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, turn := range turns {
		if err := r.indexUsageTurnTx(ctx, tx, turn.sessionID, turn.turnID); err != nil {
			return err
		}
	}
	return nil
}

// Task coverage outlives sessions, just like retained task ledger amounts.
func (r *Repository) migrateTaskUsageCoverage() error {
	exists, err := r.tableExists("task_usage_coverage")
	if err != nil || exists {
		return err
	}
	ctx := r.migrationContext()
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE task_usage_coverage (
		task_id TEXT PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE
	)`); err != nil {
		return err
	}
	rows, err := tx.QueryxContext(ctx, `
		SELECT t.task_id, t.metadata FROM task_session_turns t
		JOIN task_sessions s ON s.id = t.task_session_id AND s.task_id = t.task_id
		WHERE t.metadata LIKE '%native_usage_gap_%'
	`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	taskIDs := make(map[string]struct{})
	for rows.Next() {
		var taskID, encoded string
		if err := rows.Scan(&taskID, &encoded); err != nil {
			return err
		}
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(encoded), &metadata); err != nil {
			return err
		}
		if hasNativeUsageGap(metadata) {
			taskIDs[taskID] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for taskID := range taskIDs {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO task_usage_coverage (task_id) VALUES (?)`), taskID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) ListSessionUsageTurnPage(ctx context.Context, taskID, sessionID string, afterID int64, limit int) ([]models.TaskUsageTurnEvents, error) {
	cursors, err := r.ListSessionUsageTurnCursors(ctx, sessionID, afterID, limit)
	if err != nil {
		return nil, err
	}
	page := make([]models.TaskUsageTurnEvents, len(cursors))
	if len(cursors) == 0 {
		return page, nil
	}
	turnIDs := make([]string, len(cursors))
	positions := make(map[string]int, len(cursors))
	for i, cursor := range cursors {
		turnIDs[i] = cursor.TurnID
		positions[cursor.TurnID] = i
		page[i] = models.TaskUsageTurnEvents{TurnID: cursor.TurnID, Cursor: cursor.Cursor}
	}
	if err := r.readUsagePageCoverage(ctx, taskID, sessionID, turnIDs, positions, page); err != nil {
		return nil, err
	}
	if err := r.readUsagePageEvents(ctx, sessionID, turnIDs, positions, page); err != nil {
		return nil, err
	}
	return page, nil
}

func (r *Repository) readUsagePageCoverage(ctx context.Context, taskID, sessionID string, turnIDs []string, positions map[string]int, page []models.TaskUsageTurnEvents) error {
	query, args, err := sqlx.In(`SELECT id, task_id, task_session_id, metadata FROM task_session_turns WHERE id IN (?)`, turnIDs)
	if err != nil {
		return err
	}
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var turnID, ownerTaskID, ownerSessionID, encoded string
		if err := rows.Scan(&turnID, &ownerTaskID, &ownerSessionID, &encoded); err != nil {
			return err
		}
		if ownerTaskID != taskID || ownerSessionID != sessionID {
			return models.ErrTaskSessionNotFound
		}
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(encoded), &metadata); err != nil {
			return err
		}
		item := &page[positions[turnID]]
		for key, value := range metadata {
			if value == true {
				item.DirectIncomplete = item.DirectIncomplete || strings.HasPrefix(key, models.NativeUsageGapDirectPrefix)
				item.ChildIncomplete = item.ChildIncomplete || strings.HasPrefix(key, models.NativeUsageGapChildPrefix)
			}
		}
	}
	return rows.Err()
}

func (r *Repository) readUsagePageEvents(ctx context.Context, sessionID string, turnIDs []string, positions map[string]int, page []models.TaskUsageTurnEvents) error {
	query, args, err := sqlx.In(`SELECT `+usageEventProjectionColumns+` FROM task_usage_events WHERE session_id = ? AND turn_id IN (?) ORDER BY id ASC`, sessionID, turnIDs)
	if err != nil {
		return err
	}
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		event, err := scanTaskUsageEventRow(rows)
		if err != nil {
			return err
		}
		item := &page[positions[event.TurnID]]
		item.Events = append(item.Events, event)
	}
	return rows.Err()
}
