package backendapp

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

type userActivityCaptureStore interface {
	SetUserActivityCapture(bool)
	RecordCaptureTransition(context.Context, bool, time.Time) error
}

func initializeUserActivityCapture(ctx context.Context, cfg *config.Config, store userActivityCaptureStore, log *logger.Logger) {
	store.SetUserActivityCapture(false)
	if err := store.RecordCaptureTransition(ctx, cfg.Features.SessionUserActivity, time.Now().UTC()); err != nil {
		cfg.Features.SessionUserActivity = false
		log.Warn("Session user activity capture disabled: failed to persist capture state", zap.Error(err))
		return
	}
	store.SetUserActivityCapture(cfg.Features.SessionUserActivity)
}
