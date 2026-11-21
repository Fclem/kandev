package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"sort"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// catalogSavedSourceInput is the saved launch-input revision of one concrete
// profile: its resolved definition, flags, wrapper, plugins, provider binding
// and managed runtime selection. Its digest is an in-memory equality token; it
// is never persisted, logged or sent to a browser.
type catalogSavedSourceInput struct {
	AgentID                string                `json:"agent_id"`
	ProfileID              string                `json:"profile_id"`
	Env                    [][2]string           `json:"env"`
	CLIFlags               []string              `json:"cli_flags"`
	CommandPrefix          []string              `json:"command_prefix"`
	CopilotPlugins         []string              `json:"copilot_plugins"`
	ProviderKind           string                `json:"provider_kind"`
	ProviderBaseURL        string                `json:"provider_base_url"`
	ProviderAPIKeySecretID string                `json:"provider_api_key_secret_id"`
	Runtime                agents.CommandOptions `json:"runtime"`
}

// catalogSavedSource derives the saved-source token from the same resolvers an
// ordinary launch uses. A failed resolution yields no token, never a guess.
func (m *Manager) catalogSavedSource(ctx context.Context, profile *AgentProfileInfo, agent agents.Agent, runtime agentruntime.Runtime) (string, error) {
	if profile == nil || agent == nil || profile.ProfileID == "" {
		return "", errors.New("catalog saved source is unavailable")
	}
	flags, prefix, err := m.resolveProfileLaunchTokens(profile)
	if err != nil {
		return "", err
	}
	env, err := m.resolveAgentProfileEnvVars(ctx, profile.EnvVars)
	if err != nil {
		return "", err
	}
	options, err := m.resolveManagedRuntimeCommandOptions(ctx, runtime, agent)
	if err != nil {
		return "", err
	}
	input := catalogSavedSourceInput{
		AgentID: agent.ID(), ProfileID: profile.ProfileID, CLIFlags: flags, CommandPrefix: prefix,
		CopilotPlugins: slices.Clone(profile.CopilotSelectedPlugins), ProviderKind: profile.ProviderKind,
		ProviderBaseURL: profile.ProviderBaseURL, ProviderAPIKeySecretID: profile.ProviderAPIKeySecretID,
		Runtime: options,
	}
	for key, value := range env {
		input.Env = append(input.Env, [2]string{key, value})
	}
	sort.Slice(input.Env, func(i, j int) bool { return input.Env[i][0] < input.Env[j][0] })
	payload, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

// launchCatalogSavedSource records the token only for host processes, the
// only executions that can enter fresh task-context acquisition.
func (m *Manager) launchCatalogSavedSource(ctx context.Context, profile *AgentProfileInfo, agent agents.Agent, runtime agentruntime.Runtime) string {
	if runtime != agentruntime.RuntimeStandalone {
		return ""
	}
	source, err := m.catalogSavedSource(ctx, profile, agent, runtime)
	if err != nil {
		return ""
	}
	return source
}

// CurrentCatalogSavedSource resolves the profile's current saved-source token.
func (m *Manager) CurrentCatalogSavedSource(ctx context.Context, profileID string) (string, error) {
	if m.profileResolver == nil {
		return "", errors.New("catalog saved source is unavailable")
	}
	profile, err := m.profileResolver.ResolveProfile(ctx, profileID)
	if err != nil {
		return "", err
	}
	if profile == nil {
		return "", errors.New("catalog saved source is unavailable")
	}
	agent, err := m.resolveLaunchableAgent(profile.AgentName)
	if err != nil {
		return "", err
	}
	return m.catalogSavedSource(ctx, profile, agent, agentruntime.RuntimeStandalone)
}

// CatalogRefreshExecution reads the actual initialized execution after task
// authorization. A task launched under another or unknown saved source is not
// current; the profile's present inputs are never applied to it.
func (m *Manager) CatalogRefreshExecution(ctx context.Context, sessionID, savedSource string) (sessioncatalog.ExecutionEvidence, error) {
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
	evidence.Owner = catalogExecutionOwner(execution)
	recorded := execution.catalogSavedSource.Load()
	evidence.SavedSourceCurrent = evidence.Initialized && recorded != nil && *recorded != "" && *recorded == savedSource
	return evidence, nil
}

func catalogExecutionOwner(execution *AgentExecution) sessioncatalog.Owner {
	execution.startupLifecycleMu.Lock()
	defer execution.startupLifecycleMu.Unlock()
	attemptID, ok := execution.startupAttemptIDs[execution.startupAttemptGeneration]
	if !ok {
		attemptID = execution.ResumeAttemptID
	}
	return sessioncatalog.Owner{ExecutionID: execution.ID, AttemptID: attemptID, SourceGeneration: execution.startupAttemptGeneration}
}

// DiscoverTaskCatalogCandidates asks the execution's own authenticated
// instance for one fresh catalog in its sealed launch context. The target is
// derived from the current runtime lease, never from caller input; missing
// process or handshake evidence is unavailable, not a guessed owner.
func (m *Manager) DiscoverTaskCatalogCandidates(ctx context.Context, sessionID string, expected sessioncatalog.Owner) (sessioncatalog.TaskProbeReply, error) {
	if err := m.CheckSessionAccess(ctx, sessionID); err != nil {
		return sessioncatalog.TaskProbeReply{}, err
	}
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists || execution.ID != expected.ExecutionID || catalogExecutionOwner(execution) != expected {
		return sessioncatalog.TaskProbeReply{}, sessioncatalog.ErrTaskProbeOwner
	}
	client, nativeID, generation, source := m.catalogProbeTarget(execution)
	if client == nil {
		return sessioncatalog.TaskProbeReply{}, sessioncatalog.ErrTaskProbeUnavailable
	}
	managerID, err := client.CurrentProcessManagerID(ctx)
	if err != nil {
		return sessioncatalog.TaskProbeReply{}, err
	}
	reply, err := client.DiscoverCatalogCandidates(ctx, sessioncatalog.TaskProbeRequest{
		RequestID: uuid.NewString(), ManagerID: managerID, ProcessGeneration: generation,
		SessionID: nativeID, SessionSource: source,
	})
	if err != nil {
		return sessioncatalog.TaskProbeReply{}, err
	}
	current, stillCurrent := m.executionStore.GetBySessionID(sessionID)
	if !stillCurrent || current != execution || catalogExecutionOwner(current) != expected || client.ProcessGeneration() != generation {
		return sessioncatalog.TaskProbeReply{}, sessioncatalog.ErrTaskProbeOwner
	}
	return reply, nil
}

// catalogProbeTarget reads the current runtime lease and handshake evidence.
// The lease is released before any network work so a concurrent stop or
// client replacement is never delayed by a helper; a replaced client then
// fails the owner checks instead of being trusted.
func (m *Manager) catalogProbeTarget(execution *AgentExecution) (*agentctl.Client, string, uint64, uint64) {
	var nativeID string
	if err := m.executionStore.WithRLock(execution.ID, func(current *AgentExecution) {
		if current == execution {
			nativeID = current.ACPSessionID
		}
	}); err != nil || nativeID == "" {
		return nil, "", 0, 0
	}
	client, release := execution.AcquireAgentCtlClient()
	release()
	if client == nil {
		return nil, "", 0, 0
	}
	generation, source := client.CatalogSessionSource(nativeID)
	if generation == 0 || source == 0 {
		return nil, "", 0, 0
	}
	return client, nativeID, generation, source
}
