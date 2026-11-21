package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/kandev/kandev/internal/common/sessioncatalog"
)

const legacyCatalogRetirementPageSize = 128

// RetireLegacySessionCatalogCandidates replaces host candidates that lack
// task-owned provenance with a newer-revision notice. Provider availability,
// effective settings and every other metadata key are untouched, and no
// restart is implied. Each write is conditional on the observed record.
func (r *Repository) RetireLegacySessionCatalogCandidates(ctx context.Context) (int, error) {
	retired := 0
	for after := ""; ; {
		ids, last, err := r.legacyCatalogCandidatePage(ctx, after)
		if err != nil {
			return retired, err
		}
		for _, id := range ids {
			changed, err := r.retireLegacyCatalogCandidate(ctx, id)
			if err != nil {
				return retired, err
			}
			if changed {
				retired++
			}
		}
		if last == "" {
			return retired, nil
		}
		after = last
	}
}

// legacyCatalogCandidatePage returns sessions whose record is a legacy
// candidate, plus the page cursor ("" when the scan is complete).
func (r *Repository) legacyCatalogCandidatePage(ctx context.Context, after string) ([]string, string, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`SELECT id, metadata FROM task_sessions
 WHERE metadata LIKE ? AND id > ? ORDER BY id LIMIT ?`), `%"`+sessioncatalog.MetadataKey+`"%`, after, legacyCatalogRetirementPageSize)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	last, count := "", 0
	for rows.Next() {
		var id string
		var raw sql.NullString
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, "", err
		}
		last, count = id, count+1
		if _, legacy := legacyCatalogCandidate(raw.String); legacy {
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	if count < legacyCatalogRetirementPageSize {
		last = ""
	}
	return ids, last, nil
}

func legacyCatalogCandidate(rawMetadata string) (sessioncatalog.Observation, bool) {
	var metadata map[string]json.RawMessage
	var observation sessioncatalog.Observation
	if rawMetadata == "" || json.Unmarshal([]byte(rawMetadata), &metadata) != nil {
		return observation, false
	}
	record, ok := metadata[sessioncatalog.MetadataKey]
	if !ok || json.Unmarshal(record, &observation) != nil {
		return observation, false
	}
	return observation, observation.Disposition == sessioncatalog.DispositionHostCandidates && !observation.TaskOwnedCandidates()
}

func (r *Repository) retireLegacyCatalogCandidate(ctx context.Context, sessionID string) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if found, err := lockTaskSessionRow(ctx, tx, sessionID); err != nil || !found {
		return false, err
	}
	var raw sql.NullString
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT metadata FROM task_sessions WHERE id = ?`), sessionID).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	previous, legacy := legacyCatalogCandidate(raw.String)
	if !legacy {
		return false, nil
	}
	next := previous
	next.Disposition, next.Reason = sessioncatalog.DispositionRediscoveryRequired, sessioncatalog.ReasonContextUnverified
	next.Source, next.Digest, next.Models, next.Modes, next.Catalog, next.Probe = "", "", nil, nil, nil, nil
	if err := r.writeCatalogObservation(ctx, tx, sessionID, previous.Revision, &next); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
