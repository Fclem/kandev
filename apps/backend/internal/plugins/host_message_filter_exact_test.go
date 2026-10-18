package plugins

import (
	"context"
	"fmt"
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

func TestExactMessageDerivedTaskBoundIsIndependentOfCallerFilters(t *testing.T) {
	tasks := make([]*taskmodels.Task, maxMessageFilterValues)
	for i := range tasks {
		tasks[i] = &taskmodels.Task{ID: fmt.Sprintf("task-%03d", i), WorkspaceID: "workspace-one"}
	}
	data := &fakeTaskDataSource{
		workspaces:       []*taskmodels.Workspace{{ID: "workspace-one"}},
		tasksByWorkspace: map[string][]*taskmodels.Task{"workspace-one": tasks},
	}
	host := newExactReadHost(t, data, []string{"messages"}, []string{"host.v2.read:messages"})
	host.messageData = &fakeMessageDataSource{messages: []*taskmodels.Message{{
		ID: "human-message", TaskID: tasks[0].ID, AuthorType: taskmodels.MessageAuthorUser,
		Type: taskmodels.MessageTypeMessage, Content: "Question",
	}}}
	page, err := host.ListSanitizedMessages(context.Background(), pluginsdk.ExactMessageQuery{
		RequestID: "workspace-human-messages", WorkspaceID: "workspace-one",
		Filter: pluginsdk.MessageFilter{Types: []string{"message"}, AuthorTypes: []string{"user"}},
	})
	if err != nil {
		t.Fatalf("400 derived workspace tasks plus caller filters must remain valid: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Message.ID != "human-message" {
		t.Fatalf("human message missing from bounded workspace query: %+v", page.Items)
	}
}
