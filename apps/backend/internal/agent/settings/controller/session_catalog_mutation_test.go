package controller

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/stretchr/testify/require"
)

type catalogMutatingStore struct {
	store.Repository
	beforeMutation func()
}

func (s *catalogMutatingStore) beforeCatalogMutation() {
	if s.beforeMutation != nil {
		before := s.beforeMutation
		s.beforeMutation = nil
		before()
	}
}

func (s *catalogMutatingStore) UpdateAgentProfileWithEnabledIntent(ctx context.Context, profile *models.AgentProfile, enabled *bool) error {
	s.beforeCatalogMutation()
	return s.Repository.UpdateAgentProfileWithEnabledIntent(ctx, profile, enabled)
}

func (s *catalogMutatingStore) UpdateAgentProfileWithSelectionIntent(ctx context.Context, profile *models.AgentProfile, enabled *bool, selection *[]string) error {
	s.beforeCatalogMutation()
	return s.Repository.(store.AgentProfileSelectionIntent).UpdateAgentProfileWithSelectionIntent(ctx, profile, enabled, selection)
}

func (s *catalogMutatingStore) UpdateAgentProfileEnabled(ctx context.Context, id string, enabled bool) (time.Time, error) {
	s.beforeCatalogMutation()
	return s.Repository.UpdateAgentProfileEnabled(ctx, id, enabled)
}

func (s *catalogMutatingStore) DeleteAgentProfile(ctx context.Context, id string) error {
	s.beforeCatalogMutation()
	return s.Repository.DeleteAgentProfile(ctx, id)
}

func TestSavedProfileCatalogRefreshRejectsInFlightSavedMutation(t *testing.T) {
	for _, mutation := range []string{"wrapper", "enabled", "delete"} {
		t.Run(mutation, func(t *testing.T) {
			ctrl, repo, ctx, agentID, _ := newProviderTestController(t)
			profile := &models.AgentProfile{AgentID: agentID, Name: "Saved", Model: "saved-model", Enabled: true}
			require.NoError(t, repo.CreateAgentProfile(ctx, profile))
			changing := &catalogMutatingStore{Repository: repo}
			ctrl.repo = changing
			ctrl.hostUtility = &recordingProfileDiscoveryUtility{}
			ctrl.SetHostCLIRunner(&stubCLIRunner{listJSON: codexListJSON})
			published := false
			ctrl.SetCatalogRefreshPublisher(func(ctx context.Context, _, _ string, commit func(context.Context, func() error) error) error {
				return commit(ctx, func() error { published = true; return nil })
			})
			var observedStatus string
			changing.beforeMutation = func() {
				result, err := ctrl.RefreshProfileModelCatalog(ctx, profile.ID, "user-1")
				require.NoError(t, err)
				require.NotNil(t, result.Publication)
				observedStatus = result.Publication.Status
			}
			switch mutation {
			case "wrapper":
				prefix := "wrapper --"
				_, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: profile.ID, CommandPrefix: &prefix})
				require.NoError(t, err)
			case "enabled":
				enabled := false
				_, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: profile.ID, Enabled: &enabled})
				require.NoError(t, err)
			case "delete":
				_, err := ctrl.DeleteProfile(ctx, profile.ID, false)
				require.NoError(t, err)
			}
			require.Equal(t, "failed", observedStatus, "a snapshot read while its saved mutation is in flight cannot publish")
			require.False(t, published, "publication must not race the saved context's pending commit")
		})
	}
}
