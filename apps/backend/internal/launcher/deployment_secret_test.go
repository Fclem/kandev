package launcher

import (
	"strings"
	"testing"
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
