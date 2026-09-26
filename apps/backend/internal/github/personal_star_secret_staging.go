package github

import (
	"context"
	"errors"
	"fmt"
)

func (s *PersonalStarOAuthService) stagePersonalStarSecrets(
	ctx context.Context,
	connection *PersonalStarConnection,
	tokens GitHubOAuthTokens,
) ([]personalStarSecretRef, error) {
	staged := make([]personalStarSecretRef, 0, 2)
	accessRef := personalStarSecretRef{
		SecretID: connection.AccessSecretID, Kind: personalStarSecretKindAccess,
		Generation: connection.AccessSecretGeneration,
	}
	if err := s.secrets.Set(ctx, connection.AccessSecretID,
		"github personal star access token", tokens.AccessToken); err != nil {
		cleanupErr := s.reconcilePersonalStarSecretWriteFailure(
			ctx, connection.WorkspaceID, connection.UserID, accessRef)
		if cleanupErr != nil {
			staged = append(staged, accessRef)
		}
		return staged, errors.Join(err, cleanupErr)
	}
	staged = append(staged, accessRef)
	if tokens.RefreshToken != "" {
		refreshRef := personalStarSecretRef{
			SecretID: connection.RefreshSecretID, Kind: personalStarSecretKindRefresh,
			Generation: connection.RefreshSecretGeneration,
		}
		if err := s.secrets.Set(ctx, connection.RefreshSecretID,
			"github personal star refresh token", tokens.RefreshToken); err != nil {
			cleanupErr := s.reconcilePersonalStarSecretWriteFailure(
				ctx, connection.WorkspaceID, connection.UserID, refreshRef)
			if cleanupErr != nil {
				staged = append(staged, refreshRef)
			}
			return staged, errors.Join(err, cleanupErr)
		}
		staged = append(staged, refreshRef)
	} else {
		connection.RefreshSecretID = ""
	}
	return staged, nil
}

func (s *PersonalStarOAuthService) reconcilePersonalStarSecretWriteFailure(
	ctx context.Context,
	workspaceID, userID string,
	ref personalStarSecretRef,
) error {
	cleanupCtx := context.WithoutCancel(ctx)
	exists, err := s.secrets.Exists(cleanupCtx, ref.SecretID)
	if err != nil {
		if queueErr := s.queuePersonalStarSecretCleanup(cleanupCtx, workspaceID, userID, ref); queueErr != nil {
			return errors.Join(err, queueErr)
		}
		return nil
	}
	if !exists {
		return nil
	}
	return s.discardPersonalStarSecret(cleanupCtx, workspaceID, userID, ref)
}

func (s *PersonalStarOAuthService) discardPersonalStarSecrets(
	ctx context.Context,
	workspaceID, userID string,
	refs []personalStarSecretRef,
) error {
	ctx = context.WithoutCancel(ctx)
	var cleanupErr error
	for _, ref := range refs {
		cleanupErr = errors.Join(cleanupErr, s.discardPersonalStarSecret(ctx, workspaceID, userID, ref))
	}
	return cleanupErr
}

func (s *PersonalStarOAuthService) discardPersonalStarSecret(
	ctx context.Context,
	workspaceID, userID string,
	ref personalStarSecretRef,
) error {
	if err := s.secrets.Delete(ctx, ref.SecretID); err != nil {
		if queueErr := s.queuePersonalStarSecretCleanup(ctx, workspaceID, userID, ref); queueErr != nil {
			return fmt.Errorf("delete staged personal star secret %q: %w", ref.SecretID, errors.Join(err, queueErr))
		}
	}
	return nil
}

func (s *PersonalStarOAuthService) queuePersonalStarSecretCleanup(
	ctx context.Context,
	workspaceID, userID string,
	ref personalStarSecretRef,
) error {
	if err := s.store.EnqueuePersonalStarSecretCleanup(
		ctx, workspaceID, userID, ref.SecretID, ref.Kind, ref.Generation,
	); err != nil {
		return fmt.Errorf("enqueue personal star secret cleanup for %q: %w", ref.SecretID, err)
	}
	return nil
}
