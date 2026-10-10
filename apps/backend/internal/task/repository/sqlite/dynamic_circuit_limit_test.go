package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/db"
)

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.3
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.6
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.7
func TestCircuitProviderLimitResetKnowledgeAndClosedLeaseRoundTrip(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 3, 12, 0, 0, 123000000, time.UTC)
	wanted := []dynamic.CircuitSnapshot{
		{Key: "limit|binding|account", State: dynamic.CircuitOpen, Until: now.Add(8 * 24 * time.Hour), Code: routingerr.CodeQuotaLimited, ResetKnown: true},
		{Key: "limit|binding|model|unknown", State: dynamic.CircuitOpen, Until: now.Add(30 * time.Minute), Code: routingerr.CodeRateLimited},
		{Key: "limit|binding|model|closed-owner", State: dynamic.CircuitClosed, ProbeUntil: now.Add(10 * time.Minute)},
	}
	for _, snapshot := range wanted {
		if err := repo.SaveCircuit(ctx, snapshot); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := repo.LoadCircuits(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byKey := make(map[string]dynamic.CircuitSnapshot, len(loaded))
	for _, snapshot := range loaded {
		byKey[snapshot.Key] = snapshot
	}
	for _, expected := range wanted {
		got, exists := byKey[expected.Key]
		if !exists || got.State != expected.State || got.Code != expected.Code || got.ResetKnown != expected.ResetKnown || !got.Until.Equal(expected.Until) || !got.ProbeUntil.Equal(expected.ProbeUntil) {
			t.Errorf("restored %s = %+v; expected %+v", expected.Key, got, expected)
		}
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.5
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.7
func TestCircuitProviderLimitBatchRollbackSurvivesReopen(t *testing.T) {
	for _, failure := range []string{"second row", "commit"} {
		t.Run(failure, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "circuits.db")
			repo := openCircuitRepository(t, path)
			ctx := context.Background()
			now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
			registry := dynamic.NewCircuitRegistry(dynamic.WithCircuitPersistence(repo))
			keys := []string{"limit|binding|account", "limit|binding|model|opus"}
			for _, key := range keys {
				if err := registry.OpenDurable(ctx, key, now.Add(time.Hour), routingerr.CodeQuotaLimited, true); err != nil {
					t.Fatal(err)
				}
			}
			fault := `CREATE TRIGGER fail_limit_clear BEFORE INSERT ON dynamic_resource_circuits
				WHEN NEW.resource_key = 'limit|binding|model|opus' AND NEW.state = 'closed'
				BEGIN SELECT RAISE(ABORT, 'second snapshot rejected'); END;`
			if failure == "commit" {
				fault = `CREATE TABLE limit_clear_parent (id TEXT PRIMARY KEY);
					CREATE TABLE limit_clear_child (
						id TEXT PRIMARY KEY,
						parent_id TEXT REFERENCES limit_clear_parent(id) DEFERRABLE INITIALLY DEFERRED
					);
					CREATE TRIGGER fail_limit_clear BEFORE INSERT ON dynamic_resource_circuits
					WHEN NEW.state = 'closed'
					BEGIN INSERT INTO limit_clear_child VALUES (NEW.resource_key, 'missing'); END;`
			}
			if _, err := repo.db.Exec(fault); err != nil {
				t.Fatal(err)
			}
			if err := registry.CloseManyDurable(ctx, keys); err == nil {
				t.Fatal("faulted batch clear succeeded")
			}
			for _, key := range keys {
				if !registry.IsOpen(key, now) {
					t.Fatalf("failed %s clear published closure for %s", failure, key)
				}
			}
			if err := repo.db.Close(); err != nil {
				t.Fatal(err)
			}
			reopened := openCircuitRepository(t, path)
			restored := dynamic.NewCircuitRegistry(dynamic.WithCircuitPersistence(reopened))
			if err := restored.Restore(ctx); err != nil {
				t.Fatal(err)
			}
			for _, key := range keys {
				mark, exists := restored.Get(key)
				if !exists || mark.State != dynamic.CircuitOpen || !mark.ResetKnown || !mark.Until.Equal(now.Add(time.Hour)) {
					t.Fatalf("%s rollback did not survive reopen for %s: %+v", failure, key, mark)
				}
			}
			if _, err := reopened.db.Exec(`DROP TRIGGER fail_limit_clear`); err != nil {
				t.Fatal(err)
			}
			if err := restored.CloseManyDurable(ctx, keys); err != nil {
				t.Fatal(err)
			}
			for _, key := range keys {
				if restored.IsOpen(key, now) {
					t.Fatalf("successful retry did not clear %s", key)
				}
			}
		})
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.7
func TestCircuitProviderLimitLegacyUpgradePreservesUnknownReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-circuits.db")
	connection, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	if _, err := connection.Exec(`CREATE TABLE dynamic_resource_circuits (
		resource_key TEXT PRIMARY KEY, state TEXT NOT NULL, until_at TIMESTAMP,
		code TEXT NOT NULL DEFAULT '', probe_until TIMESTAMP, updated_at TIMESTAMP NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	until := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	if _, err := connection.Exec(`INSERT INTO dynamic_resource_circuits
		(resource_key, state, until_at, code, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"credential:legacy", dynamic.CircuitOpen, until, routingerr.CodeQuotaLimited, until.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	wrapped := sqlx.NewDb(connection, "sqlite3")
	repo, err := NewWithDB(wrapped, wrapped, nil)
	if err != nil {
		t.Fatal(err)
	}
	registry := dynamic.NewCircuitRegistry(dynamic.WithCircuitPersistence(repo))
	if err := registry.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	mark, exists := registry.Get("credential:legacy")
	if !exists || mark.ResetKnown || !mark.Until.Equal(until) || mark.Code != routingerr.CodeQuotaLimited {
		t.Fatalf("legacy upgrade changed health or invented reset knowledge: %+v", mark)
	}
}

func openCircuitRepository(t *testing.T, path string) *Repository {
	t.Helper()
	connection, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	wrapped := sqlx.NewDb(connection, "sqlite3")
	t.Cleanup(func() { _ = wrapped.Close() })
	repo, err := NewWithDB(wrapped, wrapped, nil)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}
