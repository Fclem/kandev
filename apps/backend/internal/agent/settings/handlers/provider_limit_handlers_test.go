package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/providerlimit"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.8
func TestProviderLimitsAPIHidesPrivateProfilesAndTracksClearExpiry(t *testing.T) {
	repo := newFakeSettingsRepo()
	seedAgent(repo, "saved-omp", "omp-acp", false)
	first := seedProfile(repo, "first", "saved-omp", "First", "")
	first.Model = "anthropic/opus"
	second := seedProfile(repo, "second", "saved-omp", "Second", "")
	second.Model = "anthropic/sonnet"
	other := seedProfile(repo, "other", "saved-omp", "Other", "")
	other.Model = "openai-codex/gpt-6.1-sol"
	private := seedProfile(repo, "private-office", "saved-omp", "Private", "workspace-private")
	private.Model = "anthropic/opus"
	hub := &duplicateHub{}
	router, controller, catalog := newSettingsHarness(t, repo, hub)
	if err := catalog.Register(agents.NewOmpACP()); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	persist := &profileLimitPersistence{rows: make(map[string]dynamic.CircuitSnapshot)}
	limits := providerlimit.NewService(dynamic.NewCircuitRegistry(dynamic.WithCircuitPersistence(persist), dynamic.WithCircuitClock(func() time.Time { return now })), dynamic.NewCredentialBindingResolver([]byte("profile-limit-installation")), repo, catalog, providerlimit.WithClock(func() time.Time { return now }), providerlimit.WithOnChange(func(ctx context.Context) { controller.BroadcastProfileLimits(ctx) }))
	controller.SetProviderLimitService(limits)
	reset := now.Add(time.Hour)
	subject := limits.ForProfile(first, agents.NewOmpACP())
	if _, err := limits.Record(context.Background(), subject, first.Model, &routingerr.Error{Code: routingerr.CodeQuotaLimited, LimitScope: "account", ResetHint: &reset}, now); err != nil {
		t.Fatal(err)
	}
	response := doSettingsRequest(router, http.MethodGet, "/api/v1/agent-profiles/limits", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var rows []providerlimit.ProfileLimit
	if err := json.Unmarshal(response.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]providerlimit.ProfileLimit)
	for _, row := range rows {
		byID[row.ProfileID] = row
	}
	for _, profile := range []*settingsmodels.AgentProfile{first, second} {
		row, exists := byID[profile.ID]
		if !exists || row.Model != profile.Model || row.Scope != "account" || !row.ResetKnown || !row.Until.Equal(reset) {
			t.Fatalf("profile=%s limit=%+v exists=%t", profile.ID, row, exists)
		}
	}
	for _, id := range []string{other.ID, private.ID} {
		if _, leaked := byID[id]; leaked {
			t.Fatalf("ineligible or private profile %s leaked", id)
		}
	}
	for _, value := range []string{"limit|", "BindingKey", "probe_until", "workspace-private"} {
		if strings.Contains(response.Body.String(), value) {
			t.Fatalf("internal state leaked: %s", response.Body.String())
		}
	}
	payloads := hub.payloads(ws.ActionAgentProfileLimitsUpdated)
	if len(payloads) != 1 {
		t.Fatalf("accepted mark notifications=%d", len(payloads))
	}
	encoded, err := json.Marshal(payloads[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), private.ID) {
		t.Fatalf("global notification leaked private profile: %s", encoded)
	}
	now = reset
	response = doSettingsRequest(router, http.MethodGet, "/api/v1/agent-profiles/limits", "")
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "[]" {
		t.Fatalf("expired marks remained visible: %d %s", response.Code, response.Body.String())
	}
	if _, err := limits.ClearOnSuccess(context.Background(), subject, first.Model); err != nil {
		t.Fatal(err)
	}
	payloads = hub.payloads(ws.ActionAgentProfileLimitsUpdated)
	if len(payloads) != 2 {
		t.Fatalf("accepted clear notifications=%d", len(payloads))
	}
}

type profileLimitPersistence struct {
	rows map[string]dynamic.CircuitSnapshot
}

func (p *profileLimitPersistence) SaveCircuit(_ context.Context, s dynamic.CircuitSnapshot) error {
	p.rows[s.Key] = s
	return nil
}
func (p *profileLimitPersistence) SaveCircuits(_ context.Context, snapshots []dynamic.CircuitSnapshot) error {
	for _, s := range snapshots {
		p.rows[s.Key] = s
	}
	return nil
}
func (p *profileLimitPersistence) LoadCircuits(context.Context) ([]dynamic.CircuitSnapshot, error) {
	var rows []dynamic.CircuitSnapshot
	for _, s := range p.rows {
		rows = append(rows, s)
	}
	return rows, nil
}
