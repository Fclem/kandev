//go:build linux

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/secrets"
)

// withRootUID runs a test body as if the daemon had root's effective uid, so
// the wiring can be exercised without a privileged test runner.
func withRootUID(t *testing.T, uid int) {
	t.Helper()
	original := geteuid
	geteuid = func() int { return uid }
	t.Cleanup(func() { geteuid = original })
}

func shortSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "kandev-ds")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "deployment-secret.sock")
}

// preparedDataDir holds a deployment identity plus the existing master key the
// daemon loads in production.
func preparedDataDir(t *testing.T) string {
	t.Helper()
	dataDir := t.TempDir()
	if _, err := secrets.BootstrapDeploymentIdentity(dataDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, secrets.MasterKeyFile), bytes.Repeat([]byte{5}, secrets.MasterKeySize), 0o600); err != nil {
		t.Fatal(err)
	}
	return dataDir
}

func TestSuperviseRefusesMissingDeploymentIdentityWithoutBootstrapping(t *testing.T) {
	withRootUID(t, 0)
	dataDir := t.TempDir()
	err := supervise(context.Background(), &bytes.Buffer{}, daemonConfig{
		socketPath: shortSocketPath(t),
		dataDir:    dataDir,
	})
	if err == nil || !strings.Contains(err.Error(), "bootstrap") {
		t.Fatalf("supervise error = %v, want an actionable bootstrap error", err)
	}
	if _, statErr := os.Stat(secrets.DeploymentIdentityDir(dataDir)); !os.IsNotExist(statErr) {
		t.Fatalf("the daemon bootstrapped deployment trust material (stat error %v)", statErr)
	}
}

func TestSuperviseRefusesNonRootProcess(t *testing.T) {
	withRootUID(t, 1000)
	socketPath := shortSocketPath(t)
	err := supervise(context.Background(), &bytes.Buffer{}, daemonConfig{socketPath: socketPath, dataDir: preparedDataDir(t)})
	if err == nil || !strings.Contains(err.Error(), "root") {
		t.Fatalf("supervise error = %v, want a root refusal", err)
	}
	if _, statErr := os.Stat(socketPath); !os.IsNotExist(statErr) {
		t.Fatalf("a non-root daemon created the socket (stat error %v)", statErr)
	}
}

func TestSuperviseServesTheRootSocketUntilCancelled(t *testing.T) {
	withRootUID(t, 0)
	dataDir := preparedDataDir(t)
	identity, err := secrets.LoadDeploymentIdentity(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	socketPath := shortSocketPath(t)
	var stderr bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- supervise(ctx, &stderr, daemonConfig{socketPath: socketPath, dataDir: dataDir}) }()
	waitForSocket(t, socketPath)

	info, err := os.Stat(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode = %v, want 0600", info.Mode().Perm())
	}
	if log := stderr.String(); !strings.Contains(log, socketPath) || !strings.Contains(log, identity.InstallationID) {
		t.Fatalf("startup log = %q, want the socket and installation id", log)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("supervise returned %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("supervise did not stop after cancellation")
	}
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Fatalf("shutdown left the socket behind (stat error %v)", err)
	}
	if log := stderr.String(); !strings.Contains(log, "stopped") {
		t.Fatalf("shutdown log = %q, want a stop line", log)
	}
}

func waitForSocket(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(path); err == nil && info.Mode()&os.ModeSocket != 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("socket %s was never created", path)
}
