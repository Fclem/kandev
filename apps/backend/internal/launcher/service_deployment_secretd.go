package launcher

import (
	"strings"

	"github.com/kandev/kandev/internal/launcher/deploymentsecretd"
)

// The root-owned deployment secret supervisor gets its own unit, rendered the
// same way kandev.service is: one Go generator, reused quoting, and a stable
// text that the release pipeline writes into
// kandev-deployment-secretd-<platform>.tar.gz. The launcher never installs,
// starts, stops, or reads this unit — `kandev service ...` keeps managing only
// kandev.service.

// DeploymentSecretdUnitInput overrides the generation defaults. Empty fields
// fall back to the release contract in the deploymentsecretd package.
type DeploymentSecretdUnitInput struct {
	Executable string
	HomeDir    string
	SocketPath string
}

// RenderDeploymentSecretdUnit renders the root-owned daemon unit text.
func RenderDeploymentSecretdUnit(input DeploymentSecretdUnitInput) string {
	executable := deploymentSecretdUnitValue(input.Executable, deploymentsecretd.InstallPath)
	homeDir := deploymentSecretdUnitValue(input.HomeDir, defaultSystemServiceHomeDir)
	socketPath := deploymentSecretdUnitValue(input.SocketPath, deploymentsecretd.SocketPath)
	return "# managed by kandev — install as " + deploymentsecretd.UnitPath + " and never edit by hand\n" +
		"[Unit]\nDescription=Kandev root-owned deployment secret supervisor\n\n" +
		"[Service]\nType=simple\n" +
		"ExecStart=" + quoteForUnit(executable) + " --socket " + quoteForUnit(socketPath) + "\n" +
		// The supervisor serves a root-owned socket for a root peer only, so the
		// unit pins the account explicitly instead of inheriting systemd's
		// default. It is never installed as a user unit.
		"User=root\n" +
		serviceEnvLine("KANDEV_HOME_DIR", homeDir) + "\n" +
		"Restart=on-failure\nRestartSec=5s\nKillMode=mixed\nTimeoutStopSec=30s\n\n" +
		"[Install]\nWantedBy=multi-user.target\n"
}

func deploymentSecretdUnitValue(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}
