package messagequeue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	internaldb "github.com/kandev/kandev/internal/db"
)

const sendNowClaimRecoverySchema = `
	CREATE TABLE IF NOT EXISTS queue_send_now_claims (
		session_id  TEXT PRIMARY KEY,
		claim_json  TEXT NOT NULL,
		accepted    INTEGER NOT NULL DEFAULT 0,
		created_at  TIMESTAMP NOT NULL
	)
`

func (r *sqliteRepository) ensureSendNowClaimRecoverySchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, sendNowClaimRecoverySchema); err != nil {
		return fmt.Errorf("ensure Send Now claim recovery schema: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `ALTER TABLE queue_send_now_claims ADD COLUMN accepted INTEGER NOT NULL DEFAULT 0`); err != nil && !internaldb.IsDuplicateColumnError(err) {
		return fmt.Errorf("add Send Now claim acceptance: %w", err)
	}
	return nil
}

func (r *sqliteRepository) persistSendNowClaimTx(
	ctx context.Context,
	tx *sqlx.Tx,
	claim *SendNowClaim,
) error {
	sessionID, err := sendNowClaimSessionID(claim)
	if err != nil {
		return err
	}
	claimJSON, err := json.Marshal(claim)
	if err != nil {
		return fmt.Errorf("marshal Send Now claim: %w", err)
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO queue_send_now_claims (session_id, claim_json, created_at)
		VALUES (?, ?, ?)
	`), sessionID, string(claimJSON), time.Now().UTC()); err != nil {
		return fmt.Errorf("persist Send Now claim: %w", err)
	}
	return nil
}

func (r *sqliteRepository) deleteSendNowClaimTx(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID string,
) error {
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM queue_send_now_claims WHERE session_id = ?
	`), sessionID); err != nil {
		return fmt.Errorf("delete Send Now claim: %w", err)
	}
	return nil
}

func (r *sqliteRepository) ListPendingSendNowClaims(ctx context.Context) ([]PendingSendNowClaim, error) {
	if err := r.ensureSendNowClaimRecoverySchema(ctx); err != nil {
		return nil, err
	}
	return listPendingRecoveryRecords(
		ctx,
		r.db,
		`SELECT claim_json, accepted FROM queue_send_now_claims ORDER BY created_at, session_id`,
		"Send Now claim",
		func(claim SendNowClaim, accepted bool) PendingSendNowClaim {
			return PendingSendNowClaim{Claim: claim, Accepted: accepted}
		},
	)
}

func (r *sqliteRepository) MarkPendingSendNowClaimAccepted(ctx context.Context, sessionID string) error {
	if err := r.ensureSendNowClaimRecoverySchema(ctx); err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE queue_send_now_claims SET accepted = 1 WHERE session_id = ?
	`), sessionID)
	if err != nil {
		return fmt.Errorf("mark Send Now claim accepted: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark Send Now claim accepted rows affected: %w", err)
	}
	if affected != 1 {
		return ErrSendNowClaimChanged
	}
	return nil
}

func (r *sqliteRepository) DeletePendingSendNowClaim(ctx context.Context, sessionID string) error {
	if err := r.ensureSendNowClaimRecoverySchema(ctx); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM queue_send_now_claims WHERE session_id = ?
	`), sessionID); err != nil {
		return fmt.Errorf("discard pending Send Now claim: %w", err)
	}
	return nil
}

func pendingSendNowSessionsForTaskTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
) ([]string, error) {
	rows, err := tx.QueryxContext(ctx, `SELECT session_id, claim_json FROM queue_send_now_claims`)
	if err != nil {
		return nil, fmt.Errorf("list task Send Now claims: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var sessions []string
	for rows.Next() {
		var sessionID, claimJSON string
		if err := rows.Scan(&sessionID, &claimJSON); err != nil {
			return nil, fmt.Errorf("scan task Send Now claim: %w", err)
		}
		var claim SendNowClaim
		if err := json.Unmarshal([]byte(claimJSON), &claim); err != nil {
			return nil, fmt.Errorf("unmarshal task Send Now claim: %w", err)
		}
		matches := claim.Dispatch.TaskID == taskID
		for i := range claim.Sources {
			matches = matches || claim.Sources[i].TaskID == taskID
		}
		if matches {
			sessions = append(sessions, sessionID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task Send Now claims: %w", err)
	}
	return sessions, nil
}
