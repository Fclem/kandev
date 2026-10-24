package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// sqliteJulianDayEpochMS is the Unix epoch expressed in SQLite's integer
// Julian-day milliseconds; julianday() returns that integer divided by 86400000.
const sqliteJulianDayEpochMS = 210866760000000

// julianDayOvershootsMS reports whether SQLite's julianday() difference for the
// interval, scaled back to milliseconds, lands above the exact duration.
func julianDayOvershootsMS(start, end time.Time) bool {
	day := float64(86400000)
	s := float64(start.UnixMilli()+sqliteJulianDayEpochMS) / day
	e := float64(end.UnixMilli()+sqliteJulianDayEpochMS) / day
	return (e-s)*day > float64(end.Sub(start).Milliseconds())
}

// At 2026-scale Julian days one float64 ULP is about 40 microseconds, so each
// interval's julianday() difference carries an error. Intervals whose error has
// the same sign must still sum to the exact whole-millisecond total.
func TestActivityDurationSumsWholeMillisecondIntervals(t *testing.T) {
	r, s := reviewActivitySession(t)
	ctx := context.Background()
	const intervals = 1000
	var want int64
	inserted := 0
	for offset := time.Duration(0); inserted < intervals; offset += time.Millisecond {
		start := s.StartedAt.Add(offset)
		end := start.Add(time.Millisecond)
		if !julianDayOvershootsMS(start, end) {
			continue
		}
		_, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO task_session_user_activity (session_id, task_id, actor_id, started_at, ended_at) VALUES (?, ?, ?, ?, ?)`), s.ID, s.TaskID, "actor", start.UTC(), end.UTC())
		require.NoError(t, err)
		want += end.Sub(start).Milliseconds()
		inserted++
	}
	rows, err := r.SumUserActivity(ctx, []string{s.ID})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(intervals), rows[0].IntervalCount)
	require.Equal(t, want, rows[0].ActiveMS)
}

// Beat timestamps are stored truncated to whole milliseconds. A fractional
// endpoint would make SQLite's per-interval julianday() rounding disagree with
// Postgres' exact-sum-then-truncate duration for the same stored interval.
func TestRecordUserActivityStoresWholeMillisecondEndpoints(t *testing.T) {
	repo, s := reviewActivitySession(t)
	ctx := context.Background()
	_, err := repo.RecordUserActivity(ctx, "actor", s.ID, s.StartedAt.Add(400*time.Microsecond))
	require.NoError(t, err)
	_, err = repo.RecordUserActivity(ctx, "actor", s.ID, s.StartedAt.Add(5*time.Second+900*time.Microsecond))
	require.NoError(t, err)

	var started, ended time.Time
	require.NoError(t, repo.db.QueryRowContext(ctx, repo.db.Rebind(`SELECT started_at, ended_at FROM task_session_user_activity WHERE session_id = ?`), s.ID).Scan(&started, &ended))
	require.True(t, started.Equal(s.StartedAt), "started_at = %s", started)
	require.True(t, ended.Equal(s.StartedAt.Add(5*time.Second)), "ended_at = %s", ended)

	rows, err := repo.SumUserActivity(ctx, []string{s.ID})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(5000), rows[0].ActiveMS)
}
