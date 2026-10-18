package models

import "strings"

// MessageOrigin is the privacy-safe provenance category exposed for a message.
type MessageOrigin string

const (
	MessageOriginHuman      MessageOrigin = "human"
	MessageOriginPeerTask   MessageOrigin = "peer_task"
	MessageOriginPlugin     MessageOrigin = "plugin"
	MessageOriginAutomation MessageOrigin = "automation"
	MessageOriginWorkflow   MessageOrigin = "workflow"
	MessageOriginAgent      MessageOrigin = "agent"
)

// ClassifyMessageOrigin classifies a user message from its persisted metadata.
// Explicit provenance markers take precedence over the generic auto_start bit.
func ClassifyMessageOrigin(message *Message) MessageOrigin {
	if message == nil {
		return MessageOriginHuman
	}
	if message.AuthorType == MessageAuthorAgent {
		return MessageOriginAgent
	}
	if nonEmptyMetadataString(message.Metadata, "sender_task_id") {
		return MessageOriginPeerTask
	}
	if source, ok := message.Metadata["source"].(string); ok && strings.HasPrefix(source, "plugin:") {
		return MessageOriginPlugin
	}
	switch message.Metadata["origin"] {
	case "github_pr_automation", "ci_automation", "gitlab_mr_automation":
		return MessageOriginAutomation
	}
	if message.Metadata["workflow_message"] == true {
		return MessageOriginWorkflow
	}
	if message.Metadata["auto_start"] == true {
		return MessageOriginAutomation
	}
	return MessageOriginHuman
}

func nonEmptyMetadataString(metadata map[string]interface{}, key string) bool {
	value, ok := metadata[key].(string)
	return ok && value != ""
}
