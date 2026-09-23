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
		DataDir: discoveryDataDir(),
		Socket:  deploymentsecretd.SocketPath,
	})
}

// discoveryDataDir resolves the deployment data directory the same way the
// backend does, so a `homeDir` set in config.yaml points the operator CLI at the
// store the backend actually reads. Discovery failures fall back to the
// environment-only default: the CLI must still work before a config exists, and
// its own error text names the directory it tried.
func discoveryDataDir() string {
	cfg, err := loadBootstrapConfig()
	if err != nil {
		return resolveDataDir()
	}
	return resolveDataDirForConfig(cfg)
}
