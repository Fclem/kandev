package secrets

import (
	"context"
	"fmt"
)

// DeploymentSecretReader resolves values from the root-owned deployment store.
// It intentionally has no mutation methods, so backend startup cannot provision
// or rotate deployment credentials.
type DeploymentSecretReader interface {
	RevealDeploymentSecret(context.Context, string) (string, error)
}

// DeploymentSecretResolver resolves only references reserved for deployment-
// owned GitHub personal-star OAuth credentials.
type DeploymentSecretResolver struct {
	reader DeploymentSecretReader
}

func NewDeploymentSecretResolver(reader DeploymentSecretReader) *DeploymentSecretResolver {
	return &DeploymentSecretResolver{reader: reader}
}

func (r *DeploymentSecretResolver) Resolve(ctx context.Context, reference string) (string, error) {
	if err := ValidateDeploymentSecretReference(reference); err != nil {
		return "", err
	}
	if r == nil || r.reader == nil {
		return "", fmt.Errorf("deployment secret resolver is unavailable")
	}
	value, err := r.reader.RevealDeploymentSecret(ctx, reference)
	if err != nil {
		return "", fmt.Errorf("resolve deployment secret: %w", err)
	}
	if value == "" {
		return "", fmt.Errorf("resolve deployment secret: empty value")
	}
	return value, nil
}
