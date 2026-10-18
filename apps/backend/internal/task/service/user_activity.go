package service

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const (
	userActivityOutcomeRejected = "rejected"
	userActivityOutcomeDisabled = "disabled"
)

// RecordUserActivity authorizes one session beat and uses the server clock.
func (s *Service) RecordUserActivity(ctx context.Context, actorID, sessionID string) (string, error) {
	if err := s.AuthorizeSessionAccess(ctx, sessionID); err != nil {
		if errors.Is(err, repoerrors.ErrTaskNotFound) || errors.Is(err, models.ErrTaskSessionNotFound) ||
			errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrForbidden) {
			return userActivityOutcomeRejected, nil
		}
		return "", err
	}
	if s.userActivity == nil {
		return userActivityOutcomeDisabled, nil
	}
	return s.userActivity.RecordUserActivity(ctx, actorID, sessionID, time.Now().UTC())
}

func (s *Service) SumUserActivity(ctx context.Context, sessionIDs []string) ([]models.UserActivitySummary, error) {
	if s.userActivity == nil {
		return nil, nil
	}
	return s.userActivity.SumUserActivity(ctx, sessionIDs)
}

// ListUserActivitySessions returns one filtered, keyset-paged session selection.
func (s *Service) ListUserActivitySessions(
	ctx context.Context,
	filter models.SessionActivityFilter,
	cursor string,
	limit int,
) (models.SessionActivitySessionsPage, error) {
	reader, ok := s.userActivity.(repository.SessionActivityReadRepository)
	if !ok {
		return models.SessionActivitySessionsPage{}, errors.New("session activity repository is unavailable")
	}
	return reader.ListUserActivitySessions(ctx, filter, cursor, limit)
}

// ListSessionTurns returns one filtered, keyset-paged turn selection.
func (s *Service) ListSessionTurns(
	ctx context.Context,
	filter models.SessionActivityFilter,
	cursor string,
	limit int,
) (models.SessionActivityTurnsPage, error) {
	reader, ok := s.turns.(repository.SessionActivityReadRepository)
	if !ok {
		return models.SessionActivityTurnsPage{}, errors.New("session activity repository is unavailable")
	}
	return reader.ListSessionTurns(ctx, filter, cursor, limit)
}
