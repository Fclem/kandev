package github

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Personal-star persistence statuses and error sentinels.
const (
	PersonalStarStatusActive   = "active"
	PersonalStarStatusInactive = "inactive"

	PersonalStarFlowKind = "github-personal-star"

	personalStarCleanupPending  = "pending"
	personalStarCleanupTerminal = "terminal"
)

var (
	// ErrPersonalStarFence reports a lifecycle revision or credential
	// generation that no longer matches the committed fence.
	ErrPersonalStarFence = errors.New("github personal star connection fence changed")
	// ErrPersonalStarFlowUnavailable reports an absent, consumed, or expired flow.
	ErrPersonalStarFlowUnavailable = errors.New("github personal star flow is unavailable")
)

// PersonalStarConnection is non-secret metadata for the dedicated OAuth App
// connection. Token material lives only in generation-specific secret IDs.
type PersonalStarConnection struct {
	WorkspaceID             string    `db:"workspace_id"`
	UserID                  string    `db:"user_id"`
	Login                   string    `db:"login"`
	GitHubUserID            int64     `db:"github_user_id"`
	GrantedScopes           string    `db:"granted_scopes"`
	PublicRepo              bool      `db:"public_repo"`
	Status                  string    `db:"status"`
	CredentialGeneration    int64     `db:"credential_generation"`
	LifecycleRevision       int64     `db:"lifecycle_revision"`
	AccessSecretID          string    `db:"access_secret_id"`
	RefreshSecretID         string    `db:"refresh_secret_id"`
	AccessSecretGeneration  int64     `db:"access_secret_generation"`
	RefreshSecretGeneration int64     `db:"refresh_secret_generation"`
	LastReconnectOutcome    string    `db:"last_reconnect_outcome"`
	CreatedAt               time.Time `db:"created_at"`
	UpdatedAt               time.Time `db:"updated_at"`
}

// PersonalStarLifecycle is the durable revision/credential-generation fence.
type PersonalStarLifecycle struct {
	Revision             int64     `db:"revision"`
	CredentialGeneration int64     `db:"credential_generation"`
	UpdatedAt            time.Time `db:"updated_at"`
}

const personalStarConnectionSelect = `
	SELECT workspace_id, user_id, login, github_user_id, granted_scopes, public_repo, status,
		credential_generation, lifecycle_revision, COALESCE(access_secret_id, '') AS access_secret_id,
		COALESCE(refresh_secret_id, '') AS refresh_secret_id, access_secret_generation,
		refresh_secret_generation, last_reconnect_outcome, created_at, updated_at
	FROM github_personal_star_connection`

// GetPersonalStarConnection returns the connection row, or nil when absent.
func (s *Store) GetPersonalStarConnection(ctx context.Context, workspaceID, userID string) (*PersonalStarConnection, error) {
	var connection PersonalStarConnection
	err := s.ro.GetContext(ctx, &connection,
		s.ro.Rebind(personalStarConnectionSelect+` WHERE workspace_id = ? AND user_id = ?`), workspaceID, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &connection, nil
}

// GetPersonalStarLifecycle returns the fence, or a zero value when absent.
func (s *Store) GetPersonalStarLifecycle(ctx context.Context, workspaceID, userID string) (PersonalStarLifecycle, error) {
	return getPersonalStarLifecycleOn(ctx, s.ro, workspaceID, userID)
}

// getPersonalStarLifecycleOn reads the fence on a caller-supplied executor so a
// transaction-bound read never waits on a second connection.
func getPersonalStarLifecycleOn(
	ctx context.Context,
	exec sqlx.Ext,
	workspaceID, userID string,
) (PersonalStarLifecycle, error) {
	var lifecycle PersonalStarLifecycle
	queryer, ok := exec.(sqlx.QueryerContext)
	if !ok {
		return PersonalStarLifecycle{}, errors.New("personal star lifecycle executor cannot query")
	}
	err := sqlx.GetContext(ctx, queryer, &lifecycle, exec.Rebind(`
		SELECT revision, credential_generation, updated_at FROM github_personal_star_lifecycle
		WHERE workspace_id = ? AND user_id = ?`), workspaceID, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return PersonalStarLifecycle{}, nil
	}
	return lifecycle, err
}

// BumpPersonalStarLifecycle upserts the fence and returns the new revision and
// credential generation. Every start calls this before creating flow state, so
// a disconnect that races a start always wins.
func (s *Store) BumpPersonalStarLifecycle(ctx context.Context, workspaceID, userID string) (int64, int64, error) {
	var lifecycle PersonalStarLifecycle
	err := s.db.GetContext(ctx, &lifecycle, s.db.Rebind(`
		INSERT INTO github_personal_star_lifecycle (workspace_id, user_id, revision, credential_generation, updated_at)
		VALUES (?, ?, 1, 1, ?)
		ON CONFLICT (workspace_id, user_id) DO UPDATE SET
			revision = github_personal_star_lifecycle.revision + 1,
			credential_generation = github_personal_star_lifecycle.credential_generation + 1,
			updated_at = excluded.updated_at
		RETURNING revision, credential_generation`),
		workspaceID, userID, time.Now().UTC())
	if err != nil {
		return 0, 0, fmt.Errorf("bump personal star lifecycle: %w", err)
	}
	return lifecycle.Revision, lifecycle.CredentialGeneration, nil
}

// ReplacePersonalStarConnection atomically commits a verified replacement when
// the fence still matches. Superseded secret IDs are returned so the caller can
// schedule their cleanup only after this commit succeeds.
func (s *Store) ReplacePersonalStarConnection(
	ctx context.Context,
	connection *PersonalStarConnection,
	expectedRevision, expectedGeneration int64,
) ([]string, error) {
	now := time.Now().UTC()
	previous, err := s.GetPersonalStarConnection(ctx, connection.WorkspaceID, connection.UserID)
	if err != nil {
		return nil, err
	}
	superseded := supersededPersonalStarSecrets(previous, connection)
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE github_personal_star_connection SET
			login = ?, github_user_id = ?, granted_scopes = ?, public_repo = ?, status = ?,
			credential_generation = ?, lifecycle_revision = ?, access_secret_id = ?,
			refresh_secret_id = ?, access_secret_generation = ?, refresh_secret_generation = ?,
			last_reconnect_outcome = ?, updated_at = ?
		WHERE workspace_id = ? AND user_id = ?
			AND lifecycle_revision = ? AND credential_generation = ?`),
		connection.Login, connection.GitHubUserID, connection.GrantedScopes, connection.PublicRepo,
		PersonalStarStatusActive, connection.CredentialGeneration, connection.LifecycleRevision,
		nullableSecretID(connection.AccessSecretID), nullableSecretID(connection.RefreshSecretID),
		connection.AccessSecretGeneration, connection.RefreshSecretGeneration,
		connection.LastReconnectOutcome, now,
		connection.WorkspaceID, connection.UserID, expectedRevision, expectedGeneration)
	if err != nil {
		return nil, fmt.Errorf("replace personal star connection: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, ErrPersonalStarFence
	}
	connection.CreatedAt, connection.UpdatedAt = now, now
	return superseded, nil
}

// InsertPersonalStarConnection commits the first connection for a key.
func (s *Store) InsertPersonalStarConnection(
	ctx context.Context,
	connection *PersonalStarConnection,
	expectedRevision, expectedGeneration int64,
) error {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO github_personal_star_connection (
			workspace_id, user_id, login, github_user_id, granted_scopes, public_repo, status,
			credential_generation, lifecycle_revision, access_secret_id, refresh_secret_id,
			access_secret_generation, refresh_secret_generation, last_reconnect_outcome, created_at, updated_at)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		WHERE EXISTS (
			SELECT 1 FROM github_personal_star_lifecycle
			WHERE workspace_id = ? AND user_id = ? AND revision = ? AND credential_generation = ?)`),
		connection.WorkspaceID, connection.UserID, connection.Login, connection.GitHubUserID,
		connection.GrantedScopes, connection.PublicRepo, PersonalStarStatusActive,
		connection.CredentialGeneration, connection.LifecycleRevision,
		nullableSecretID(connection.AccessSecretID), nullableSecretID(connection.RefreshSecretID),
		connection.AccessSecretGeneration, connection.RefreshSecretGeneration,
		connection.LastReconnectOutcome, now, now,
		connection.WorkspaceID, connection.UserID, expectedRevision, expectedGeneration)
	if err != nil {
		return fmt.Errorf("insert personal star connection: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrPersonalStarFence
	}
	connection.CreatedAt, connection.UpdatedAt = now, now
	return nil
}

// SetPersonalStarReconnectOutcome records the last start/callback outcome.
func (s *Store) SetPersonalStarReconnectOutcome(ctx context.Context, workspaceID, userID, outcome string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE github_personal_star_connection SET last_reconnect_outcome = ?, updated_at = ?
		WHERE workspace_id = ? AND user_id = ?`), outcome, time.Now().UTC(), workspaceID, userID)
	if err != nil {
		return fmt.Errorf("record personal star reconnect outcome: %w", err)
	}
	return nil
}

// DisconnectPersonalStarConnection creates or updates an inactive tombstone and
// bumps the fence, so any in-flight flow can no longer commit.
func (s *Store) DisconnectPersonalStarConnection(
	ctx context.Context,
	workspaceID, userID string,
) ([]string, error) {
	revision, generation, err := s.BumpPersonalStarLifecycle(ctx, workspaceID, userID)
	if err != nil {
		return nil, err
	}
	previous, err := s.GetPersonalStarConnection(ctx, workspaceID, userID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	superseded := personalStarSecretIDs(previous)
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO github_personal_star_connection (
			workspace_id, user_id, login, github_user_id, granted_scopes, public_repo, status,
			credential_generation, lifecycle_revision, access_secret_id, refresh_secret_id,
			access_secret_generation, refresh_secret_generation, last_reconnect_outcome, created_at, updated_at)
		VALUES (?, ?, '', 0, '', 0, ?, ?, ?, ?, ?, 0, 0, '', ?, ?)
		ON CONFLICT (workspace_id, user_id) DO UPDATE SET
			status = excluded.status,
			credential_generation = excluded.credential_generation,
			lifecycle_revision = excluded.lifecycle_revision,
			access_secret_id = NULL,
			refresh_secret_id = NULL,
			updated_at = excluded.updated_at`),
		workspaceID, userID, PersonalStarStatusInactive, generation, revision, nil, nil, now, now); err != nil {
		return nil, fmt.Errorf("disconnect personal star connection: %w", err)
	}
	return superseded, nil
}

// DeletePersonalStarWorkspaceState removes every dedicated star row for a
// workspace. Secret cleanup rows are returned so callers can delete the
// encrypted values they name.
func (s *Store) DeletePersonalStarWorkspaceState(ctx context.Context, workspaceID string) ([]string, error) {
	var secretIDs []string
	if err := s.db.SelectContext(ctx, &secretIDs, s.db.Rebind(`
		SELECT secret_id FROM github_personal_star_cleanup WHERE workspace_id = ?`), workspaceID); err != nil {
		return nil, fmt.Errorf("list personal star secrets: %w", err)
	}
	for _, statement := range []string{
		`DELETE FROM github_personal_star_cleanup WHERE workspace_id = ?`,
		`DELETE FROM github_personal_star_flow WHERE workspace_id = ?`,
		`DELETE FROM github_personal_star_connection WHERE workspace_id = ?`,
		`DELETE FROM github_personal_star_lifecycle WHERE workspace_id = ?`,
	} {
		if _, err := s.db.ExecContext(ctx, s.db.Rebind(statement), workspaceID); err != nil {
			return nil, fmt.Errorf("delete personal star workspace state: %w", err)
		}
	}
	return secretIDs, nil
}

// EnqueuePersonalStarSecretCleanup durably records a generation-specific secret
// that must be deleted. Re-enqueueing the same tuple is idempotent.
func (s *Store) EnqueuePersonalStarSecretCleanup(
	ctx context.Context,
	workspaceID, userID, secretID, secretKind string,
	generation int64,
) error {
	if secretID == "" {
		return nil
	}
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO github_personal_star_cleanup (
			id, workspace_id, user_id, secret_id, secret_kind, credential_generation,
			attempts, next_attempt_at, status, last_error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?, '', ?)
		ON CONFLICT (workspace_id, user_id, secret_id, credential_generation) DO NOTHING`),
		uuid.NewString(), workspaceID, userID, secretID, secretKind, generation,
		now, personalStarCleanupPending, now)
	if err != nil {
		return fmt.Errorf("enqueue personal star secret cleanup: %w", err)
	}
	return nil
}

func supersededPersonalStarSecrets(previous, replacement *PersonalStarConnection) []string {
	if previous == nil {
		return nil
	}
	superseded := make([]string, 0, 2)
	for _, candidate := range []string{previous.AccessSecretID, previous.RefreshSecretID} {
		if candidate == "" || candidate == replacement.AccessSecretID || candidate == replacement.RefreshSecretID {
			continue
		}
		superseded = append(superseded, candidate)
	}
	return superseded
}

func personalStarSecretIDs(connection *PersonalStarConnection) []string {
	if connection == nil {
		return nil
	}
	ids := make([]string, 0, 2)
	if connection.AccessSecretID != "" {
		ids = append(ids, connection.AccessSecretID)
	}
	if connection.RefreshSecretID != "" {
		ids = append(ids, connection.RefreshSecretID)
	}
	return ids
}

func nullableSecretID(secretID string) any {
	if secretID == "" {
		return nil
	}
	return secretID
}
