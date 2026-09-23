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

	"github.com/kandev/kandev/internal/launcher/deploymentsecretd"
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

// TestSuperviseRefusesSocketPathBeyondThePlatformLimit pins the preflight that
// keeps a deep socket path from reaching bind(), where the kernel reports only
// an unusable "invalid argument". The daemon must fail closed without creating
// the socket's directory, and the error must name the path, the limit, and the
// remedy.
func TestSuperviseRefusesSocketPathBeyondThePlatformLimit(t *testing.T) {
	withRootUID(t, 0)
	dataDir := preparedDataDir(t)
	deepParent := filepath.Join(t.TempDir(), strings.Repeat("repository-star-", 8))
	socketPath := filepath.Join(deepParent, "deployment-secret.sock")
	if len(socketPath) <= deploymentsecretd.UnixSocketPathLimit {
		t.Fatalf("probe socket path is only %d bytes; the test needs one above the limit", len(socketPath))
	}
	err := supervise(context.Background(), &bytes.Buffer{}, daemonConfig{socketPath: socketPath, dataDir: dataDir})
	if err == nil {
		t.Fatal("the daemon accepted a socket path the kernel cannot bind")
	}
	for _, want := range []string{socketPath, "107", "--socket"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("supervise error %q does not name %q", err, want)
		}
	}
	if _, statErr := os.Stat(deepParent); !os.IsNotExist(statErr) {
		t.Fatalf("the daemon created the socket directory before failing (stat error %v)", statErr)
	}
}

// TestSuperviseBindsSocketPathAtTheExactPlatformLimit proves the constant is the
// kernel's real ceiling rather than a guess: a path one byte shorter than the
// limit binds, so a daemon that rejected it would refuse a valid install.
func TestSuperviseBindsSocketPathAtTheExactPlatformLimit(t *testing.T) {
	withRootUID(t, 0)
	dataDir := preparedDataDir(t)
	base, err := os.MkdirTemp("", "kandev-ds-limit")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	const suffix = "/deployment-secret.sock"
	padding := deploymentsecretd.UnixSocketPathLimit - len(base) - len(suffix) - 1
	if padding < 1 {
		t.Fatalf("temp dir %s is too deep (%d bytes) to build a boundary path", base, len(base))
	}
	socketPath := filepath.Join(base, strings.Repeat("d", padding), "deployment-secret.sock")
	if len(socketPath) != deploymentsecretd.UnixSocketPathLimit {
		t.Fatalf("probe socket path is %d bytes, want exactly %d", len(socketPath), deploymentsecretd.UnixSocketPathLimit)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- supervise(ctx, &bytes.Buffer{}, daemonConfig{socketPath: socketPath, dataDir: dataDir})
	}()
	waitForSocket(t, socketPath)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("supervise returned %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("supervise did not stop after cancellation")
	}
}
