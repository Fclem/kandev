package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"github.com/kandev/kandev/internal/task/models"
)

// PersistSessionCatalogAvailability updates only provider availability. Host
// notice eligibility is deliberately checked by the separate settlement CAS.
func (r *Repository) PersistSessionCatalogAvailability(ctx context.Context, sessionID string, owner sessioncatalog.Owner, result sessioncatalog.Discovery) (*sessioncatalog.ProviderCatalog, error) {
	if err := sessioncatalog.ValidateDiscovery(&result); err != nil {
		return nil, err
	}
	if owner.ExecutionID == "" || owner.SourceGeneration == 0 {
		return nil, errors.New("invalid provider catalog owner")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	profileID, metadata, err := r.lockCatalogProviderOwner(ctx, tx, sessionID, &owner)
	if err != nil || metadata == nil {
		return nil, err
	}
	snapshot := make(map[string]json.RawMessage)
	if err := json.Unmarshal(metadata[models.SessionMetaKeyACPModelState], &snapshot); err != nil {
		return nil, err
	}
	provider, err := nextSessionProviderCatalog(snapshot[sessioncatalog.ProviderMetadataField], profileID, &owner, &result)
	if err != nil || provider == nil {
		return nil, err
	}
	encoded, err := json.Marshal(provider)
	if err != nil {
		return nil, err
	}
	snapshot[sessioncatalog.ProviderMetadataField] = encoded
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	if len(payload) > 2<<20 {
		return nil, errors.New("provider catalog snapshot exceeds metadata limit")
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(metadataKeyUpdateQuery("task_sessions", r.db.DriverName())), metadataKeyUpdateArgs(r.db.DriverName(), models.SessionMetaKeyACPModelState, string(payload), r.nowUTC(), sessionID)...)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return provider, nil
}

func (r *Repository) lockCatalogProviderOwner(ctx context.Context, tx *sqlx.Tx, sessionID string, owner *sessioncatalog.Owner) (string, map[string]json.RawMessage, error) {
	var profileID string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT execution_profile_id FROM executors_running WHERE session_id = ? AND agent_execution_id = ?`), sessionID, owner.ExecutionID).Scan(&profileID)
	if errors.Is(err, sql.ErrNoRows) || err == nil && profileID == "" {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	metadata, err := r.lockCatalogSessionOwner(ctx, tx, sessionID, owner.ExecutionID, profileID, catalogOwnerTerminal)
	if err != nil || metadata == nil {
		return "", nil, err
	}
	source, err := catalogSettingsSource(metadata)
	if err != nil {
		return "", nil, err
	}
	if source.ExecutionID != owner.ExecutionID || source.AttemptID != owner.AttemptID || source.Generation != owner.SourceGeneration {
		return "", nil, nil
	}
	return profileID, metadata, nil
}

func nextSessionProviderCatalog(raw json.RawMessage, profileID string, owner *sessioncatalog.Owner, result *sessioncatalog.Discovery) (*sessioncatalog.ProviderCatalog, error) {
	var previous sessioncatalog.ProviderCatalog
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &previous); err != nil {
			return nil, err
		}
		if previous.Owner == *owner && previous.Result.Generation >= result.Generation {
			return nil, nil
		}
	}
	if previous.Revision == math.MaxUint64 {
		return nil, errors.New("provider catalog revision is exhausted")
	}
	return &sessioncatalog.ProviderCatalog{Revision: previous.Revision + 1, Owner: *owner, ProfileID: profileID, Result: *result}, nil
}
