package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/launcher"
	"github.com/kandev/kandev/internal/launcher/deploymentsecretd"
)

func runUnitgen(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestUnitgenWritesTheReleasedUnit(t *testing.T) {
	code, stdout, stderr := runUnitgen(t)
	if code != 0 || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if want := launcher.RenderDeploymentSecretdUnit(launcher.DeploymentSecretdUnitInput{}); stdout != want {
		t.Fatalf("unitgen output drifted from the generator:\n%s", stdout)
	}
	if !strings.Contains(stdout, deploymentsecretd.UnitPath) {
		t.Fatalf("unit does not name its install path:\n%s", stdout)
	}
}

func TestUnitgenAppliesOverrides(t *testing.T) {
	code, stdout, stderr := runUnitgen(t,
		"--executable", "/usr/lib/kandev/kandev-deployment-secretd",
		"--home-dir=/srv/kandev",
		"--socket", "/run/kandev/secret.sock")
	if code != 0 || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	for _, want := range []string{
		"ExecStart=/usr/lib/kandev/kandev-deployment-secretd --socket /run/kandev/secret.sock",
		"Environment=KANDEV_HOME_DIR=/srv/kandev",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("unitgen did not apply %q:\n%s", want, stdout)
		}
	}
}

func TestUnitgenRejectsUnknownFlags(t *testing.T) {
	code, stdout, stderr := runUnitgen(t, "--platform", "linux-x64")
	if code != 2 || stdout != "" || !strings.Contains(stderr, "unknown flag") {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
	code, stdout, stderr = runUnitgen(t, "--home-dir")
	if code != 2 || stdout != "" || !strings.Contains(stderr, "requires a value") {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
}

// TestUnitgenRejectsUnbindableSocketPath pins the install-time half of the
// socket path preflight: a unit whose ExecStart names a path past the platform
// limit installs cleanly and only fails later, as a bind error with no remedy.
func TestUnitgenRejectsUnbindableSocketPath(t *testing.T) {
	socketPath := "/srv/" + strings.Repeat("repository-star-", 8) + "deployment-secret.sock"
	if len(socketPath) <= deploymentsecretd.UnixSocketPathLimit {
		t.Fatalf("probe socket path is only %d bytes; the test needs one above the limit", len(socketPath))
	}
	code, stdout, stderr := runUnitgen(t, "--socket", socketPath)
	if code != 2 || stdout != "" {
		t.Fatalf("exit = %d, stdout = %q", code, stdout)
	}
	for _, want := range []string{socketPath, "107", "--socket"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr %q does not name %q", stderr, want)
		}
	}
}
