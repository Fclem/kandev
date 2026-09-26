//go:build linux

package secrets

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

const deploymentIdentityHelperEnv = "KANDEV_DEPLOYMENT_IDENTITY_NON_ROOT_HELPER"

func TestDeploymentIdentityCanBeLoadedByNonRootServiceUser(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root to exercise a distinct service user")
	}

	root, err := os.MkdirTemp("/tmp", "kandev-identity-nonroot-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(root) }()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(root, "data")
	if err := os.Mkdir(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := BootstrapDeploymentIdentity(dataDir); err != nil {
		t.Fatal(err)
	}

	helperPath := filepath.Join(root, "identity-test-helper")
	source, err := os.Open(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	helper, err := os.OpenFile(helperPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		_ = source.Close()
		t.Fatal(err)
	}
	_, copyErr := io.Copy(helper, source)
	closeErr := errors.Join(source.Close(), helper.Close())
	if err := errors.Join(copyErr, closeErr); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(helperPath, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(helperPath, "-test.run=^TestDeploymentIdentityNonRootHelper$")
	cmd.Env = append(os.Environ(), deploymentIdentityHelperEnv+"=1", "KANDEV_DEPLOYMENT_IDENTITY_DATA_DIR="+dataDir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534}}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("non-root identity helper failed: %v\n%s", err, output)
	}
}

func TestDeploymentIdentityNonRootHelper(t *testing.T) {
	if os.Getenv(deploymentIdentityHelperEnv) != "1" {
		return
	}
	dataDir := os.Getenv("KANDEV_DEPLOYMENT_IDENTITY_DATA_DIR")
	if _, err := LoadDeploymentIdentity(dataDir); err != nil {
		t.Fatalf("LoadDeploymentIdentity() as service user: %v", err)
	}
	if _, err := LoadDeploymentRootPrivateKey(dataDir); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("LoadDeploymentRootPrivateKey() as service user error = %v, want permission denied", err)
	}
}
