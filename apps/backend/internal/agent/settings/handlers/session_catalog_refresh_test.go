package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/httpmw"
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

// slowProfileRepo stands in for a saved refresh whose forced probe and
// session fan-out run longer than the server-wide write timeout.
type slowProfileRepo struct {
	*fakeSettingsRepo
}

func (r slowProfileRepo) GetAgentProfile(ctx context.Context, id string) (*models.AgentProfile, error) {
	select {
	case <-time.After(300 * time.Millisecond):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return r.fakeSettingsRepo.GetAgentProfile(ctx, id)
}

// The saved refresh must extend its write deadline, so a refresh slower than
// the server-wide write timeout still delivers its discovery result instead of
// a dropped connection that the editor would report as a failed discovery.
func TestSavedProfileCatalogRefreshOutlivesServerWriteTimeout(t *testing.T) {
	repo := newFakeSettingsRepo()
	seedAgent(repo, "agent-1", "devin-acp", false)
	seedProfile(repo, "profile-1", "agent-1", "Catalog profile", "")
	router, _, reg := newSettingsHarness(t, slowProfileRepo{repo}, nil)
	if err := reg.Register(agents.NewDevinACP()); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(router)
	server.Config.WriteTimeout = 100 * time.Millisecond
	server.Start()
	t.Cleanup(server.Close)

	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/agent-profiles/profile-1/model-catalog/refresh", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(httpmw.InterimSettingsInterlockHeader, "test-interlock")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("saved refresh lost its response: %v", err)
	}
	body, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("saved refresh = %d %q (read error %v)", response.StatusCode, body, readErr)
	}
	var result dto.DynamicModelsResponse
	if err := json.Unmarshal(body, &result); err != nil || result.AgentName != "devin-acp" {
		t.Fatalf("saved refresh body = %q (%v), want the discovery result", body, err)
	}
}
