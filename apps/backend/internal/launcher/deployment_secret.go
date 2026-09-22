package launcher

import (
	"github.com/kandev/kandev/internal/launcher/cli"
	"github.com/kandev/kandev/internal/launcher/deploymentsecretd"
)

// deploymentSecretGroupName is the `kandev deployment-secret` command group.
const deploymentSecretGroupName = "deployment-secret"

// runDeploymentSecret dispatches the root-only deployment secret operator CLI.
// The launcher owns home/data-dir discovery, so it resolves the defaults here
// and package cli stays independent of the config stack.
func runDeploymentSecret(argv []string) int {
	return cli.RunDeploymentSecret(argv, cli.DeploymentSecretDefaults{
		DataDir: resolveDataDir(),
		Socket:  deploymentsecretd.SocketPath,
	})
}
