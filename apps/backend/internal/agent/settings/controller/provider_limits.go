package controller

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/providerlimit"
	ws "github.com/kandev/kandev/pkg/websocket"
)

var ErrProfileLimitStoreUnavailable = errors.New("provider limit store unavailable")

func (c *Controller) SetProviderLimitService(limits *providerlimit.Service) {
	c.providerLimits = limits
}

func (c *Controller) ListProfileLimits(ctx context.Context) ([]providerlimit.ProfileLimit, error) {
	if c.providerLimits == nil {
		return nil, ErrProfileLimitStoreUnavailable
	}
	return c.providerLimits.ListProfiles(ctx)
}

func (c *Controller) BroadcastProfileLimits(ctx context.Context) {
	if c.hub == nil {
		return
	}
	limits, err := c.ListProfileLimits(ctx)
	if err != nil {
		c.logger.Warn("broadcast provider limits failed", zap.Error(err))
		return
	}
	message, err := ws.NewNotification(ws.ActionAgentProfileLimitsUpdated, map[string]any{"limits": limits})
	if err != nil {
		c.logger.Warn("encode provider limits failed", zap.Error(err))
		return
	}
	c.hub.Broadcast(message)
}
