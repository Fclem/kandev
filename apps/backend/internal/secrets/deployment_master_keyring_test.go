package secrets

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDeploymentMasterKeyringOpenExistingDoesNotGenerateKey(t *testing.T) {
	dir := t.TempDir()
	if _, err := DeploymentMasterKeyringOpenExisting(dir); err == nil {
		t.Fatal("OpenExisting() succeeded without an existing key")
	}
	if _, err := os.Stat(filepath.Join(dir, MasterKeyFile)); !os.IsNotExist(err) {
		t.Fatalf("master key stat error = %v, want no file", err)
	}

	key := bytes.Repeat([]byte{7}, MasterKeySize)
	if err := os.WriteFile(filepath.Join(dir, MasterKeyFile), key, 0o600); err != nil {
		t.Fatal(err)
	}
	ring, err := DeploymentMasterKeyringOpenExisting(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ring.ID() != LegacyDeploymentMasterKeyID {
		t.Fatalf("ID() = %q", ring.ID())
	}
	if !bytes.Equal(ring.Key(), key) {
		t.Fatalf("Key() = %x, want %x", ring.Key(), key)
	}
}

func TestDeploymentMasterKeyringReturnsKeyCopy(t *testing.T) {
	dir := t.TempDir()
	key := bytes.Repeat([]byte{7}, MasterKeySize)
	if err := os.WriteFile(filepath.Join(dir, MasterKeyFile), key, 0o600); err != nil {
		t.Fatal(err)
	}
	ring, err := DeploymentMasterKeyringOpenExisting(dir)
	if err != nil {
		t.Fatal(err)
	}

	returned := ring.Key()
	returned[0] = 0
	if ring.Key()[0] != key[0] {
		t.Fatal("Key() returned mutable keyring storage")
	}
}
