package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/modelcatalog"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func seedCatalogEligibilitySession(t *testing.T, repo *sqliterepo.Repository) map[string]any {
	t.Helper()
	ctx := context.Background()
	seedTaskAndSession(t, repo, "catalog-task", "catalog-session", models.TaskSessionStateIdle)
	session, err := repo.GetTaskSession(ctx, "catalog-session")
	require.NoError(t, err)
	session.AgentProfileID = "routing-parent"
	session.ExecutionProfileID = "concrete-profile"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "catalog-execution")
	running, err := repo.GetExecutorRunningBySessionID(ctx, session.ID)
	require.NoError(t, err)
	running.ExecutionProfileID = "concrete-profile"
	require.NoError(t, repo.UpsertExecutorRunning(ctx, running))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, session.ID, models.SessionMetaKeyACPModelState, map[string]any{"settings_source_execution_id": "catalog-execution", "settings_source_generation": 1, "settings_attempt_id": "attempt"}))
	effective := map[string]any{"model": "selected-model", "mode": "selected-mode", "config_options": map[string]any{"reasoning": "selected-effort"}}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, session.ID, "runtime_config", effective))
	return effective
}

func newCatalogEligibilityManager(executorType string, savedSourceCurrent bool) *catalogFanoutManager {
	return &catalogFanoutManager{
		mockAgentManager: &mockAgentManager{}, probes: map[string]int{}, savedSource: "saved-source",
		evidence: map[string]sessioncatalog.ExecutionEvidence{"catalog-session": {
			Owner:     sessioncatalog.Owner{ExecutionID: "catalog-execution", AttemptID: "attempt", SourceGeneration: 1},
			ProfileID: "concrete-profile", AgentID: "codex-app-server", ExecutorType: executorType,
			Initialized: true, SavedSourceCurrent: savedSourceCurrent,
		}},
	}
}

func readCatalogObservation(t *testing.T, repo *sqliterepo.Repository, sessionID string) sessioncatalog.Observation {
	t.Helper()
	stored, err := repo.GetTaskSession(context.Background(), sessionID)
	require.NoError(t, err)
	payload, err := json.Marshal(stored.Metadata[sessioncatalog.MetadataKey])
	require.NoError(t, err)
	var observation sessioncatalog.Observation
	require.NoError(t, json.Unmarshal(payload, &observation))
	return observation
}

func subscribeCommittedCatalogEvents(t *testing.T, svc *Service, repo *sqliterepo.Repository) {
	t.Helper()
	subscription, err := svc.eventBus.Subscribe(events.BuildSessionCatalogRefreshWildcardSubject(), func(ctx context.Context, event *bus.Event) error {
		update := event.Data.(sessioncatalog.Update)
		require.Equal(t, update.Record, readCatalogObservation(t, repo, update.SessionID), "event must describe an already committed observation")
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = subscription.Unsubscribe() })
}

// @covers AC-AGENTS-SESSION-CATALOG-001.1
// @covers AC-AGENTS-SESSION-CATALOG-001.2
// @covers AC-AGENTS-SESSION-CATALOG-001.4
// @covers AC-AGENTS-SESSION-CATALOG-001.5
// @covers AC-AGENTS-SESSION-CATALOG-001.7
func TestSessionCatalogRefreshEligibility(t *testing.T) {
	cases := []struct {
		executorType string
		current      bool
		probed       bool
		reason       string
	}{
		{executorType: "local", current: true, probed: true, reason: sessioncatalog.ReasonNewCandidates},
		{executorType: "worktree", current: true, probed: true, reason: sessioncatalog.ReasonNewCandidates},
		{executorType: "local_pc", current: true, probed: true, reason: sessioncatalog.ReasonNewCandidates},
		{executorType: "local", current: false, reason: sessioncatalog.ReasonContextUnverified},
		{executorType: "", current: true, reason: sessioncatalog.ReasonContextUnverified},
		{executorType: "unknown", current: true, reason: sessioncatalog.ReasonContextUnverified},
		{executorType: "local_docker", current: true, reason: sessioncatalog.ReasonExecutorScoped},
		{executorType: "remote_docker", current: true, reason: sessioncatalog.ReasonExecutorScoped},
		{executorType: "ssh", current: true, reason: sessioncatalog.ReasonExecutorScoped},
		{executorType: "sprites", current: true, reason: sessioncatalog.ReasonExecutorScoped},
		{executorType: "k8s", current: true, reason: sessioncatalog.ReasonExecutorScoped},
		{executorType: "plugin-remote", current: true, reason: sessioncatalog.ReasonExecutorScoped},
	}
	for _, tc := range cases {
		name := tc.executorType
		if name == "" {
			name = "empty-locality"
		}
		if !tc.current {
			name += "-older-saved-source"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			effective := seedCatalogEligibilitySession(t, repo)
			manager := newCatalogEligibilityManager(tc.executorType, tc.current)
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
			svc.eventBus = bus.NewMemoryEventBus(testLogger())
			subscribeCommittedCatalogEvents(t, svc, repo)
			require.NoError(t, svc.PublishProfileCatalog(ctx, "concrete-profile", "codex-app-server", catalogFanoutGuard))
			stored, err := repo.GetTaskSession(ctx, "catalog-session")
			require.NoError(t, err)
			require.Equal(t, effective, stored.Metadata["runtime_config"])
			observation := readCatalogObservation(t, repo, "catalog-session")
			require.Equal(t, tc.reason, observation.Reason)
			if tc.probed {
				require.Equal(t, 1, manager.probeCount("catalog-session"))
				require.Equal(t, sessioncatalog.DispositionHostCandidates, observation.Disposition)
				require.Equal(t, sessioncatalog.SourceTaskProbeV1, observation.Source)
				require.True(t, observation.TaskOwnedCandidates())
				require.Equal(t, []sessioncatalog.Choice{{ID: "model-of-catalog-session", Name: "Model of catalog-session"}}, observation.Models)
			} else {
				require.Zero(t, manager.probeCount("catalog-session"), "notice-only sessions never receive an automatic task probe")
				require.Equal(t, sessioncatalog.DispositionRediscoveryRequired, observation.Disposition)
				require.False(t, observation.TaskOwnedCandidates())
				require.Empty(t, observation.Models)
				require.Empty(t, observation.Modes)
				require.Nil(t, observation.Catalog)
			}
			require.Equal(t, uint64(1), observation.Revision)
			require.NoError(t, svc.PublishProfileCatalog(ctx, "concrete-profile", "codex-app-server", catalogFanoutGuard))
			require.Equal(t, observation, readCatalogObservation(t, repo, "catalog-session"), "same pending observation must coalesce")
		})
	}
}

// @covers AC-AGENTS-SESSION-CATALOG-001.7
// @covers AC-AGENTS-SESSION-CATALOG-001.8
func TestSessionCatalogRefreshTaskProbeOutcomes(t *testing.T) {
	grouped := &modelcatalog.Catalog{Version: 1, Nodes: []modelcatalog.Node{{Kind: modelcatalog.NodeKindGroup, ID: "group", Label: "Group", Children: []modelcatalog.Node{{Kind: modelcatalog.NodeKindChoice, Value: "grouped", Label: "Grouped"}}}}}
	cases := map[string]struct {
		reply       sessioncatalog.TaskProbeReply
		err         error
		disposition string
		reason      string
		models      []sessioncatalog.Choice
		modes       []sessioncatalog.Choice
	}{
		"unsupported capability": {reply: sessioncatalog.TaskProbeReply{Result: sessioncatalog.Discovery{Outcome: sessioncatalog.OutcomeUnsupported}}, disposition: sessioncatalog.DispositionRediscoveryRequired, reason: sessioncatalog.ReasonTaskDiscoveryUnsupported},
		"failed helper":          {reply: sessioncatalog.TaskProbeReply{Result: sessioncatalog.Discovery{Outcome: sessioncatalog.OutcomeFailed}}, disposition: sessioncatalog.DispositionRediscoveryRequired, reason: sessioncatalog.ReasonTaskDiscoveryFailed},
		"unavailable instance":   {err: sessioncatalog.ErrTaskProbeUnavailable, disposition: sessioncatalog.DispositionRediscoveryRequired, reason: sessioncatalog.ReasonTaskDiscoveryFailed},
		"supported empty models": {
			reply:       sessioncatalog.TaskProbeReply{Capability: "fixture", Result: sessioncatalog.Discovery{Outcome: sessioncatalog.OutcomeComplete, Models: sessioncatalog.Surface{Present: true, Supported: true, Choices: []sessioncatalog.Choice{}}}},
			disposition: sessioncatalog.DispositionHostCandidates, reason: sessioncatalog.ReasonNewCandidates,
		},
		"models and modes": {
			reply: sessioncatalog.TaskProbeReply{Capability: "fixture", Result: sessioncatalog.Discovery{Outcome: sessioncatalog.OutcomeComplete,
				Models: sessioncatalog.Surface{Present: true, Supported: true, Choices: []sessioncatalog.Choice{{ID: "grouped", Name: "Grouped"}}, Catalog: grouped},
				Modes:  sessioncatalog.Surface{Present: true, Supported: true, Choices: []sessioncatalog.Choice{{ID: "plan", Name: "Plan"}}}}},
			disposition: sessioncatalog.DispositionHostCandidates, reason: sessioncatalog.ReasonNewCandidates,
			models: []sessioncatalog.Choice{{ID: "grouped", Name: "Grouped"}}, modes: []sessioncatalog.Choice{{ID: "plan", Name: "Plan"}},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedCatalogEligibilitySession(t, repo)
			manager := newCatalogEligibilityManager("worktree", true)
			manager.probe = func(context.Context, string) (sessioncatalog.TaskProbeReply, error) { return tc.reply, tc.err }
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
			svc.eventBus = bus.NewMemoryEventBus(testLogger())
			require.NoError(t, svc.PublishProfileCatalog(ctx, "concrete-profile", "codex-app-server", catalogFanoutGuard))
			observation := readCatalogObservation(t, repo, "catalog-session")
			require.Equal(t, tc.disposition, observation.Disposition)
			require.Equal(t, tc.reason, observation.Reason)
			require.Equal(t, tc.models, observation.Models)
			require.Equal(t, tc.modes, observation.Modes)
			require.Nil(t, observation.Result, "a task probe is not provider startup evidence")
			if tc.disposition == sessioncatalog.DispositionHostCandidates {
				require.Equal(t, &tc.reply.Result, observation.Probe, "the record retains the target's own per-surface support")
			} else {
				require.Nil(t, observation.Probe)
				require.Empty(t, observation.Source)
			}
		})
	}
}

// @covers AC-AGENTS-SESSION-CATALOG-001.7
func TestSessionCatalogRefreshReplacesLegacyGenericCandidates(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedCatalogEligibilitySession(t, repo)
	legacy := sessioncatalog.Observation{Version: 1, Revision: 4, ProfileID: "concrete-profile", Digest: "generic",
		Owner:       sessioncatalog.Owner{ExecutionID: "catalog-execution", AttemptID: "attempt", SourceGeneration: 1},
		Disposition: sessioncatalog.DispositionHostCandidates, Reason: sessioncatalog.ReasonNewCandidates,
		Models: []sessioncatalog.Choice{{ID: "generic-utility-model"}}}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "catalog-session", sessioncatalog.MetadataKey, legacy))
	manager := newCatalogEligibilityManager("worktree", true)
	manager.probe = func(context.Context, string) (sessioncatalog.TaskProbeReply, error) {
		return sessioncatalog.TaskProbeReply{}, errors.New("helper unavailable")
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.eventBus = bus.NewMemoryEventBus(testLogger())
	require.NoError(t, svc.PublishProfileCatalog(ctx, "concrete-profile", "codex-app-server", catalogFanoutGuard))
	observation := readCatalogObservation(t, repo, "catalog-session")
	require.Equal(t, uint64(5), observation.Revision, "a newer revision replaces the legacy record")
	require.Equal(t, sessioncatalog.DispositionRediscoveryRequired, observation.Disposition)
	require.Empty(t, observation.Models, "a failed task probe must not retain or transfer generic candidates")
}

// Busy means the instance's helper slot is held, not that the target's
// discovery failed: the prior task-owned record stays, nothing is retried, and
// the refresh reports the session unpublished.
// @covers AC-AGENTS-SESSION-CATALOG-001.7
func TestSessionCatalogRefreshBusyHelperKeepsPriorRecord(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedCatalogEligibilitySession(t, repo)
	manager := newCatalogEligibilityManager("worktree", true)
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.eventBus = bus.NewMemoryEventBus(testLogger())
	require.NoError(t, svc.PublishProfileCatalog(ctx, "concrete-profile", "codex-app-server", catalogFanoutGuard))
	prior := readCatalogObservation(t, repo, "catalog-session")
	require.Equal(t, sessioncatalog.SourceTaskProbeV1, prior.Source)
	manager.probe = func(context.Context, string) (sessioncatalog.TaskProbeReply, error) {
		return sessioncatalog.TaskProbeReply{}, sessioncatalog.ErrTaskProbeBusy
	}
	logCore, logs := observer.New(zap.WarnLevel)
	var err error
	svc.logger, err = logger.NewFromZap(zap.New(logCore))
	require.NoError(t, err)
	require.Error(t, svc.PublishProfileCatalog(ctx, "concrete-profile", "codex-app-server", catalogFanoutGuard),
		"an unpublished session must not be reported as published")
	require.Equal(t, prior, readCatalogObservation(t, repo, "catalog-session"), "busy never replaces task-owned evidence")
	require.Equal(t, 2, manager.probeCount("catalog-session"), "busy is not retried")
	failures := logs.FilterMessage("session catalog publication failed").All()
	require.Len(t, failures, 1)
	require.Equal(t, "helper_busy", failures[0].ContextMap()["reason"])
}
