package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestSessionCatalogRefreshOrderingAndSettlement(t *testing.T) {
	for _, tc := range []struct {
		name                string
		result              sessioncatalog.Discovery
		disposition, reason string
	}{
		{"complete-empty", sessioncatalog.Discovery{Outcome: "complete", Models: sessioncatalog.Surface{Present: true, Supported: true, Choices: []sessioncatalog.Choice{}}}, "cleared", "discovery_complete"},
		{"mode-only", sessioncatalog.Discovery{Outcome: "complete", Modes: sessioncatalog.Surface{Present: true, Supported: true, Choices: []sessioncatalog.Choice{{ID: "plan", Name: "Plan"}}}}, "cleared", "discovery_complete"},
		{"unsupported", sessioncatalog.Discovery{Outcome: "unsupported"}, "rediscovery_required", "unsupported"},
		{"failed", sessioncatalog.Discovery{Outcome: "failed"}, "rediscovery_required", "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "catalog-settle-task", "catalog-settle-session", models.TaskSessionStateIdle)
			session, err := repo.GetTaskSession(ctx, "catalog-settle-session")
			require.NoError(t, err)
			session.AgentProfileID = "concrete-profile"
			session.ExecutionProfileID = "concrete-profile"
			require.NoError(t, repo.UpdateTaskSession(ctx, session))
			seedExecutorRunning(t, repo, session.ID, session.TaskID, "old-execution")
			running, err := repo.GetExecutorRunningBySessionID(ctx, session.ID)
			require.NoError(t, err)
			running.ExecutionProfileID = "concrete-profile"
			require.NoError(t, repo.UpsertExecutorRunning(ctx, running))
			oldOwner := sessioncatalog.Owner{ExecutionID: "old-execution", AttemptID: "old-attempt", SourceGeneration: 1}
			require.NoError(t, repo.SetSessionMetadataKey(ctx, session.ID, models.SessionMetaKeyACPModelState, map[string]any{"settings_source_execution_id": oldOwner.ExecutionID, "settings_attempt_id": oldOwner.AttemptID, "settings_source_generation": 1, "current_model_id": "selected-model"}))
			intent := map[string]any{"model": "selected-model", "mode": "selected-mode", "config_options": map[string]any{"reasoning": "high"}}
			require.NoError(t, repo.SetSessionMetadataKey(ctx, session.ID, "runtime_config", intent))
			require.NoError(t, repo.SetSessionMetadataKey(ctx, session.ID, "composer_draft", "unsent work"))
			first, err := repo.PublishSessionCatalog(ctx, session.ID, oldOwner.ExecutionID, "concrete-profile", nil, sessioncatalog.Observation{ProfileID: "concrete-profile", Owner: oldOwner, Digest: "host-catalog", Disposition: "host_candidates", Reason: "new_candidates", Models: []sessioncatalog.Choice{{ID: "host-choice", Name: "Host choice"}}})
			require.NoError(t, err)
			require.NotNil(t, first)
			running.AgentExecutionID = "replacement-execution"
			require.NoError(t, repo.UpsertExecutorRunning(ctx, running))
			owner := sessioncatalog.Owner{ExecutionID: running.AgentExecutionID, AttemptID: "accepted-attempt", SourceGeneration: 1}
			launch, err := repo.BeginSessionCatalogDiscovery(ctx, session.ID, "concrete-profile", owner, "strict")
			require.NoError(t, err)
			require.Equal(t, uint64(1), launch.NoticeRevision)
			afterBegin, err := repo.GetTaskSession(ctx, session.ID)
			require.NoError(t, err)
			reserved := afterBegin.Metadata[models.SessionMetaKeyACPModelState].(map[string]any)
			require.NotContains(t, reserved, "current_model_id", "the previous owner's model is not relabelled for the replacement")
			require.Equal(t, owner.ExecutionID, reserved["settings_source_execution_id"])
			service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
			service.eventBus = bus.NewMemoryEventBus(testLogger())
			eventCount := 0
			subscription, err := service.eventBus.Subscribe(events.BuildSessionCatalogRefreshWildcardSubject(), func(ctx context.Context, event *bus.Event) error {
				eventCount++
				update := event.Data.(sessioncatalog.Update)
				persisted, readErr := repo.GetTaskSession(ctx, session.ID)
				if readErr != nil {
					return readErr
				}
				record, ok := loadCatalogObservation(persisted.Metadata[sessioncatalog.MetadataKey])
				require.True(t, ok)
				require.Equal(t, record, update.Record, "settlement notification follows its durable commit")
				return nil
			})
			require.NoError(t, err)
			t.Cleanup(func() { _ = subscription.Unsubscribe() })
			payload := &lifecycle.AgentStreamEventPayload{TaskID: session.TaskID, SessionID: session.ID, ExecutionID: owner.ExecutionID, AttemptID: owner.AttemptID, SessionSettingsSourceGeneration: owner.SourceGeneration, Data: &lifecycle.AgentStreamEventData{Type: streams.EventTypeSessionCatalogDiscovery, CatalogDiscovery: &tc.result}}
			read := func() sessioncatalog.Observation {
				current, readErr := repo.GetTaskSession(ctx, session.ID)
				require.NoError(t, readErr)
				record, ok := loadCatalogObservation(current.Metadata[sessioncatalog.MetadataKey])
				require.True(t, ok)
				require.Equal(t, intent, current.Metadata["runtime_config"])
				require.Equal(t, "unsent work", current.Metadata["composer_draft"])
				// Settlement never edits the reserved settings snapshot's selection.
				require.Equal(t, reserved["current_model_id"], current.Metadata[models.SessionMetaKeyACPModelState].(map[string]any)["current_model_id"])
				return record
			}
			stale := *payload
			stale.AttemptID = "obsolete-attempt"
			if tc.name == "complete-empty" {
				for _, result := range []sessioncatalog.Discovery{
					{Outcome: "working"},
					{Outcome: "complete"},
					{Outcome: "complete", Models: sessioncatalog.Surface{Supported: true, Choices: []sessioncatalog.Choice{{ID: "unobserved"}}}},
					{Outcome: "failed", Models: sessioncatalog.Surface{Present: true, Supported: true, Choices: []sessioncatalog.Choice{{ID: "stale-choice"}}}},
				} {
					malformed := *payload
					malformed.Data = &lifecycle.AgentStreamEventData{Type: streams.EventTypeSessionCatalogDiscovery, CatalogDiscovery: &result}
					service.handleAgentStreamEvent(ctx, &malformed)
					require.Equal(t, *first, read(), "malformed or nonterminal evidence cannot retire the notice")
				}
			}
			service.handleAgentStreamEvent(ctx, &stale)
			require.Equal(t, *first, read(), "obsolete attempt cannot settle the notice")
			absent := *payload
			absent.Data = &lifecycle.AgentStreamEventData{Type: streams.EventTypeSessionCatalogDiscovery}
			service.handleAgentStreamEvent(ctx, &absent)
			require.Equal(t, *first, read(), "an absent catalog is not terminal evidence")
			service.handleAgentStreamEvent(ctx, payload)
			settled := read()
			require.Equal(t, uint64(2), settled.Revision, "current explicit terminal discovery must settle its captured notice")
			require.Equal(t, tc.disposition, settled.Disposition)
			require.Equal(t, tc.reason, settled.Reason)
			require.Equal(t, owner, settled.Owner)
			require.Equal(t, &tc.result, settled.Result)
			require.Empty(t, settled.Models, "host candidates retire even after unsupported or failed discovery")
			require.Equal(t, 1, eventCount)
			service.handleAgentStreamEvent(ctx, payload)
			require.Equal(t, settled, read(), "duplicate terminal report is idempotent")
			require.Equal(t, 1, eventCount)
			newer, err := repo.PublishSessionCatalog(ctx, session.ID, owner.ExecutionID, "concrete-profile", settled, sessioncatalog.Observation{Owner: owner, ProfileID: "concrete-profile", Digest: "newer-catalog", Disposition: "host_candidates", Reason: "new_candidates"})
			require.NoError(t, err)
			require.NotNil(t, newer)
			service.handleAgentStreamEvent(ctx, payload)
			require.Equal(t, *newer, read(), "replacement cannot clear a refresh published after it began")
		})
	}
}

// A new owner's catalog reservation must not relabel the previous execution's
// settled options and current mode as its own: the new owner's first,
// unsettled report then reads as unsettled with no mode.
func TestSessionCatalogReservationStartsNewOwnerSettingsUnsettled(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "reserve-task", "reserve-session", models.TaskSessionStateStarting)
	session, err := repo.GetTaskSession(ctx, "reserve-session")
	require.NoError(t, err)
	session.AgentProfileID = "concrete-profile"
	session.ExecutionProfileID = "concrete-profile"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	previous := sessioncatalog.Owner{ExecutionID: "previous-execution", AttemptID: "previous-attempt", SourceGeneration: 4}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, session.ID, models.SessionMetaKeyACPModelState, lifecycle.SessionModelsSnapshot{
		SettingsSourceExecutionID: previous.ExecutionID, SettingsAttemptID: previous.AttemptID, SettingsSourceGeneration: previous.SourceGeneration,
		SettingsPolicy: streams.SessionSettingsPolicyStrict, CurrentModelID: "old-model", CurrentModeID: "old-mode",
		Models:               []streams.SessionModelInfo{{ModelID: "old-model", Name: "Old model"}},
		ConfigOptions:        []streams.ConfigOption{{ID: "effort", CurrentValue: "high"}},
		ConfigOptionsSettled: true,
	}))
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "next-execution")
	running, err := repo.GetExecutorRunningBySessionID(ctx, session.ID)
	require.NoError(t, err)
	running.ExecutionProfileID = "concrete-profile"
	require.NoError(t, repo.UpsertExecutorRunning(ctx, running))
	next := sessioncatalog.Owner{ExecutionID: "next-execution", AttemptID: "next-attempt", SourceGeneration: 1}
	launch, err := repo.BeginSessionCatalogDiscovery(ctx, session.ID, "concrete-profile", next, "strict")
	require.NoError(t, err)
	require.NotNil(t, launch)

	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	service.eventBus = bus.NewMemoryEventBus(testLogger())
	service.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID: session.TaskID, SessionID: session.ID, ExecutionID: next.ExecutionID, AttemptID: next.AttemptID,
		SessionSettingsSourceGeneration: next.SourceGeneration,
		Data: &lifecycle.AgentStreamEventData{
			Type:          streams.EventTypeSessionModels,
			SessionModels: []streams.SessionModelInfo{{ModelID: "new-model", Name: "New model"}},
			Data:          map[string]any{"config_options_settled": false},
		},
	})
	current, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	snapshot, ok := lifecycle.LoadSessionModelsSnapshot(current.Metadata[models.SessionMetaKeyACPModelState])
	require.True(t, ok)
	require.Equal(t, next.ExecutionID, snapshot.SettingsSourceExecutionID)
	require.False(t, snapshot.ConfigOptionsSettled, "the previous owner's settled options must not settle the new owner")
	require.Empty(t, snapshot.CurrentModeID, "the previous owner's mode must not become the new owner's")
}
