package websocket

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizedOrderedSessionPayloadStripsSystemContent(t *testing.T) {
	payload := sanitizedOrderedSessionPayload("message.added", map[string]any{
		"session_id":             "session-1",
		"task_id":                "task-1",
		"message_id":             "message-1",
		orderedContentPayloadKey: "visible <kandev-system>secret</kandev-system>",
	})

	require.Equal(t, "visible", payload["content"])
}
