package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

// sourceProfile returns a fully-populated kanban-flavour profile so every
// copy assertion has a concrete value to compare against.
func sourceProfile() *models.AgentProfile {
	failureThreshold := 4
	lastRun := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	return &models.AgentProfile{
		ID:               "source-1",
		AgentID:          "agent-1",
		Name:             "Default",
		AgentDisplayName: "Claude Code",
		Model:            "claude-sonnet",
		FallbackModel:    "claude-haiku",
		AutoFallback:     true,
		Mode:             "plan",
		ConfigOptions:    map[string]string{"effort": "high"},
		AllowIndexing:    true,
		AutoApprove:      true,
		CLIPassthrough:   true,
		CLIFlags: []models.CLIFlag{
			{Description: "Allow all tools", Flag: "--allow-all-tools", Enabled: true},
			{Description: "Custom", Flag: "--my-flag value", Enabled: false},
		},
		EnvVars: []models.ProfileEnvVar{
			{Key: "FOO", Value: "bar"},
			{Key: "TOKEN", SecretID: "sec-1"},
		},
		CommandPrefix: "greywall --",
		UserModified:  true,
		Enabled:       true,
		// Office enrichment configuration — must be copied.
		WorkspaceID:           "ws-1",
		Role:                  "worker",
		Icon:                  "🤖",
		ReportsTo:             "profile-other",
		SkillIDs:              `["s1","s2"]`,
		DesiredSkills:         `["ds1"]`,
		MaxConcurrentSessions: 2,
		CooldownSec:           60,
		SkipIdleRuns:          true,
		FailureThreshold:      &failureThreshold,
		ExecutorPreference:    "exec-local",
		BudgetMonthlyCents:    12500,
		Settings:              `{"office_theme":"dark"}`,
		Permissions:           `{"spawn_subagents":true}`,
		// Runtime state — must NOT be copied.
		Status:              "working",
		PauseReason:         "auto-paused",
		LastRunFinishedAt:   &lastRun,
		ConsecutiveFailures: 3,
	}
}

func duplicateSetup(source *models.AgentProfile) (*Controller, *fakeStore) {
	ctrl := newTestController(nil)
	st := newFakeStore()
	st.agents[source.AgentID] = &models.Agent{ID: source.AgentID, Name: "test-agent"}
	st.profiles[source.AgentID] = []*models.AgentProfile{source}
	ctrl.repo = st
	return ctrl, st
}

func TestDuplicateProfile_CopiesFullConfiguration(t *testing.T) {
	source := sourceProfile()
	ctrl, st := duplicateSetup(source)

	result, err := ctrl.DuplicateProfile(context.Background(), DuplicateProfileRequest{ID: source.ID})
	if err != nil {
		t.Fatalf("DuplicateProfile: %v", err)
	}

	if result.ID == source.ID || result.ID == "" {
		t.Fatalf("copy ID = %q, want a fresh non-empty ID", result.ID)
	}
	if result.Name != "Default Copy" {
		t.Errorf("copy name = %q, want %q", result.Name, "Default Copy")
	}
	if result.AgentID != source.AgentID {
		t.Errorf("copy agent_id = %q, want %q", result.AgentID, source.AgentID)
	}
	if result.AgentDisplayName != source.AgentDisplayName {
		t.Errorf("copy display name = %q, want %q", result.AgentDisplayName, source.AgentDisplayName)
	}
	if result.Model != source.Model || result.FallbackModel != source.FallbackModel || !result.AutoFallback {
		t.Errorf("copy model fields = (%q, %q, %v), want (%q, %q, true)",
			result.Model, result.FallbackModel, result.AutoFallback, source.Model, source.FallbackModel)
	}
	if result.Mode != source.Mode {
		t.Errorf("copy mode = %q, want %q", result.Mode, source.Mode)
	}
	if len(result.ConfigOptions) != 1 || result.ConfigOptions["effort"] != "high" {
		t.Errorf("copy config options = %+v, want effort=high", result.ConfigOptions)
	}
	if !result.AllowIndexing || !result.AutoApprove || !result.CLIPassthrough {
		t.Errorf("copy boolean config not preserved: allow_indexing=%v auto_approve=%v cli_passthrough=%v",
			result.AllowIndexing, result.AutoApprove, result.CLIPassthrough)
	}
	if !result.UserModified {
		t.Error("copy user_modified = false, want true")
	}
	if !result.Enabled {
		t.Error("copy enabled = false, want true (source enabled)")
	}
	if result.CommandPrefix != "greywall --" {
		t.Errorf("copy command prefix = %q, want %q", result.CommandPrefix, "greywall --")
	}
	if len(result.CLIFlags) != 2 ||
		result.CLIFlags[0].Flag != "--allow-all-tools" || !result.CLIFlags[0].Enabled ||
		result.CLIFlags[1].Flag != "--my-flag value" || result.CLIFlags[1].Enabled {
		t.Errorf("copy cli flags = %+v, want both source entries with their enabled states", result.CLIFlags)
	}
	if len(result.EnvVars) != 2 ||
		result.EnvVars[0].Key != "FOO" || result.EnvVars[0].Value != "bar" || result.EnvVars[0].SecretID != "" ||
		result.EnvVars[1].Key != "TOKEN" || result.EnvVars[1].SecretID != "sec-1" {
		t.Errorf("copy env vars = %+v, want both source entries incl. secret ref", result.EnvVars)
	}

	if len(st.created) != 1 {
		t.Fatalf("stored copies = %d, want 1", len(st.created))
	}
	stored := st.created[0]
	if stored.ID != result.ID {
		t.Errorf("stored copy ID = %q, response ID = %q", stored.ID, result.ID)
	}
	// Office enrichment configuration copied; runtime state reset.
	if stored.WorkspaceID != "ws-1" || stored.Role != "worker" || stored.Icon != "🤖" ||
		stored.ReportsTo != "profile-other" || stored.SkillIDs != `["s1","s2"]` ||
		stored.DesiredSkills != `["ds1"]` || stored.MaxConcurrentSessions != 2 ||
		stored.CooldownSec != 60 || !stored.SkipIdleRuns ||
		stored.FailureThreshold == nil || *stored.FailureThreshold != 4 ||
		stored.ExecutorPreference != "exec-local" || stored.BudgetMonthlyCents != 12500 ||
		stored.Settings != `{"office_theme":"dark"}` || stored.Permissions != `{"spawn_subagents":true}` {
		t.Errorf("stored office enrichment not copied: %+v", stored)
	}
	if stored.Status != "" || stored.PauseReason != "" || stored.LastRunFinishedAt != nil || stored.ConsecutiveFailures != 0 {
		t.Errorf("runtime state must not be copied: status=%q pause=%q last_run=%v failures=%d",
			stored.Status, stored.PauseReason, stored.LastRunFinishedAt, stored.ConsecutiveFailures)
	}
	if stored.MigratedFrom != "" || stored.CustomPrompt != "" {
		t.Errorf("legacy fields must not be copied: migrated_from=%q custom_prompt=%q",
			stored.MigratedFrom, stored.CustomPrompt)
	}
}

func TestDuplicateProfile_CopiesDisabledState(t *testing.T) {
	source := sourceProfile()
	source.Enabled = false
	ctrl, st := duplicateSetup(source)

	result, err := ctrl.DuplicateProfile(context.Background(), DuplicateProfileRequest{ID: source.ID})
	if err != nil {
		t.Fatalf("DuplicateProfile: %v", err)
	}
	if result.Enabled {
		t.Error("copy enabled = true, want false (source disabled)")
	}
	if len(st.created) != 1 || st.created[0].Enabled {
		t.Errorf("stored copy enabled = %v, want false", st.created[0].Enabled)
	}
}

func TestDuplicateProfile_CopiesMcpConfig(t *testing.T) {
	source := sourceProfile()
	ctrl, st := duplicateSetup(source)
	st.mcpConfigs[source.ID] = &models.AgentProfileMcpConfig{
		ProfileID: source.ID,
		Enabled:   true,
		Servers: map[string]interface{}{
			"github": map[string]interface{}{"url": "https://api.github.com", "enabled": true},
		},
		Meta: map[string]interface{}{"source": "user"},
	}

	result, err := ctrl.DuplicateProfile(context.Background(), DuplicateProfileRequest{ID: source.ID})
	if err != nil {
		t.Fatalf("DuplicateProfile: %v", err)
	}

	copied, ok := st.mcpConfigs[result.ID]
	if !ok {
		t.Fatalf("no mcp config row for copy %q", result.ID)
	}
	if !copied.Enabled {
		t.Error("copied mcp config enabled = false, want true")
	}
	servers, ok := copied.Servers["github"].(map[string]interface{})
	if !ok || servers["url"] != "https://api.github.com" || servers["enabled"] != true {
		t.Errorf("copied mcp servers = %+v, want github entry preserved", copied.Servers)
	}
	if copied.Meta["source"] != "user" {
		t.Errorf("copied mcp meta = %+v, want source=user", copied.Meta)
	}
	// The source row must be untouched.
	if _, ok := st.mcpConfigs[source.ID]; !ok {
		t.Error("source mcp config row disappeared")
	}
}

func TestDuplicateProfile_NoMcpConfigLeavesCopyWithoutRow(t *testing.T) {
	source := sourceProfile()
	ctrl, st := duplicateSetup(source)

	if _, err := ctrl.DuplicateProfile(context.Background(), DuplicateProfileRequest{ID: source.ID}); err != nil {
		t.Fatalf("DuplicateProfile: %v", err)
	}
	if len(st.mcpConfigs) != 0 {
		t.Errorf("copy created an mcp config row when the source had none: %+v", st.mcpConfigs)
	}
}

func TestDuplicateProfile_NotFound(t *testing.T) {
	ctrl, _ := duplicateSetup(sourceProfile())

	_, err := ctrl.DuplicateProfile(context.Background(), DuplicateProfileRequest{ID: "missing"})
	if !errors.Is(err, ErrAgentProfileNotFound) {
		t.Fatalf("err = %v, want ErrAgentProfileNotFound", err)
	}
}

func TestDuplicateProfile_NameSuffixFromEmptySourceName(t *testing.T) {
	source := sourceProfile()
	source.Name = ""
	ctrl, st := duplicateSetup(source)

	result, err := ctrl.DuplicateProfile(context.Background(), DuplicateProfileRequest{ID: source.ID})
	if err != nil {
		t.Fatalf("DuplicateProfile: %v", err)
	}
	if result.Name != " Copy" {
		t.Errorf("copy name = %q, want %q", result.Name, " Copy")
	}
	_ = st
}
