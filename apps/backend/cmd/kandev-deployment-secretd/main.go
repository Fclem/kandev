// Command kandev-deployment-secretd is the root-owned deployment secret
// supervisor. It serves exactly one privileged Unix socket for the root-only
// `kandev deployment-secret` CLI, stores the sealed deployment secrets, and
// never exposes trust material to the backend process: the backend cannot
// provision, and the daemon never bootstraps an identity.
//
// It is Linux-only. The non-Linux build compiles so `go build ./...` stays
// portable, and exits non-zero with an explicit message at runtime.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/launcher/deploymentsecretd"
)

const daemonUsage = `kandev-deployment-secretd — root-owned deployment secret supervisor

Usage:
  kandev-deployment-secretd [--socket <path>] [--data-dir <path>]

Options:
  --socket     Socket to serve. Defaults to ` + deploymentsecretd.SocketPath + `.
  --data-dir   Deployment data directory holding deployment/ and master.key.
               Defaults to <KANDEV_HOME_DIR or ~/.kandev>/data.
  --help, -h   Show help.

The daemon runs as root and loads the existing deployment identity and master
key; it refuses to start when either is missing, because only
` + "`kandev deployment-secret bootstrap`" + ` creates them. Secrets are logged nowhere.
`

const (
	socketFlag  = "--socket"
	dataDirFlag = "--data-dir"
)

type daemonConfig struct {
	socketPath string
	dataDir    string
	showHelp   bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run parses flags and serves until interrupted. It returns the process exit
// code: 0 on a clean shutdown, 2 for usage errors, 1 for startup failures.
func run(argv []string, stdout, stderr io.Writer) int {
	config, err := parseDaemonArgs(argv)
	if err != nil {
		logf(stderr, "%v", err)
		return 2
	}
	if config.showHelp {
		_, _ = fmt.Fprint(stdout, daemonUsage)
		return 0
	}
	return serve(stderr, config)
}

func parseDaemonArgs(argv []string) (daemonConfig, error) {
	config := daemonConfig{socketPath: deploymentsecretd.SocketPath, dataDir: defaultDataDir()}
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		switch {
		case arg == "--help" || arg == "-h":
			config.showHelp = true
		case arg == socketFlag || arg == dataDirFlag:
			value, err := takeDaemonValue(argv, i, arg)
			if err != nil {
				return config, err
			}
			if err := assignDaemonValue(&config, arg, value); err != nil {
				return config, err
			}
			i++
		case strings.HasPrefix(arg, socketFlag+"="):
			if err := assignDaemonValue(&config, socketFlag, strings.TrimPrefix(arg, socketFlag+"=")); err != nil {
				return config, err
			}
		case strings.HasPrefix(arg, dataDirFlag+"="):
			if err := assignDaemonValue(&config, dataDirFlag, strings.TrimPrefix(arg, dataDirFlag+"=")); err != nil {
				return config, err
			}
		default:
			return config, fmt.Errorf("unknown flag %q; run %s --help", arg, deploymentsecretd.BinaryName)
		}
	}
	return config, nil
}

func takeDaemonValue(argv []string, i int, flag string) (string, error) {
	if i+1 >= len(argv) || strings.HasPrefix(argv[i+1], "-") {
		return "", errors.New(flag + " requires a value")
	}
	return argv[i+1], nil
}

func assignDaemonValue(config *daemonConfig, flag, value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New(flag + " requires a non-empty value")
	}
	if flag == socketFlag {
		config.socketPath = value
		return nil
	}
	config.dataDir = value
	return nil
}

// defaultDataDir resolves the deployment data directory the way the launcher
// does: <KANDEV_HOME_DIR or ~/.kandev>/data. The released unit sets
// KANDEV_HOME_DIR explicitly so the daemon reads the backend's own data root
// (and therefore the same master.key).
func defaultDataDir() string {
	home := strings.TrimSpace(os.Getenv("KANDEV_HOME_DIR"))
	if home == "" {
		if resolved, err := os.UserHomeDir(); err == nil {
			home = filepath.Join(resolved, ".kandev")
		} else {
			home = ".kandev"
		}
	}
	return filepath.Join(home, "data")
}

func logf(writer io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(writer, "%s: %s\n", deploymentsecretd.BinaryName, fmt.Sprintf(format, args...))
}
