//go:build linux

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/system/deploymentsecret"
)

// backendUID is the deployment context the supervisor serves. The daemon only
// asserts the target shape it was installed for: a Linux system service whose
// backend runs as a normal user. A root-run backend, a container, or a user
// systemd unit must never reach this socket, so the supervisor refuses to serve
// them.
const backendUID = 1000

// geteuid is a seam so tests can exercise the supervisor wiring without root.
var geteuid = os.Geteuid

// serve runs until SIGINT/SIGTERM and returns the process exit code.
func serve(stderr io.Writer, config daemonConfig) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := supervise(ctx, stderr, config); err != nil {
		logf(stderr, "error: %v", err)
		return 1
	}
	return 0
}

// supervise wires the supervisor to the deployment-owned identity, master key,
// and secret store, then serves the root-owned socket until ctx is cancelled.
func supervise(ctx context.Context, stderr io.Writer, config daemonConfig) error {
	if uid := geteuid(); uid != 0 {
		return fmt.Errorf("refusing to run as uid %d: the deployment secret supervisor must run as root", uid)
	}
	identity, err := secrets.LoadDeploymentIdentity(config.dataDir)
	if err != nil {
		return fmt.Errorf("load the deployment identity from %s: %w (bootstrap it first with `kandev deployment-secret bootstrap`)",
			config.dataDir, err)
	}
	keyring, err := secrets.DeploymentMasterKeyringOpenExisting(config.dataDir)
	if err != nil {
		return fmt.Errorf("open the existing deployment master key in %s: %w", config.dataDir, err)
	}
	store, err := secrets.OpenDeploymentSecretStoreWriter(config.dataDir, identity, keyring)
	if err != nil {
		return fmt.Errorf("open the deployment secret store: %w (build the daemon with CGO_ENABLED=1: it uses the cgo sqlite3 driver)", err)
	}
	defer func() { _ = store.Close() }()
	server, err := deploymentsecret.NewServer(deploymentsecret.Config{
		SocketPath:  config.socketPath,
		Target:      deploymentsecret.Target{GOOS: runtime.GOOS, BackendUID: backendUID},
		Identity:    identity,
		Provisioner: secrets.NewDeploymentSecretProvisioner(store),
	})
	if err != nil {
		return fmt.Errorf("configure the deployment secret supervisor: %w", err)
	}
	if err := server.Listen(); err != nil {
		return err
	}
	logf(stderr, "listening on %s for installation %s", config.socketPath, identity.InstallationID)
	served := make(chan error, 1)
	go func() { served <- server.Serve(ctx) }()
	select {
	case err := <-served:
		_ = server.Close()
		if err != nil {
			return err
		}
	case <-ctx.Done():
		if err := server.Close(); err != nil {
			return fmt.Errorf("stop the deployment secret supervisor: %w", err)
		}
		if err := <-served; err != nil {
			return err
		}
	}
	logf(stderr, "stopped")
	return nil
}
