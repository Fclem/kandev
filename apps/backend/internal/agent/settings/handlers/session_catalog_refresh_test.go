package handlers

import (
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

// @covers AC-AGENTS-SESSION-CATALOG-001.2
// @covers AC-AGENTS-SESSION-CATALOG-001.3
func TestSavedProfileCatalogRefresh(t *testing.T) {
	repo := newFakeSettingsRepo()
	seedAgent(repo, "agent-1", "devin-acp", false)
	profile := seedProfile(repo, "profile-1", "agent-1", "Catalog profile", "")
	profile.Model = "saved-model"
	router, _, reg := newSettingsHarness(t, repo, nil)
	if err := reg.Register(agents.NewDevinACP()); err != nil {
		t.Fatal(err)
	}
	response := doSettingsRequest(router, http.MethodPost, "/api/v1/agent-profiles/profile-1/model-catalog/refresh", "")
	if response.Code != http.StatusOK {
		t.Fatalf("saved refresh status = %d, body = %s; want discovery result", response.Code, response.Body.String())
	}
	var result dto.DynamicModelsResponse
	decodeSettingsJSON(t, response, &result)
	if result.AgentName != "devin-acp" || result.Status != "not_configured" {
		t.Fatalf("missing utility must remain explicitly unconfigured: %+v", result)
	}
	if profile.Model != "saved-model" || len(repo.updatedProfiles) != 0 {
		t.Fatal("refresh changed the saved selection")
	}
}
