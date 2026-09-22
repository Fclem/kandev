package github

import (
	"context"
	"strings"
)

// PersonalStarOAuthCoordinator owns the dedicated OAuth App consent lifecycle.
type PersonalStarOAuthCoordinator interface {
	Start(context.Context, PersonalStarStartRequest) (PersonalStarStartResult, error)
	RefreshPersonalStarConnection(context.Context, string, string) error
	CompleteCallback(context.Context, GitHubStarCallbackQuery, string, string) (PersonalStarCallbackOutcome, error)
	DisconnectPersonalStarConnection(context.Context, string, string) error
}

// SetPersonalStarOAuthCoordinator wires the dedicated consent lifecycle.
func (s *Service) SetPersonalStarOAuthCoordinator(coordinator PersonalStarOAuthCoordinator) {
	if s != nil {
		s.personalStarOAuth = coordinator
	}
}

// StartPersonalStarConsent authorizes the workspace before creating state.
func (s *Service) StartPersonalStarConsent(
	ctx context.Context,
	workspaceID, userID, sessionID, returnPath string,
) (PersonalStarStartResult, error) {
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return PersonalStarStartResult{}, err
	}
	if s == nil || s.personalStarOAuth == nil {
		return PersonalStarStartResult{}, ErrGitHubNotConfigured
	}
	sessionSubject := strings.TrimSpace(userID)
	if sessionSubject == "" {
		return PersonalStarStartResult{}, ErrGitHubWorkspaceRequired
	}
	return s.personalStarOAuth.Start(ctx, PersonalStarStartRequest{
		WorkspaceID:    workspaceID,
		UserID:         userID,
		SessionSubject: sessionSubject,
		SessionID:      sessionID,
		ReturnPath:     returnPath,
	})
}

// CompletePersonalStarConsent consumes one callback. The workspace is recovered
// from the server-side flow row, never from callback parameters.
func (s *Service) CompletePersonalStarConsent(
	ctx context.Context,
	query GitHubStarCallbackQuery,
	sessionSubject, sessionID string,
) (PersonalStarCallbackOutcome, error) {
	if s == nil || s.personalStarOAuth == nil {
		return PersonalStarCallbackOutcome{}, ErrGitHubNotConfigured
	}
	return s.personalStarOAuth.CompleteCallback(ctx, query, sessionSubject, sessionID)
}

// RefreshPersonalStarConnection rotates the dedicated access token. It is
// reachable only through the dedicated star path, never through App
// credentials, and authorizes the workspace first.
func (s *Service) RefreshPersonalStarConnection(
	ctx context.Context,
	workspaceID, userID string,
) error {
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return err
	}
	if s == nil || s.personalStarOAuth == nil {
		return ErrGitHubNotConfigured
	}
	return s.personalStarOAuth.RefreshPersonalStarConnection(ctx, workspaceID, userID)
}

// DisconnectPersonalStarConnection authorizes the workspace before mutating.
func (s *Service) DisconnectPersonalStarConnection(
	ctx context.Context,
	workspaceID, userID string,
) error {
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return err
	}
	if s == nil || s.personalStarOAuth == nil {
		return ErrGitHubNotConfigured
	}
	return s.personalStarOAuth.DisconnectPersonalStarConnection(ctx, workspaceID, userID)
}
