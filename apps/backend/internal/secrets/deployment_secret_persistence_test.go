package secrets

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func deploymentTestKey(t *testing.T, dataDir string) []byte {
	t.Helper()
	key := bytes.Repeat([]byte{11}, MasterKeySize)
	if err := os.WriteFile(filepath.Join(dataDir, MasterKeyFile), key, 0o600); err != nil {
		t.Fatal(err)
	}
	return key
}

func deploymentTestStore(t *testing.T, dataDir string, identity DeploymentIdentity) *DeploymentSecretStore {
	t.Helper()
	keyring, err := DeploymentMasterKeyringOpenExisting(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenDeploymentSecretStoreWriter(dataDir, identity, keyring)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func seedDeploymentSecret(t *testing.T, store *DeploymentSecretStore, version int64, plaintext string) {
	t.Helper()
	sealed, err := SealDeploymentSecret(store.keyring.ID(), store.keyring.Key(), []byte(plaintext))
	if err != nil {
		t.Fatal(err)
	}
	provisioner := NewDeploymentSecretProvisioner(store)
	if err := provisioner.Provision(context.Background(), DeploymentProvisionRequest{
		InstallationID:   store.identity.InstallationID,
		SecretClass:      DeploymentSecretClassGitHubPersonalStar,
		ReferenceVersion: version,
		SealedEnvelope:   sealed.Bytes(),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentSecretStoreRoundTripAndRotation(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	identity, err := BootstrapDeploymentIdentity(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	deploymentTestKey(t, dataDir)
	store := deploymentTestStore(t, dataDir, identity)
	provisioner := NewDeploymentSecretProvisioner(store)

	if _, err := store.RevealDeploymentSecret(ctx, DeploymentSecretReference(1)); !errors.Is(err, ErrDeploymentSecretNotFound) {
		t.Fatalf("RevealDeploymentSecret() error = %v, want not found", err)
	}

	seedDeploymentSecret(t, store, 1, "first-secret")
	reference := DeploymentSecretReference(1)
	if _, err := DeploymentSecretReferenceVersion(reference); err != nil {
		t.Fatal(err)
	}
	value, err := store.RevealDeploymentSecret(ctx, reference)
	if err != nil {
		t.Fatal(err)
	}
	if value != "first-secret" {
		t.Fatalf("RevealDeploymentSecret() = %q", value)
	}
	record, err := store.Load(ctx, reference)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(record.SealedEnvelope, "first-secret") {
		t.Fatal("sealed envelope stored plaintext")
	}

	seedDeploymentSecret(t, store, 2, "second-secret")
	duplicate, err := SealDeploymentSecret(store.keyring.ID(), store.keyring.Key(), []byte("third-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if err := provisioner.Provision(ctx, DeploymentProvisionRequest{
		InstallationID:   identity.InstallationID,
		SecretClass:      DeploymentSecretClassGitHubPersonalStar,
		ReferenceVersion: 1,
		SealedEnvelope:   duplicate.Bytes(),
	}); !errors.Is(err, ErrDeploymentSecretExists) {
		t.Fatalf("Provision() duplicate error = %v, want already existing", err)
	}

	if err := provisioner.Revoke(ctx, DeploymentRevokeRequest{
		InstallationID:   identity.InstallationID,
		SecretClass:      DeploymentSecretClassGitHubPersonalStar,
		ReferenceVersion: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RevealDeploymentSecret(ctx, reference); !errors.Is(err, ErrDeploymentSecretRevoked) {
		t.Fatalf("RevealDeploymentSecret() after revoke error = %v", err)
	}
	if err := provisioner.Revoke(ctx, DeploymentRevokeRequest{
		InstallationID:   identity.InstallationID,
		SecretClass:      DeploymentSecretClassGitHubPersonalStar,
		ReferenceVersion: 1,
	}); err != nil {
		t.Fatalf("repeated Revoke() error = %v", err)
	}
	value, err = store.RevealDeploymentSecret(ctx, DeploymentSecretReference(2))
	if err != nil || value != "second-secret" {
		t.Fatalf("rotation left version 2 unreachable: %q, %v", value, err)
	}
}

func TestDeploymentSecretStoreFailsClosed(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	identity, err := BootstrapDeploymentIdentity(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	foreignIdentity, err := BootstrapDeploymentIdentity(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	deploymentTestKey(t, dataDir)
	store := deploymentTestStore(t, dataDir, identity)
	seedDeploymentSecret(t, store, 1, "first-secret")

	if _, err := OpenDeploymentSecretStoreReader(t.TempDir(), identity, store.keyring); err == nil {
		t.Fatal("OpenDeploymentSecretStoreReader() created a missing store")
	}
	foreignKeyring, err := DeploymentMasterKeyringOpenExisting(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	foreignStore, err := OpenDeploymentSecretStoreReader(dataDir, foreignIdentity, foreignKeyring)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = foreignStore.Close() }()
	if _, err := foreignStore.RevealDeploymentSecret(ctx, DeploymentSecretReference(1)); !errors.Is(err, ErrDeploymentSecretBinding) {
		t.Fatalf("cross-installation reveal error = %v, want binding mismatch", err)
	}

	if _, err := store.RevealDeploymentSecret(ctx, "github:user:workspace:access"); err == nil {
		t.Fatal("RevealDeploymentSecret() accepted a user secret reference")
	}
	if _, err := store.RevealDeploymentSecret(ctx, "kandev-runtime:github-personal-star:client-secret:01"); err == nil {
		t.Fatal("RevealDeploymentSecret() accepted a non-canonical version")
	}
	if _, err := store.Load(ctx, DeploymentSecretReference(9)); !errors.Is(err, ErrDeploymentSecretNotFound) {
		t.Fatalf("Load() unknown reference error = %v", err)
	}

	provisioner := NewDeploymentSecretProvisioner(store)
	if err := provisioner.Provision(ctx, DeploymentProvisionRequest{
		InstallationID:   foreignIdentity.InstallationID,
		SecretClass:      DeploymentSecretClassGitHubPersonalStar,
		ReferenceVersion: 4,
		SealedEnvelope:   []byte(`{"version":1}`),
	}); !errors.Is(err, ErrDeploymentSecretBinding) {
		t.Fatalf("Provision() foreign installation error = %v", err)
	}
	if err := provisioner.Provision(ctx, DeploymentProvisionRequest{
		InstallationID:   identity.InstallationID,
		SecretClass:      "github-app-client-secret",
		ReferenceVersion: 4,
		SealedEnvelope:   []byte(`{"version":1}`),
	}); !errors.Is(err, ErrDeploymentSecretBinding) {
		t.Fatalf("Provision() foreign class error = %v", err)
	}
	sealed, err := SealDeploymentSecret("other-master-key", store.keyring.Key(), []byte("value"))
	if err != nil {
		t.Fatal(err)
	}
	if err := provisioner.Provision(ctx, DeploymentProvisionRequest{
		InstallationID:   identity.InstallationID,
		SecretClass:      DeploymentSecretClassGitHubPersonalStar,
		ReferenceVersion: 5,
		SealedEnvelope:   sealed.Bytes(),
	}); err == nil {
		t.Fatal("Provision() accepted a foreign master key identity")
	}
	if _, err := store.RevealDeploymentSecret(ctx, DeploymentSecretReference(5)); !errors.Is(err, ErrDeploymentSecretNotFound) {
		t.Fatalf("failed provision left a row: %v", err)
	}
}
