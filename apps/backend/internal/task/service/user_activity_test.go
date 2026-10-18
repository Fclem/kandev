package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecordUserActivityMissingAndInaccessibleSessionsAreUniform(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)
	repo.SetUserActivityCapture(true)
	svc.userActivity = repo

	ctx := ctxAs("user-a")
	missing, err := svc.RecordUserActivity(ctx, "user-a", "missing-session")
	require.NoError(t, err)
	inaccessible, err := svc.RecordUserActivity(ctx, "user-a", "sess-b")
	require.NoError(t, err)
	authDisabled, err := svc.RecordUserActivity(ctxSynthetic(), "default-user", "missing-session")
	require.NoError(t, err)
	if missing != userActivityOutcomeRejected || inaccessible != missing || authDisabled != missing {
		t.Fatalf("missing outcome = %q, inaccessible outcome = %q, auth-disabled outcome = %q, want identical %q", missing, inaccessible, authDisabled, userActivityOutcomeRejected)
	}
}
