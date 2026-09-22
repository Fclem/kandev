package launcher

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/launcher/deploymentsecretd"
)

func TestRenderDeploymentSecretdUnitIsRootOwnedAndSeparate(t *testing.T) {
	unit := RenderDeploymentSecretdUnit(DeploymentSecretdUnitInput{})
	for _, want := range []string{
		"[Unit]",
		"ExecStart=" + deploymentsecretd.InstallPath + " --socket " + deploymentsecretd.SocketPath,
		"User=root",
		serviceEnvLine("KANDEV_HOME_DIR", defaultSystemServiceHomeDir),
		"Restart=on-failure",
		"[Install]\nWantedBy=multi-user.target",
		deploymentsecretd.UnitPath,
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("unit does not contain %q:\n%s", want, unit)
		}
	}
	// The daemon must never be managed through the kandev.service unit.
	if strings.Contains(unit, serviceUnitName) {
		t.Fatalf("daemon unit references %s:\n%s", serviceUnitName, unit)
	}
	if strings.Contains(unit, "kandev.service install") {
		t.Fatalf("daemon unit points operators at the kandev service installer:\n%s", unit)
	}
}

func TestRenderDeploymentSecretdUnitQuotesValues(t *testing.T) {
	unit := RenderDeploymentSecretdUnit(DeploymentSecretdUnitInput{
		Executable: "/opt/kandev bin/kandev-deployment-secretd",
		HomeDir:    "/srv/kandev home",
		SocketPath: "/run/kandev my/deployment-secret.sock",
	})
	for _, want := range []string{
		`ExecStart="/opt/kandev bin/kandev-deployment-secretd" --socket "/run/kandev my/deployment-secret.sock"`,
		serviceEnvLine("KANDEV_HOME_DIR", "/srv/kandev home"),
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("unit does not quote %q:\n%s", want, unit)
		}
	}
	if strings.Contains(unit, "ExecStart=/opt/kandev bin") {
		t.Fatalf("unit left the executable path unquoted:\n%s", unit)
	}
}

func TestRenderDeploymentSecretdUnitIgnoresBlankOverrides(t *testing.T) {
	unit := RenderDeploymentSecretdUnit(DeploymentSecretdUnitInput{Executable: "  ", HomeDir: "", SocketPath: " "})
	if !strings.Contains(unit, "ExecStart="+deploymentsecretd.InstallPath+" --socket "+deploymentsecretd.SocketPath) {
		t.Fatalf("blank overrides did not fall back to the contract:\n%s", unit)
	}
}
