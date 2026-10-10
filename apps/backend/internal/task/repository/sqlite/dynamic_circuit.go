package sqlite

import (
	"context"
	"database/sql"
	"time"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

var _ dynamicruntime.CircuitPersistence = (*Repository)(nil)

const saveCircuitSQL = `
	INSERT INTO dynamic_resource_circuits
		(resource_key, state, until_at, code, probe_until, reset_known, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(resource_key) DO UPDATE SET
		state = excluded.state,
		until_at = excluded.until_at,
		code = excluded.code,
		probe_until = excluded.probe_until,
		reset_known = excluded.reset_known,
		updated_at = excluded.updated_at
`

func (r *Repository) SaveCircuit(ctx context.Context, snapshot dynamicruntime.CircuitSnapshot) error {
	resetKnown := 0
	if snapshot.ResetKnown {
		resetKnown = 1
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(saveCircuitSQL), snapshot.Key, snapshot.State,
		nullableTime(snapshot.Until), snapshot.Code, nullableTime(snapshot.ProbeUntil), resetKnown, time.Now().UTC())
	return err
}

func (r *Repository) SaveCircuits(ctx context.Context, snapshots []dynamicruntime.CircuitSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	query := r.db.Rebind(saveCircuitSQL)
	updatedAt := time.Now().UTC()
	for _, snapshot := range snapshots {
		resetKnown := 0
		if snapshot.ResetKnown {
			resetKnown = 1
		}
		if _, err := tx.ExecContext(ctx, query, snapshot.Key, snapshot.State, nullableTime(snapshot.Until),
			snapshot.Code, nullableTime(snapshot.ProbeUntil), resetKnown, updatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) LoadCircuits(ctx context.Context) ([]dynamicruntime.CircuitSnapshot, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT resource_key, state, until_at, code, probe_until, reset_known
		FROM dynamic_resource_circuits
		WHERE state <> ? OR probe_until IS NOT NULL ORDER BY resource_key
	`), dynamicruntime.CircuitClosed)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var snapshots []dynamicruntime.CircuitSnapshot
	for rows.Next() {
		var snapshot dynamicruntime.CircuitSnapshot
		var until, probeUntil sql.NullTime
		var state, code string
		if err := rows.Scan(&snapshot.Key, &state, &until, &code, &probeUntil, &snapshot.ResetKnown); err != nil {
			return nil, err
		}
		snapshot.State = dynamicruntime.CircuitState(state)
		snapshot.Code = routingerr.Code(code)
		if until.Valid {
			snapshot.Until = until.Time
		}
		if probeUntil.Valid {
			snapshot.ProbeUntil = probeUntil.Time
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, rows.Err()
}
