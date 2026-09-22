package github

import (
	"context"
	"strings"
)

// PersonalStarDeploymentStatus is the deployment-owned capability surface the
// resolver consults before touching any credential.
type PersonalStarDeploymentStatus struct {
	Available bool
	Settings  PersonalStarOAuthSettings
}

// PersonalStarResolverService resolves only dedicated OAuth App star
// connections. Every other credential source fails closed.
type PersonalStarResolverService struct {
	store      *Store
	secrets    PersonalStarSecrets
	deployment PersonalStarDeploymentStatus
}

// NewPersonalStarResolverService wires the strict dedicated resolver.
func NewPersonalStarResolverService(
	store *Store,
	secrets PersonalStarSecrets,
	deployment PersonalStarDeploymentStatus,
) *PersonalStarResolverService {
	return &PersonalStarResolverService{store: store, secrets: secrets, deployment: deployment}
}

// Available reports whether the deployment can serve dedicated star connections.
func (r *PersonalStarResolverService) Available() bool {
	return r != nil && r.deployment.Available && r.deployment.Settings.Configured()
}

// ConnectionStatus returns non-secret state for one workspace/user.
func (r *PersonalStarResolverService) ConnectionStatus(
	ctx context.Context,
	workspaceID, userID string,
) (PersonalStarConnectionStatus, error) {
	status := PersonalStarConnectionStatus{UserID: userID, Available: r.Available()}
	if !status.Available {
		return status, nil
	}
	lifecycle, err := r.store.GetPersonalStarLifecycle(ctx, workspaceID, userID)
	if err != nil {
		return PersonalStarConnectionStatus{}, err
	}
	connection, err := r.store.GetPersonalStarConnection(ctx, workspaceID, userID)
	if err != nil {
		return PersonalStarConnectionStatus{}, err
	}
	status.CredentialGeneration = lifecycle.CredentialGeneration
	if connection == nil {
		status.ReconnectRequired = lifecycle.CredentialGeneration >= 1
		return status, nil
	}
	status.CredentialGeneration = connection.CredentialGeneration
	status.Active = connection.Status == PersonalStarStatusActive && connection.AccessSecretID != ""
	status.Capable = status.Active && connection.PublicRepo
	everConnected := lifecycle.CredentialGeneration >= 1
	status.ReconnectRequired = (everConnected && (!status.Active || !status.Capable)) ||
		(status.Active && status.Capable && personalStarOutcomeNeedsReconnect(connection.LastReconnectOutcome))
	return status, nil
}

// ResolvePersonalStarClient returns the narrow star client for the committed
// dedicated connection, or fails closed.
func (r *PersonalStarResolverService) ResolvePersonalStarClient(
	ctx context.Context,
	workspaceID, userID string,
) (PersonalStarClient, error) {
	status, err := r.ConnectionStatus(ctx, workspaceID, userID)
	if err != nil {
		return nil, err
	}
	if !status.Available {
		return nil, ErrGitHubNotConfigured
	}
	if !status.Active {
		return nil, ErrGitHubPersonalRequired
	}
	if !status.Capable {
		return nil, ErrGitHubCapabilityDenied
	}
	connection, err := r.store.GetPersonalStarConnection(ctx, workspaceID, userID)
	if err != nil {
		return nil, err
	}
	if connection == nil || connection.AccessSecretID == "" {
		return nil, ErrGitHubPersonalRequired
	}
	token, err := r.secrets.Reveal(ctx, connection.AccessSecretID)
	if err != nil || strings.TrimSpace(token) == "" {
		return nil, ErrGitHubPersonalRequired
	}
	return NewPersonalStarClient(token), nil
}

// personalStarOutcomeNeedsReconnect reports whether the latest recorded
// reconnect attempt left the user without a usable grant.
func personalStarOutcomeNeedsReconnect(outcome string) bool {
	return outcome == personalStarOutcomeDeclined ||
		outcome == personalStarOutcomeInsufficientScope ||
		outcome == personalStarOutcomeFailed
}

var _ PersonalStarResolver = (*PersonalStarResolverService)(nil)
