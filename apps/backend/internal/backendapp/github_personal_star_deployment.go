package backendapp

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/system/deploymentsecret"
)

// buildPersonalStarDeployment assembles the deployment-owned personal-star
// boundary for the GitHub provider. Every failure degrades the feature to
// unavailable instead of failing startup: the backend stays healthy, the
// connection reports available=false, and no OAuth or provisioning path is
// reachable.
func buildPersonalStarDeployment(cfg *config.Config, log *logger.Logger) github.PersonalStarDeployment {
	group := cfg.GitHub.PersonalStarOAuth
	if err := group.Validate(); err != nil {
		log.Info("GitHub personal star OAuth settings are unusable", zap.Error(err))
		return github.PersonalStarDeployment{}
	}
	settings := github.PersonalStarOAuthSettings{
		ClientID:    strings.TrimSpace(group.ClientID),
		CallbackURL: strings.TrimSpace(group.CallbackURL),
	}
	if settings.ClientID == "" || settings.CallbackURL == "" {
		return github.PersonalStarDeployment{}
	}
	if target := deploymentsecret.DetectBackendTarget(); !deploymentsecret.Available(target) {
		log.Info("deployment secret supervisor is unavailable for this backend")
		return github.PersonalStarDeployment{}
	}
	dataDir := cfg.ResolvedDataDir()
	keyring, err := secrets.DeploymentMasterKeyringOpenExisting(dataDir)
	if err != nil {
		log.Info("deployment master key is unavailable", zap.Error(err))
		return github.PersonalStarDeployment{}
	}
	identity, err := secrets.LoadDeploymentIdentity(dataDir)
	if err != nil {
		log.Info("deployment identity is unavailable", zap.Error(err))
		return github.PersonalStarDeployment{}
	}
	store, err := secrets.OpenDeploymentSecretStoreReader(dataDir, identity, keyring)
	if err != nil {
		log.Info("deployment secret store is unavailable", zap.Error(err))
		return github.PersonalStarDeployment{}
	}
	resolver := secrets.NewDeploymentSecretResolver(store)
	reference := strings.TrimSpace(group.ClientSecretRef)
	return github.PersonalStarDeployment{
		Available: true,
		Settings:  settings,
		ReadClientSecret: func(ctx context.Context) (string, error) {
			return resolver.Resolve(ctx, reference)
		},
		Close: store.Close,
	}
}
