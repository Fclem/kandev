package models

import "testing"

func TestClassifyMessageOrigin(t *testing.T) {
	tests := []struct {
		name     string
		author   MessageAuthorType
		metadata map[string]interface{}
		want     MessageOrigin
	}{
		{name: "default user is human", author: MessageAuthorUser, want: MessageOriginHuman},
		{name: "peer task precedes plugin", author: MessageAuthorUser, metadata: map[string]interface{}{"sender_task_id": "task-1", "source": "plugin:sample"}, want: MessageOriginPeerTask},
		{name: "plugin source", author: MessageAuthorUser, metadata: map[string]interface{}{"source": "plugin:sample"}, want: MessageOriginPlugin},
		{name: "github PR automation", author: MessageAuthorUser, metadata: map[string]interface{}{"origin": "github_pr_automation"}, want: MessageOriginAutomation},
		{name: "CI automation", author: MessageAuthorUser, metadata: map[string]interface{}{"origin": "ci_automation"}, want: MessageOriginAutomation},
		{name: "GitLab MR automation", author: MessageAuthorUser, metadata: map[string]interface{}{"origin": "gitlab_mr_automation"}, want: MessageOriginAutomation},
		{name: "workflow precedes auto start", author: MessageAuthorUser, metadata: map[string]interface{}{"workflow_message": true, "auto_start": true}, want: MessageOriginWorkflow},
		{name: "bare auto start", author: MessageAuthorUser, metadata: map[string]interface{}{"auto_start": true}, want: MessageOriginAutomation},
		{name: "agent row", author: MessageAuthorAgent, metadata: map[string]interface{}{"source": "plugin:sample"}, want: MessageOriginAgent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyMessageOrigin(&Message{AuthorType: tt.author, Metadata: tt.metadata})
			if got != tt.want {
				t.Fatalf("ClassifyMessageOrigin() = %q, want %q", got, tt.want)
			}
		})
	}
}
