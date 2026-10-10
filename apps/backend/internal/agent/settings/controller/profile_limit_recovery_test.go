package controller

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"testing"
)

func assertProfileLimitRecovery(t *testing.T, value any, fallback, resume bool) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["limit_fallback"] != fallback || fields["resume_after_reset"] != resume {
		t.Fatalf("recovery settings %s, want %v/%v", raw, fallback, resume)
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.4
func TestLimitRecoveryPartialUpdateAndDuplicate(t *testing.T) {
	ctrl := newTestController(map[string]agents.Agent{"test-agent": &testAgent{id: "test-agent", name: "test-agent", displayName: "Test Agent", enabled: true}})
	st := newFakeStore()
	agent := &models.Agent{ID: "agent-1", Name: "test-agent"}
	st.agents[agent.ID] = agent
	st.byName[agent.Name] = agent
	ctrl.repo = st
	var create dto.ProfileCreateRequest
	if err := json.Unmarshal([]byte(`{"agent_id":"agent-1","name":"Limits","model":"primary","fallback_model":"fallback","limit_fallback":true,"resume_after_reset":true}`), &create); err != nil {
		t.Fatal(err)
	}
	profile, err := ctrl.CreateProfile(context.Background(), CreateProfileRequestFromDTO(create))
	if err != nil {
		t.Fatal(err)
	}
	assertProfileLimitRecovery(t, profile, true, true)
	updated, err := ctrl.UpdateProfile(context.Background(), UpdateProfileRequest{ID: profile.ID, Name: new("Renamed")})
	if err != nil {
		t.Fatal(err)
	}
	assertProfileLimitRecovery(t, updated, true, true)
	stored, err := st.GetAgentProfile(context.Background(), profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertProfileLimitRecovery(t, duplicateClone(stored), true, true)
	var update dto.ProfileUpdateRequest
	if err := json.Unmarshal([]byte(`{"limit_fallback":false}`), &update); err != nil {
		t.Fatal(err)
	}
	update.ID = profile.ID
	updated, err = ctrl.UpdateProfile(context.Background(), UpdateProfileRequestFromDTO(update))
	if err != nil {
		t.Fatal(err)
	}
	assertProfileLimitRecovery(t, updated, false, true)
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.6
func TestLimitRecoveryDynamicUpdateRejected(t *testing.T) {
	for _, field := range []string{"limit_fallback", "resume_after_reset"} {
		for _, value := range []string{"true", "false"} {
			t.Run(field+"/"+value, func(t *testing.T) {
				ctrl := newTestController(nil)
				ctrl.dynamicAgentRoutingEnabled = true
				st := newFakeStore()
				ctrl.repo = st
				profile := &models.AgentProfile{ID: "dynamic", AgentID: agents.DynamicAgentID, Name: "Dynamic"}
				if err := st.CreateAgentProfile(context.Background(), profile); err != nil {
					t.Fatal(err)
				}
				var update dto.ProfileUpdateRequest
				if err := json.Unmarshal([]byte(`{"`+field+`":`+value+`}`), &update); err != nil {
					t.Fatal(err)
				}
				update.ID = profile.ID
				_, err := ctrl.UpdateProfile(context.Background(), UpdateProfileRequestFromDTO(update))
				if err == nil {
					t.Fatal("dynamic recovery write accepted")
				}
			})
		}
	}
}

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.6
func TestLimitRecoveryDynamicCreateRejected(t *testing.T) {
	ctrl, repo := newSQLiteBackedController(t)
	if err := ctrl.agentRegistry.Register(agents.NewDynamicAgent()); err != nil {
		t.Fatal(err)
	}
	ctrl.SetDynamicAgentRoutingEnabled(true)
	if err := repo.CreateAgent(context.Background(), &models.Agent{ID: agents.DynamicAgentID, Name: agents.DynamicAgentID}); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"limit_fallback", "resume_after_reset"} {
		for _, value := range []string{"true", "false"} {
			t.Run(field+"/"+value, func(t *testing.T) {
				var create dto.ProfileCreateRequest
				if err := json.Unmarshal([]byte(`{"agent_id":"dynamic","name":"Limits","`+field+`":`+value+`}`), &create); err != nil {
					t.Fatal(err)
				}
				create.AgentID = agents.DynamicAgentID
				_, err := ctrl.CreateProfile(context.Background(), CreateProfileRequestFromDTO(create))
				if !errors.Is(err, ErrDynamicProfileRule) {
					t.Fatalf("error = %v, want unsupported recovery rule", err)
				}
			})
		}
	}
}
