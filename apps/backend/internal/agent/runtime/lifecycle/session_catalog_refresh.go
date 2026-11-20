package lifecycle

import (
	"context"
	"reflect"
	"slices"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type catalogLaunchCommand struct {
	flags          []string
	prefix         []string
	runtimeOptions agents.CommandOptions
	provider       string
	plugins        []string
}

// CatalogRefreshExecution reads actual launch evidence after task authorization.
// Older/reconstructed executions without complete evidence stay notice-only.
func (m *Manager) CatalogRefreshExecution(ctx context.Context, sessionID string, expected sessioncatalog.LaunchContext) (sessioncatalog.ExecutionEvidence, error) {
	var evidence sessioncatalog.ExecutionEvidence
	if err := m.CheckSessionAccess(ctx, sessionID); err != nil {
		return evidence, err
	}
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return evidence, ErrNoExecutionForSession
	}
	if err := m.executionStore.WithRLock(execution.ID, func(current *AgentExecution) {
		if current != execution {
			return
		}
		evidence.ProfileID = current.ExecutionProfileID
		if evidence.ProfileID == "" {
			evidence.ProfileID = current.AgentProfileID
		}
		evidence.AgentID = current.AgentID
		evidence.ExecutorType = current.ExecutorType
		evidence.Initialized = current.isSessionInitialized() && (current.Status == v1.AgentStatusRunning || current.Status == v1.AgentStatusReady)
	}); err != nil {
		return evidence, err
	}
	execution.startupLifecycleMu.Lock()
	attemptID, ok := execution.startupAttemptIDs[execution.startupAttemptGeneration]
	if !ok {
		attemptID = execution.ResumeAttemptID
	}
	evidence.Owner = sessioncatalog.Owner{ExecutionID: execution.ID, AttemptID: attemptID, SourceGeneration: execution.startupAttemptGeneration}
	execution.startupLifecycleMu.Unlock()
	command := execution.catalogCommand.Load()
	if !evidence.Initialized || !catalogLaunchCommandMatches(execution, command, &expected) {
		return evidence, nil
	}
	agent, exists := m.registry.Get(command.provider)
	if !exists {
		return evidence, nil
	}
	options, err := m.resolveManagedRuntimeCommandOptions(ctx, execution.RuntimeName, agent)
	if err != nil {
		return evidence, err
	}
	evidence.Compatible = reflect.DeepEqual(command.runtimeOptions, options)
	return evidence, nil
}

func catalogLaunchCommandMatches(execution *AgentExecution, command *catalogLaunchCommand, expected *sessioncatalog.LaunchContext) bool {
	return command != nil &&
		execution.ManagedRuntimeInstallerEnvDigest != "" &&
		execution.catalogEnvironmentRevision.Load() == execution.catalogInitializedEnvironment.Load() &&
		slices.Equal(command.flags, expected.CLIFlags) &&
		slices.Equal(command.prefix, expected.CommandPrefix) &&
		slices.Equal(command.plugins, expected.CopilotSelectedPlugins) &&
		execution.ManagedRuntimeInstallerEnvDigest == managedruntime.InstallerEnvironmentDigest(expected.Env)
}
