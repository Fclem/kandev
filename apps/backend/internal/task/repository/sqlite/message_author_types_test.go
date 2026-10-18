package sqlite

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestMessageListAuthorTypesFilter(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, "task-author-filter", "session-author-filter", "turn-author-filter")
	createdAt := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	insertPluginMsg(t, repo, "user-message", "session-author-filter", "task-author-filter", "turn-author-filter", "user", "message", "human prompt", createdAt)
	insertPluginMsg(t, repo, "agent-message", "session-author-filter", "task-author-filter", "turn-author-filter", "agent", "message", "agent response", createdAt.Add(time.Second))

	filter := models.PluginMessageFilter{SessionIDs: []string{"session-author-filter"}, AuthorTypes: []string{"user"}}
	messages, err := repo.ListMessagesForPlugin(t.Context(), filter)
	if err != nil {
		t.Fatalf("ListMessagesForPlugin: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != "user-message" {
		t.Fatalf("filtered message IDs = %v, want [user-message]", messageIDs(messages))
	}
}
