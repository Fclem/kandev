package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// mockStarKey is the per-workspace/user repository state key.
type mockStarKey struct {
	WorkspaceID string
	UserID      string
	OwnerRepo   string
}

// mockStarState records the star state and injected failures for E2E runs.
// Nothing here is reachable unless RegisterMockRoutes mounted the controls.
type mockStarState struct {
	mu       sync.Mutex
	states   map[mockStarKey]bool
	status   bool
	mutation bool
}

func newMockStarState() *mockStarState {
	return &mockStarState{states: map[mockStarKey]bool{}}
}

// SetPersonalStarState stores one repository's star state for a user.
func (m *MockClient) SetPersonalStarState(workspaceID, userID, ownerRepo string, starred bool) {
	m.starState.mu.Lock()
	defer m.starState.mu.Unlock()
	m.starState.states[mockStarKey{workspaceID, userID, strings.ToLower(ownerRepo)}] = starred
}

// SetPersonalStarFailures injects provider failures for status and mutation calls.
func (m *MockClient) SetPersonalStarFailures(status, mutation bool) {
	m.starState.mu.Lock()
	defer m.starState.mu.Unlock()
	m.starState.status = status
	m.starState.mutation = mutation
}

// PersonalStarClientFor returns a star client bound to one workspace/user.
func (m *MockClient) PersonalStarClientFor(workspaceID, userID string) PersonalStarClient {
	return &mockPersonalStarClient{mock: m, workspaceID: workspaceID, userID: userID}
}

type mockPersonalStarClient struct {
	mock        *MockClient
	workspaceID string
	userID      string
}

func (c *mockPersonalStarClient) IsStarred(_ context.Context, repository GitHubRepositoryRef) (bool, error) {
	c.mock.starState.mu.Lock()
	defer c.mock.starState.mu.Unlock()
	if c.mock.starState.status {
		return false, errors.New("mock personal star status failure")
	}
	return c.mock.starState.states[c.key(repository)], nil
}

func (c *mockPersonalStarClient) Star(_ context.Context, repository GitHubRepositoryRef) (bool, error) {
	c.mock.starState.mu.Lock()
	defer c.mock.starState.mu.Unlock()
	if c.mock.starState.mutation {
		return false, errors.New("mock personal star mutation failure")
	}
	c.mock.starState.states[c.key(repository)] = true
	return true, nil
}

func (c *mockPersonalStarClient) Unstar(_ context.Context, repository GitHubRepositoryRef) (bool, error) {
	c.mock.starState.mu.Lock()
	defer c.mock.starState.mu.Unlock()
	if c.mock.starState.mutation {
		return false, errors.New("mock personal star mutation failure")
	}
	c.mock.starState.states[c.key(repository)] = false
	return false, nil
}

func (c *mockPersonalStarClient) key(repository GitHubRepositoryRef) mockStarKey {
	return mockStarKey{c.workspaceID, c.userID, repository.String()}
}

// SeedPersonalStarConnectionForE2E commits a dedicated connection without an
// OAuth round trip, so E2E specs can drive the connected/capable states.
func (s *Service) SeedPersonalStarConnectionForE2E(
	ctx context.Context,
	workspaceID, userID, login string,
	publicRepo bool,
	outcome string,
) error {
	if s == nil || s.store == nil || s.connectionSecrets == nil {
		return fmt.Errorf("github service is not ready for seeded star connections")
	}
	revision, generation, err := s.store.BumpPersonalStarLifecycle(ctx, workspaceID, userID)
	if err != nil {
		return err
	}
	connection := &PersonalStarConnection{
		WorkspaceID:             workspaceID,
		UserID:                  userID,
		Login:                   login,
		GitHubUserID:            1,
		GrantedScopes:           PersonalStarScope,
		PublicRepo:              publicRepo,
		CredentialGeneration:    generation,
		LifecycleRevision:       revision,
		AccessSecretID:          personalStarAccessSecretID(workspaceID, userID, generation),
		RefreshSecretID:         personalStarRefreshSecretID(workspaceID, userID, generation),
		AccessSecretGeneration:  generation,
		RefreshSecretGeneration: generation,
		LastReconnectOutcome:    outcome,
	}
	if err := s.connectionSecrets.Set(ctx, connection.AccessSecretID, "github personal star access token", "mock-star-token"); err != nil {
		return err
	}
	existing, err := s.store.GetPersonalStarConnection(ctx, workspaceID, userID)
	if err != nil {
		return err
	}
	if existing == nil {
		return s.store.InsertPersonalStarConnection(ctx, connection, revision, generation)
	}
	_, err = s.store.ReplacePersonalStarConnection(ctx, connection, revision, generation)
	return err
}

// SetPersonalStarReconnectOutcomeForE2E records a callback outcome without an
// OAuth exchange and applies the capability it implies.
func (s *Service) SetPersonalStarReconnectOutcomeForE2E(
	ctx context.Context,
	workspaceID, userID, outcome string,
) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("github service is not ready for seeded star connections")
	}
	existing, err := s.store.GetPersonalStarConnection(ctx, workspaceID, userID)
	if err != nil {
		return err
	}
	if existing == nil {
		return nil
	}
	if outcome == personalStarOutcomeInsufficientScope {
		if _, err := s.store.db.ExecContext(ctx, s.store.db.Rebind(`
			UPDATE github_personal_star_connection SET public_repo = 0, updated_at = ?
			WHERE workspace_id = ? AND user_id = ?`), time.Now().UTC(), workspaceID, userID); err != nil {
			return err
		}
	}
	return s.store.SetPersonalStarReconnectOutcome(ctx, workspaceID, userID, outcome)
}

// ClearPersonalStarConnectionForE2E removes the dedicated connection and its
// generation-specific secrets.
func (s *Service) ClearPersonalStarConnectionForE2E(ctx context.Context, workspaceID, userID string) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("github service is not ready for seeded star connections")
	}
	secretIDs, err := s.store.DisconnectPersonalStarConnection(ctx, workspaceID, userID)
	if err != nil {
		return err
	}
	if s.connectionSecrets != nil {
		for _, secretID := range secretIDs {
			if err := s.connectionSecrets.Delete(ctx, secretID); err != nil {
				return err
			}
		}
	}
	return nil
}
