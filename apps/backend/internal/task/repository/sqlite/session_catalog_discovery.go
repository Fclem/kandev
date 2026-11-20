package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

type catalogOwnerPhase uint8

const (
	catalogOwnerLive catalogOwnerPhase = iota
	catalogOwnerStarting
	catalogOwnerTerminal
)

const (
	catalogOwnerQueryPrefix = `SELECT COALESCE(ts.metadata, '{}') FROM task_sessions ts
 JOIN tasks t ON t.id = ts.task_id JOIN executors_running er ON er.session_id = ts.id
 WHERE ts.id = ? AND er.agent_execution_id = ? AND er.execution_profile_id = ?
 AND (ts.execution_profile_id = ? OR ts.execution_profile_id = '') `
	catalogOwnerLiveQuery = catalogOwnerQueryPrefix +
		`AND er.status IN ` + catalogLiveExecutorStatuses + ` AND t.state NOT IN ` + catalogTerminalTaskStates +
		` AND ts.state IN ` + catalogLiveStates + ` AND t.archived_at IS NULL`
	catalogOwnerStartingQuery = catalogOwnerQueryPrefix +
		`AND er.status IN ('starting', 'running', 'ready')
 AND ts.state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT', 'IDLE') AND t.archived_at IS NULL`
	catalogOwnerTerminalQuery = catalogOwnerQueryPrefix +
		`AND er.status IN ('starting', 'running', 'ready', 'failed')
 AND ts.state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT', 'IDLE', 'FAILED') AND t.archived_at IS NULL`
)

type catalogSource struct {
	ExecutionID     string `json:"settings_source_execution_id"`
	AttemptID       string `json:"settings_attempt_id"`
	Generation      uint64 `json:"settings_source_generation"`
	ModelGeneration uint64 `json:"current_model_generation"`
	ModeGeneration  uint64 `json:"current_mode_generation"`
	Policy          string `json:"settings_policy"`
}

func catalogSettingsSource(metadata map[string]json.RawMessage) (catalogSource, error) {
	var source catalogSource
	if raw := metadata[models.SessionMetaKeyACPModelState]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &source); err != nil {
			return source, err
		}
	}
	return source, nil
}

func (r *Repository) lockCatalogSessionOwner(ctx context.Context, tx *sqlx.Tx, sessionID, executionID, profileID string, phase catalogOwnerPhase) (map[string]json.RawMessage, error) {
	var taskID string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT task_id FROM task_sessions WHERE id = ?`), sessionID).Scan(&taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := kandevdb.LockTaskRowInTx(ctx, tx, r.db.DriverName(), taskID); err != nil {
		return nil, err
	}
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), sessionID); err != nil {
		return nil, err
	}
	if err := lockAgentDeliveryWrites(ctx, tx, r.db.DriverName(), []string{sessionID}, nil); err != nil {
		return nil, err
	}
	if _, err := lockTaskSessionRow(ctx, tx, sessionID); err != nil {
		return nil, err
	}
	query := catalogOwnerLiveQuery
	switch phase {
	case catalogOwnerStarting:
		query = catalogOwnerStartingQuery
	case catalogOwnerTerminal:
		query = catalogOwnerTerminalQuery
	}
	var raw string
	err = tx.QueryRowContext(ctx, r.db.Rebind(query), sessionID, executionID, profileID, profileID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	metadata := make(map[string]json.RawMessage)
	if raw != "" && raw != jsonNull {
		if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
			return nil, err
		}
	}
	return metadata, nil
}

// BeginSessionCatalogDiscovery captures the notice before any provider handshake.
// Re-entry by the same owner preserves its original notice revision. Ownership
// rotation and the source snapshot commit in the same transaction as the marker.
func (r *Repository) BeginSessionCatalogDiscovery(ctx context.Context, sessionID, profileID string, owner sessioncatalog.Owner, policy string) (*sessioncatalog.Launch, error) {
	if owner.ExecutionID == "" || owner.SourceGeneration == 0 || sessionID == "" || profileID == "" {
		return nil, errors.New("invalid catalog discovery owner")
	}
	if policy != sessioncatalog.PolicyStrict && policy != sessioncatalog.PolicyProviderRestored {
		return nil, errors.New("invalid catalog source policy")
	}
	return r.beginSessionCatalogDiscovery(ctx, sessionID, profileID, &owner, policy)
}

func (r *Repository) beginSessionCatalogDiscovery(ctx context.Context, sessionID, profileID string, owner *sessioncatalog.Owner, policy string) (*sessioncatalog.Launch, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	metadata, err := r.lockCatalogSessionOwner(ctx, tx, sessionID, owner.ExecutionID, profileID, catalogOwnerStarting)
	if err != nil || metadata == nil {
		return nil, err
	}
	current, err := catalogDiscoverySourceCurrent(metadata, owner)
	if err != nil || !current {
		return nil, err
	}
	var previous sessioncatalog.Launch
	if raw := metadata[sessioncatalog.LaunchMetadataKey]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &previous); err != nil {
			return nil, err
		}
		if previous.Owner == *owner {
			return &previous, nil
		}
		if previous.Owner.ExecutionID == owner.ExecutionID && previous.Owner.SourceGeneration >= owner.SourceGeneration {
			return nil, nil
		}
	}
	launch := &sessioncatalog.Launch{Owner: *owner}
	launch.NoticeRevision, err = catalogLaunchNoticeRevision(metadata[sessioncatalog.MetadataKey], profileID)
	if err != nil {
		return nil, err
	}
	if err := r.reserveCatalogSettingsSource(ctx, tx, sessionID, metadata, *owner, policy); err != nil {
		return nil, err
	}
	if err := r.writeCatalogLaunch(ctx, tx, sessionID, launch); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return launch, nil
}

func (r *Repository) writeCatalogLaunch(ctx context.Context, tx *sqlx.Tx, sessionID string, launch *sessioncatalog.Launch) error {
	payload, err := json.Marshal(launch)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(metadataKeyUpdateQuery("task_sessions", r.db.DriverName())), metadataKeyUpdateArgs(r.db.DriverName(), sessioncatalog.LaunchMetadataKey, string(payload), r.nowUTC(), sessionID)...)
	return err
}

func catalogDiscoverySourceCurrent(metadata map[string]json.RawMessage, owner *sessioncatalog.Owner) (bool, error) {
	source, err := catalogSettingsSource(metadata)
	if err != nil {
		return false, err
	}
	stale := source.ExecutionID == owner.ExecutionID &&
		(source.Generation > owner.SourceGeneration || source.Generation == owner.SourceGeneration && source.AttemptID != owner.AttemptID)
	return !stale, nil
}

func catalogLaunchNoticeRevision(raw json.RawMessage, profileID string) (uint64, error) {
	if len(raw) == 0 {
		return 0, nil
	}
	var observation sessioncatalog.Observation
	if err := json.Unmarshal(raw, &observation); err != nil {
		return 0, err
	}
	if observation.Version == 1 && observation.ProfileID == profileID && observation.Disposition != sessioncatalog.DispositionCleared {
		return observation.Revision, nil
	}
	return 0, nil
}

func (r *Repository) reserveCatalogSettingsSource(ctx context.Context, tx *sqlx.Tx, sessionID string, metadata map[string]json.RawMessage, owner sessioncatalog.Owner, policy string) error {
	snapshot := make(map[string]json.RawMessage)
	if raw := metadata[models.SessionMetaKeyACPModelState]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &snapshot); err != nil {
			return err
		}
	}
	update, err := json.Marshal(catalogSource{ExecutionID: owner.ExecutionID, AttemptID: owner.AttemptID, Generation: owner.SourceGeneration, Policy: policy})
	if err != nil {
		return err
	}
	if err := json.Unmarshal(update, &snapshot); err != nil {
		return err
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(metadataKeyUpdateQuery("task_sessions", r.db.DriverName())), metadataKeyUpdateArgs(r.db.DriverName(), models.SessionMetaKeyACPModelState, string(payload), r.nowUTC(), sessionID)...)
	return err
}
