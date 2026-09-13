package automation

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSafeWebhookTriggerDataUsesBoundedJSONPointers(t *testing.T) {
	data, err := safeWebhookTriggerData(
		[]byte(`{"pull_request":{"number":7,"title":"private title"},"token":"secret"}`),
		[]string{"/pull_request/number"}, "webhook-trigger", "delivery-7",
	)
	require.NoError(t, err)
	require.NotContains(t, string(data), "private title")
	require.NotContains(t, string(data), "secret")
	var projection map[string]any
	require.NoError(t, json.Unmarshal(data, &projection))
	require.Equal(t, "delivery-7", projection["delivery_id"])
	require.Equal(t, map[string]any{"/pull_request/number": float64(7)}, projection["payload"])
}

func TestSafeWebhookTriggerDataRejectsUnboundedPointers(t *testing.T) {
	_, err := safeWebhookTriggerData([]byte(`{"value":1}`), []string{"/value", "bad"}, "trigger", "delivery")
	require.Error(t, err)
}

func TestSafeWebhookTriggerDataRejectsMalformedJSONPointerSyntax(t *testing.T) {
	for _, pointer := range []string{"/value~2", "/items/01", "/items/-"} {
		t.Run(pointer, func(t *testing.T) {
			_, err := safeWebhookTriggerData(
				[]byte(`{"value":"ok","items":["first","second"]}`),
				[]string{pointer}, "trigger", "delivery",
			)
			require.Error(t, err)
		})
	}
}

func TestSafeRetryTriggerProjectionKeepsRoutingMetadata(t *testing.T) {
	data := SafeRetryTriggerProjection(
		TriggerTypeGitHubPRMerged, "trigger", []byte(`{
			"repo":"acme/api","head_branch":"feature","base_branch":"main",
			"task_id":"task-1","token":"secret"
		}`), "delivery-1",
	)
	var projection map[string]any
	require.NoError(t, json.Unmarshal(data, &projection))
	require.Equal(t, "acme/api", projection["repo"])
	require.Equal(t, "feature", projection["head_branch"])
	require.Equal(t, "main", projection["base_branch"])
	require.Equal(t, "task-1", projection["task_id"])
	require.NotContains(t, projection, "token")
}
