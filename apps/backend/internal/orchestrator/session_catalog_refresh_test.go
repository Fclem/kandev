package orchestrator

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/common/modelcatalog"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type catalogPublicationService interface {
	PublishProfileCatalog(context.Context, string, string, hostutility.ProfileProbeContext, hostutility.AgentCapabilities, func(context.Context, func() error) error) error
}

type catalogAgentManager struct {
	*mockAgentManager
	evidence sessioncatalog.ExecutionEvidence
}

func (m *catalogAgentManager) CatalogRefreshExecution(context.Context, string, sessioncatalog.LaunchContext) (sessioncatalog.ExecutionEvidence, error) {
	return m.evidence, nil
}

// @covers AC-AGENTS-SESSION-CATALOG-001.1
// @covers AC-AGENTS-SESSION-CATALOG-001.2
// @covers AC-AGENTS-SESSION-CATALOG-001.4
// @covers AC-AGENTS-SESSION-CATALOG-001.5
func TestSessionCatalogRefreshEligibility(t *testing.T) {
	for _, executorType := range []string{"local", "worktree", "local_pc", "", "unknown", "local_docker", "remote_docker", "ssh", "sprites", "k8s", "plugin-remote"} {
		name := executorType
		if name == "" {
			name = "empty-locality"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
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
			manager := &catalogAgentManager{mockAgentManager: &mockAgentManager{}, evidence: sessioncatalog.ExecutionEvidence{Owner: sessioncatalog.Owner{ExecutionID: "catalog-execution", AttemptID: "attempt", SourceGeneration: 1}, ProfileID: "concrete-profile", AgentID: "claude-acp", ExecutorType: executorType, Initialized: true, Compatible: true}}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
			svc.eventBus = bus.NewMemoryEventBus(testLogger())
			subscription, err := svc.eventBus.Subscribe(events.BuildSessionCatalogRefreshWildcardSubject(), func(ctx context.Context, event *bus.Event) error {
				update := event.Data.(sessioncatalog.Update)
				persisted, readErr := repo.GetTaskSession(ctx, update.SessionID)
				if readErr != nil {
					return readErr
				}
				payload, marshalErr := json.Marshal(persisted.Metadata[sessioncatalog.MetadataKey])
				if marshalErr != nil {
					return marshalErr
				}
				var committed sessioncatalog.Observation
				if decodeErr := json.Unmarshal(payload, &committed); decodeErr != nil {
					return decodeErr
				}
				require.Equal(t, update.Record, committed, "event must describe an already committed observation")
				return nil
			})
			require.NoError(t, err)
			t.Cleanup(func() { _ = subscription.Unsubscribe() })
			publisher, ok := any(svc).(catalogPublicationService)
			if !ok {
				t.Fatal("saved profile observation does not reach existing authorized sessions")
			}
			guard := func(ctx context.Context, commit func() error) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				return commit()
			}
			caps := hostutility.AgentCapabilities{Status: hostutility.StatusOK, CurrentModelID: "probe-default", CurrentModeID: "probe-mode", Models: []hostutility.Model{{ID: "candidate", Name: "Candidate"}}, Modes: []hostutility.Mode{{ID: "candidate-mode", Name: "Candidate mode"}}}
			catalog := &modelcatalog.Catalog{Version: 1, Nodes: []modelcatalog.Node{{Kind: modelcatalog.NodeKindGroup, ID: "provider-group", Label: "Provider group", Children: []modelcatalog.Node{{Kind: modelcatalog.NodeKindChoice, Value: "candidate", Label: "Candidate"}}}}}
			caps.ConfigOptions = []hostutility.ConfigOption{{ID: "model", Name: "Model", CurrentValue: "probe-default", Catalog: catalog}}
			require.NoError(t, publisher.PublishProfileCatalog(ctx, "concrete-profile", "claude-acp", hostutility.ProfileProbeContext{}, caps, guard))
			stored, err := repo.GetTaskSession(ctx, session.ID)
			require.NoError(t, err)
			require.Equal(t, effective, stored.Metadata["runtime_config"])
			payload, err := json.Marshal(stored.Metadata[sessioncatalog.MetadataKey])
			require.NoError(t, err)
			var observation sessioncatalog.Observation
			require.NoError(t, json.Unmarshal(payload, &observation))
			host := executorType == "local" || executorType == "worktree" || executorType == "local_pc"
			if host {
				require.Equal(t, "host_candidates", observation.Disposition)
				require.Equal(t, []sessioncatalog.Choice{{ID: "candidate", Name: "Candidate"}}, observation.Models)
				require.Equal(t, []sessioncatalog.Choice{{ID: "candidate-mode", Name: "Candidate mode"}}, observation.Modes)
				require.Equal(t, catalog, observation.Catalog)
			} else {
				require.Equal(t, "rediscovery_required", observation.Disposition)
				require.Empty(t, observation.Models)
				require.Empty(t, observation.Modes)
				require.Nil(t, observation.Catalog, "notice-only executors must not receive the host catalog")
			}
			require.Equal(t, uint64(1), observation.Revision)
			require.NoError(t, publisher.PublishProfileCatalog(ctx, "concrete-profile", "claude-acp", hostutility.ProfileProbeContext{}, caps, guard))
			duplicate, err := repo.GetTaskSession(ctx, session.ID)
			require.NoError(t, err)
			require.Equal(t, stored.Metadata[sessioncatalog.MetadataKey], duplicate.Metadata[sessioncatalog.MetadataKey], "same pending observation must coalesce")
		})
	}
}
