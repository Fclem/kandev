package plugins

import (
	"context"
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMessageListAuthorTypesFilterExactBound(t *testing.T) {
	base := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	task := &taskmodels.Task{ID: "task-one", WorkspaceID: "workspace-one", State: "in_progress", UpdatedAt: base}
	session := &taskmodels.TaskSession{ID: "session-one", TaskID: task.ID, State: "running", UpdatedAt: base}
	data := &fakeTaskDataSource{
		workspaces:       []*taskmodels.Workspace{{ID: "workspace-one"}},
		tasksByWorkspace: map[string][]*taskmodels.Task{"workspace-one": {task}},
		sessionsByTask:   map[string][]*taskmodels.TaskSession{task.ID: {session}},
	}
	host := newExactReadHost(t, data, []string{"tasks", "messages"}, []string{"host.v2.read:tasks", "host.v2.read:messages"})
	messages := &fakeMessageDataSource{}
	host.messageData = messages
	filter := pluginsdk.MessageFilter{Types: []string{"message"}, AuthorTypes: make([]string, maxMessageFilterValues)}
	_, err := host.ListSanitizedMessages(context.Background(), pluginsdk.ExactMessageQuery{
		RequestID: "author-filter-bound", WorkspaceID: "workspace-one", Filter: filter,
		Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("ListSanitizedMessages error = %v, want ResourceExhausted", err)
	}
	if messages.calls != 0 {
		t.Fatalf("message data calls = %d, want 0 for an over-bound filter", messages.calls)
	}
}
