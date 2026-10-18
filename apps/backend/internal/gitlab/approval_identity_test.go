package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFeedbackApprovalReaderRetainsUserID(t *testing.T) {
	username := "before-rename"
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := fmt.Fprintf(w, `{"approved_by":[{"user":{"id":42,"username":%q}}],"updated_at":"2026-01-02T03:04:05Z"}`, username)
		require.NoError(t, err)
	}))
	t.Cleanup(stop)
	client := NewPATClient(host, "private-token")
	for _, name := range []string{"before-rename", "after-rename"} {
		username = name
		rows, err := client.ListMRApprovals(context.Background(), "group/project", 7)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		encoded, err := json.Marshal(rows[0])
		require.NoError(t, err)
		var approval map[string]interface{}
		require.NoError(t, json.Unmarshal(encoded, &approval))
		require.Equal(t, float64(42), approval["user_id"])
	}
}
