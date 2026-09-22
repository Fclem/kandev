package github

import (
	"context"
	"fmt"
	"strings"
)

const maxRepositoryStarStatusURLs = 100

// PersonalStarConnectionStatus is non-secret state for the current user's
// dedicated personal GitHub star connection.
type PersonalStarConnectionStatus struct {
	UserID               string `json:"user_id"`
	CredentialGeneration int64  `json:"credential_generation"`
	Active               bool   `json:"active"`
	Capable              bool   `json:"capable"`
	ReconnectRequired    bool   `json:"reconnect_required"`
	Available            bool   `json:"available"`
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
	if s == nil || s.personalStarResolver == nil {
		return PersonalStarConnectionStatus{UserID: userID}, nil
	}
	return s.personalStarResolver.ConnectionStatus(ctx, workspaceID, userID)
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
	client, err := s.resolvePersonalStarClient(ctx, workspaceID, userID)
	if err != nil {
		return nil, err
	}
	states := make(map[string]*bool, len(repositories))
	for key, repository := range repositories {
		starred, statusErr := client.IsStarred(ctx, repository)
		if statusErr != nil {
			states[key] = nil
			continue
		}
		states[key] = &starred
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
	client, err := s.resolvePersonalStarClient(ctx, workspaceID, userID)
	if err != nil {
		return false, err
	}
	if star {
		return client.Star(ctx, repository)
	}
	return client.Unstar(ctx, repository)
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
