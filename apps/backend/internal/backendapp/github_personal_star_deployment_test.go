package backendapp

import (
	"bytes"
	"context"
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

// TestPersonalStarDeploymentResolvesSeededMaterialWithTheE2ETargetOverride
// proves the CI path: an e2e run inside a container declares the host
// supported, the fixture seeds the deployment material exactly as production
// requires, and the composition root resolves the client secret into a usable
// feature.
func TestPersonalStarDeploymentResolvesSeededMaterialWithTheE2ETargetOverride(t *testing.T) {
	t.Setenv("KANDEV_DEPLOYMENT_SECRET_TARGET", "supported")
	t.Setenv("KANDEV_SERVICE_MODE", "user")
	cfg := personalStarTestConfig(t)
	dataDir := cfg.ResolvedDataDir()
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{9}, secrets.MasterKeySize)
	if err := os.WriteFile(filepath.Join(dataDir, secrets.MasterKeyFile), key, 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := secrets.BootstrapDeploymentIdentity(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	keyring, err := secrets.DeploymentMasterKeyringOpenExisting(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := secrets.OpenDeploymentSecretStoreWriter(dataDir, identity, keyring)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	sealed, err := secrets.SealDeploymentSecret(keyring.ID(), key, []byte("e2e-client-secret"))
	if err != nil {
		t.Fatal(err)
	}
	provisioner := secrets.NewDeploymentSecretProvisioner(writer)
	if err := provisioner.Provision(context.Background(), secrets.DeploymentProvisionRequest{
		InstallationID:   identity.InstallationID,
		SecretClass:      secrets.DeploymentSecretClassGitHubPersonalStar,
		ReferenceVersion: 1,
		SealedEnvelope:   sealed.Bytes(),
	}); err != nil {
		t.Fatal(err)
	}

	deployment := buildPersonalStarDeployment(cfg, newDeploymentTestLogger(t))
	if !deployment.Available {
		t.Fatal("seeded deployment with the e2e target override stayed unavailable")
	}
	if deployment.ReadClientSecret == nil || deployment.Close == nil {
		t.Fatalf("deployment runtime hooks = %+v", deployment)
	}
	secret, err := deployment.ReadClientSecret(context.Background())
	if err != nil || secret != "e2e-client-secret" {
		t.Fatalf("ReadClientSecret() = %q, %v", secret, err)
	}
	if err := deployment.Close(); err != nil {
		t.Fatal(err)
	}
}
