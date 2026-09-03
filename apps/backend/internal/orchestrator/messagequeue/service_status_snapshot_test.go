package messagequeue

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

func TestGetStatusReturnsDetachedEntrySnapshots(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	svc := NewServiceMemory(log)
	ctx := context.Background()
	_, err = svc.QueueMessageWithMetadata(ctx, "session", "task", "content", "", QueuedByUser, false,
		[]MessageAttachment{{AttachmentID: "attachment-1", Name: "original.txt"}},
		map[string]interface{}{"nested": map[string]interface{}{"value": "original"}},
	)
	require.NoError(t, err)

	status := svc.GetStatus(ctx, "session")
	require.Len(t, status.Entries, 1)
	status.Entries[0].Attachments[0].Name = "mutated.txt"
	status.Entries[0].Metadata["nested"].(map[string]interface{})["value"] = "mutated"

	fresh := svc.GetStatus(ctx, "session")
	require.Equal(t, "original.txt", fresh.Entries[0].Attachments[0].Name)
	require.Equal(t, "original", fresh.Entries[0].Metadata["nested"].(map[string]interface{})["value"])
}
