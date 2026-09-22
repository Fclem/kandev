package secrets

import (
	"fmt"
	"os"
	"path/filepath"
)

const LegacyDeploymentMasterKeyID = "legacy-master-key-v1"

// DeploymentMasterKeyring exposes the existing backend master key to the
// deployment-secret supervisor without allowing key creation or rotation.
type DeploymentMasterKeyring struct {
	key []byte
}

func DeploymentMasterKeyringOpenExisting(dataDir string) (*DeploymentMasterKeyring, error) {
	key, err := os.ReadFile(filepath.Join(dataDir, MasterKeyFile))
	if err != nil {
		return nil, fmt.Errorf("open existing deployment master key: %w", err)
	}
	if len(key) != MasterKeySize {
		return nil, fmt.Errorf("open existing deployment master key: invalid key size %d", len(key))
	}
	return &DeploymentMasterKeyring{key: key}, nil
}

func (r *DeploymentMasterKeyring) ID() string {
	return LegacyDeploymentMasterKeyID
}

func (r *DeploymentMasterKeyring) Key() []byte {
	return append([]byte(nil), r.key...)
}
