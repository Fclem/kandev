package secrets

import (
	"context"
	"errors"
	"fmt"
)

// DeploymentProvisionRequest is one verified provision instruction.
type DeploymentProvisionRequest struct {
	InstallationID   string
	SecretClass      string
	ReferenceVersion int64
	SealedEnvelope   []byte
}

// DeploymentRevokeRequest is one verified revoke instruction.
type DeploymentRevokeRequest struct {
	InstallationID   string
	SecretClass      string
	ReferenceVersion int64
}

// DeploymentSecretProvisioner is the only writer of deployment secret rows.
type DeploymentSecretProvisioner struct {
	store *DeploymentSecretStore
}

// NewDeploymentSecretProvisioner wraps a writable deployment secret store.
func NewDeploymentSecretProvisioner(store *DeploymentSecretStore) *DeploymentSecretProvisioner {
	return &DeploymentSecretProvisioner{store: store}
}

// Provision stores a sealed secret as a new immutable reference version.
func (p *DeploymentSecretProvisioner) Provision(ctx context.Context, request DeploymentProvisionRequest) error {
	reference, err := p.validate(request.InstallationID, request.SecretClass, request.ReferenceVersion)
	if err != nil {
		return err
	}
	if p.store.keyring == nil {
		return errors.New("deployment master keyring is unavailable")
	}
	if _, err := ParseSealedDeploymentSecret(request.SealedEnvelope, p.store.keyring.ID()); err != nil {
		return err
	}
	return p.store.Insert(ctx, &DeploymentSecretRecord{
		Reference:      reference,
		SecretClass:    request.SecretClass,
		InstallationID: request.InstallationID,
		Version:        request.ReferenceVersion,
		SealedEnvelope: string(request.SealedEnvelope),
	})
}

// Revoke marks a reference version revoked. Repeated revokes are no-ops.
func (p *DeploymentSecretProvisioner) Revoke(ctx context.Context, request DeploymentRevokeRequest) error {
	reference, err := p.validate(request.InstallationID, request.SecretClass, request.ReferenceVersion)
	if err != nil {
		return err
	}
	_, err = p.store.MarkRevoked(ctx, reference, p.store.now())
	return err
}

func (p *DeploymentSecretProvisioner) validate(installationID, secretClass string, version int64) (string, error) {
	if p == nil || p.store == nil {
		return "", errors.New("deployment secret provisioner is unavailable")
	}
	if installationID != p.store.identity.InstallationID {
		return "", fmt.Errorf("%w: installation mismatch", ErrDeploymentSecretBinding)
	}
	if secretClass != DeploymentSecretClassGitHubPersonalStar {
		return "", fmt.Errorf("%w: secret class mismatch", ErrDeploymentSecretBinding)
	}
	if version < 1 {
		return "", fmt.Errorf("%w: version must be positive", ErrDeploymentSecretBinding)
	}
	return DeploymentSecretReference(version), nil
}
