package messagequeue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

func (r *sqliteRepository) ensureSendNowClaimRecoverySchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS queue_send_now_claims (
			session_id  TEXT PRIMARY KEY,
			claim_json  TEXT NOT NULL,
			created_at  TIMESTAMP NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("ensure Send Now claim recovery schema: %w", err)
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

func (r *sqliteRepository) ListPendingSendNowClaims(ctx context.Context) ([]SendNowClaim, error) {
	if err := r.ensureSendNowClaimRecoverySchema(ctx); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryxContext(ctx, `
		SELECT claim_json FROM queue_send_now_claims ORDER BY created_at, session_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list pending Send Now claims: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var claims []SendNowClaim
	for rows.Next() {
		var claimJSON string
		if err := rows.Scan(&claimJSON); err != nil {
			return nil, fmt.Errorf("scan pending Send Now claim: %w", err)
		}
		var claim SendNowClaim
		if err := json.Unmarshal([]byte(claimJSON), &claim); err != nil {
			return nil, fmt.Errorf("unmarshal pending Send Now claim: %w", err)
		}
		claims = append(claims, claim)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending Send Now claims: %w", err)
	}
	return claims, nil
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
