package updates

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/persistence"
)

func configureManagedNPMInstall(t *testing.T) string {
	t.Helper()
	homeDir := t.TempDir()
	metadataPath, _ := writeServiceInstallForTest(t, homeDir, serviceInstallMetadata{
		Manager:     serviceManagerSystemd,
		Mode:        installModeUser,
		Kind:        installKindNPM,
		HomeDir:     homeDir,
		LogDir:      filepath.Join(homeDir, "logs"),
		ServicePath: filepath.Join(homeDir, "kandev.service"),
		NodePath:    "/usr/bin/node",
		CLIEntry:    "/usr/lib/node_modules/kandev/bin/cli.js",
	})
	t.Setenv(envRunningAsService, "true")
	t.Setenv(envServiceMode, installModeUser)
	t.Setenv(envServiceManager, serviceManagerSystemd)
	t.Setenv(envInstallKind, installKindNPM)
	t.Setenv(envServiceMetadata, metadataPath)
	return homeDir
}

type memorySettingsStore struct {
	mu      sync.Mutex
	value   []byte
	present bool
	getErr  error
	saveErr error
}

func (s *memorySettingsStore) Get(context.Context, string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.value...), s.present, s.getErr
}

func (s *memorySettingsStore) Save(_ context.Context, _ string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	s.value = append([]byte(nil), value...)
	s.present = true
	return nil
}

func TestSelectedChannelDefaultsInvalidAndMissingValuesToStable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		store   *memorySettingsStore
		wantErr bool
	}{
		{name: "missing", store: &memorySettingsStore{}},
		{name: "invalid", store: &memorySettingsStore{value: []byte("preview"), present: true}},
		{name: "read failure", store: &memorySettingsStore{getErr: errors.New("db down")}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(newTestPool(t), "v1.0.0", nil, logger.Default(), WithSettingsStore(tc.store))
			got, err := svc.selectedChannel(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatal("selectedChannel error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("selectedChannel: %v", err)
			}
			if got != ChannelStable {
				t.Fatalf("channel=%q want %q", got, ChannelStable)
			}
		})
	}
}

func TestPersistedNightlyChannelSurvivesServiceRestart(t *testing.T) {
	store := &memorySettingsStore{}
	first := NewService(newTestPool(t), "v1.0.0", nil, logger.Default(), WithSettingsStore(store))
	if err := first.persistChannel(context.Background(), ChannelNightly); err != nil {
		t.Fatalf("persistChannel: %v", err)
	}
	second := NewService(first.pool, "v1.0.0", nil, logger.Default(), WithSettingsStore(store))
	got, err := second.selectedChannel(context.Background())
	if err != nil {
		t.Fatalf("selectedChannel: %v", err)
	}
	if got != ChannelNightly {
		t.Fatalf("channel=%q want nightly", got)
	}

	if err := second.persistChannel(context.Background(), Channel("preview")); err == nil {
		t.Fatal("persistChannel accepted invalid channel")
	}
}

func TestGetReadsOnlyTheSelectedChannelCache(t *testing.T) {
	homeDir := configureManagedNPMInstall(t)
	pool := newTestPool(t)
	stableAt := time.Unix(1_700_000_000, 0).UTC()
	nightlyAt := stableAt.Add(time.Hour)
	if err := persistence.WriteLatestVersion(pool.Writer(), "v1.2.3", "https://example/stable", stableAt); err != nil {
		t.Fatal(err)
	}
	if err := persistence.WriteLatestNightlyVersion(
		pool.Writer(),
		"1.2.4-nightly.shaabc123def456",
		"https://example/nightly",
		nightlyAt,
	); err != nil {
		t.Fatal(err)
	}
	store := &memorySettingsStore{value: []byte(ChannelNightly), present: true}
	svc := NewService(pool, "v1.2.3", nil, logger.Default(), WithHomeDir(homeDir), WithSettingsStore(store))
	resp, err := svc.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.Latest != "1.2.4-nightly.shaabc123def456" || resp.LatestURL != "https://example/nightly" || !resp.LatestCheckedAt.Equal(nightlyAt) {
		t.Fatalf("Get returned wrong cache: %+v", resp)
	}
}

func TestNightlyAvailabilityTreatsUnequalSHAsAsNewTargets(t *testing.T) {
	svc := NewService(nil, "v1.2.4-nightly.shaffffffffffff", nil, logger.Default())
	if !svc.updateAvailableFor(ChannelNightly, "1.2.4-nightly.sha000000000000") {
		t.Fatal("unequal authoritative nightly SHA should be available regardless of lexical order")
	}
	if svc.updateAvailableFor(ChannelNightly, "1.2.4-nightly.shaffffffffffff") {
		t.Fatal("identical nightly version should not be available")
	}
}

func TestCheckUsesNightlyResolverAndWritesOnlyNightlyCache(t *testing.T) {
	homeDir := configureManagedNPMInstall(t)
	pool := newTestPool(t)
	store := &memorySettingsStore{value: []byte(ChannelNightly), present: true}
	svc := NewService(pool, "v1.2.3", nil, logger.Default(), WithHomeDir(homeDir), WithSettingsStore(store))
	stableCalled := false
	svc.SetFetcher(func(context.Context) (string, string, error) {
		stableCalled = true
		return "", "", errors.New("stable resolver should not run")
	})
	svc.SetNightlyFetcher(func(context.Context) (string, string, error) {
		return "1.2.4-nightly.shaabc123def456", "https://example/nightly", nil
	})

	resp, err := svc.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if stableCalled {
		t.Fatal("stable resolver ran for nightly channel")
	}
	if resp.Latest != "1.2.4-nightly.shaabc123def456" {
		t.Fatalf("latest=%q", resp.Latest)
	}
	stable, _, _, err := persistence.ReadLatestVersion(pool.Reader())
	if err != nil {
		t.Fatal(err)
	}
	nightly, _, _, err := persistence.ReadLatestNightlyVersion(pool.Reader())
	if err != nil {
		t.Fatal(err)
	}
	if stable != "" || nightly != "1.2.4-nightly.shaabc123def456" {
		t.Fatalf("cache isolation failed: stable=%q nightly=%q", stable, nightly)
	}
}

func TestUnsupportedInstallForcesPersistedNightlyPreferenceToStable(t *testing.T) {
	pool := newTestPool(t)
	if err := persistence.WriteLatestVersion(pool.Writer(), "v1.2.3", "https://example/stable", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := persistence.WriteLatestNightlyVersion(
		pool.Writer(),
		"1.2.4-nightly.shaabc123def456",
		"https://example/nightly",
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}
	store := &memorySettingsStore{value: []byte(ChannelNightly), present: true}
	svc := NewService(pool, "v1.2.3", nil, logger.Default(), WithSettingsStore(store))
	resp, err := svc.Get()
	if err != nil {
		t.Fatal(err)
	}
	if resp.Channel != ChannelStable || resp.Latest != "v1.2.3" || resp.ChannelEditable {
		t.Fatalf("unsupported effective response=%+v", resp)
	}
}
