package plugins

import (
	"reflect"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestClassifyMessageOrigin(t *testing.T) {
	cases := []struct {
		name     string
		author   taskmodels.MessageAuthorType
		metadata map[string]interface{}
		want     string
	}{
		{name: "human fallback", author: taskmodels.MessageAuthorUser, want: "human"},
		{name: "peer task wins", author: taskmodels.MessageAuthorUser, metadata: map[string]interface{}{"sender_task_id": "sender", "source": "plugin:test"}, want: "peer_task"},
		{name: "plugin", author: taskmodels.MessageAuthorUser, metadata: map[string]interface{}{"source": "plugin:test"}, want: "plugin"},
		{name: "pull request automation", author: taskmodels.MessageAuthorUser, metadata: map[string]interface{}{"origin": "github_pr_automation", "workflow_message": true, "auto_start": true}, want: "automation"},
		{name: "CI automation", author: taskmodels.MessageAuthorUser, metadata: map[string]interface{}{"origin": "ci_automation"}, want: "automation"},
		{name: "GitLab merge request automation", author: taskmodels.MessageAuthorUser, metadata: map[string]interface{}{"origin": "gitlab_mr_automation"}, want: "automation"},
		{name: "workflow beats auto start", author: taskmodels.MessageAuthorUser, metadata: map[string]interface{}{"workflow_message": true, "auto_start": true}, want: "workflow"},
		{name: "bare auto start", author: taskmodels.MessageAuthorUser, metadata: map[string]interface{}{"auto_start": true}, want: "automation"},
		{name: "agent author", author: taskmodels.MessageAuthorAgent, metadata: map[string]interface{}{"source": "plugin:test"}, want: "agent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dto := messageModelToDTO(&taskmodels.Message{AuthorType: tc.author, Metadata: tc.metadata})
			field := reflect.ValueOf(dto).FieldByName("Origin")
			if !field.IsValid() {
				t.Fatal("Message DTO has no Origin field")
			}
			if got := field.String(); got != tc.want {
				t.Fatalf("origin = %q, want %q", got, tc.want)
			}
			if reflect.ValueOf(dto).FieldByName("Metadata").IsValid() {
				t.Fatal("message metadata must not be exposed in the DTO")
			}
		})
	}
}
