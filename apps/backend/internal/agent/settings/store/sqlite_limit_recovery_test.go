package store

import (
	"context"
	"encoding/json"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"testing"
)

func assertSavedLimitRecovery(t *testing.T, profile *models.AgentProfile, fallback, resume bool) {
	t.Helper()
	raw, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["limit_fallback"] != fallback || fields["resume_after_reset"] != resume {
		t.Fatalf("limit recovery = %v / %v, want %v / %v", fields["limit_fallback"], fields["resume_after_reset"], fallback, resume)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.1
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.4
func TestLimitRecoveryPersistence(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()
	agent := &models.Agent{Name: "claude-acp"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatal(err)
	}
	profile := &models.AgentProfile{AgentID: agent.ID, Name: "Limits"}
	if err := json.Unmarshal([]byte(`{"limit_fallback":true,"resume_after_reset":true}`), profile); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertSavedLimitRecovery(t, got, true, true)
	if err := repo.initSchema(); err != nil {
		t.Fatal(err)
	}
	got, err = repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertSavedLimitRecovery(t, got, true, true)
	if err := json.Unmarshal([]byte(`{"limit_fallback":false,"resume_after_reset":false}`), got); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateAgentProfile(ctx, got); err != nil {
		t.Fatal(err)
	}
	got, err = repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertSavedLimitRecovery(t, got, false, false)
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.1
func TestLimitRecoveryUpgradeDefaults(t *testing.T) {
	db := newLegacyDB(t)
	for _, statement := range []string{
		`INSERT INTO agents (id,name,created_at,updated_at) VALUES ('a','claude-acp',datetime('now'),datetime('now'))`,
		`INSERT INTO agent_profiles (id,agent_id,name,agent_display_name,model,created_at,updated_at) VALUES ('p','a','Limits','Claude','model',datetime('now'),datetime('now'))`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	repo, err := newSQLiteRepository(db, db, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	existing, err := repo.GetAgentProfile(context.Background(), "p")
	if err != nil {
		t.Fatal(err)
	}
	assertSavedLimitRecovery(t, existing, false, false)
	id := seedAgentProfile(t, repo, "upgrade", "fresh-agent")
	got, err := repo.GetAgentProfile(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	assertSavedLimitRecovery(t, got, false, false)
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.4
func TestLimitRecoveryInterruptedUpgradePreservesSettings(t *testing.T) {
	db := newLegacyDB(t)
	for _, statement := range []string{
		`ALTER TABLE agent_profiles ADD COLUMN limit_fallback INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE agent_profiles ADD COLUMN resume_after_reset INTEGER NOT NULL DEFAULT 0`,
		`INSERT INTO agents (id,name,created_at,updated_at) VALUES ('a','claude-acp',datetime('now'),datetime('now'))`,
		`INSERT INTO agent_profiles (id,agent_id,name,agent_display_name,model,limit_fallback,resume_after_reset,created_at,updated_at) VALUES ('p','a','Limits','Claude','model',1,1,datetime('now'),datetime('now'))`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	repo, err := newSQLiteRepository(db, db, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetAgentProfile(context.Background(), "p")
	if err != nil {
		t.Fatal(err)
	}
	assertSavedLimitRecovery(t, got, true, true)
}
