package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	maxRepositoryStarStatusURLs    = 100
	personalStarStatusBatchTimeout = 30 * time.Second
)

// PersonalStarConnectionStatus is non-secret state for the current user's
// dedicated personal GitHub star connection.
type PersonalStarConnectionStatus struct {
	UserID               string `json:"user_id"`
	CredentialGeneration int64  `json:"credential_generation"`
	Active               bool   `json:"active"`
	Capable              bool   `json:"capable"`
	ReconnectRequired    bool   `json:"reconnect_required"`
	Available            bool   `json:"available"`
	// Actor names the GitHub identity star calls act as; nil when none resolved.
	Actor *PersonalStarActor `json:"actor"`
	// Connect names the consent flow that can produce an actor, or empty.
	Connect string `json:"connect,omitempty"`
	// Remediation is an operator hint when the actor cannot star.
	Remediation string `json:"remediation,omitempty"`
}

// PersonalStarResolver isolates the dedicated OAuth App connection from every
// existing GitHub credential source.
type PersonalStarResolver interface {
	ConnectionStatus(context.Context, string, string) (PersonalStarConnectionStatus, error)
	ResolvePersonalStarClient(context.Context, string, string) (PersonalStarClient, error)
}

// SetPersonalStarClientBuilder overrides how a resolved dedicated access token
// becomes a star client. Only the E2E mock runtime substitutes it.
func (s *Service) SetPersonalStarClientBuilder(builder PersonalStarClientBuilder) {
	if s == nil {
		return
	}
	if setter, ok := s.personalStarResolver.(interface {
		SetClientBuilder(PersonalStarClientBuilder)
	}); ok {
		setter.SetClientBuilder(builder)
	}
}

func (s *Service) SetPersonalStarResolver(resolver PersonalStarResolver) {
	if s != nil {
		s.personalStarResolver = resolver
	}
}

func (s *Service) PersonalStarConnectionStatus(
	ctx context.Context,
	workspaceID, userID string,
) (PersonalStarConnectionStatus, error) {
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return PersonalStarConnectionStatus{}, err
	}
	status, _, err := s.resolveStarActor(ctx, workspaceID, userID)
	return status, err
}

// resolveStarActor resolves the connection status and, when one exists, the
// single actor a star request runs as: the dedicated connection first, then
// the App personal connection or the workspace's human credential.
func (s *Service) resolveStarActor(
	ctx context.Context,
	workspaceID, userID string,
) (PersonalStarConnectionStatus, *starActor, error) {
	status := PersonalStarConnectionStatus{UserID: userID}
	if s != nil && s.personalStarResolver != nil {
		dedicated, err := s.personalStarResolver.ConnectionStatus(ctx, workspaceID, userID)
		if err != nil {
			return PersonalStarConnectionStatus{}, nil, err
		}
		status = dedicated
		if dedicated.Available && dedicated.Active && dedicated.Capable {
			status.Actor = &PersonalStarActor{Source: StarActorSourceDedicated}
			return status, nil, nil
		}
	}
	dedicatedAvailable := status.Available
	actor := s.resolveCredentialStarActor(ctx, workspaceID, userID)
	if actor == nil {
		if connect := s.starConnectPath(ctx, workspaceID, userID, dedicatedAvailable); connect != "" {
			status.Available = true
			status.Connect = connect
		}
		return status, nil, nil
	}
	actorView := actor.PersonalStarActor
	return PersonalStarConnectionStatus{
		UserID:               userID,
		CredentialGeneration: actor.generation,
		Active:               true,
		Capable:              actor.capable,
		ReconnectRequired:    status.ReconnectRequired,
		Available:            true,
		Actor:                &actorView,
		Remediation:          actor.remedy,
	}, actor, nil
}
func (s *Service) RepositoryStarStatus(
	ctx context.Context,
	workspaceID, userID string,
	repositoryURLs []string,
) (map[string]*bool, error) {
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return nil, err
	}
	repositories, err := canonicalStarRepositories(repositoryURLs)
	if err != nil {
		return nil, err
	}
	states := make(map[string]*bool, len(repositories))
	for key := range repositories {
		states[key] = nil
	}
	batchCtx, cancel := context.WithTimeout(ctx, personalStarStatusBatchTimeout)
	defer cancel()
	for key, repository := range repositories {
		if batchCtx.Err() != nil {
			break
		}
		starred, statusErr := s.withPersonalStarRefresh(batchCtx, workspaceID, userID,
			func(client PersonalStarClient) (bool, error) {
				return client.IsStarred(batchCtx, repository)
			})
		if statusErr == nil {
			states[key] = &starred
		}
	}
	return states, nil
}

func (s *Service) StarRepository(
	ctx context.Context,
	workspaceID, userID, rawRepositoryURL string,
) (bool, error) {
	return s.mutateRepositoryStar(ctx, workspaceID, userID, rawRepositoryURL, true)
}

func (s *Service) UnstarRepository(
	ctx context.Context,
	workspaceID, userID, rawRepositoryURL string,
) (bool, error) {
	return s.mutateRepositoryStar(ctx, workspaceID, userID, rawRepositoryURL, false)
}

func (s *Service) mutateRepositoryStar(
	ctx context.Context,
	workspaceID, userID, rawRepositoryURL string,
	star bool,
) (bool, error) {
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return false, err
	}
	repository, err := ParseGitHubRepositoryURL(rawRepositoryURL)
	if err != nil {
		return false, err
	}
	return s.withPersonalStarRefresh(ctx, workspaceID, userID, func(client PersonalStarClient) (bool, error) {
		if star {
			return client.Star(ctx, repository)
		}
		return client.Unstar(ctx, repository)
	})
}

// withPersonalStarRefresh runs one star operation as the resolved actor. For
// the dedicated connection, a rejected access token is rotated once through the
// stored refresh token before retrying. Every other failure returns unchanged:
// no request is ever retried under another credential source.
func (s *Service) withPersonalStarRefresh(
	ctx context.Context,
	workspaceID, userID string,
	operation func(PersonalStarClient) (bool, error),
) (bool, error) {
	_, actor, err := s.resolveStarActor(ctx, workspaceID, userID)
	if err != nil {
		return false, err
	}
	if actor != nil {
		if !actor.capable {
			return false, ErrGitHubCapabilityDenied
		}
		value, opErr := operation(s.starClientForActor(actor, workspaceID, userID))
		s.recordStarDenial(actor, opErr)
		return value, opErr
	}
	// No credential actor: the dedicated connection is the only candidate,
	// and its resolver reports why it cannot serve the call.
	client, err := s.resolvePersonalStarClient(ctx, workspaceID, userID)
	if err != nil {
		return false, err
	}
	value, err := operation(client)
	if err == nil || !isPersonalStarUnauthorized(err) {
		return value, err
	}
	if refreshErr := s.RefreshPersonalStarConnection(ctx, workspaceID, userID); refreshErr != nil {
		return false, err
	}
	refreshed, err := s.resolvePersonalStarClient(ctx, workspaceID, userID)
	if err != nil {
		return false, err
	}
	return operation(refreshed)
}

func isPersonalStarUnauthorized(err error) bool {
	var apiError *GitHubAPIError
	return errors.As(err, &apiError) && apiError.StatusCode == http.StatusUnauthorized
}

func (s *Service) resolvePersonalStarClient(
	ctx context.Context,
	workspaceID, userID string,
) (PersonalStarClient, error) {
	if s == nil || s.personalStarResolver == nil {
		return nil, ErrGitHubNotConfigured
	}
	status, err := s.personalStarResolver.ConnectionStatus(ctx, workspaceID, userID)
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
	return s.personalStarResolver.ResolvePersonalStarClient(ctx, workspaceID, userID)
}

func canonicalStarRepositories(rawURLs []string) (map[string]GitHubRepositoryRef, error) {
	if len(rawURLs) > maxRepositoryStarStatusURLs {
		return nil, fmt.Errorf("at most %d repository URLs are allowed", maxRepositoryStarStatusURLs)
	}
	repositories := make(map[string]GitHubRepositoryRef, len(rawURLs))
	for _, rawURL := range rawURLs {
		repository, err := ParseGitHubRepositoryURL(strings.TrimSpace(rawURL))
		if err != nil {
			return nil, err
		}
		repositories[repository.String()] = repository
	}
	return repositories, nil
}
