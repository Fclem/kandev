package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type catalogExecutionReader interface {
	CatalogRefreshExecution(context.Context, string, sessioncatalog.LaunchContext) (sessioncatalog.ExecutionEvidence, error)
}

// @covers AC-AGENTS-SESSION-CATALOG-001.5
func TestSessionCatalogRefreshLaunchCompatibility(t *testing.T) {
	ctx := context.Background()
	manager := newTestManager(t)
	reader, ok := any(manager).(catalogExecutionReader)
	if !ok {
		t.Fatal("saved refresh cannot attest the initialized execution's actual launch inputs")
	}
	agent := agents.NewClaudeACP()
	profile := &AgentProfileInfo{ProfileID: "profile", AgentName: agent.ID(), Model: "chosen", CLIFlags: []settingsmodels.CLIFlag{{Flag: "--catalog saved", Enabled: true}}, EnvVars: []settingsmodels.ProfileEnvVar{{Key: "ACCOUNT_SCOPE", Value: "saved-account"}}}
	request := &LaunchRequest{TaskID: "task", SessionID: "session", AgentProfileID: "profile", ExecutorType: "local"}
	env, err := manager.resolveStrictEnvironment(ctx, "execution", request, agent, profile)
	require.NoError(t, err)
	create := &ExecutorCreateRequest{InstanceID: "execution", TaskID: "task", SessionID: "session", AgentProfileID: "profile", AgentConfig: agent, Env: env, ManagedRuntimeInstallerEnvDigest: request.ManagedRuntimeInstallerEnvDigest}
	execution, err := manager.buildExecutionFromInstance(ctx, request, create, &ExecutorInstance{InstanceID: "execution", SessionID: "session"}, &MockExecutor{name: executor.NameStandalone}, profile, agent, nil)
	require.NoError(t, err)
	execution.ExecutorType = "local"
	execution.beginStartupAttemptWithID("attempt")
	execution.setSessionInitialized(true)
	execution.Status = v1.AgentStatusReady
	require.NoError(t, manager.executionStore.Add(execution))
	expected := sessioncatalog.LaunchContext{Env: map[string]string{"ACCOUNT_SCOPE": "saved-account"}, CLIFlags: []string{"--catalog", "saved"}}
	for key, value := range agents.RuntimeEnvFor(agent) {
		expected.Env[key] = value
	}
	evidence, err := reader.CatalogRefreshExecution(ctx, "session", expected)
	require.NoError(t, err)
	require.True(t, evidence.Compatible, "matching recorded launch must allow host candidates")
	require.Equal(t, "profile", evidence.ProfileID)
	require.Equal(t, "local", evidence.ExecutorType)
	require.Equal(t, "execution", evidence.Owner.ExecutionID)
	require.Equal(t, "attempt", evidence.Owner.AttemptID)
	expected.Env["ACCOUNT_SCOPE"] = "different-account"
	evidence, err = reader.CatalogRefreshExecution(ctx, "session", expected)
	require.NoError(t, err)
	require.False(t, evidence.Compatible, "same host/profile with another credential context is notice-only")
	expected.Env["ACCOUNT_SCOPE"] = "saved-account"
	expected.CLIFlags = []string{"--catalog", "edited"}
	evidence, err = reader.CatalogRefreshExecution(ctx, "session", expected)
	require.NoError(t, err)
	require.False(t, evidence.Compatible, "editing the saved launch does not rewrite running inputs")
	expected.CLIFlags = []string{"--catalog", "saved"}
	manager.executionStore.UpdateStatus(execution.ID, v1.AgentStatusStopped)
	evidence, err = reader.CatalogRefreshExecution(ctx, "session", expected)
	require.NoError(t, err)
	require.False(t, evidence.Initialized, "an initialized but stopped process cannot receive a live catalog publication")
	require.False(t, evidence.Compatible)
}

// @covers AC-AGENTS-SESSION-CATALOG-002.3, AC-AGENTS-SESSION-CATALOG-002.4
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
	agent, exists := manager.registry.Get("auggie")
	require.True(t, exists)
	inference, supportsInference := agent.(agents.InferenceAgent)
	require.True(t, supportsInference)
	expected := func(account, wrapper string) sessioncatalog.LaunchContext {
		result := sessioncatalog.LaunchContext{
			Env:      map[string]string{"ACCOUNT_SCOPE": account},
			CLIFlags: []string{"--catalog", account}, CommandPrefix: []string{wrapper, "--"},
		}
		for key, value := range agents.RuntimeEnvFor(inference) {
			result.Env[key] = value
		}
		return result
	}
	launch := func(previousID string) *AgentExecution {
		execution, err := manager.Launch(ctx, &LaunchRequest{
			TaskID: "task-current-inputs", SessionID: "session-current-inputs",
			AgentProfileID: profile.ProfileID, ExecutorType: "local",
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
	before := expected("before", "/before wrapper")
	after := expected("after", "/after wrapper")
	first := launch("")
	initial, err := manager.CatalogRefreshExecution(ctx, first.SessionID, before)
	require.NoError(t, err)
	require.True(t, initial.Compatible)
	edited := *profile
	edited.CLIFlags = []settingsmodels.CLIFlag{{Flag: "--catalog after", Enabled: true}}
	edited.CommandPrefix = `"/after wrapper" --`
	edited.EnvVars = []settingsmodels.ProfileEnvVar{{Key: "ACCOUNT_SCOPE", Value: "after"}}
	resolver.info = &edited
	stale, err := manager.CatalogRefreshExecution(ctx, first.SessionID, after)
	require.NoError(t, err)
	require.False(t, stale.Compatible, "saving inputs cannot rewrite the initialized process")
	require.NoError(t, manager.StopAgent(ctx, first.ID, false))
	resumed := launch(first.ID)
	current, err := manager.CatalogRefreshExecution(ctx, resumed.SessionID, after)
	require.NoError(t, err)
	require.True(t, current.Compatible, "ordinary resume must use current environment, flags and wrapper")
	historical, err := manager.CatalogRefreshExecution(ctx, resumed.SessionID, before)
	require.NoError(t, err)
	require.False(t, historical.Compatible, "historical launch inputs must not govern the resumed process")
}
