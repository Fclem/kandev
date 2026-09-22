package backendapp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
)

func personalStarTestConfig(t *testing.T) *config.Config {
	t.Helper()
	homeDir := t.TempDir()
	cfg := &config.Config{}
	cfg.HomeDir = homeDir
	cfg.GitHub.PersonalStarOAuth = config.PersonalStarOAuthConfig{
		ClientID:        "star-client-id",
		ClientSecretRef: "kandev-runtime:github-personal-star:client-secret:1",
		CallbackURL:     "https://kandev.example/api/v1/github/user/starred/oauth/callback",
	}
	return cfg
}

func newDeploymentTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	if err != nil {
		t.Fatal(err)
	}
	return log
}

// assertNoDeploymentSideEffects proves the composition root never created or
// repaired deployment-owned material for an unsupported or unconfigured run.
func assertNoDeploymentSideEffects(t *testing.T, cfg *config.Config) {
	t.Helper()
	dataDir := cfg.ResolvedDataDir()
	for _, path := range []string{
		secrets.DeploymentIdentityDir(dataDir),
		secrets.DeploymentSecretStorePath(dataDir),
		filepath.Join(dataDir, secrets.MasterKeyFile),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("deployment side effect at %s: %v", path, err)
		}
	}
}

func TestUnsupportedUserSystemdBackendReportsUnavailableStarRuntime(t *testing.T) {
	t.Setenv("KANDEV_SERVICE_MODE", "user")
	cfg := personalStarTestConfig(t)

	deployment := buildPersonalStarDeployment(cfg, newDeploymentTestLogger(t))
	if deployment.Available {
		t.Fatal("user-scoped service reported an available personal star deployment")
	}
	if deployment.ReadClientSecret != nil || deployment.Close != nil {
		t.Fatalf("unavailable deployment exposed runtime hooks: %+v", deployment)
	}
	assertNoDeploymentSideEffects(t, cfg)
}

func TestUnavailablePlatformLeavesNoProvisioningSideEffect(t *testing.T) {
	t.Setenv("KANDEV_SERVICE_MODE", "user")
	cfg := personalStarTestConfig(t)

	deployment := buildPersonalStarDeployment(cfg, newDeploymentTestLogger(t))
	if deployment.Available {
		t.Fatal("unsupported target reported an available personal star deployment")
	}
	assertNoDeploymentSideEffects(t, cfg)
}

func TestPersonalStarDeploymentStaysUnavailableAfterReferencesFail(t *testing.T) {
	cfg := personalStarTestConfig(t)
	// A deployment with a supported target but no provisioned store must still
	// degrade to unavailable rather than provisioning material itself.
	deployment := buildPersonalStarDeployment(cfg, newDeploymentTestLogger(t))
	if deployment.Available && deployment.ReadClientSecret == nil {
		t.Fatal("available deployment exposed no secret reader")
	}
	assertNoDeploymentSideEffects(t, cfg)
}
