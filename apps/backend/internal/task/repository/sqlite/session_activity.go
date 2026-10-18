package sqlite

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

const sessionActivityReadPageMax = 200

func (r *Repository) ListUserActivitySessions(
	ctx context.Context,
	filter models.SessionActivityFilter,
	cursor string,
	limit int,
) (models.SessionActivitySessionsPage, error) {
	limit = normalizeSessionActivityReadLimit(limit)
	cursorTime, cursorID, err := decodeSessionActivityCursor(cursor, "sessions")
	if err != nil {
		return models.SessionActivitySessionsPage{}, err
	}
	query := fmt.Sprintf(`SELECT id, task_id, started_at FROM task_sessions
		WHERE EXISTS (
			SELECT 1 FROM tasks visible_task
			JOIN workspaces visible_workspace ON visible_workspace.id = visible_task.workspace_id
			WHERE visible_task.id = task_sessions.task_id AND %s
		)`, excludeConfigModePredicate(r.ro.DriverName(), "visible_task.metadata"))
	args := make([]interface{}, 0, 4)
	if len(filter.SessionIDs) > 0 {
		query += ` AND id IN (?)`
		args = append(args, filter.SessionIDs)
	}
	if len(filter.TaskIDs) > 0 {
		query += ` AND task_id IN (?)`
		args = append(args, filter.TaskIDs)
	}
	if cursorID != "" {
		query += ` AND (started_at > ? OR (started_at = ? AND id > ?))`
		args = append(args, cursorTime, cursorTime, cursorID)
	}
	query += ` ORDER BY started_at ASC, id ASC LIMIT ?`
	args = append(args, limit+1)
	query, args, err = sqlx.In(query, args...)
	if err != nil {
		return models.SessionActivitySessionsPage{}, err
	}
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return models.SessionActivitySessionsPage{}, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]models.SessionActivitySession, 0, limit+1)
	for rows.Next() {
		var item models.SessionActivitySession
		if err := rows.Scan(&item.SessionID, &item.TaskID, &item.StartedAt); err != nil {
			return models.SessionActivitySessionsPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return models.SessionActivitySessionsPage{}, err
	}
	page := models.SessionActivitySessionsPage{Sessions: items}
	if len(items) > limit {
		page.HasMore = true
		page.Sessions = items[:limit]
		last := page.Sessions[len(page.Sessions)-1]
		page.NextCursor = encodeSessionActivityCursor("sessions", last.StartedAt, last.SessionID)
	}
	return page, nil
}

func (r *Repository) ListSessionTurns(
	ctx context.Context,
	filter models.SessionActivityFilter,
	cursor string,
	limit int,
) (models.SessionActivityTurnsPage, error) {
	limit = normalizeSessionActivityReadLimit(limit)
	cursorTime, cursorID, err := decodeSessionActivityCursor(cursor, "turns")
	if err != nil {
		return models.SessionActivityTurnsPage{}, err
	}
	query := fmt.Sprintf(`
		SELECT turn_row.id, turn_row.task_session_id, turn_row.task_id, turn_row.started_at, turn_row.completed_at
		FROM task_session_turns turn_row
		WHERE %s AND NOT (%s) AND EXISTS (
			SELECT 1 FROM tasks visible_task
			JOIN workspaces visible_workspace ON visible_workspace.id = visible_task.workspace_id
			WHERE visible_task.id = turn_row.task_id AND %s
		)`, turnHistoryPredicate(r.ro.DriverName(), "turn_row"),
		turnLifecycleOnlyPredicate(r.ro.DriverName(), "turn_row"),
		excludeConfigModePredicate(r.ro.DriverName(), "visible_task.metadata"))
	args := make([]interface{}, 0, 4)
	if len(filter.SessionIDs) > 0 {
		query += ` AND turn_row.task_session_id IN (?)`
		args = append(args, filter.SessionIDs)
	}
	if len(filter.TaskIDs) > 0 {
		query += ` AND turn_row.task_id IN (?)`
		args = append(args, filter.TaskIDs)
	}
	if cursorID != "" {
		query += ` AND (turn_row.started_at > ? OR (turn_row.started_at = ? AND turn_row.id > ?))`
		args = append(args, cursorTime, cursorTime, cursorID)
	}
	query += ` ORDER BY turn_row.started_at ASC, turn_row.id ASC LIMIT ?`
	args = append(args, limit+1)
	query, args, err = sqlx.In(query, args...)
	if err != nil {
		return models.SessionActivityTurnsPage{}, err
	}
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return models.SessionActivityTurnsPage{}, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]models.SessionActivityTurn, 0, limit+1)
	for rows.Next() {
		var item models.SessionActivityTurn
		var completed sql.NullTime
		if err := rows.Scan(&item.ID, &item.SessionID, &item.TaskID, &item.StartedAt, &completed); err != nil {
			return models.SessionActivityTurnsPage{}, err
		}
		if completed.Valid {
			item.CompletedAt = &completed.Time
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return models.SessionActivityTurnsPage{}, err
	}
	page := models.SessionActivityTurnsPage{Turns: items}
	if len(items) > limit {
		page.HasMore = true
		page.Turns = items[:limit]
		last := page.Turns[len(page.Turns)-1]
		page.NextCursor = encodeSessionActivityCursor("turns", last.StartedAt, last.ID)
	}
	return page, nil
}

func normalizeSessionActivityReadLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > sessionActivityReadPageMax {
		return sessionActivityReadPageMax
	}
	return limit
}

func encodeSessionActivityCursor(kind string, at time.Time, id string) string {
	value := strings.Join([]string{"v1", kind, at.UTC().Format(time.RFC3339Nano), id}, "\x00")
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decodeSessionActivityCursor(cursor, kind string) (time.Time, string, error) {
	if cursor == "" {
		return time.Time{}, "", nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid session activity cursor")
	}
	parts := strings.SplitN(string(decoded), "\x00", 4)
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != kind || parts[3] == "" {
		return time.Time{}, "", fmt.Errorf("invalid session activity cursor")
	}
	at, err := time.Parse(time.RFC3339Nano, parts[2])
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid session activity cursor")
	}
	return at, parts[3], nil
}
