package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

const turnUpdatedEvent = "turn.updated"

func TestPatchTurnMetadataPublishesSanitizedTimingUpdate(t *testing.T) {
	for _, completed := range []bool{false, true} {
		name := "active"
		if completed {
			name = "completed"
		}
		t.Run(name, func(t *testing.T) {
			svc, eventBus, repo := createTestService(t)
			ctx := context.Background()
			setupTestTask(t, repo)
			sessionID := setupTestSession(t, repo)
			turn, err := svc.ReserveTurn(ctx, sessionID, &models.PromptDispatchRecovery{})
			if err != nil {
				t.Fatalf("ReserveTurn: %v", err)
			}
			oldUpdatedAt := time.Now().UTC().Add(-time.Minute)
			if _, err := repo.DB().ExecContext(ctx,
				"UPDATE task_session_turns SET updated_at = ? WHERE id = ?", oldUpdatedAt, turn.ID,
			); err != nil {
				t.Fatalf("backdate turn updated_at: %v", err)
			}
			if completed {
				if err := repo.CompleteTurn(ctx, turn.ID); err != nil {
					t.Fatalf("CompleteTurn: %v", err)
				}
				if _, err := repo.DB().ExecContext(ctx,
					"UPDATE task_session_turns SET updated_at = ? WHERE id = ?", oldUpdatedAt, turn.ID,
				); err != nil {
					t.Fatalf("backdate completed turn updated_at: %v", err)
				}
			}

			nonTimingUpdates := []map[string]interface{}{
				{"prompt_usage": map[string]interface{}{"input_tokens": float64(4)}},
				{models.TurnMetaKeyCodexNativeTurnID: "provider-turn"},
				{models.TurnMetaKeyErrorTerminated: true},
			}
			for _, updates := range nonTimingUpdates {
				if err := svc.PatchTurnMetadata(ctx, sessionID, turn.ID, updates); err != nil {
					t.Fatalf("patch non-timing metadata: %v", err)
				}
			}
			if got := len(eventBus.GetPublishedEvents()); got != 0 {
				t.Fatalf("non-timing patches published %d events, want none", got)
			}
			beforeTiming, err := repo.GetTurn(ctx, turn.ID)
			if err != nil {
				t.Fatalf("read turn before timing patch: %v", err)
			}
			timing := map[string]interface{}{"ttft_ms": float64(120)}
			if err := svc.PatchTurnMetadata(ctx, sessionID, turn.ID, map[string]interface{}{
				"timing":                                timing,
				models.TurnMetaKeyPromptDispatchPending: true,
			}); err != nil {
				t.Fatalf("patch timing metadata: %v", err)
			}
			published := eventBus.GetPublishedEvents()
			if len(published) != 1 || published[0].Type != turnUpdatedEvent {
				t.Fatalf("published events = %#v, want one turn.updated", published)
			}
			data, ok := published[0].Data.(map[string]interface{})
			if !ok {
				t.Fatalf("turn.updated payload has type %T", published[0].Data)
			}
			updatedAt, ok := data["updated_at"].(time.Time)
			if !ok || !updatedAt.After(beforeTiming.UpdatedAt) {
				t.Fatalf("updated_at = %#v, want newer than %v", data["updated_at"], beforeTiming.UpdatedAt)
			}
			metadata, ok := data["metadata"].(map[string]interface{})
			if !ok || metadata["timing"] == nil {
				t.Fatalf("turn.updated metadata = %#v, want timing", data["metadata"])
			}
			if _, exposed := metadata[models.TurnMetaKeyPromptDispatchPending]; exposed {
				t.Fatalf("turn.updated exposed private dispatch state: %#v", metadata)
			}
		})
	}
}
