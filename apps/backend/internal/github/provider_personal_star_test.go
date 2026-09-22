package github

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

type closeCountingDeployment struct {
	closes int
	err    error
}

func (c *closeCountingDeployment) close() error {
	c.closes++
	return c.err
}

func newProviderTestDatabase(t *testing.T) *sqlx.DB {
	t.Helper()
	database, err := db.OpenSQLite(filepath.Join(t.TempDir(), "github.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlxDB := sqlx.NewDb(database, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	return sqlxDB
}

func TestProvidePersonalStarRuntimeUnavailableLeavesFeatureOff(t *testing.T) {
	ctx := context.Background()
	sqlxDB := newProviderTestDatabase(t)
	log := newTestLogger()
	counter := &closeCountingDeployment{}

	service, cleanup, err := Provide(sqlxDB, sqlxDB, nil, newFakePersonalStarSecrets(), nil, log,
		PersonalStarDeployment{Available: false, Close: counter.close})
	if err != nil {
		t.Fatal(err)
	}
	status, err := service.PersonalStarConnectionStatus(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if status.Available {
		t.Fatalf("unavailable deployment reported available status: %+v", status)
	}
	if counter.closes != 1 {
		t.Fatalf("deployment closes = %d, want the owner released once", counter.closes)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if counter.closes != 1 {
		t.Fatalf("deployment closes after cleanup = %d, want 1", counter.closes)
	}
}

func TestProvidePersonalStarRuntimeFailsClosedWithoutClientSecret(t *testing.T) {
	sqlxDB := newProviderTestDatabase(t)
	log := newTestLogger()
	counter := &closeCountingDeployment{}

	_, _, err := Provide(sqlxDB, sqlxDB, nil, newFakePersonalStarSecrets(), nil, log, PersonalStarDeployment{
		Available: true,
		Settings: PersonalStarOAuthSettings{
			ClientID:    "client-id",
			CallbackURL: "https://kandev.example/api/v1/github/user/starred/oauth/callback",
		},
		ReadClientSecret: func(context.Context) (string, error) {
			return "", errors.New("deployment secret is revoked")
		},
		Close: counter.close,
	})
	if err == nil {
		t.Fatal("Provide() succeeded without a resolvable client secret")
	}
	if counter.closes != 1 {
		t.Fatalf("deployment closes = %d, want the owner released on the failure path", counter.closes)
	}
}

func TestProvidePersonalStarRuntimeAppliesResolvedSecretAndReleasesOnce(t *testing.T) {
	ctx := context.Background()
	sqlxDB := newProviderTestDatabase(t)
	log := newTestLogger()
	counter := &closeCountingDeployment{}

	service, cleanup, err := Provide(sqlxDB, sqlxDB, nil, newFakePersonalStarSecrets(), nil, log, PersonalStarDeployment{
		Available: true,
		Settings: PersonalStarOAuthSettings{
			ClientID:    "client-id",
			CallbackURL: "https://kandev.example/api/v1/github/user/starred/oauth/callback",
		},
		ReadClientSecret: func(context.Context) (string, error) { return "client-secret", nil },
		Close:            counter.close,
	})
	if err != nil {
		t.Fatal(err)
	}
	if counter.closes != 0 {
		t.Fatalf("deployment was released before cleanup: %d", counter.closes)
	}
	status, err := service.PersonalStarConnectionStatus(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Available || status.Active || status.Capable {
		t.Fatalf("status for a never-connected deployment = %+v", status)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if counter.closes != 1 {
		t.Fatalf("deployment closes after cleanup = %d, want 1", counter.closes)
	}
	// A second cleanup is the shutdown path running twice; it must not
	// double-release deployment-owned handles.
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if counter.closes != 1 {
		t.Fatalf("deployment closes after repeated cleanup = %d, want 1", counter.closes)
	}
}

func TestProvideWithoutConnectionSecretStoreSkipsStarRuntime(t *testing.T) {
	sqlxDB := newProviderTestDatabase(t)
	log := newTestLogger()
	counter := &closeCountingDeployment{}

	service, cleanup, err := Provide(sqlxDB, sqlxDB, nil, nil, nil, log, PersonalStarDeployment{
		Available:        true,
		Settings:         PersonalStarOAuthSettings{ClientID: "client-id", CallbackURL: "https://kandev.example/cb"},
		ReadClientSecret: func(context.Context) (string, error) { return "client-secret", nil },
		Close:            counter.close,
	})
	if err != nil {
		t.Fatal(err)
	}
	if service == nil {
		t.Fatal("Provide() returned no service")
	}
	if counter.closes != 1 {
		t.Fatalf("deployment closes = %d, want released once without a star runtime", counter.closes)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
}
