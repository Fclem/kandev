package deploymentsecret

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kandev/kandev/internal/secrets"
)

func TestDetectBackendTargetReportsTheCurrentProcessIdentity(t *testing.T) {
	target := DetectBackendTarget()
	if target.GOOS != runtime.GOOS || target.BackendUID != os.Geteuid() {
		t.Fatalf("DetectBackendTarget() = %+v, want the current process identity", target)
	}
}

func TestUnsupportedUserSystemdTargetIsRejected(t *testing.T) {
	t.Setenv(serviceModeEnvVar, serviceModeUser)
	target := DetectBackendTarget()
	if !target.UserSystemd {
		t.Fatal("DetectBackendTarget() ignored the user-scope service marker")
	}
	if Available(target) {
		t.Fatalf("Available(%+v) = true for a user-scoped service", target)
	}
	if err := ValidateTarget(target); err == nil {
		t.Fatal("ValidateTarget() accepted a user-scoped service")
	}
}

func TestUnsupportedContainerTargetIsRejected(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	target := DetectBackendTarget()
	if !target.Container {
		t.Fatal("DetectBackendTarget() ignored the Kubernetes container marker")
	}
	if Available(target) || ValidateTarget(target) == nil {
		t.Fatalf("container target %+v was accepted", target)
	}
}

func TestUnsupportedRootRunTargetIsRejected(t *testing.T) {
	target := Target{GOOS: linuxOS, BackendUID: 0}
	if Available(target) || ValidateTarget(target) == nil {
		t.Fatalf("root-run backend target %+v was accepted", target)
	}
}

func TestUnavailablePlatformNeverCreatesASocket(t *testing.T) {
	dataDir := t.TempDir()
	identity, err := secrets.BootstrapDeploymentIdentity(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(dataDir, "run", "deployment-secret.sock")
	server, err := NewServer(Config{
		SocketPath:  socketPath,
		Target:      Target{GOOS: linuxOS, BackendUID: 1000},
		Identity:    identity,
		Provisioner: &recordingProvisioner{},
	})
	if err != nil {
		t.Fatal(err)
	}
	// A target that becomes unsupported between construction and listen must
	// still refuse to create the socket and its parent directory.
	server.config.Target = Target{GOOS: linuxOS, BackendUID: 1000, Container: true}
	if err := server.Listen(); err == nil {
		t.Fatal("Listen() accepted an unsupported target")
	}
	if _, err := os.Stat(filepath.Dir(socketPath)); !os.IsNotExist(err) {
		t.Fatalf("socket directory stat = %v, want absent", err)
	}
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Fatalf("socket stat = %v, want absent", err)
	}
}

func TestDeploymentTargetOverrideClearsOnlyHostFacts(t *testing.T) {
	t.Setenv(targetOverrideEnvVar, targetOverrideSupported)
	t.Setenv(serviceModeEnvVar, serviceModeUser)
	target := DetectBackendTarget()
	if target.Container || target.UserSystemd {
		t.Fatalf("override did not clear the host facts: %+v", target)
	}
	if target.GOOS != runtime.GOOS || target.BackendUID != os.Geteuid() {
		t.Fatalf("override changed the process identity: %+v", target)
	}
	// The security guards the override must never relax.
	if Available(Target{GOOS: linuxOS, BackendUID: 0}) {
		t.Fatal("override allowed a root-run backend")
	}
	if Available(Target{GOOS: "darwin", BackendUID: 1000}) {
		t.Fatal("override allowed a non-Linux backend")
	}
}

func TestDeploymentTargetOverrideIgnoresOtherValues(t *testing.T) {
	t.Setenv(targetOverrideEnvVar, "yes")
	t.Setenv(serviceModeEnvVar, serviceModeUser)
	if !DetectBackendTarget().UserSystemd {
		t.Fatal("an unrecognised override value changed the detected facts")
	}
}
