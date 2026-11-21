package lifecycle

import (
	"context"
	"errors"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func launchCatalogExecution(t *testing.T, manager *Manager, profileID, previousID string) *AgentExecution {
	t.Helper()
	ctx := context.Background()
	execution, err := manager.Launch(ctx, &LaunchRequest{
		TaskID: "task-current-inputs", SessionID: "session-current-inputs",
		AgentProfileID: profileID, ExecutorType: "local",
		PreviousExecutionID: previousID, ACPSessionID: "native-retained",
		EnvironmentResolutionRequired: true,
	})
	require.NoError(t, err)
	require.NoError(t, manager.executionStore.WithLock(execution.ID, func(current *AgentExecution) {
		current.setSessionInitialized(true)
		current.Status = v1.AgentStatusReady
	}))
	return execution
}

// @covers AC-AGENTS-SESSION-CATALOG-001.7
// @covers AC-AGENTS-SESSION-CATALOG-002.3
func TestSessionCatalogRefreshUsesCurrentLaunchSettings(t *testing.T) {
	ctx := context.Background()
	profile := &AgentProfileInfo{
		ProfileID: "profile-current-inputs", AgentName: "auggie",
		CLIFlags:      []settingsmodels.CLIFlag{{Flag: "--catalog before", Enabled: true}},
		CommandPrefix: `"/before wrapper" --`,
		EnvVars:       []settingsmodels.ProfileEnvVar{{Key: "ACCOUNT_SCOPE", Value: "before"}},
	}
	resolver := &countingProfileResolver{info: profile}
	manager, _ := newEnvironmentExecutionTestManagerWithProfileResolver(t, nil, resolver)
	first := launchCatalogExecution(t, manager, profile.ProfileID, "")
	before, err := manager.CurrentCatalogSavedSource(ctx, profile.ProfileID)
	require.NoError(t, err)
	initial, err := manager.CatalogRefreshExecution(ctx, first.SessionID, before)
	require.NoError(t, err)
	require.True(t, initial.SavedSourceCurrent, "a process launched under the current saved source can be probed")

	for name, edit := range map[string]func(*AgentProfileInfo){
		"environment value": func(p *AgentProfileInfo) {
			p.EnvVars = []settingsmodels.ProfileEnvVar{{Key: "ACCOUNT_SCOPE", Value: "after"}}
		},
		"secret binding": func(p *AgentProfileInfo) {
			p.ProviderAPIKeySecretID = "rebound-secret"
		},
		"CLI flags": func(p *AgentProfileInfo) {
			p.CLIFlags = []settingsmodels.CLIFlag{{Flag: "--catalog after", Enabled: true}}
		},
		"wrapper": func(p *AgentProfileInfo) { p.CommandPrefix = `"/after wrapper" --` },
		"provider binding": func(p *AgentProfileInfo) {
			p.ProviderKind, p.ProviderBaseURL = "openai-compatible", "https://provider.invalid"
		},
	} {
		t.Run(name, func(t *testing.T) {
			edited := *profile
			edit(&edited)
			resolver.info = &edited
			t.Cleanup(func() { resolver.info = profile })
			after, err := manager.CurrentCatalogSavedSource(ctx, profile.ProfileID)
			require.NoError(t, err)
			stale, err := manager.CatalogRefreshExecution(ctx, first.SessionID, after)
			require.NoError(t, err)
			require.NotEqual(t, before, after)
			require.False(t, stale.SavedSourceCurrent, "saving inputs cannot rewrite the initialized process")
		})
	}

	edited := *profile
	edited.CLIFlags = []settingsmodels.CLIFlag{{Flag: "--catalog after", Enabled: true}}
	edited.CommandPrefix = `"/after wrapper" --`
	edited.EnvVars = []settingsmodels.ProfileEnvVar{{Key: "ACCOUNT_SCOPE", Value: "after"}}
	resolver.info = &edited
	after, err := manager.CurrentCatalogSavedSource(ctx, profile.ProfileID)
	require.NoError(t, err)
	require.NoError(t, manager.StopAgent(ctx, first.ID, false))
	resumed := launchCatalogExecution(t, manager, profile.ProfileID, first.ID)
	current, err := manager.CatalogRefreshExecution(ctx, resumed.SessionID, after)
	require.NoError(t, err)
	require.True(t, current.SavedSourceCurrent, "ordinary resume must use current environment, flags and wrapper")
	historical, err := manager.CatalogRefreshExecution(ctx, resumed.SessionID, before)
	require.NoError(t, err)
	require.False(t, historical.SavedSourceCurrent, "historical launch inputs must not govern the resumed process")

	manager.executionStore.UpdateStatus(resumed.ID, v1.AgentStatusStopped)
	stopped, err := manager.CatalogRefreshExecution(ctx, resumed.SessionID, after)
	require.NoError(t, err)
	require.False(t, stopped.Initialized, "a stopped process cannot receive a live catalog publication")
	require.False(t, stopped.SavedSourceCurrent)
}

// @covers AC-AGENTS-SESSION-CATALOG-001.7
func TestSessionCatalogTaskDiscoveryRequiresCurrentRuntimeEvidence(t *testing.T) {
	ctx := context.Background()
	profile := &AgentProfileInfo{ProfileID: "profile-probe-owner", AgentName: "auggie"}
	manager, _ := newEnvironmentExecutionTestManagerWithProfileResolver(t, nil, &countingProfileResolver{info: profile})
	execution := launchCatalogExecution(t, manager, profile.ProfileID, "")
	evidence, err := manager.CatalogRefreshExecution(ctx, execution.SessionID, "")
	require.NoError(t, err)

	stale := evidence.Owner
	stale.AttemptID = "replaced-attempt"
	_, err = manager.DiscoverTaskCatalogCandidates(ctx, execution.SessionID, stale)
	require.True(t, errors.Is(err, sessioncatalog.ErrTaskProbeOwner), "a stale owner must not reach agentctl: %v", err)

	_, err = manager.DiscoverTaskCatalogCandidates(ctx, execution.SessionID, evidence.Owner)
	require.True(t, errors.Is(err, sessioncatalog.ErrTaskProbeUnavailable), "absent handshake epoch evidence is unavailable, not a guessed owner: %v", err)
}
