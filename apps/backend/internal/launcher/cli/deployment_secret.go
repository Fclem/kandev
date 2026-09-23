package cli

import (
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/launcher/deploymentsecretd"
	"io"
	"os"
	"strconv"
	"strings"
)

// The `kandev deployment-secret` group is the only writer path for
// deployment-owned secrets. It runs as root, talks to the root-owned
// kandev-deployment-secretd socket, and never echoes secret material.
const (
	deploymentSecretGroup = "deployment-secret"

	deploymentSecretCommandBootstrap = "bootstrap"
	deploymentSecretCommandStar      = "github-personal-star"

	deploymentSecretActionBootstrap = "bootstrap"
	deploymentSecretActionProvision = "provision"
	deploymentSecretActionRevoke    = "revoke"

	// deploymentSecretStdinPath reads the client secret from standard input.
	deploymentSecretStdinPath = "-"

	deploymentSecretHelpFlag      = "--help"
	deploymentSecretHelpShorthand = "-h"
	deploymentSecretDataDirFlag   = "--data-dir"
	deploymentSecretSocketFlag    = "--socket"
	deploymentSecretVersionFlag   = "--version"
	deploymentSecretFileFlag      = "--client-secret-file"

	deploymentSecretExitFailure = 1
	deploymentSecretExitUsage   = 2
)

// DeploymentSecretDefaults carries the launcher-resolved defaults so this
// package stays independent of home/data-dir discovery.
type DeploymentSecretDefaults struct {
	DataDir string
	Socket  string
}

type deploymentSecretOptions struct {
	action           string
	dataDir          string
	socket           string
	version          int64
	clientSecretFile string
	showHelp         bool
}

// deploymentSecretUsageError reports operator input mistakes. Every parse
// failure is a usage error, so the group exits 2 (matching the launcher's
// convention) instead of reporting a runtime failure.
type deploymentSecretUsageError struct {
	message string
}

func (e deploymentSecretUsageError) Error() string {
	return e.message
}

type deploymentSecretRunner struct {
	out      io.Writer
	errOut   io.Writer
	input    io.Reader
	defaults DeploymentSecretDefaults
	// geteuid reads the effective uid; it is a seam so tests can exercise the
	// root-only paths without a privileged test runner.
	geteuid func() int
}

const deploymentSecretHelp = `kandev deployment-secret — root-only deployment secret administration

Usage:
  kandev deployment-secret bootstrap [--data-dir <path>]
  kandev deployment-secret github-personal-star provision --version <n> --client-secret-file <path|->
  kandev deployment-secret github-personal-star revoke --version <n>
  kandev deployment-secret github-personal-star provision|revoke [--socket <path>] [--data-dir <path>]

bootstrap is the only step that creates the deployment root identity
(installation id + root key pair). It is idempotent and never overwrites
existing trust material.

provision reads the GitHub personal-star OAuth client secret from
--client-secret-file ("-" reads stdin), strips one trailing line break, and
seals it with the existing deployment master key before signing the request.
The plaintext is never written to disk, logged, or revealed by any command.
The sealed value is stored by the root-owned daemon under the immutable
reference kandev-runtime:github-personal-star:client-secret:<version>; rotate by
provisioning a new --version and revoking the previous one.

revoke marks an existing --version revoked. It never deletes rows and never
reveals the sealed value.

Options:
  --version             Immutable reference version to provision or revoke.
  --client-secret-file  File holding the OAuth client secret, or "-" for stdin.
  --socket              Daemon socket. Defaults to /run/kandev/deployment-secret.sock.
  --data-dir            Deployment data directory. Defaults to $KANDEV_HOME_DIR/data.
  --help, -h            Show help.
`

// RunDeploymentSecret runs one `kandev deployment-secret` invocation and
// returns the process exit code: 0 on success, 2 for usage errors, and 1 for
// runtime failures, including the non-root refusal.
func RunDeploymentSecret(argv []string, defaults DeploymentSecretDefaults) int {
	runner := deploymentSecretRunner{
		out:      os.Stdout,
		errOut:   os.Stderr,
		input:    os.Stdin,
		defaults: defaults,
		geteuid:  os.Geteuid,
	}
	return runner.run(argv)
}

func (r *deploymentSecretRunner) run(argv []string) int {
	options, err := parseDeploymentSecretArgs(argv, r.defaults)
	if err != nil {
		r.fail(err)
		return deploymentSecretExitUsage
	}
	if options.showHelp {
		_, _ = fmt.Fprint(r.out, deploymentSecretHelp)
		return 0
	}
	// The refusal happens before any filesystem or socket access: a non-root
	// operator must not be able to create deployment-owned trust material, read
	// the deployment master key, or reach the privileged socket.
	if r.geteuid() != 0 {
		r.fail(errors.New("kandev " + deploymentSecretGroup +
			" requires root: re-run it with sudo (the deployment identity, master key, and daemon socket are root-owned)"))
		return deploymentSecretExitFailure
	}
	if strings.TrimSpace(options.dataDir) == "" {
		r.fail(errors.New("deployment data directory is unavailable; pass --data-dir <path>"))
		return deploymentSecretExitFailure
	}
	// A path past the platform limit can never be bound, so dialing it would
	// only surface a raw connect error. Reported before any filesystem or
	// socket access, like every other operator input refusal here.
	if err := deploymentsecretd.ValidateSocketPath(options.socket); err != nil {
		r.fail(err)
		return deploymentSecretExitFailure
	}
	switch options.action {
	case deploymentSecretActionBootstrap:
		return r.bootstrap(options)
	default:
		return r.provisionOrRevoke(options)
	}
}

func (r *deploymentSecretRunner) fail(err error) {
	_, _ = fmt.Fprintln(r.errOut, "[kandev] "+err.Error())
}

func parseDeploymentSecretArgs(argv []string, defaults DeploymentSecretDefaults) (deploymentSecretOptions, error) {
	options := deploymentSecretOptions{dataDir: defaults.DataDir, socket: defaults.Socket}
	if len(argv) == 0 {
		options.showHelp = true
		return options, nil
	}
	group := argv[0]
	if isDeploymentSecretHelpArg(group) {
		options.showHelp = true
		return options, nil
	}
	rest := argv[1:]
	switch group {
	case deploymentSecretCommandBootstrap:
		options.action = deploymentSecretActionBootstrap
	case deploymentSecretCommandStar:
		if len(rest) == 0 {
			return options, deploymentSecretUsageError{message: "kandev " + deploymentSecretGroup + " " + deploymentSecretCommandStar +
				" requires provision or revoke"}
		}
		switch rest[0] {
		case deploymentSecretActionProvision:
			options.action = deploymentSecretActionProvision
		case deploymentSecretActionRevoke:
			options.action = deploymentSecretActionRevoke
		default:
			return options, deploymentSecretUsageError{message: fmt.Sprintf("unknown deployment secret action %q; expected provision or revoke", rest[0])}
		}
		rest = rest[1:]
	default:
		return options, deploymentSecretUsageError{message: fmt.Sprintf("unknown deployment secret command %q; expected %s or %s",
			group, deploymentSecretCommandBootstrap, deploymentSecretCommandStar)}
	}
	for i := 0; i < len(rest); i++ {
		next, err := parseDeploymentSecretFlag(rest, i, &options)
		if err != nil {
			return options, err
		}
		i = next
	}
	if options.showHelp {
		return options, nil
	}
	return options, validateDeploymentSecretOptions(options)
}

func parseDeploymentSecretFlag(argv []string, i int, options *deploymentSecretOptions) (int, error) {
	arg := argv[i]
	switch {
	case isDeploymentSecretHelpArg(arg):
		options.showHelp = true
	case arg == deploymentSecretDataDirFlag:
		value, err := takeDeploymentSecretValue(argv, i, arg)
		if err != nil {
			return i, err
		}
		return i + 1, assignDeploymentSecretValue(&options.dataDir, arg, value)
	case strings.HasPrefix(arg, deploymentSecretDataDirFlag+"="):
		return i, assignDeploymentSecretValue(&options.dataDir, deploymentSecretDataDirFlag, strings.TrimPrefix(arg, deploymentSecretDataDirFlag+"="))
	case arg == deploymentSecretSocketFlag:
		if err := requireDeploymentSecretSocketFlag(options.action, arg); err != nil {
			return i, err
		}
		value, err := takeDeploymentSecretValue(argv, i, arg)
		if err != nil {
			return i, err
		}
		return i + 1, assignDeploymentSecretValue(&options.socket, arg, value)
	case strings.HasPrefix(arg, deploymentSecretSocketFlag+"="):
		if err := requireDeploymentSecretSocketFlag(options.action, arg); err != nil {
			return i, err
		}
		return i, assignDeploymentSecretValue(&options.socket, deploymentSecretSocketFlag, strings.TrimPrefix(arg, deploymentSecretSocketFlag+"="))
	case arg == deploymentSecretVersionFlag || strings.HasPrefix(arg, deploymentSecretVersionFlag+"="):
		return parseDeploymentSecretVersionFlag(argv, i, options)
	case arg == deploymentSecretFileFlag || strings.HasPrefix(arg, deploymentSecretFileFlag+"="):
		return parseDeploymentSecretFileFlag(argv, i, options)
	default:
		return i, deploymentSecretUsageError{message: fmt.Sprintf("unknown flag %q for kandev %s %s", arg, deploymentSecretGroup, options.action)}
	}
	return i, nil
}

func parseDeploymentSecretVersionFlag(argv []string, i int, options *deploymentSecretOptions) (int, error) {
	if options.action == deploymentSecretActionBootstrap {
		return i, deploymentSecretUsageError{message: "--version is valid only for github-personal-star provision or revoke"}
	}
	raw := strings.TrimPrefix(argv[i], deploymentSecretVersionFlag+"=")
	if argv[i] == deploymentSecretVersionFlag {
		value, err := takeDeploymentSecretValue(argv, i, deploymentSecretVersionFlag)
		if err != nil {
			return i, err
		}
		raw = value
		i++
	}
	version, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || version < 1 {
		return i, deploymentSecretUsageError{message: fmt.Sprintf("--version must be a positive integer, got %q", raw)}
	}
	options.version = version
	return i, nil
}

func parseDeploymentSecretFileFlag(argv []string, i int, options *deploymentSecretOptions) (int, error) {
	if options.action != deploymentSecretActionProvision {
		return i, deploymentSecretUsageError{message: "--client-secret-file is valid only for github-personal-star provision"}
	}
	if strings.HasPrefix(argv[i], deploymentSecretFileFlag+"=") {
		return i, assignDeploymentSecretValue(&options.clientSecretFile, deploymentSecretFileFlag, strings.TrimPrefix(argv[i], deploymentSecretFileFlag+"="))
	}
	value, err := takeDeploymentSecretValue(argv, i, deploymentSecretFileFlag)
	if err != nil {
		return i, err
	}
	return i + 1, assignDeploymentSecretValue(&options.clientSecretFile, deploymentSecretFileFlag, value)
}

func validateDeploymentSecretOptions(options deploymentSecretOptions) error {
	if options.action == deploymentSecretActionBootstrap {
		return nil
	}
	if options.version < 1 {
		return deploymentSecretUsageError{message: "kandev " + deploymentSecretGroup + " " + deploymentSecretCommandStar +
			" " + options.action + " requires --version <n>"}
	}
	if options.action == deploymentSecretActionProvision && options.clientSecretFile == "" {
		return deploymentSecretUsageError{message: "kandev " + deploymentSecretGroup + " " + deploymentSecretCommandStar +
			" provision requires --client-secret-file <path|->"}
	}
	return nil
}

func requireDeploymentSecretSocketFlag(action, flag string) error {
	if action == deploymentSecretActionBootstrap {
		return deploymentSecretUsageError{message: fmt.Sprintf("%s is valid only for github-personal-star provision or revoke", flag)}
	}
	return nil
}

func assignDeploymentSecretValue(target *string, flag, value string) error {
	if strings.TrimSpace(value) == "" {
		return deploymentSecretUsageError{message: fmt.Sprintf("%s requires a non-empty value", flag)}
	}
	*target = value
	return nil
}

func takeDeploymentSecretValue(argv []string, i int, flag string) (string, error) {
	if i+1 >= len(argv) {
		return "", deploymentSecretUsageError{message: fmt.Sprintf("%s requires a value", flag)}
	}
	value := argv[i+1]
	// A bare "-" is a valid value (stdin); any other leading dash means the
	// operator forgot the value and named the next flag instead.
	if strings.HasPrefix(value, "-") && value != deploymentSecretStdinPath {
		return "", deploymentSecretUsageError{message: fmt.Sprintf("%s requires a value", flag)}
	}
	return value, nil
}

func isDeploymentSecretHelpArg(arg string) bool {
	return arg == deploymentSecretHelpFlag || arg == deploymentSecretHelpShorthand
}
