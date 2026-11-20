package handlers

import (
	"testing"

	repoerrors "github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestHTTPTransferConflictCodeAllowlistsRelationsBusy(t *testing.T) {
	if got := httpTaskTransferConflictCode(repoerrors.TaskTransferRelationsBusy); got != "relations_busy" {
		t.Fatalf("relations_busy must reach the wire as its own code, got %q", got)
	}
}
