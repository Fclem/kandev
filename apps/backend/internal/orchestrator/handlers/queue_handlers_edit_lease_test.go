package handlers

import (
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestQueueEditErrorMapsMissingLeaseToEditConflict(t *testing.T) {
	msg := createTestMessage(t, ws.ActionMessageQueueEditRenew, map[string]interface{}{})
	response := queueEditLeaseError(msg, messagequeue.ErrEditLeaseNotFound)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Equal(t, "edit_conflict", parseError(t, response).Code)
}
