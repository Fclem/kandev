package secrets

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// DeploymentInstallationIDBytes is the random length of the installation ID.
	DeploymentInstallationIDBytes = 16
	deploymentIdentityDirName     = "deployment"
	deploymentInstallationIDFile  = "installation_id"
	deploymentPrivateKeyFile      = "installation_key"
	deploymentPublicKeyFile       = "installation_key.pub"
)

// DeploymentIdentity is the deployment root of trust: the immutable
// installation ID and the bootstrap Ed25519 public key. Neither value is
// request-derived or writable through user/secret APIs.
type DeploymentIdentity struct {
	InstallationID string
	RootPublicKey  ed25519.PublicKey
}

// DeploymentIdentityDir returns the deployment-owned metadata directory.
func DeploymentIdentityDir(dataDir string) string {
	return filepath.Join(dataDir, deploymentIdentityDirName)
}

// LoadDeploymentIdentity reads the identity without creating or repairing it.
func LoadDeploymentIdentity(dataDir string) (DeploymentIdentity, error) {
	dir := DeploymentIdentityDir(dataDir)
	installationID, err := readTrimmedFile(filepath.Join(dir, deploymentInstallationIDFile))
	if err != nil {
		return DeploymentIdentity{}, fmt.Errorf("read deployment installation id: %w", err)
	}
	if err := ValidateDeploymentInstallationID(installationID); err != nil {
		return DeploymentIdentity{}, fmt.Errorf("read deployment installation id: %w", err)
	}
	publicKey, err := os.ReadFile(filepath.Join(dir, deploymentPublicKeyFile))
	if err != nil {
		return DeploymentIdentity{}, fmt.Errorf("read deployment root public key: %w", err)
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return DeploymentIdentity{}, fmt.Errorf("read deployment root public key: invalid size %d", len(publicKey))
	}
	return DeploymentIdentity{InstallationID: installationID, RootPublicKey: ed25519.PublicKey(publicKey)}, nil
}

// BootstrapDeploymentIdentity generates the identity exactly once. A partial or
// malformed existing identity is an error: bootstrap never repairs or
// overwrites deployment-owned trust material.
func BootstrapDeploymentIdentity(dataDir string) (DeploymentIdentity, error) {
	existing, err := LoadDeploymentIdentity(dataDir)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return DeploymentIdentity{}, err
	}

	dir := DeploymentIdentityDir(dataDir)
	if findErr := findDeploymentIdentityFile(dir); findErr != nil {
		return DeploymentIdentity{}, findErr
	}
	if mkErr := os.MkdirAll(dir, 0o700); mkErr != nil {
		return DeploymentIdentity{}, fmt.Errorf("create deployment identity dir: %w", mkErr)
	}
	installationID, err := randomDeploymentHex(DeploymentInstallationIDBytes)
	if err != nil {
		return DeploymentIdentity{}, err
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return DeploymentIdentity{}, fmt.Errorf("generate deployment root key: %w", err)
	}
	if err := writeDeploymentIdentityFiles(dir, installationID, publicKey, privateKey); err != nil {
		return DeploymentIdentity{}, err
	}
	return DeploymentIdentity{InstallationID: installationID, RootPublicKey: publicKey}, nil
}

// LoadDeploymentRootPrivateKey loads the root private key for the operator CLI,
// which runs as the privileged deployment owner.
func LoadDeploymentRootPrivateKey(dataDir string) (ed25519.PrivateKey, error) {
	privateKey, err := os.ReadFile(filepath.Join(DeploymentIdentityDir(dataDir), deploymentPrivateKeyFile))
	if err != nil {
		return nil, fmt.Errorf("read deployment root private key: %w", err)
	}
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("read deployment root private key: invalid size %d", len(privateKey))
	}
	return ed25519.PrivateKey(privateKey), nil
}

func writeDeploymentIdentityFiles(
	dir, installationID string,
	publicKey ed25519.PublicKey,
	privateKey ed25519.PrivateKey,
) error {
	installationPath := filepath.Join(dir, deploymentInstallationIDFile)
	if err := os.WriteFile(installationPath, []byte(installationID+"\n"), 0o644); err != nil {
		return fmt.Errorf("write deployment installation id: %w", err)
	}
	privateKeyPath := filepath.Join(dir, deploymentPrivateKeyFile)
	if err := os.WriteFile(privateKeyPath, privateKey, 0o600); err != nil {
		return fmt.Errorf("write deployment root private key: %w", err)
	}
	publicKeyPath := filepath.Join(dir, deploymentPublicKeyFile)
	if err := os.WriteFile(publicKeyPath, publicKey, 0o644); err != nil {
		return fmt.Errorf("write deployment root public key: %w", err)
	}
	return nil
}

func findDeploymentIdentityFile(dir string) error {
	for _, name := range []string{deploymentInstallationIDFile, deploymentPrivateKeyFile, deploymentPublicKeyFile} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return fmt.Errorf("deployment identity is incomplete: %s exists without a complete identity", name)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func readTrimmedFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func randomDeploymentHex(length int) (string, error) {
	buffer := make([]byte, length)
	if _, err := io.ReadFull(rand.Reader, buffer); err != nil {
		return "", fmt.Errorf("generate deployment identifier: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}
