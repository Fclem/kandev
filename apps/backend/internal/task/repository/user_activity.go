package repository

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type UserActivityRepository interface {
	RecordUserActivity(context.Context, string, string, time.Time) (string, error)
	SumUserActivity(context.Context, []string) ([]models.UserActivitySummary, error)
}

// SessionActivityReadRepository provides bounded session activity and turn pages.
type SessionActivityReadRepository interface {
	ListUserActivitySessions(context.Context, models.SessionActivityFilter, string, int) (models.SessionActivitySessionsPage, error)
	ListSessionTurns(context.Context, models.SessionActivityFilter, string, int) (models.SessionActivityTurnsPage, error)
}
