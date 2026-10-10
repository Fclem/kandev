package store

import (
	"context"
	"github.com/kandev/kandev/internal/testutil"
	"testing"
)

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.4
func TestPostgresLimitRecoveryPersistence(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := newSQLiteRepositoryWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	id := seedAgentProfile(t, repo, "limits", "claude-acp")
	profile, err := repo.GetAgentProfile(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	assertSavedLimitRecovery(t, profile, false, false)
	profile.LimitFallback = true
	profile.ResumeAfterReset = true
	if err := repo.UpdateAgentProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	if err := repo.initSchema(); err != nil {
		t.Fatal(err)
	}
	profile, err = repo.GetAgentProfile(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	assertSavedLimitRecovery(t, profile, true, true)
}
