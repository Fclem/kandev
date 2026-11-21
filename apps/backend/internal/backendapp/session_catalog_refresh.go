package backendapp

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"go.uber.org/zap"
)

const legacyCatalogRetirementTimeout = 30 * time.Second

type legacyCatalogRetirer interface {
	RetireLegacySessionCatalogCandidates(context.Context) (int, error)
}

func (a *lifecycleAdapter) CurrentCatalogSavedSource(ctx context.Context, profileID string) (string, error) {
	return a.mgr.CurrentCatalogSavedSource(ctx, profileID)
}

func (a *lifecycleAdapter) CatalogRefreshExecution(ctx context.Context, sessionID, savedSource string) (sessioncatalog.ExecutionEvidence, error) {
	return a.mgr.CatalogRefreshExecution(ctx, sessionID, savedSource)
}

func (a *lifecycleAdapter) DiscoverTaskCatalogCandidates(ctx context.Context, sessionID string, owner sessioncatalog.Owner) (sessioncatalog.TaskProbeReply, error) {
	return a.mgr.DiscoverTaskCatalogCandidates(ctx, sessionID, owner)
}

// retireLegacySessionCatalogCandidates runs once before browsers hydrate, so
// generic-origin candidates become a newer notice rather than selectable
// task-owned choices. The frontend independently refuses them.
func retireLegacySessionCatalogCandidates(ctx context.Context, retirer legacyCatalogRetirer, log *logger.Logger) {
	ctx, cancel := context.WithTimeout(ctx, legacyCatalogRetirementTimeout)
	defer cancel()
	retired, err := retirer.RetireLegacySessionCatalogCandidates(ctx)
	if err != nil {
		log.Warn("legacy session catalog candidates were not retired", zap.Int("retired", retired), zap.Error(err))
		return
	}
	if retired > 0 {
		log.Info("retired legacy session catalog candidates", zap.Int("retired", retired))
	}
}
