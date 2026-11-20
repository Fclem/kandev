package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/common/sessioncatalog"
)

func (a *lifecycleAdapter) CatalogRefreshExecution(ctx context.Context, sessionID string, expected sessioncatalog.LaunchContext) (sessioncatalog.ExecutionEvidence, error) {
	return a.mgr.CatalogRefreshExecution(ctx, sessionID, expected)
}
