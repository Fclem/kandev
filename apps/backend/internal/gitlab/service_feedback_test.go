package gitlab

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestListChangeRequestFeedbackExactWorkspaceHost(t *testing.T) {
	service, mock := newMRAutomationAuthFixture(t)
	mock.SeedMR("group/repo", &MR{IID: 7, Title: "workspace MR"})
	feedback, err := service.GetMRFeedbackForWorkspace(context.Background(), "ws-1", "https://gitlab.example.com", "group/repo", 7)
	require.NoError(t, err)
	require.Equal(t, "workspace MR", feedback.MR.Title)
	for _, host := range []string{"https://other.example.com", ""} {
		_, err := service.GetMRFeedbackForWorkspace(context.Background(), "ws-1", host, "group/repo", 7)
		require.Error(t, err, "missing or mismatched linked host must not use the workspace client")
	}
}
