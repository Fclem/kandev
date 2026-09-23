package github

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
)

// Personal-star persistence statuses and error sentinels.
const (
	PersonalStarStatusActive   = "active"
	PersonalStarStatusInactive = "inactive"

	personalStarSecretKindAccess  = "github-personal-star-access"
	personalStarSecretKindRefresh = "github-personal-star-refresh"

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

// PersonalStarFence is a lifecycle revision/generation pair.
type PersonalStarFence struct {
	Revision   int64
	Generation int64
}

// ReplacePersonalStarConnection atomically commits a verified replacement when
// the committed row still carries rowFence and the lifecycle row still carries
// expectedFence. The lifecycle guard is what stops a disconnect or a competing
// start that landed after the caller claimed its fence from being overwritten:
// the caller's own fence read is not enough, because the row it is replacing
// can already have been tombstoned by that disconnect.
//
// The superseded secret ids are recorded in the durable cleanup queue inside the
// same transaction, so a replacement can never commit while the tokens it
// stopped referencing are unqueued: that window would orphan ciphertext that no
// row points at any more.
func (s *Store) ReplacePersonalStarConnection(
	ctx context.Context,
	connection *PersonalStarConnection,
	rowFence, expectedFence PersonalStarFence,
) ([]string, error) {
	now := time.Now().UTC()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("replace personal star connection: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	previous, err := personalStarConnectionForUpdate(ctx, tx, connection.WorkspaceID, connection.UserID)
	if err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE github_personal_star_connection SET
			login = ?, github_user_id = ?, granted_scopes = ?, public_repo = ?, status = ?,
			credential_generation = ?, lifecycle_revision = ?, access_secret_id = ?,
			refresh_secret_id = ?, access_secret_generation = ?, refresh_secret_generation = ?,
			last_reconnect_outcome = ?, updated_at = ?
		WHERE workspace_id = ? AND user_id = ?
			AND lifecycle_revision = ? AND credential_generation = ?
			AND EXISTS (
				SELECT 1 FROM github_personal_star_lifecycle
				WHERE workspace_id = ? AND user_id = ?
					AND revision = ? AND credential_generation = ?)`),
		connection.Login, connection.GitHubUserID, connection.GrantedScopes, connection.PublicRepo,
		PersonalStarStatusActive, connection.CredentialGeneration, connection.LifecycleRevision,
		nullableSecretID(connection.AccessSecretID), nullableSecretID(connection.RefreshSecretID),
		connection.AccessSecretGeneration, connection.RefreshSecretGeneration,
		connection.LastReconnectOutcome, now,
		connection.WorkspaceID, connection.UserID, rowFence.Revision, rowFence.Generation,
		connection.WorkspaceID, connection.UserID, expectedFence.Revision, expectedFence.Generation)
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
	supersededIDs := supersededPersonalStarSecretRefs(previous, connection)
	if err := enqueuePersonalStarSecretRefsTx(ctx, tx, connection.WorkspaceID, connection.UserID, supersededIDs); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("replace personal star connection: %w", err)
	}
	connection.CreatedAt, connection.UpdatedAt = now, now
	return supersededSecretIDs(supersededIDs), nil
}

// personalStarRowLockClause locks the selected row on PostgreSQL. The driver
// name is "pgx" there, so the check must go through the shared dialect helper:
// an equality test against "postgres" silently never matches and leaves the
// read unguarded.
func personalStarRowLockClause(driver string) string {
	if dialect.IsPostgres(driver) {
		return " FOR UPDATE"
	}
	return ""
}

// personalStarSecretRef names one generation-specific secret awaiting deletion.
type personalStarSecretRef struct {
	SecretID   string
	Kind       string
	Generation int64
}

// personalStarConnectionForUpdate reads a connection row inside a transaction,
// locking it on PostgreSQL so a concurrent replacement cannot interleave.
func personalStarConnectionForUpdate(
	ctx context.Context,
	tx *sqlx.Tx,
	workspaceID, userID string,
) (*PersonalStarConnection, error) {
	query := personalStarConnectionSelect + ` WHERE workspace_id = ? AND user_id = ?` + personalStarRowLockClause(tx.DriverName())
	var connection PersonalStarConnection
	err := tx.GetContext(ctx, &connection, tx.Rebind(query), workspaceID, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load personal star connection: %w", err)
	}
	return &connection, nil
}

// enqueuePersonalStarSecretRefsTx records cleanup rows on the caller's
// transaction. Duplicate tuples are ignored so a retried replacement stays
// idempotent.
func enqueuePersonalStarSecretRefsTx(
	ctx context.Context,
	tx *sqlx.Tx,
	workspaceID, userID string,
	refs []personalStarSecretRef,
) error {
	if len(refs) == 0 {
		return nil
	}
	now := time.Now().UTC()
	for _, ref := range refs {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`
			INSERT INTO github_personal_star_cleanup (
				id, workspace_id, user_id, secret_id, secret_kind, credential_generation,
				attempts, next_attempt_at, status, last_error, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?, '', ?)
			ON CONFLICT (workspace_id, user_id, secret_id, credential_generation) DO NOTHING`),
			uuid.NewString(), workspaceID, userID, ref.SecretID, ref.Kind, ref.Generation,
			now, personalStarCleanupPending, now); err != nil {
			return fmt.Errorf("queue personal star secret cleanup: %w", err)
		}
	}
	return nil
}

func supersededSecretIDs(refs []personalStarSecretRef) []string {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		ids = append(ids, ref.SecretID)
	}
	return ids
}

func personalStarSecretIDsFor(connection *PersonalStarConnection) []personalStarSecretRef {
	if connection == nil {
		return nil
	}
	refs := make([]personalStarSecretRef, 0, 2)
	if connection.AccessSecretID != "" {
		refs = append(refs, personalStarSecretRef{
			SecretID: connection.AccessSecretID, Kind: personalStarSecretKindAccess,
			Generation: connection.AccessSecretGeneration,
		})
	}
	if connection.RefreshSecretID != "" {
		refs = append(refs, personalStarSecretRef{
			SecretID: connection.RefreshSecretID, Kind: personalStarSecretKindRefresh,
			Generation: connection.RefreshSecretGeneration,
		})
	}
	return refs
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
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("disconnect personal star connection: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	previous, err := personalStarConnectionForUpdate(ctx, tx, workspaceID, userID)
	if err != nil {
		return nil, err
	}
	var lifecycle PersonalStarLifecycle
	if err := tx.GetContext(ctx, &lifecycle, tx.Rebind(`
		INSERT INTO github_personal_star_lifecycle (workspace_id, user_id, revision, credential_generation, updated_at)
		VALUES (?, ?, 1, 1, ?)
		ON CONFLICT (workspace_id, user_id) DO UPDATE SET
			revision = github_personal_star_lifecycle.revision + 1,
			credential_generation = github_personal_star_lifecycle.credential_generation + 1,
			updated_at = excluded.updated_at
		RETURNING revision, credential_generation`), workspaceID, userID, time.Now().UTC()); err != nil {
		return nil, fmt.Errorf("bump personal star lifecycle: %w", err)
	}
	revision, generation := lifecycle.Revision, lifecycle.CredentialGeneration
	now := time.Now().UTC()
	superseded := personalStarSecretIDsFor(previous)
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
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
	if err := enqueuePersonalStarSecretRefsTx(ctx, tx, workspaceID, userID, superseded); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("disconnect personal star connection: %w", err)
	}
	return supersededSecretIDs(superseded), nil
}

// ListPersonalStarConnections returns every dedicated connection in a
// workspace: state is keyed by (workspace, user), so a workspace can hold one
// per participating user.
func (s *Store) ListPersonalStarConnections(ctx context.Context, workspaceID string) ([]*PersonalStarConnection, error) {
	var connections []*PersonalStarConnection
	err := s.ro.SelectContext(ctx, &connections,
		s.ro.Rebind(personalStarConnectionSelect+` WHERE workspace_id = ? ORDER BY user_id`), workspaceID)
	return connections, err
}

// DeletePersonalStarWorkspaceState removes the workspace's dedicated star rows
// but deliberately leaves github_personal_star_cleanup intact: those rows are
// the only remaining record of the generation-specific secrets that still have
// to be deleted, so dropping them would orphan encrypted token material.
func (s *Store) DeletePersonalStarWorkspaceState(ctx context.Context, workspaceID string) error {
	for _, statement := range []string{
		`DELETE FROM github_personal_star_flow WHERE workspace_id = ?`,
		`DELETE FROM github_personal_star_connection WHERE workspace_id = ?`,
		`DELETE FROM github_personal_star_lifecycle WHERE workspace_id = ?`,
	} {
		if _, err := s.db.ExecContext(ctx, s.db.Rebind(statement), workspaceID); err != nil {
			return fmt.Errorf("delete personal star workspace state: %w", err)
		}
	}
	return nil
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

// supersededPersonalStarSecretRefs returns the previous generation's secrets
// that the replacement no longer references, each with the generation it was
// written for.
func supersededPersonalStarSecretRefs(previous, replacement *PersonalStarConnection) []personalStarSecretRef {
	if previous == nil {
		return nil
	}
	superseded := make([]personalStarSecretRef, 0, 2)
	for _, candidate := range personalStarSecretIDsFor(previous) {
		if candidate.SecretID == replacement.AccessSecretID || candidate.SecretID == replacement.RefreshSecretID {
			continue
		}
		superseded = append(superseded, candidate)
	}
	return superseded
}

func nullableSecretID(secretID string) any {
	if secretID == "" {
		return nil
	}
	return secretID
}
