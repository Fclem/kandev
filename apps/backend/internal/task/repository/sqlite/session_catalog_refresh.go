package sqlite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"github.com/kandev/kandev/internal/task/models"
)

const catalogLiveStates = "('RUNNING', 'WAITING_FOR_INPUT', 'IDLE')"
const catalogLiveExecutorStatuses = "('running', 'ready')"
const catalogTerminalTaskStates = "('COMPLETED', 'FAILED', 'CANCELLED')"

// ListCatalogRefreshSessions pages current concrete-profile executions, not routing parents.
func (r *Repository) ListCatalogRefreshSessions(ctx context.Context, profileID, after string, limit int) ([]*models.TaskSession, error) {
	if profileID == "" || limit < 1 || limit > 128 {
		return nil, errors.New("invalid catalog refresh page")
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`SELECT `+taskSessionSelectCols+` `+taskSessionFromClause+`
 JOIN tasks catalog_task ON catalog_task.id = ts.task_id
 WHERE er.execution_profile_id = ? AND er.agent_execution_id <> ''
 AND ts.state IN `+catalogLiveStates+` AND catalog_task.archived_at IS NULL
 AND er.status IN `+catalogLiveExecutorStatuses+` AND catalog_task.state NOT IN `+catalogTerminalTaskStates+`
 AND ts.id > ? ORDER BY ts.id LIMIT ?`), profileID, after, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return r.scanTaskSessions(ctx, rows)
}

// PublishSessionCatalog reserves the task/session owner before inspecting the prior
// observation. Executor upserts use the same task/session locks on PostgreSQL;
// SQLite reserves its writer at BEGIN. Only this metadata key is written.
func (r *Repository) PublishSessionCatalog(ctx context.Context, sessionID, executionID, profileID string, expected any, next sessioncatalog.Observation) (*sessioncatalog.Observation, error) {
	return r.writeSessionCatalog(ctx, sessionID, executionID, profileID, expected, next, false)
}

// SettleSessionCatalogDiscovery accepts only the notice captured by this source
// before the provider handshake. A newer publication remains pending.
func (r *Repository) SettleSessionCatalogDiscovery(ctx context.Context, sessionID, executionID, profileID string, expected any, next sessioncatalog.Observation) (*sessioncatalog.Observation, error) {
	if err := sessioncatalog.ValidateDiscovery(next.Result); err != nil {
		return nil, err
	}
	return r.writeSessionCatalog(ctx, sessionID, executionID, profileID, expected, next, true)
}

func (r *Repository) writeSessionCatalog(ctx context.Context, sessionID, executionID, profileID string, expected any, next sessioncatalog.Observation, settlement bool) (*sessioncatalog.Observation, error) {
	if sessionID == "" || executionID == "" || profileID == "" || next.ProfileID != profileID || next.Owner.ExecutionID != executionID {
		return nil, errors.New("invalid catalog publication owner")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	phase := catalogOwnerLive
	if settlement {
		phase = catalogOwnerTerminal
	}
	metadata, err := r.lockCatalogSessionOwner(ctx, tx, sessionID, executionID, profileID, phase)
	if err != nil || metadata == nil {
		return nil, err
	}
	var previous sessioncatalog.Observation
	matches, err := catalogPublicationPriorMatches(metadata, executionID, expected, &next, &previous, settlement)
	if err != nil || !matches {
		return nil, err
	}
	if err := r.writeCatalogObservation(ctx, tx, sessionID, previous.Revision, &next); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &next, nil
}

func catalogPublicationPriorMatches(metadata map[string]json.RawMessage, executionID string, expected any, next, previous *sessioncatalog.Observation, settlement bool) (bool, error) {
	source, err := catalogSettingsSource(metadata)
	if err != nil {
		return false, err
	}
	if (source.ExecutionID != "" && source.ExecutionID != executionID) || source.AttemptID != next.Owner.AttemptID || source.Generation != next.Owner.SourceGeneration {
		return false, nil
	}
	prior := metadata[sessioncatalog.MetadataKey]
	matches, err := catalogPriorMatches(prior, expected)
	if err != nil || !matches {
		return false, err
	}
	if len(prior) > 0 {
		if err := json.Unmarshal(prior, previous); err != nil {
			return false, err
		}
	}
	return !settlement || catalogLaunchMatches(metadata, next.Owner, *previous), nil
}

func (r *Repository) writeCatalogObservation(ctx context.Context, tx *sqlx.Tx, sessionID string, previousRevision uint64, next *sessioncatalog.Observation) error {
	if previousRevision == math.MaxUint64 {
		return errors.New("catalog revision exhausted")
	}
	next.Version = 1
	next.Revision = previousRevision + 1
	payload, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if len(payload) > 2*1024*1024 {
		return errors.New("catalog observation exceeds size limit")
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(metadataKeyUpdateQuery("task_sessions", r.db.DriverName())), metadataKeyUpdateArgs(r.db.DriverName(), sessioncatalog.MetadataKey, string(payload), r.nowUTC(), sessionID)...)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("catalog session disappeared during publication")
	}
	return nil
}

func catalogLaunchMatches(metadata map[string]json.RawMessage, owner sessioncatalog.Owner, previous sessioncatalog.Observation) bool {
	var launch sessioncatalog.Launch
	if err := json.Unmarshal(metadata[sessioncatalog.LaunchMetadataKey], &launch); err != nil {
		return false
	}
	return launch.Owner == owner && launch.NoticeRevision > 0 && launch.NoticeRevision == previous.Revision && previous.Version == 1 && previous.Disposition != sessioncatalog.DispositionCleared
}

func catalogPriorMatches(prior json.RawMessage, expected any) (bool, error) {
	if len(prior) == 0 {
		prior = json.RawMessage("null")
	}
	expectedJSON, err := json.Marshal(expected)
	if err != nil {
		return false, err
	}
	var actualValue, expectedValue any
	actual := json.NewDecoder(bytes.NewReader(prior))
	actual.UseNumber()
	if err := actual.Decode(&actualValue); err != nil {
		return false, err
	}
	wanted := json.NewDecoder(bytes.NewReader(expectedJSON))
	wanted.UseNumber()
	if err := wanted.Decode(&expectedValue); err != nil {
		return false, err
	}
	return reflect.DeepEqual(actualValue, expectedValue), nil
}
