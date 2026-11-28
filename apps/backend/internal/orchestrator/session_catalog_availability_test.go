package orchestrator

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestSessionCatalogDiscoveryAvailabilityWithoutEligibleNotice(t *testing.T) {
	for _, newerNotice := range []bool{false, true} {
		t.Run(map[bool]string{false: "without-notice", true: "newer-notice"}[newerNotice], func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "availability-task", "availability-session", models.TaskSessionStateIdle)
			session, err := repo.GetTaskSession(ctx, "availability-session")
			require.NoError(t, err)
			session.ExecutionProfileID = "concrete-profile"
			require.NoError(t, repo.UpdateTaskSession(ctx, session))
			seedExecutorRunning(t, repo, session.ID, session.TaskID, "availability-execution")
			running, err := repo.GetExecutorRunningBySessionID(ctx, session.ID)
			require.NoError(t, err)
			running.ExecutionProfileID = session.ExecutionProfileID
			require.NoError(t, repo.UpsertExecutorRunning(ctx, running))
			owner := sessioncatalog.Owner{ExecutionID: running.AgentExecutionID, AttemptID: "availability-attempt", SourceGeneration: 1}
			require.NoError(t, repo.SetSessionMetadataKey(ctx, session.ID, models.SessionMetaKeyACPModelState, map[string]any{"current_model_id": "selected-model", "models": []map[string]any{{"id": "stale-model", "name": "Stale model"}}}))
			require.NoError(t, repo.SetSessionMetadataKey(ctx, session.ID, "runtime_config", map[string]any{"model": "selected-model", "mode": "selected-mode"}))
			require.NoError(t, repo.SetSessionMetadataKey(ctx, session.ID, "acp_config_baseline", map[string]any{"reasoning": "high"}))
			launch, err := repo.BeginSessionCatalogDiscovery(ctx, session.ID, session.ExecutionProfileID, owner, "strict")
			require.NoError(t, err)
			require.NotNil(t, launch)
			require.Zero(t, launch.NoticeRevision)
			reserved, err := repo.GetTaskSession(ctx, session.ID)
			require.NoError(t, err)
			require.NotContains(t, reserved.Metadata[models.SessionMetaKeyACPModelState], "current_model_id", "an unowned selection is not relabelled for the owner")
			// The owner then reports its own selection.
			require.NoError(t, repo.SetSessionMetadataKey(ctx, session.ID, models.SessionMetaKeyACPModelState, map[string]any{"settings_source_execution_id": owner.ExecutionID, "settings_attempt_id": owner.AttemptID, "settings_source_generation": owner.SourceGeneration, "settings_policy": "strict", "current_model_id": "owner-selected-model"}))
			var notice *sessioncatalog.Observation
			if newerNotice {
				notice, err = repo.PublishSessionCatalog(ctx, session.ID, owner.ExecutionID, session.ExecutionProfileID, nil, sessioncatalog.Observation{Owner: owner, ProfileID: session.ExecutionProfileID, Digest: "newer-host", Disposition: "host_candidates", Models: []sessioncatalog.Choice{{ID: "new-host-model"}}})
				require.NoError(t, err)
				require.NotNil(t, notice)
			}
			before, err := repo.GetTaskSession(ctx, session.ID)
			require.NoError(t, err)
			service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
			service.eventBus = bus.NewMemoryEventBus(testLogger())
			delivered := 0
			subscription, err := service.eventBus.Subscribe("session.catalog_discovery_updated."+session.ID, func(ctx context.Context, event *bus.Event) error {
				delivered++
				current, readErr := repo.GetTaskSession(ctx, session.ID)
				if readErr != nil {
					return readErr
				}
				snapshot := current.Metadata[models.SessionMetaKeyACPModelState].(map[string]any)
				require.Contains(t, snapshot, "catalog_discovery", "provider availability must be committed before notification")
				encoded, encodeErr := json.Marshal(event.Data)
				require.NoError(t, encodeErr)
				var wire map[string]any
				require.NoError(t, json.Unmarshal(encoded, &wire))
				require.Equal(t, snapshot["catalog_discovery"], wire["provider"])
				return nil
			})
			require.NoError(t, err)
			t.Cleanup(func() { _ = subscription.Unsubscribe() })
			result := &sessioncatalog.Discovery{Outcome: "unsupported", Generation: 7}
			payload := &lifecycle.AgentStreamEventPayload{TaskID: session.TaskID, SessionID: session.ID, ExecutionID: owner.ExecutionID, AttemptID: owner.AttemptID, SessionSettingsSourceGeneration: owner.SourceGeneration, Data: &lifecycle.AgentStreamEventData{Type: streams.EventTypeSessionCatalogDiscovery, CatalogDiscovery: result}}
			service.handleAgentStreamEvent(ctx, payload)
			after, err := repo.GetTaskSession(ctx, session.ID)
			require.NoError(t, err)
			snapshot := after.Metadata[models.SessionMetaKeyACPModelState].(map[string]any)
			require.Contains(t, snapshot, "catalog_discovery", "unsupported evidence must survive reload independently of host-notice settlement")
			initialProvider := snapshot["catalog_discovery"].(map[string]any)
			require.Equal(t, float64(1), initialProvider["revision"], "availability needs a durable ordering barrier before any host notice exists")
			require.Equal(t, before.Metadata["runtime_config"], after.Metadata["runtime_config"])
			require.Equal(t, before.Metadata["acp_config_baseline"], after.Metadata["acp_config_baseline"])
			require.Equal(t, "owner-selected-model", snapshot["current_model_id"], "availability evidence never edits the owner's selection")
			require.Equal(t, before.Metadata[sessioncatalog.MetadataKey], after.Metadata[sessioncatalog.MetadataKey], "a result begun before a notice must not clear that notice")
			require.Equal(t, 1, delivered)
			service.handleAgentStreamEvent(ctx, payload)
			require.Equal(t, 1, delivered, "duplicate provider evidence is idempotent")
			live := *payload
			live.Data = &lifecycle.AgentStreamEventData{
				Type: streams.EventTypeSessionModels, SessionSettingsGeneration: 8,
				SessionModels:    []streams.SessionModelInfo{{ModelID: "fresh-provider-model", Name: "Fresh provider model"}},
				CatalogDiscovery: &sessioncatalog.Discovery{Outcome: "complete", Generation: 8, Models: sessioncatalog.Surface{Present: true, Supported: true, Choices: []sessioncatalog.Choice{{ID: "fresh-provider-model", Name: "Fresh provider model"}}}},
				Data:             map[string]any{"config_options_source": "provider_update"},
			}
			service.handleAgentStreamEvent(ctx, &live)
			current, err := repo.GetTaskSession(ctx, session.ID)
			require.NoError(t, err)
			currentSnapshot := current.Metadata[models.SessionMetaKeyACPModelState].(map[string]any)
			provider := currentSnapshot["catalog_discovery"].(map[string]any)
			require.Equal(t, "complete", provider["result"].(map[string]any)["outcome"], "a later genuine advertisement must replace unsupported availability")
			require.Equal(t, float64(2), provider["revision"])
			require.Equal(t, before.Metadata[sessioncatalog.MetadataKey], current.Metadata[sessioncatalog.MetadataKey], "an ordinary live advertisement cannot settle the host notice")
			require.Equal(t, before.Metadata["runtime_config"], current.Metadata["runtime_config"])
			require.Equal(t, before.Metadata["acp_config_baseline"], current.Metadata["acp_config_baseline"])
			require.Equal(t, 2, delivered)
			nextOwner := owner
			nextOwner.AttemptID, nextOwner.SourceGeneration = "next-attempt", 2
			_, err = repo.BeginSessionCatalogDiscovery(ctx, session.ID, session.ExecutionProfileID, nextOwner, "strict")
			require.NoError(t, err)
			nextModels := live
			nextModels.AttemptID, nextModels.SessionSettingsSourceGeneration = nextOwner.AttemptID, nextOwner.SourceGeneration
			nextModels.Data = &lifecycle.AgentStreamEventData{Type: streams.EventTypeSessionModels, SessionSettingsGeneration: 1, SessionModels: []streams.SessionModelInfo{{ModelID: "next-provider-model", Name: "Next provider model"}}}
			service.handleAgentStreamEvent(ctx, &nextModels)
			nextTerminal := *payload
			nextTerminal.AttemptID, nextTerminal.SessionSettingsSourceGeneration = nextOwner.AttemptID, nextOwner.SourceGeneration
			service.handleAgentStreamEvent(ctx, &nextTerminal)
			service.handleAgentStreamEvent(ctx, payload)
			rotated, err := repo.GetTaskSession(ctx, session.ID)
			require.NoError(t, err)
			rotatedSnapshot := rotated.Metadata[models.SessionMetaKeyACPModelState].(map[string]any)
			rotatedProvider := rotatedSnapshot["catalog_discovery"].(map[string]any)
			require.Equal(t, float64(3), rotatedProvider["revision"], "source rotation and intervening ordinary model snapshots must not reset provider ordering")
			require.Equal(t, "next-attempt", rotatedProvider["owner"].(map[string]any)["attempt_id"])
			require.Equal(t, 3, delivered, "the obsolete source must not publish again")
		})
	}
}
