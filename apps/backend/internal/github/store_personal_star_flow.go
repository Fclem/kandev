package github

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// PersonalStarFlow is one-time dedicated OAuth flow state. Only the SHA-256
// digest of the raw state is persisted; the raw state is returned to the
// browser and never stored, logged, or echoed in errors.
type PersonalStarFlow struct {
	StateDigest        string     `db:"state_sha256"`
	FlowKind           string     `db:"flow_kind"`
	WorkspaceID        string     `db:"workspace_id"`
	UserID             string     `db:"user_id"`
	SessionSubject     string     `db:"session_subject"`
	SessionID          string     `db:"session_id"`
	ExpectedRevision   int64      `db:"expected_revision"`
	ExpectedGeneration int64      `db:"expected_generation"`
	CodeVerifier       string     `db:"code_verifier"`
	CodeChallenge      string     `db:"code_challenge"`
	RedirectURI        string     `db:"redirect_uri"`
	ReturnPath         string     `db:"return_path"`
	CreatedAt          time.Time  `db:"created_at"`
	ExpiresAt          time.Time  `db:"expires_at"`
	ConsumedAt         *time.Time `db:"consumed_at"`
}

const personalStarFlowSelect = `
	SELECT state_sha256, flow_kind, workspace_id, user_id, session_subject,
		COALESCE(session_id, '') AS session_id, expected_revision, expected_generation,
		code_verifier, code_challenge, redirect_uri, return_path, created_at, expires_at, consumed_at
	FROM github_personal_star_flow`

// PersonalStarSecretCleanup is one durable generation-specific secret deletion.
type PersonalStarSecretCleanup struct {
	ID                   string     `db:"id"`
	WorkspaceID          string     `db:"workspace_id"`
	UserID               string     `db:"user_id"`
	SecretID             string     `db:"secret_id"`
	SecretKind           string     `db:"secret_kind"`
	CredentialGeneration int64      `db:"credential_generation"`
	Attempts             int        `db:"attempts"`
	NextAttemptAt        time.Time  `db:"next_attempt_at"`
	ClaimLeaseUntil      *time.Time `db:"claim_lease_until"`
	Status               string     `db:"status"`
	LastError            string     `db:"last_error"`
	CreatedAt            time.Time  `db:"created_at"`
}

const personalStarCleanupSelect = `
	SELECT id, workspace_id, user_id, secret_id, secret_kind, credential_generation,
		attempts, next_attempt_at, claim_lease_until, status, last_error, created_at
	FROM github_personal_star_cleanup`

// CreatePersonalStarFlow stores one-time flow state only while its workspace
// lifecycle remains active.
func (s *Store) CreatePersonalStarFlow(ctx context.Context, flow *PersonalStarFlow) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("create personal star flow: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockPersonalStarWorkspaceLifecycleForUpdate(ctx, tx, flow.WorkspaceID); err != nil {
		return fmt.Errorf("create personal star flow: %w", err)
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO github_personal_star_flow (
			state_sha256, flow_kind, workspace_id, user_id, session_subject, session_id,
			expected_revision, expected_generation, code_verifier, code_challenge,
			redirect_uri, return_path, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		flow.StateDigest, flow.FlowKind, flow.WorkspaceID, flow.UserID, flow.SessionSubject,
		nullableSecretID(flow.SessionID), flow.ExpectedRevision, flow.ExpectedGeneration,
		flow.CodeVerifier, flow.CodeChallenge, flow.RedirectURI, flow.ReturnPath,
		flow.CreatedAt, flow.ExpiresAt); err != nil {
		return fmt.Errorf("create personal star flow: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit personal star flow: %w", err)
	}
	return nil
}

// ClaimPersonalStarFlowForExchange atomically consumes a flow exactly once.
// The caller's validation runs inside the transaction, so a tampered PKCE pair
// rolls the claim back and no token exchange can follow.
func (s *Store) ClaimPersonalStarFlowForExchange(
	ctx context.Context,
	stateDigest string,
	now time.Time,
	validate func(context.Context, sqlx.Ext, *PersonalStarFlow) error,
) (*PersonalStarFlow, error) {
	var workspaceID string
	if err := s.db.GetContext(ctx, &workspaceID, s.db.Rebind(`
		SELECT workspace_id FROM github_personal_star_flow WHERE state_sha256 = ?`), stateDigest); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPersonalStarFlowUnavailable
		}
		return nil, fmt.Errorf("load personal star flow workspace: %w", err)
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("claim personal star flow: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockPersonalStarWorkspaceLifecycleForUpdate(ctx, tx, workspaceID); err != nil {
		return nil, err
	}

	statement := personalStarFlowSelect + ` WHERE state_sha256 = ?` + personalStarRowLockClause(s.db.DriverName())
	var flow PersonalStarFlow
	err = tx.GetContext(ctx, &flow, tx.Rebind(statement), stateDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPersonalStarFlowUnavailable
	}
	if err != nil {
		return nil, fmt.Errorf("claim personal star flow: %w", err)
	}
	if flow.WorkspaceID != workspaceID {
		return nil, ErrPersonalStarFlowUnavailable
	}
	if flow.ConsumedAt != nil || !now.Before(flow.ExpiresAt) {
		return nil, ErrPersonalStarFlowUnavailable
	}
	if err := validate(ctx, tx, &flow); err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE github_personal_star_flow SET consumed_at = ?
		WHERE state_sha256 = ? AND consumed_at IS NULL`), now, stateDigest)
	if err != nil {
		return nil, fmt.Errorf("consume personal star flow: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, ErrPersonalStarFlowUnavailable
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit personal star flow: %w", err)
	}
	flow.ConsumedAt = &now
	return &flow, nil
}

// DeleteExpiredPersonalStarFlows removes flow rows that can no longer be used.
func (s *Store) DeleteExpiredPersonalStarFlows(ctx context.Context, before time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		DELETE FROM github_personal_star_flow WHERE expires_at <= ?`), before)
	if err != nil {
		return 0, fmt.Errorf("delete expired personal star flows: %w", err)
	}
	return result.RowsAffected()
}

// ClaimDuePersonalStarSecretCleanups leases due cleanup rows transactionally.
// A row already leased by another worker is skipped, so concurrent workers
// never both claim the same record.
func (s *Store) ClaimDuePersonalStarSecretCleanups(
	ctx context.Context,
	now, leaseUntil time.Time,
	limit int,
) ([]*PersonalStarSecretCleanup, error) {
	var candidates []*PersonalStarSecretCleanup
	err := s.ro.SelectContext(ctx, &candidates, s.ro.Rebind(personalStarCleanupSelect+`
		WHERE status = ? AND next_attempt_at <= ?
			AND (claim_lease_until IS NULL OR claim_lease_until <= ?)
		ORDER BY next_attempt_at LIMIT ?`),
		personalStarCleanupPending, now, now, limit)
	if err != nil {
		return nil, fmt.Errorf("list personal star cleanup queue: %w", err)
	}
	claimed := make([]*PersonalStarSecretCleanup, 0, len(candidates))
	for _, candidate := range candidates {
		result, err := s.db.ExecContext(ctx, s.db.Rebind(`
			UPDATE github_personal_star_cleanup SET claim_lease_until = ?
			WHERE id = ? AND status = ? AND next_attempt_at <= ?
				AND (claim_lease_until IS NULL OR claim_lease_until <= ?)`),
			leaseUntil, candidate.ID, personalStarCleanupPending, now, now)
		if err != nil {
			return nil, fmt.Errorf("claim personal star cleanup: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if affected == 0 {
			continue
		}
		candidate.ClaimLeaseUntil = &leaseUntil
		claimed = append(claimed, candidate)
	}
	return claimed, nil
}

// ReschedulePersonalStarSecretCleanup records a bounded-backoff retry.
func (s *Store) ReschedulePersonalStarSecretCleanup(
	ctx context.Context,
	id string,
	attempts int,
	nextAttemptAt time.Time,
	lastError string,
) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE github_personal_star_cleanup SET attempts = ?, next_attempt_at = ?,
			claim_lease_until = NULL, last_error = ?
		WHERE id = ?`), attempts, nextAttemptAt, truncatePersonalStarError(lastError), id)
	if err != nil {
		return fmt.Errorf("reschedule personal star cleanup: %w", err)
	}
	return nil
}

// MarkPersonalStarSecretCleanupTerminal keeps a permanently failed deletion
// visible to operators instead of retrying forever. The attempt count is
// persisted with the terminal state so the row records how hard it tried.
func (s *Store) MarkPersonalStarSecretCleanupTerminal(ctx context.Context, id string, attempts int, lastError string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE github_personal_star_cleanup SET status = ?, attempts = ?, claim_lease_until = NULL, last_error = ?
		WHERE id = ?`), personalStarCleanupTerminal, attempts, truncatePersonalStarError(lastError), id)
	if err != nil {
		return fmt.Errorf("terminate personal star cleanup: %w", err)
	}
	return nil
}

// CompletePersonalStarSecretCleanup removes a successfully deleted record.
func (s *Store) CompletePersonalStarSecretCleanup(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(`
		DELETE FROM github_personal_star_cleanup WHERE id = ?`), id); err != nil {
		return fmt.Errorf("complete personal star cleanup: %w", err)
	}
	return nil
}

// GetPersonalStarSecretCleanup returns one queue row for verification.
func (s *Store) GetPersonalStarSecretCleanup(ctx context.Context, id string) (*PersonalStarSecretCleanup, error) {
	var record PersonalStarSecretCleanup
	err := s.ro.GetContext(ctx, &record, s.ro.Rebind(personalStarCleanupSelect+` WHERE id = ?`), id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// ListPersonalStarSecretCleanups returns a workspace's queue rows oldest first.
func (s *Store) ListPersonalStarSecretCleanups(ctx context.Context, workspaceID string) ([]*PersonalStarSecretCleanup, error) {
	var records []*PersonalStarSecretCleanup
	err := s.ro.SelectContext(ctx, &records,
		s.ro.Rebind(personalStarCleanupSelect+` WHERE workspace_id = ? ORDER BY created_at`), workspaceID)
	return records, err
}

func truncatePersonalStarError(message string) string {
	const limit = 512
	if len(message) <= limit {
		return message
	}
	return message[:limit]
}
