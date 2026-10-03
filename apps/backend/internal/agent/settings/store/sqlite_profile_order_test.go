package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

func TestListAgentProfiles_UsesSavedOrderAndNewestFirstDefault(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	agent := &models.Agent{Name: "profile-order"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatal(err)
	}
	profiles := []*models.AgentProfile{
		{AgentID: agent.ID, Name: "older", Model: "model"},
		{AgentID: agent.ID, Name: "newer", Model: "model"},
	}
	for _, profile := range profiles {
		if err := repo.CreateAgentProfile(ctx, profile); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.ListAgentProfiles(ctx, agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != profiles[1].ID {
		t.Fatalf("default ordering = %#v, want newest profile first", got)
	}
	updatedAtByID := map[string]time.Time{got[0].ID: got[0].UpdatedAt, got[1].ID: got[1].UpdatedAt}
	revision, changed, err := repo.(*sqliteRepository).ReorderAgentProfiles(ctx, agent.ID, []string{profiles[0].ID, profiles[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	if !changed || revision != 1 {
		t.Fatalf("reorder = revision %d, changed %t, want revision 1 changed", revision, changed)
	}
	got, err = repo.ListAgentProfiles(ctx, agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != profiles[0].ID || got[1].ID != profiles[1].ID {
		t.Fatalf("saved ordering = %#v, want older then newer", got)
	}
	for _, profile := range got {
		if !profile.UpdatedAt.Equal(updatedAtByID[profile.ID]) {
			t.Fatalf("profile %s updated_at changed during reorder: %v -> %v", profile.ID, updatedAtByID[profile.ID], profile.UpdatedAt)
		}
	}
	revision, changed, err = repo.(*sqliteRepository).ReorderAgentProfiles(ctx, agent.ID, []string{profiles[0].ID, profiles[1].ID})
	if err != nil || changed || revision != 1 {
		t.Fatalf("unchanged reorder = revision %d, changed %t, err %v; want revision 1 unchanged", revision, changed, err)
	}
	if _, _, err := repo.(*sqliteRepository).ReorderAgentProfiles(ctx, agent.ID, []string{profiles[0].ID, "foreign"}); err != ErrProfileOrderSetMismatch {
		t.Fatalf("mismatched reorder error = %v, want ErrProfileOrderSetMismatch", err)
	}
	if _, _, err := repo.(*sqliteRepository).ReorderAgentProfiles(ctx, agent.ID, []string{profiles[0].ID, profiles[0].ID}); err != ErrProfileOrderSetMismatch {
		t.Fatalf("duplicate reorder error = %v, want ErrProfileOrderSetMismatch", err)
	}
}

func TestReorderAgentProfilesMissingAgentIsNotAStaleMembershipSet(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	_, _, err := repo.ReorderAgentProfiles(context.Background(), "missing-agent", nil)
	if err == nil || !strings.Contains(err.Error(), "agent not found") {
		t.Fatalf("reorder missing agent error = %v, want missing-agent error", err)
	}
}
