package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestSQLiteProfileMembershipMutationsSerializeWithReorder(t *testing.T) {
	for _, scenario := range profileOrderMutationScenarios() {
		for _, winner := range []string{"mutation-first", "reorder-first"} {
			t.Run(scenario.name+"/"+winner, func(t *testing.T) {
				repo := openSQLiteProfileOrderRaceRepo(t)
				runProfileOrderMutationRace(t, repo, scenario, winner)
			})
		}
	}
}

func TestSQLiteProfileUpdateRetriesAfterOwnerChangesDuringRead(t *testing.T) {
	runProfileOwnerChangedDuringRead(t, openSQLiteProfileOrderRaceRepo(t))
}

func TestPostgresProfileUpdateRetriesAfterOwnerChangesDuringRead(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	db.SetMaxOpenConns(8)
	repo, err := newSQLiteRepositoryWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize profile order schema: %v", err)
	}
	runProfileOwnerChangedDuringRead(t, repo)
}

func runProfileOwnerChangedDuringRead(t *testing.T, repo *sqliteRepository) {
	t.Helper()
	ctx := context.Background()
	a := &models.Agent{Name: "profile-owner-a"}
	b := &models.Agent{Name: "profile-owner-b"}
	c := &models.Agent{Name: "profile-owner-c"}
	for _, agent := range []*models.Agent{a, b, c} {
		if err := repo.CreateAgent(ctx, agent); err != nil {
			t.Fatal(err)
		}
	}
	profile := &models.AgentProfile{AgentID: a.ID, Name: "owner-race", Model: "model"}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	staleUpdate, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	staleUpdate.AgentID = b.ID

	observedA := make(chan struct{})
	releaseOwnerRead := make(chan struct{})
	var paused atomic.Bool
	repo.profileOrderAfterOwnershipRead = func(id, ownerID, _ string) error {
		if id == profile.ID && ownerID == a.ID && paused.CompareAndSwap(false, true) {
			close(observedA)
			<-releaseOwnerRead
		}
		return nil
	}
	t.Cleanup(func() {
		select {
		case <-releaseOwnerRead:
		default:
			close(releaseOwnerRead)
		}
	})
	var mu sync.Mutex
	var lockedAgents []string
	repo.profileOrderAfterLock = func(operation, agentID string) error {
		if operation == "update-profile" {
			mu.Lock()
			lockedAgents = append(lockedAgents, agentID)
			mu.Unlock()
		}
		return nil
	}
	outerDone := make(chan error, 1)
	go func() { outerDone <- repo.UpdateAgentProfile(ctx, staleUpdate) }()
	awaitProfileOrderBarrier(t, observedA)

	concurrentUpdate, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	concurrentUpdate.AgentID = c.ID
	if err := repo.UpdateAgentProfile(ctx, concurrentUpdate); err != nil {
		t.Fatalf("concurrent A-to-C owner update: %v", err)
	}
	close(releaseOwnerRead)
	if err := <-outerDone; err != nil {
		t.Fatalf("retried A-to-B owner update: %v", err)
	}
	current, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.AgentID != b.ID {
		t.Fatalf("profile owner = %s, want final owner %s", current.AgentID, b.ID)
	}
	mu.Lock()
	gotLocks := append([]string(nil), lockedAgents...)
	mu.Unlock()
	wantLocks := []string{}
	for _, pair := range [][2]string{{a.ID, c.ID}, {a.ID, b.ID}, {b.ID, c.ID}} {
		if pair[0] < pair[1] {
			wantLocks = append(wantLocks, pair[0], pair[1])
		} else {
			wantLocks = append(wantLocks, pair[1], pair[0])
		}
	}
	if len(gotLocks) != len(wantLocks) {
		t.Fatalf("membership locks = %v, want sorted actual-owner lock sets %v", gotLocks, wantLocks)
	}
	for i := range gotLocks {
		if gotLocks[i] != wantLocks[i] {
			t.Fatalf("membership locks = %v, want sorted actual-owner lock sets %v", gotLocks, wantLocks)
		}
	}
}

func TestPostgresProfileMembershipMutationsSerializeWithReorder(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	db.SetMaxOpenConns(8)
	repo, err := newSQLiteRepositoryWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize profile order schema: %v", err)
	}
	for _, scenario := range profileOrderMutationScenarios() {
		for _, winner := range []string{"mutation-first", "reorder-first"} {
			t.Run(scenario.name+"/"+winner, func(t *testing.T) {
				runProfileOrderMutationRace(t, repo, scenario, winner)
			})
		}
	}
}

type profileOrderMutationScenario struct {
	name      string
	operation string
	mutate    func(context.Context, *sqliteRepository, string, string, *models.AgentProfile) error
	missing   bool
}

func profileOrderMutationScenarios() []profileOrderMutationScenario {
	return []profileOrderMutationScenario{
		{
			name:      "create",
			operation: "create-profile",
			mutate: func(ctx context.Context, repo *sqliteRepository, a, _ string, _ *models.AgentProfile) error {
				return repo.CreateAgentProfile(ctx, &models.AgentProfile{AgentID: a, Name: "created", Model: "model"})
			},
		},
		{
			name:      "duplicate",
			operation: "duplicate-profile",
			mutate: func(ctx context.Context, repo *sqliteRepository, a, _ string, source *models.AgentProfile) error {
				return repo.DuplicateAgentProfile(ctx, DuplicateAgentProfileInput{
					Source:  source,
					Profile: &models.AgentProfile{AgentID: a, Name: "duplicate", Model: source.Model},
				})
			},
		},
		{
			name:      "soft-delete",
			operation: "delete-profile",
			mutate: func(ctx context.Context, repo *sqliteRepository, _, _ string, source *models.AgentProfile) error {
				return repo.DeleteAgentProfile(ctx, source.ID)
			},
		},
		{
			name:      "agent-cascade-delete",
			operation: "delete-agent",
			mutate: func(ctx context.Context, repo *sqliteRepository, a, _ string, _ *models.AgentProfile) error {
				return repo.DeleteAgent(ctx, a)
			},
			missing: true,
		},
		{
			name:      "global-owner-a-to-b",
			operation: "update-profile",
			mutate: func(ctx context.Context, repo *sqliteRepository, _, b string, source *models.AgentProfile) error {
				source.AgentID = b
				return repo.UpdateAgentProfile(ctx, source)
			},
		},
		{
			name:      "global-to-workspace",
			operation: "update-profile",
			mutate: func(ctx context.Context, repo *sqliteRepository, _, _ string, source *models.AgentProfile) error {
				source.WorkspaceID = "workspace-order-test"
				return repo.UpdateAgentProfile(ctx, source)
			},
		},
		{
			name:      "workspace-to-global",
			operation: "update-profile",
			mutate: func(ctx context.Context, repo *sqliteRepository, a, _ string, source *models.AgentProfile) error {
				source.AgentID = a
				source.WorkspaceID = ""
				return repo.UpdateAgentProfile(ctx, source)
			},
		},
	}
}

func openSQLiteProfileOrderRaceRepo(t *testing.T) *sqliteRepository {
	t.Helper()
	path := filepath.Join(t.TempDir(), "profile-order-race.db")
	dsn := path + "?_journal_mode=WAL&_busy_timeout=5000"
	writer, err := sqlx.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open SQLite writer: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	writer.SetMaxOpenConns(8)
	reader, err := sqlx.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open SQLite reader: %v", err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	reader.SetMaxOpenConns(8)
	repo, err := newSQLiteRepository(writer, reader, nil, false)
	if err != nil {
		t.Fatalf("initialize SQLite profile order schema: %v", err)
	}
	return repo
}

func runProfileOrderMutationRace(t *testing.T, repo *sqliteRepository, scenario profileOrderMutationScenario, winner string) {
	t.Helper()
	ctx := context.Background()
	a := &models.Agent{Name: "order-race-a"}
	b := &models.Agent{Name: "order-race-b"}
	if err := repo.CreateAgent(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateAgent(ctx, b); err != nil {
		t.Fatal(err)
	}
	profiles := []*models.AgentProfile{
		{AgentID: a.ID, Name: "one", Model: "model"},
		{AgentID: a.ID, Name: "two", Model: "model"},
	}
	for _, profile := range profiles {
		if err := repo.CreateAgentProfile(ctx, profile); err != nil {
			t.Fatal(err)
		}
	}
	source, err := repo.GetAgentProfile(ctx, profiles[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if scenario.name == "workspace-to-global" {
		source = &models.AgentProfile{AgentID: a.ID, Name: "workspace-profile", Model: "model", WorkspaceID: "workspace-order-test"}
		if err := repo.CreateAgentProfile(ctx, source); err != nil {
			t.Fatal(err)
		}
		source, err = repo.GetAgentProfile(ctx, source.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	ordered, err := globalProfileOrderIDs(ctx, repo, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	requested := []string{ordered[1], ordered[0]}
	locked := make(chan struct{})
	release := make(chan struct{})
	repo.profileOrderAfterLock = func(operation, agentID string) error {
		shouldPause := (winner == "mutation-first" && operation == scenario.operation) ||
			(winner == "reorder-first" && operation == "reorder")
		if shouldPause && agentID == a.ID {
			select {
			case <-locked:
			default:
				close(locked)
			}
			<-release
		}
		return nil
	}
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	reorderDone := make(chan profileOrderReorderResult, 1)
	mutationDone := make(chan error, 1)
	mutation := func() error {
		return scenario.mutate(ctx, repo, a.ID, b.ID, source)
	}
	reorder := func() {
		revision, changed, err := repo.ReorderAgentProfiles(ctx, a.ID, requested)
		reorderDone <- profileOrderReorderResult{revision: revision, changed: changed, err: err}
	}
	if winner == "mutation-first" {
		runMutationFirstOrderRace(t, scenario, mutation, reorder, locked, release, mutationDone, reorderDone)
	} else {
		runReorderFirstOrderRace(t, mutation, reorder, locked, release, mutationDone, reorderDone)
	}
	assertProfileOrderMutationResult(t, ctx, repo, scenario.name, a.ID, b.ID, profiles[0].ID)
}

type profileOrderReorderResult struct {
	revision int64
	changed  bool
	err      error
}

func runMutationFirstOrderRace(
	t *testing.T,
	scenario profileOrderMutationScenario,
	mutation func() error,
	reorder func(),
	locked, release chan struct{},
	mutationDone chan error,
	reorderDone <-chan profileOrderReorderResult,
) {
	t.Helper()
	go func() { mutationDone <- mutation() }()
	awaitProfileOrderBarrier(t, locked)
	started := make(chan struct{})
	go func() {
		close(started)
		reorder()
	}()
	awaitProfileOrderBarrier(t, started)
	close(release)
	if err := <-mutationDone; err != nil {
		t.Fatalf("membership mutation: %v", err)
	}
	result := <-reorderDone
	if scenario.missing {
		if !errors.Is(result.err, ErrProfileOrderAgentNotFound) {
			t.Fatalf("reorder after agent deletion error = %v, want missing-agent", result.err)
		}
		return
	}
	if !errors.Is(result.err, ErrProfileOrderSetMismatch) {
		t.Fatalf("reorder after %s error = %v, want stale membership", scenario.name, result.err)
	}
}

func runReorderFirstOrderRace(
	t *testing.T,
	mutation func() error,
	reorder func(),
	locked, release chan struct{},
	mutationDone chan error,
	reorderDone <-chan profileOrderReorderResult,
) {
	t.Helper()
	go reorder()
	awaitProfileOrderBarrier(t, locked)
	started := make(chan struct{})
	go func() {
		close(started)
		mutationDone <- mutation()
	}()
	awaitProfileOrderBarrier(t, started)
	close(release)
	result := <-reorderDone
	if result.err != nil || !result.changed || result.revision != 1 {
		t.Fatalf("reorder before mutation = revision %d changed %t err %v; want committed revision 1", result.revision, result.changed, result.err)
	}
	if err := <-mutationDone; err != nil {
		t.Fatalf("membership mutation after reorder: %v", err)
	}
}

func globalProfileOrderIDs(ctx context.Context, repo *sqliteRepository, agentID string) ([]string, error) {
	profiles, err := repo.ListAgentProfiles(ctx, agentID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		if profile.WorkspaceID == "" {
			ids = append(ids, profile.ID)
		}
	}
	return ids, nil
}

func awaitProfileOrderBarrier(t *testing.T, barrier <-chan struct{}) {
	t.Helper()
	select {
	case <-barrier:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for profile-order concurrency barrier")
	}
}

func assertProfileOrderMutationResult(t *testing.T, ctx context.Context, repo *sqliteRepository, scenario, aID, bID, sourceID string) {
	t.Helper()
	if scenario == "agent-cascade-delete" {
		if _, err := repo.GetAgent(ctx, aID); err == nil {
			t.Fatal("agent still exists after cascade delete")
		}
		return
	}
	profilesA, err := repo.ListAgentProfiles(ctx, aID)
	if err != nil {
		t.Fatal(err)
	}
	profilesB, err := repo.ListAgentProfiles(ctx, bID)
	if err != nil {
		t.Fatal(err)
	}
	contains := func(profiles []*models.AgentProfile, id string) bool {
		for _, profile := range profiles {
			if profile.ID == id && profile.WorkspaceID == "" {
				return true
			}
		}
		return false
	}
	switch scenario {
	case "create", "duplicate":
		ids, err := globalProfileOrderIDs(ctx, repo, aID)
		if err != nil || len(ids) != 3 || ids[0] == sourceID {
			t.Fatalf("new profile membership/order = %v, err %v; want three profiles with new profile first", ids, err)
		}
	case "soft-delete", "global-to-workspace":
		if contains(profilesA, sourceID) {
			t.Fatalf("profile %s remains in agent %s global membership", sourceID, aID)
		}
	case "global-owner-a-to-b":
		if contains(profilesA, sourceID) || !contains(profilesB, sourceID) {
			t.Fatalf("profile ownership: agent A contains=%t, agent B contains=%t", contains(profilesA, sourceID), contains(profilesB, sourceID))
		}
	case "workspace-to-global":
		if !contains(profilesA, sourceID) {
			t.Fatal("promoted workspace profile is absent from global membership")
		}
	}
}
