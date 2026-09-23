package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/config"
)

// The deployment secret group is dispatched before the launcher's own command
// parsing, so it must never fall through to run/dev/start.
func TestRunDispatchesDeploymentSecretGroup(t *testing.T) {
	t.Setenv("KANDEV_HOME_DIR", t.TempDir())
	code := -1
	output := captureLauncherStdout(t, func() {
		code = Run([]string{deploymentSecretGroupName, "--help"}, BuildInfo{})
	})
	if code != 0 {
		t.Fatalf("kandev %s --help exit = %d, want 0", deploymentSecretGroupName, code)
	}
	for _, want := range []string{
		"kandev deployment-secret bootstrap",
		"github-personal-star provision",
		"github-personal-star revoke",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("help does not describe %q:\n%s", want, output)
		}
	}
}

// TestDiscoveryDataDirHonoursConfiguredHomeDir pins that the operator CLI reads
// the same data root as the backend: a `homeDir` in config.yaml must move the
// deployment store with it, not silently send provisioning to ~/.kandev.
func TestDiscoveryDataDirHonoursConfiguredHomeDir(t *testing.T) {
	tmp := t.TempDir()
	configured := filepath.Join(tmp, "srv-kandev")
	workingDir := filepath.Join(tmp, "working")
	if err := os.MkdirAll(workingDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(workingDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("homeDir: "+configured+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(workingDir)
	t.Setenv("KANDEV_HOME_DIR", "")
	t.Setenv(config.InternalConfigFileEnv, configPath)

	if got, want := discoveryDataDir(), filepath.Join(configured, "data"); got != want {
		t.Fatalf("discoveryDataDir() = %q, want %q", got, want)
	}
}

// TestDiscoveryDataDirFallsBackWithoutConfig keeps the CLI usable before any
// configuration exists (the documented env-only default).
func TestDiscoveryDataDirFallsBackWithoutConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("KANDEV_HOME_DIR", "")
	t.Setenv(config.InternalConfigFileEnv, "")

	if got, want := discoveryDataDir(), filepath.Join(resolveHomeDir(), "data"); got != want {
		t.Fatalf("discoveryDataDir() = %q, want %q", got, want)
	}
}
