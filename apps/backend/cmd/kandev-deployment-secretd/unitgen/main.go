// Command unitgen prints the systemd unit text for the root-owned deployment
// secret supervisor.
//
// The release pipeline writes the output into
// systemd/kandev-deployment-secretd.service inside
// kandev-deployment-secretd-<platform>.tar.gz, so the shipped unit can never
// drift from the Go generator that `kandev service ...`-style installers reuse.
// It is a packaging tool: nothing at runtime shells out to it.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kandev/kandev/internal/launcher"
	"github.com/kandev/kandev/internal/launcher/deploymentsecretd"
)

const unitgenUsage = `unitgen — render the kandev-deployment-secretd systemd unit

Usage:
  unitgen [--executable <path>] [--home-dir <path>] [--socket <path>]

The unit text is written to stdout. Empty options fall back to the release
contract (installed binary, /var/lib/kandev, /run/kandev/deployment-secret.sock).
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(argv []string, stdout, stderr io.Writer) int {
	input, showHelp, err := parseUnitgenArgs(argv)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "unitgen: "+err.Error())
		return 2
	}
	if showHelp {
		_, _ = fmt.Fprint(stdout, unitgenUsage)
		return 0
	}
	_, _ = fmt.Fprint(stdout, launcher.RenderDeploymentSecretdUnit(input))
	return 0
}

func parseUnitgenArgs(argv []string) (launcher.DeploymentSecretdUnitInput, bool, error) {
	var input launcher.DeploymentSecretdUnitInput
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		switch {
		case arg == "--help" || arg == "-h":
			return input, true, nil
		case arg == "--executable" || arg == "--home-dir" || arg == "--socket":
			value, err := takeUnitgenValue(argv, i, arg)
			if err != nil {
				return input, false, err
			}
			assignUnitgenValue(&input, arg, value)
			i++
		case strings.HasPrefix(arg, "--executable="):
			assignUnitgenValue(&input, "--executable", strings.TrimPrefix(arg, "--executable="))
		case strings.HasPrefix(arg, "--home-dir="):
			assignUnitgenValue(&input, "--home-dir", strings.TrimPrefix(arg, "--home-dir="))
		case strings.HasPrefix(arg, "--socket="):
			assignUnitgenValue(&input, "--socket", strings.TrimPrefix(arg, "--socket="))
		default:
			return input, false, fmt.Errorf("unknown flag %q; run unitgen --help", arg)
		}
	}
	// The unit's ExecStart names the resolved socket, so an unbindable path is
	// rejected at generation time instead of becoming a daemon that cannot
	// start.
	if err := deploymentsecretd.ValidateSocketPath(launcher.DeploymentSecretdUnitSocketPath(input)); err != nil {
		return input, false, err
	}
	return input, false, nil
}

func takeUnitgenValue(argv []string, i int, flag string) (string, error) {
	if i+1 >= len(argv) || strings.HasPrefix(argv[i+1], "-") {
		return "", errors.New(flag + " requires a value")
	}
	return argv[i+1], nil
}

func assignUnitgenValue(input *launcher.DeploymentSecretdUnitInput, flag, value string) {
	switch flag {
	case "--executable":
		input.Executable = value
	case "--home-dir":
		input.HomeDir = value
	default:
		input.SocketPath = value
	}
}
